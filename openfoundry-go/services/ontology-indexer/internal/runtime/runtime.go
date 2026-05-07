// Package runtime hosts the ontology-indexer worker loop.
package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	databus "github.com/openfoundry/openfoundry-go/libs/event-bus-data"
	searchabstraction "github.com/openfoundry/openfoundry-go/libs/search-abstraction"
	"github.com/openfoundry/openfoundry-go/libs/search-abstraction/opensearch"
	"github.com/openfoundry/openfoundry-go/libs/search-abstraction/vespa"
	repos "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/ontology-indexer/internal/config"
	"github.com/segmentio/kafka-go"
)

// Topics the indexer subscribes to on startup.
const (
	TopicObjectChangedV1 = "ontology.objects.changed.v1"
	TopicLinkChangedV1   = "ontology.links.changed.v1"
	TopicDLQ             = "ontology-indexer.dlq.v1"
)

// SubscribeTopics pins all live topics consumed by this service.
var SubscribeTopics = []string{TopicObjectChangedV1, TopicLinkChangedV1}

// ConsumerGroup pinned here so replicas don't fork rebalance state.
const ConsumerGroup = "ontology-indexer"

// KafkaMessage is the small Kafka record surface the runtime needs.
type KafkaMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Key       []byte
	Value     []byte
	Time      time.Time
	Raw       any
}

// KafkaReader is the injectable Kafka consumer surface used by the indexer.
type KafkaReader interface {
	Subscribe(ctx context.Context, topics []string) error
	FetchMessage(ctx context.Context) (KafkaMessage, error)
	CommitMessages(ctx context.Context, msgs ...KafkaMessage) error
	Close() error
}

type kafkaGoReader struct {
	brokers []string
	groupID string
	log     *slog.Logger
	reader  *kafka.Reader
}

func NewKafkaReader(brokers []string, groupID string, log *slog.Logger) KafkaReader {
	if log == nil {
		log = slog.Default()
	}
	return &kafkaGoReader{brokers: brokers, groupID: groupID, log: log}
}

func (r *kafkaGoReader) Subscribe(_ context.Context, topics []string) error {
	if len(r.brokers) == 0 || len(r.brokers) == 1 && strings.TrimSpace(r.brokers[0]) == "" {
		return fmt.Errorf("KAFKA_BOOTSTRAP_SERVERS not set")
	}
	if r.groupID == "" {
		r.groupID = ConsumerGroup
	}
	r.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        r.brokers,
		GroupID:        r.groupID,
		GroupTopics:    topics,
		CommitInterval: 0, // synchronous commits: caller commits only after indexing succeeds.
		MinBytes:       1,
		MaxBytes:       10e6,
	})
	r.log.Info("ontology-indexer subscribed", slog.Any("topics", topics), slog.String("consumer_group", r.groupID))
	return nil
}

func (r *kafkaGoReader) FetchMessage(ctx context.Context) (KafkaMessage, error) {
	if r.reader == nil {
		return KafkaMessage{}, fmt.Errorf("kafka reader is not subscribed")
	}
	m, err := r.reader.FetchMessage(ctx)
	if err != nil {
		return KafkaMessage{}, err
	}
	return KafkaMessage{Topic: m.Topic, Partition: m.Partition, Offset: m.Offset, Key: m.Key, Value: m.Value, Time: m.Time, Raw: m}, nil
}

func (r *kafkaGoReader) CommitMessages(ctx context.Context, msgs ...KafkaMessage) error {
	if r.reader == nil {
		return fmt.Errorf("kafka reader is not subscribed")
	}
	km := make([]kafka.Message, 0, len(msgs))
	for _, msg := range msgs {
		if raw, ok := msg.Raw.(kafka.Message); ok {
			km = append(km, raw)
			continue
		}
		km = append(km, kafka.Message{Topic: msg.Topic, Partition: msg.Partition, Offset: msg.Offset, Key: msg.Key, Value: msg.Value, Time: msg.Time})
	}
	return r.reader.CommitMessages(ctx, km...)
}

func (r *kafkaGoReader) Close() error {
	if r.reader == nil {
		return nil
	}
	return r.reader.Close()
}

type ObjectChangedV1 struct {
	Tenant    repos.TenantId  `json:"tenant"`
	ID        repos.ObjectId  `json:"id"`
	TypeID    repos.TypeId    `json:"type_id"`
	Version   uint64          `json:"version"`
	Payload   json.RawMessage `json:"payload"`
	Embedding []float32       `json:"embedding,omitempty"`
	Deleted   bool            `json:"deleted"`
}

type LinkChangedV1 struct {
	Tenant   repos.TenantId   `json:"tenant"`
	ID       repos.ObjectId   `json:"id,omitempty"`
	LinkType repos.LinkTypeId `json:"link_type"`
	From     repos.ObjectId   `json:"from"`
	To       repos.ObjectId   `json:"to"`
	Version  uint64           `json:"version"`
	Payload  json.RawMessage  `json:"payload,omitempty"`
	Deleted  bool             `json:"deleted"`
	// Alternate producer spellings accepted during migration.
	TypeID   repos.LinkTypeId `json:"type_id,omitempty"`
	SourceID repos.ObjectId   `json:"source_id,omitempty"`
	TargetID repos.ObjectId   `json:"target_id,omitempty"`
}

type RecordOutcome string

const (
	OutcomeIndexed      RecordOutcome = "indexed"
	OutcomeDeleted      RecordOutcome = "deleted"
	OutcomeDecodeError  RecordOutcome = "decode_error"
	OutcomeSkippedStale RecordOutcome = "skipped_stale"
)

func Run(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	backend, err := NewSearchBackend(cfg)
	if err != nil {
		return err
	}
	brokers := splitCSV(cfg.KafkaBootstrap)
	reader := NewKafkaReader(brokers, defaultStr(cfg.ConsumerGroup, ConsumerGroup), log)
	var dlq DLQPublisher
	if cfg.DLQTopic != "" {
		publisher, err := databus.NewKafkaPublisher(databus.NewConfig(brokers, databus.InsecureDev("ontology-indexer-dlq")))
		if err != nil {
			return err
		}
		defer publisher.Close()
		dlq = publisher
	}
	return RunWithOptions(ctx, cfg, log, reader, backend, dlq)
}

// NewSearchBackend builds the concrete search backend selected by service config.
func NewSearchBackend(cfg *config.Config) (searchabstraction.SearchBackend, error) {
	if cfg.SearchEndpoint == "" {
		return nil, repos.Invalid("SEARCH_ENDPOINT not set")
	}
	authHeader := searchAuthHeader(cfg)
	switch cfg.BackendKind {
	case config.BackendOpenSearch:
		return opensearch.NewWithOptions(cfg.SearchEndpoint, opensearch.WithAuthHeader(authHeader)), nil
	case config.BackendVespa:
		return vespa.NewWithOptions(cfg.SearchEndpoint, vespa.WithAuthHeader(authHeader)), nil
	default:
		return nil, repos.Invalidf("unknown SEARCH_BACKEND value: %q", cfg.BackendKind)
	}
}

func searchAuthHeader(cfg *config.Config) string {
	if token := strings.TrimSpace(cfg.SearchBearerToken); token != "" {
		return "Bearer " + token
	}
	if key := strings.TrimSpace(cfg.SearchAPIKey); key != "" {
		return "ApiKey " + key
	}
	if cfg.SearchUsername != "" || cfg.SearchPassword != "" {
		raw := cfg.SearchUsername + ":" + cfg.SearchPassword
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
	}
	return ""
}

type DLQPublisher interface {
	Publish(ctx context.Context, topic string, key, payload []byte, lineage *databus.OpenLineageHeaders) error
}

type ProjectionIndex struct {
	seen map[string]uint64
}

func NewProjectionIndex() *ProjectionIndex {
	return &ProjectionIndex{seen: map[string]uint64{}}
}

func (p *ProjectionIndex) ShouldApply(tenant repos.TenantId, id repos.ObjectId, version uint64) bool {
	if p == nil {
		return true
	}
	key := projectionKey(tenant, id)
	seen, ok := p.seen[key]
	return !ok || seen < version
}

func (p *ProjectionIndex) MarkApplied(tenant repos.TenantId, id repos.ObjectId, version uint64) {
	if p == nil {
		return
	}
	p.seen[projectionKey(tenant, id)] = version
}

func projectionKey(tenant repos.TenantId, id repos.ObjectId) string {
	return string(tenant) + "\x00" + string(id)
}

func RunWithReader(ctx context.Context, cfg *config.Config, log *slog.Logger, reader KafkaReader, backend searchabstraction.SearchBackend) error {
	return RunWithOptions(ctx, cfg, log, reader, backend, nil)
}

func RunWithOptions(ctx context.Context, cfg *config.Config, log *slog.Logger, reader KafkaReader, backend searchabstraction.SearchBackend, dlq DLQPublisher) error {
	if log == nil {
		log = slog.Default()
	}
	log.Info("ontology-indexer starting",
		slog.String("backend", string(cfg.BackendKind)),
		slog.String("search_endpoint", redactedEndpoint(cfg.SearchEndpoint)),
		slog.String("kafka_bootstrap", cfg.KafkaBootstrap),
		slog.String("consumer_group", defaultStr(cfg.ConsumerGroup, ConsumerGroup)),
	)
	if err := reader.Subscribe(ctx, SubscribeTopics); err != nil {
		return err
	}
	defer func() {
		if err := reader.Close(); err != nil {
			log.Warn("kafka reader close failed", slog.String("error", err.Error()))
		}
	}()

	projector := NewProjectionIndex()
	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				log.Info("ontology-indexer stopping")
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		outcome, err := processWithRetry(ctx, backend, projector, msg, log, cfg.RetryMaxAttempts, cfg.RetryInitialBackoff, cfg.RetryMaxBackoff)
		if err != nil {
			if dlq == nil || cfg.DLQTopic == "" {
				return err
			}
			if pubErr := dlq.Publish(ctx, cfg.DLQTopic, msg.Key, msg.Value, nil); pubErr != nil {
				return fmt.Errorf("publish %s after processing failure: %w (original: %v)", cfg.DLQTopic, pubErr, err)
			}
			outcome = OutcomeDecodeError
		}
		if err := reader.CommitMessages(ctx, msg); err != nil {
			return err
		}
		log.Debug("ontology-indexer committed record", slog.String("topic", msg.Topic), slog.Int("partition", msg.Partition), slog.Int64("offset", msg.Offset), slog.String("outcome", string(outcome)))
	}
}

func processWithRetry(ctx context.Context, backend searchabstraction.SearchBackend, projector *ProjectionIndex, msg KafkaMessage, log *slog.Logger, attempts int, initial, max time.Duration) (RecordOutcome, error) {
	if attempts < 1 {
		attempts = 1
	}
	if initial <= 0 {
		initial = 100 * time.Millisecond
	}
	if max <= 0 {
		max = 2 * time.Second
	}
	var outcome RecordOutcome
	var err error
	backoff := initial
	for attempt := 1; attempt <= attempts; attempt++ {
		outcome, err = ProcessMessageWithProjector(ctx, backend, projector, msg, log)
		if err == nil {
			return outcome, nil
		}
		if attempt == attempts {
			break
		}
		log.Warn("ontology-indexer retrying failed record", slog.String("topic", msg.Topic), slog.Int64("offset", msg.Offset), slog.Int("attempt", attempt), slog.String("error", err.Error()))
		select {
		case <-ctx.Done():
			return outcome, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > max {
			backoff = max
		}
	}
	return outcome, err
}

func ProcessMessage(ctx context.Context, backend searchabstraction.SearchBackend, msg KafkaMessage, log *slog.Logger) (RecordOutcome, error) {
	return ProcessMessageWithProjector(ctx, backend, nil, msg, log)
}

func ProcessMessageWithProjector(ctx context.Context, backend searchabstraction.SearchBackend, projector *ProjectionIndex, msg KafkaMessage, log *slog.Logger) (RecordOutcome, error) {
	if log == nil {
		log = slog.Default()
	}
	switch msg.Topic {
	case TopicObjectChangedV1:
		return processObjectChanged(ctx, backend, projector, msg, log)
	case TopicLinkChangedV1:
		return processLinkChanged(ctx, backend, projector, msg, log)
	default:
		log.Warn("ontology-indexer skipping record from unknown topic", slog.String("topic", msg.Topic))
		return OutcomeDecodeError, nil
	}
}

func processObjectChanged(ctx context.Context, backend searchabstraction.SearchBackend, projector *ProjectionIndex, msg KafkaMessage, log *slog.Logger) (RecordOutcome, error) {
	var evt ObjectChangedV1
	if err := json.Unmarshal(msg.Value, &evt); err != nil || evt.Tenant == "" || evt.ID == "" || evt.TypeID == "" {
		log.Warn("ontology-indexer skipping malformed object event", slog.String("topic", msg.Topic), slog.Int64("offset", msg.Offset), slog.Any("error", err))
		return OutcomeDecodeError, nil
	}
	if !projector.ShouldApply(evt.Tenant, evt.ID, evt.Version) {
		return OutcomeSkippedStale, nil
	}
	if evt.Deleted {
		_, err := backend.Delete(ctx, evt.Tenant, evt.ID)
		if err == nil {
			projector.MarkApplied(evt.Tenant, evt.ID, evt.Version)
		}
		return OutcomeDeleted, err
	}
	if len(evt.Payload) == 0 {
		evt.Payload = json.RawMessage(`{}`)
	}
	doc := searchabstraction.IndexDoc{Tenant: evt.Tenant, ID: evt.ID, TypeID: evt.TypeID, Payload: cloneRaw(evt.Payload), Version: evt.Version, Embedding: append([]float32(nil), evt.Embedding...)}
	err := backend.Index(ctx, doc)
	if err == nil {
		projector.MarkApplied(evt.Tenant, evt.ID, evt.Version)
	}
	return OutcomeIndexed, err
}

func processLinkChanged(ctx context.Context, backend searchabstraction.SearchBackend, projector *ProjectionIndex, msg KafkaMessage, log *slog.Logger) (RecordOutcome, error) {
	var evt LinkChangedV1
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		log.Warn("ontology-indexer skipping malformed link event", slog.String("topic", msg.Topic), slog.Int64("offset", msg.Offset), slog.Any("error", err))
		return OutcomeDecodeError, nil
	}
	normalizeLinkEvent(&evt)
	if evt.Tenant == "" || evt.LinkType == "" || evt.From == "" || evt.To == "" {
		log.Warn("ontology-indexer skipping malformed link event", slog.String("topic", msg.Topic), slog.Int64("offset", msg.Offset))
		return OutcomeDecodeError, nil
	}
	id := linkDocumentID(evt)
	if !projector.ShouldApply(evt.Tenant, id, evt.Version) {
		return OutcomeSkippedStale, nil
	}
	if evt.Deleted {
		_, err := backend.Delete(ctx, evt.Tenant, id)
		if err == nil {
			projector.MarkApplied(evt.Tenant, id, evt.Version)
		}
		return OutcomeDeleted, err
	}
	payload := linkPayload(evt)
	doc := searchabstraction.IndexDoc{Tenant: evt.Tenant, ID: id, TypeID: linkDocType(evt.LinkType), Payload: payload, Version: evt.Version}
	err := backend.Index(ctx, doc)
	if err == nil {
		projector.MarkApplied(evt.Tenant, id, evt.Version)
	}
	return OutcomeIndexed, err
}

func normalizeLinkEvent(evt *LinkChangedV1) {
	if evt.LinkType == "" {
		evt.LinkType = evt.TypeID
	}
	if evt.From == "" {
		evt.From = evt.SourceID
	}
	if evt.To == "" {
		evt.To = evt.TargetID
	}
}

func linkDocumentID(evt LinkChangedV1) repos.ObjectId {
	if evt.ID != "" {
		return evt.ID
	}
	return repos.ObjectId("link:" + string(evt.LinkType) + ":" + string(evt.From) + ":" + string(evt.To))
}

func linkDocType(linkType repos.LinkTypeId) repos.TypeId {
	return repos.TypeId("__link_" + searchabstraction.SanitizeDocType(string(linkType)))
}

func linkPayload(evt LinkChangedV1) json.RawMessage {
	var props json.RawMessage = json.RawMessage(`{}`)
	if len(evt.Payload) > 0 {
		props = cloneRaw(evt.Payload)
	}
	b, _ := json.Marshal(map[string]any{
		"kind":      "ontology_link",
		"link_type": string(evt.LinkType),
		"from":      string(evt.From),
		"to":        string(evt.To),
		"payload":   json.RawMessage(props),
	})
	return b
}

func cloneRaw(v json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), v...) }

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// redactedEndpoint hides query strings / passwords from logs.
func redactedEndpoint(ep string) string {
	if ep == "" {
		return "(unset)"
	}
	if i := indexAt(ep); i >= 0 {
		return "***" + ep[i:]
	}
	return ep
}

func indexAt(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '@' {
			return i
		}
	}
	return -1
}
