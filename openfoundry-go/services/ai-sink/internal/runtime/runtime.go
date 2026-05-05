package runtime

import (
	"context"
	"errors"
	"log/slog"
	"time"

	databus "github.com/openfoundry/openfoundry-go/libs/event-bus-data"
	"github.com/openfoundry/openfoundry-go/services/ai-sink/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ai-sink/internal/envelope"
	"github.com/openfoundry/openfoundry-go/services/ai-sink/internal/writer"
)

// Outcome label values for ai_sink_commits_total.
const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
	OutcomePoison  = "poison"
)

// Run is the Kafka → batch-by-table → Writer.Append → CommitMessages loop.
//
// Per-table batching: every record is grouped by Iceberg target table
// (prompts/responses/evaluations/traces) so the writer can do one
// Iceberg append per table per flush. Kafka offsets advance only after
// every per-table append succeeds (atomic per batch).
func Run(ctx context.Context, cfg *config.Config, sub databus.Subscriber, w writer.Writer, m *Metrics, log *slog.Logger) error {
	byTable := make(map[string][]envelope.AiEventEnvelope, 4)
	pending := make([]*databus.DataMessage, 0, cfg.BatchPolicy.MaxRecords)
	totalRecords := 0
	batchStart := time.Now()

	flushAndReset := func() error {
		if totalRecords == 0 && len(pending) == 0 {
			return nil
		}
		if err := flush(ctx, sub, w, m, log, byTable, pending, batchStart); err != nil {
			return err
		}
		for k := range byTable {
			byTable[k] = byTable[k][:0]
		}
		pending = pending[:0]
		totalRecords = 0
		batchStart = time.Now()
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			if err := flushAndReset(); err != nil {
				log.Error("final flush failed", slog.String("error", err.Error()))
				return err
			}
			return nil
		default:
		}

		remaining := cfg.BatchPolicy.MaxWait - time.Since(batchStart)
		if remaining <= 0 {
			remaining = 1 * time.Millisecond
		}
		pollCtx, cancel := context.WithTimeout(ctx, remaining)
		msg, err := sub.Poll(pollCtx)
		cancel()

		switch {
		case err == nil:
			env, decodeErr := envelope.Decode(msg.Value)
			if decodeErr != nil {
				log.Warn("skipping poison ai record",
					slog.String("topic", msg.Topic),
					slog.Int("partition", msg.Partition),
					slog.Int64("offset", msg.Offset),
					slog.String("error", decodeErr.Error()))
				m.CommitsTotal.WithLabelValues("unknown", OutcomePoison).Inc()
				pending = append(pending, msg)
				continue
			}
			table, routeErr := envelope.Route(&env)
			if routeErr != nil {
				log.Warn("skipping unknown ai event kind",
					slog.String("kind", string(env.Kind)),
					slog.String("error", routeErr.Error()))
				m.CommitsTotal.WithLabelValues("unknown", OutcomePoison).Inc()
				pending = append(pending, msg)
				continue
			}
			byTable[table] = append(byTable[table], env)
			pending = append(pending, msg)
			totalRecords++

		case errors.Is(err, context.DeadlineExceeded):
			// Poll timeout — fall through to flush check.

		case errors.Is(err, context.Canceled):
			if err := flushAndReset(); err != nil {
				return err
			}
			return nil

		default:
			return err
		}

		if cfg.BatchPolicy.ShouldFlush(totalRecords, time.Since(batchStart)) {
			if err := flushAndReset(); err != nil {
				return err
			}
		}
	}
}

// flush appends each per-table group to the writer in one call (the
// writer is responsible for issuing per-table Iceberg appends). On
// success, commits every Kafka offset (good + poison) in one
// CommitMessages call.
func flush(
	ctx context.Context,
	sub databus.Subscriber,
	w writer.Writer,
	m *Metrics,
	log *slog.Logger,
	byTable map[string][]envelope.AiEventEnvelope,
	pending []*databus.DataMessage,
	batchStart time.Time,
) error {
	hasRecords := false
	for _, group := range byTable {
		if len(group) > 0 {
			hasRecords = true
			break
		}
	}

	if hasRecords {
		if err := w.Append(ctx, byTable); err != nil {
			for table, group := range byTable {
				if len(group) > 0 {
					m.CommitsTotal.WithLabelValues(table, OutcomeFailure).Inc()
				}
			}
			return err
		}
	}

	if err := sub.CommitMessages(ctx, pending); err != nil {
		for table, group := range byTable {
			if len(group) > 0 {
				m.CommitsTotal.WithLabelValues(table, OutcomeFailure).Inc()
			}
		}
		return err
	}

	totalRecords := 0
	for table, group := range byTable {
		if len(group) == 0 {
			continue
		}
		totalRecords += len(group)
		oldestMicros := group[0].At
		for _, e := range group[1:] {
			if e.At < oldestMicros {
				oldestMicros = e.At
			}
		}
		lag := time.Since(time.UnixMicro(oldestMicros)).Seconds()
		m.LagSeconds.WithLabelValues(table).Observe(lag)
		m.RecordsTotal.WithLabelValues(table).Add(float64(len(group)))
		m.BatchSize.WithLabelValues(table).Observe(float64(len(group)))
		m.CommitsTotal.WithLabelValues(table, OutcomeSuccess).Inc()
	}

	log.Info("flushed ai batch",
		slog.Int("records", totalRecords),
		slog.Int("offsets_committed", len(pending)),
		slog.Duration("elapsed", time.Since(batchStart)))
	return nil
}
