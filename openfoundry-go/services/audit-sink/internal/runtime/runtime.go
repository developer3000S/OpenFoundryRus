package runtime

import (
	"context"
	"errors"
	"log/slog"
	"time"

	databus "github.com/openfoundry/openfoundry-go/libs/event-bus-data"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/config"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/envelope"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/writer"
)

// Outcome label values for audit_sink_commits_total.
const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
	OutcomePoison  = "poison"
)

// Run is the Kafka → batch → Writer.Append → CommitMessages loop.
//
// Flow per iteration:
//
//  1. Poll the next message (blocking, ctx-cancellable).
//  2. Decode JSON. Poison messages are NOT parsed but the underlying
//     Kafka message is still tracked so the next CommitMessages call
//     advances past them — otherwise the partition stalls forever.
//  3. Append decoded envelopes to the in-memory batch.
//  4. When the batch policy fires, call Writer.Append. On success,
//     commit every pending offset (good + poison) in a single
//     Subscriber.CommitMessages call. On failure, return — the
//     supervisor restarts the process and the batch replays from Kafka.
//  5. Repeat until ctx is done.
//
// At-least-once delivery: Writer.Append happens BEFORE Kafka commit.
// Crashes between append and commit replay the batch on restart;
// downstream dedup (Iceberg primary key by event_id, or a separate
// idempotency.Store) handles duplicates.
func Run(ctx context.Context, cfg *config.Config, sub databus.Subscriber, w writer.Writer, m *Metrics, log *slog.Logger) error {
	tableLabel := cfg.Service.Name

	batch := make([]envelope.AuditEnvelope, 0, cfg.BatchPolicy.MaxRecords)
	pending := make([]*databus.DataMessage, 0, cfg.BatchPolicy.MaxRecords)
	batchStart := time.Now()

	flushAndReset := func() error {
		if len(batch) == 0 && len(pending) == 0 {
			return nil
		}
		if err := flush(ctx, sub, w, m, log, batch, pending, batchStart, tableLabel); err != nil {
			return err
		}
		batch = batch[:0]
		pending = pending[:0]
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

		// Compute remaining wait time so the loop respects the
		// max_wait deadline even when no messages arrive.
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
				log.Warn("skipping poison audit record",
					slog.String("topic", msg.Topic),
					slog.Int("partition", msg.Partition),
					slog.Int64("offset", msg.Offset),
					slog.String("error", decodeErr.Error()))
				m.CommitsTotal.WithLabelValues(tableLabel, OutcomePoison).Inc()
				// Track the poison message so the next commit moves
				// past it; without this, the partition wedges.
				pending = append(pending, msg)
				continue
			}
			batch = append(batch, env)
			pending = append(pending, msg)

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

		if cfg.BatchPolicy.ShouldFlush(len(batch), time.Since(batchStart)) {
			if err := flushAndReset(); err != nil {
				return err
			}
		}
	}
}

// flush appends the batch to the writer, commits every Kafka offset
// in a single Subscriber.CommitMessages call, and updates metrics.
// Errors propagate so the runtime exits and the supervisor restarts.
func flush(
	ctx context.Context,
	sub databus.Subscriber,
	w writer.Writer,
	m *Metrics,
	log *slog.Logger,
	batch []envelope.AuditEnvelope,
	pending []*databus.DataMessage,
	batchStart time.Time,
	tableLabel string,
) error {
	if len(batch) > 0 {
		if err := w.Append(ctx, batch); err != nil {
			m.CommitsTotal.WithLabelValues(tableLabel, OutcomeFailure).Inc()
			return err
		}
	}

	if err := sub.CommitMessages(ctx, pending); err != nil {
		m.CommitsTotal.WithLabelValues(tableLabel, OutcomeFailure).Inc()
		return err
	}

	if len(batch) > 0 {
		// Lag = wall time since the oldest record in this batch was produced.
		oldestMicros := batch[0].At
		for _, e := range batch[1:] {
			if e.At < oldestMicros {
				oldestMicros = e.At
			}
		}
		lag := time.Since(time.UnixMicro(oldestMicros)).Seconds()
		m.LagSeconds.WithLabelValues(tableLabel).Observe(lag)
		m.RecordsTotal.WithLabelValues(tableLabel).Add(float64(len(batch)))
		m.BatchSize.WithLabelValues(tableLabel).Observe(float64(len(batch)))
		m.CommitsTotal.WithLabelValues(tableLabel, OutcomeSuccess).Inc()
	}

	log.Info("flushed audit batch",
		slog.Int("records", len(batch)),
		slog.Int("offsets_committed", len(pending)),
		slog.Duration("elapsed", time.Since(batchStart)),
		slog.String("table", tableLabel))
	return nil
}
