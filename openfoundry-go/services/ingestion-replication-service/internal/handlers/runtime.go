package handlers

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	kafka "github.com/segmentio/kafka-go"

	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/models"
)

// RuntimeErrorKind classifies runtime failures so HTTP handlers can map them
// deterministically instead of collapsing Kafka/Flink errors into 500s.
type RuntimeErrorKind string

const (
	RuntimeUnavailable RuntimeErrorKind = "unavailable"
	RuntimeValidation  RuntimeErrorKind = "validation"
	RuntimeConflict    RuntimeErrorKind = "conflict"
	RuntimeUpstream    RuntimeErrorKind = "upstream"
)

type RuntimeError struct {
	Kind RuntimeErrorKind
	Msg  string
}

func (e *RuntimeError) Error() string { return e.Msg }

func runtimeErr(kind RuntimeErrorKind, format string, args ...any) error {
	return &RuntimeError{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

func runtimeHTTPStatus(err error) int {
	var rt *RuntimeError
	if !errors.As(err, &rt) {
		return http.StatusInternalServerError
	}
	switch rt.Kind {
	case RuntimeUnavailable:
		return http.StatusServiceUnavailable
	case RuntimeValidation:
		return http.StatusBadRequest
	case RuntimeConflict:
		return http.StatusConflict
	case RuntimeUpstream:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

type KafkaTopicSpec struct {
	Topic          string          `json:"topic"`
	Partitions     int32           `json:"partitions"`
	RetentionHours int32           `json:"retention_hours"`
	Compression    bool            `json:"compression"`
	Schema         json.RawMessage `json:"schema"`
	SourceBinding  json.RawMessage `json:"source_binding"`
}

type FlinkJobSpec struct {
	JobName              string          `json:"job_name"`
	Topic                string          `json:"topic"`
	StreamID             uuid.UUID       `json:"stream_id"`
	StreamName           string          `json:"stream_name"`
	CheckpointIntervalMS int32           `json:"checkpoint_interval_ms"`
	PipelineConsistency  string          `json:"pipeline_consistency"`
	Schema               json.RawMessage `json:"schema"`
}

type CdcRegistrationSpec struct {
	StreamID        uuid.UUID       `json:"stream_id"`
	Slug            string          `json:"slug"`
	SourceKind      string          `json:"source_kind"`
	SourceRef       string          `json:"source_ref"`
	Topic           string          `json:"topic"`
	PrimaryKeys     json.RawMessage `json:"primary_keys"`
	WatermarkColumn *string         `json:"watermark_column,omitempty"`
	IncrementalMode string          `json:"incremental_mode"`
}

type CdcRegistrationResult struct {
	Checkpoint *models.CheckpointUpdate `json:"checkpoint,omitempty"`
	Resolution *models.ResolutionUpdate `json:"resolution,omitempty"`
}

type KafkaAdmin interface {
	ProvisionTopic(ctx context.Context, spec KafkaTopicSpec) error
	UpdateTopic(ctx context.Context, spec KafkaTopicSpec) error
	DeleteTopic(ctx context.Context, topic string) error
	RegisterCDCSource(ctx context.Context, spec CdcRegistrationSpec) (*CdcRegistrationResult, error)
}

type FlinkDeployer interface {
	DeployStream(ctx context.Context, spec FlinkJobSpec) error
	UpdateStream(ctx context.Context, spec FlinkJobSpec) error
	RegisterCDCJob(ctx context.Context, spec CdcRegistrationSpec) (*CdcRegistrationResult, error)
}

// ProductionStreamingRuntime materializes streams in Kafka and Flink. It is
// intentionally not a no-op: missing adapters are surfaced as RuntimeUnavailable.
type ProductionStreamingRuntime struct {
	Kafka KafkaAdmin
	Flink FlinkDeployer
}

func NewProductionStreamingRuntime(kafka KafkaAdmin, flink FlinkDeployer) *ProductionStreamingRuntime {
	return &ProductionStreamingRuntime{Kafka: kafka, Flink: flink}
}

func (r *ProductionStreamingRuntime) ProvisionStream(ctx context.Context, stream *models.StreamDefinition) error {
	if stream == nil {
		return runtimeErr(RuntimeValidation, "stream is required")
	}
	if r == nil || r.Kafka == nil || r.Flink == nil {
		return runtimeErr(RuntimeUnavailable, "streaming runtime is not configured")
	}
	topic := topicName(stream)
	if err := r.Kafka.ProvisionTopic(ctx, topicSpec(stream, topic)); err != nil {
		return wrapRuntimeUpstream("kafka provision topic", err)
	}
	if err := r.Flink.DeployStream(ctx, flinkSpec(stream, topic)); err != nil {
		return wrapRuntimeUpstream("flink deploy stream", err)
	}
	return nil
}

func (r *ProductionStreamingRuntime) UpdateStream(ctx context.Context, stream *models.StreamDefinition) error {
	if stream == nil {
		return runtimeErr(RuntimeValidation, "stream is required")
	}
	if r == nil || r.Kafka == nil || r.Flink == nil {
		return runtimeErr(RuntimeUnavailable, "streaming runtime is not configured")
	}
	topic := topicName(stream)
	if err := r.Kafka.UpdateTopic(ctx, topicSpec(stream, topic)); err != nil {
		return wrapRuntimeUpstream("kafka update topic", err)
	}
	if err := r.Flink.UpdateStream(ctx, flinkSpec(stream, topic)); err != nil {
		return wrapRuntimeUpstream("flink update stream", err)
	}
	return nil
}

// ResetStream truncates the Kafka topic backing this stream by deleting
// it and re-provisioning it with the same partition / retention spec.
// Effect: all records are dropped and consumer-group offsets become
// invalid, which is exactly the "reset" semantics callers expect. Both
// DeleteTopic and ProvisionTopic are idempotent so retries after a
// partial failure converge.
func (r *ProductionStreamingRuntime) ResetStream(ctx context.Context, stream *models.StreamDefinition) error {
	if stream == nil {
		return runtimeErr(RuntimeValidation, "stream is required")
	}
	if r == nil || r.Kafka == nil {
		return runtimeErr(RuntimeUnavailable, "streaming runtime is not configured")
	}
	topic := topicName(stream)
	if err := r.Kafka.DeleteTopic(ctx, topic); err != nil {
		return wrapRuntimeUpstream("kafka delete topic", err)
	}
	if err := r.Kafka.ProvisionTopic(ctx, topicSpec(stream, topic)); err != nil {
		return wrapRuntimeUpstream("kafka re-provision topic", err)
	}
	return nil
}

func (r *ProductionStreamingRuntime) RegisterCDC(ctx context.Context, stream *models.CdcStream) (*CdcRegistrationResult, error) {
	if stream == nil {
		return nil, runtimeErr(RuntimeValidation, "cdc stream is required")
	}
	if r == nil || r.Kafka == nil || r.Flink == nil {
		return nil, runtimeErr(RuntimeUnavailable, "streaming runtime is not configured")
	}
	topic := "cdc." + sanitizeName(stream.Slug)
	if stream.UpstreamTopic != nil && strings.TrimSpace(*stream.UpstreamTopic) != "" {
		topic = strings.TrimSpace(*stream.UpstreamTopic)
	}
	spec := CdcRegistrationSpec{
		StreamID:        stream.ID,
		Slug:            stream.Slug,
		SourceKind:      stream.SourceKind,
		SourceRef:       stream.SourceRef,
		Topic:           topic,
		PrimaryKeys:     stream.PrimaryKeys,
		WatermarkColumn: stream.WatermarkColumn,
		IncrementalMode: stream.IncrementalMode,
	}
	kafkaResult, err := r.Kafka.RegisterCDCSource(ctx, spec)
	if err != nil {
		return nil, wrapRuntimeUpstream("kafka register cdc source", err)
	}
	flinkResult, err := r.Flink.RegisterCDCJob(ctx, spec)
	if err != nil {
		return nil, wrapRuntimeUpstream("flink register cdc job", err)
	}
	return mergeCDCResults(kafkaResult, flinkResult), nil
}

func wrapRuntimeUpstream(operation string, err error) error {
	var rt *RuntimeError
	if errors.As(err, &rt) {
		return rt
	}
	return runtimeErr(RuntimeUpstream, "%s: %v", operation, err)
}

func mergeCDCResults(results ...*CdcRegistrationResult) *CdcRegistrationResult {
	merged := &CdcRegistrationResult{}
	for _, result := range results {
		if result == nil {
			continue
		}
		if result.Checkpoint != nil {
			merged.Checkpoint = result.Checkpoint
		}
		if result.Resolution != nil {
			merged.Resolution = result.Resolution
		}
	}
	return merged
}

func topicSpec(stream *models.StreamDefinition, topic string) KafkaTopicSpec {
	return KafkaTopicSpec{Topic: topic, Partitions: stream.Partitions, RetentionHours: stream.RetentionHours, Compression: stream.Compression, Schema: stream.Schema, SourceBinding: stream.SourceBinding}
}

func flinkSpec(stream *models.StreamDefinition, topic string) FlinkJobSpec {
	return FlinkJobSpec{JobName: "flink-" + topic, Topic: topic, StreamID: stream.ID, StreamName: stream.Name, CheckpointIntervalMS: stream.CheckpointIntervalMS, PipelineConsistency: stream.PipelineConsistency, Schema: stream.Schema}
}

func topicName(stream *models.StreamDefinition) string {
	return "stream." + sanitizeName(stream.Name) + "." + shortID(stream.ID)
}

var nonTopicChar = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeName(v string) string {
	v = strings.Trim(nonTopicChar.ReplaceAllString(strings.ToLower(strings.TrimSpace(v)), "-"), "-._")
	if v == "" {
		return "unnamed"
	}
	return v
}

func shortID(id uuid.UUID) string {
	sum := sha1.Sum([]byte(id.String()))
	return hex.EncodeToString(sum[:])[:12]
}

// HTTPKafkaAdmin is the production adapter for Kafka topic management and
// the Kafka Connect control plane.
//
// Topic lifecycle (ProvisionTopic / UpdateTopic / DeleteTopic) goes
// directly to the broker via segmentio/kafka-go's admin API when
// BootstrapServers is set, mirroring the Rust hot_buffer::kafka path
// (AdminClient::create_topics with idempotent TopicAlreadyExists). When
// BootstrapServers is empty the legacy HTTP shim at BaseURL is used so
// existing deployments keep working.
//
// CDC source registration (RegisterCDCSource) always goes via HTTP — it
// targets a Kafka Connect cluster, not the broker admin API.
type HTTPKafkaAdmin struct {
	BaseURL string
	Client  *http.Client

	BootstrapServers  []string
	Transport         kafka.RoundTripper
	ReplicationFactor int
	RequestTimeout    time.Duration

	adminClient kafkaTopicAdminClient
}

// kafkaTopicAdminClient is the tiny subset of segmentio/kafka-go's
// *kafka.Client that HTTPKafkaAdmin uses, exposed as an interface so
// tests can inject a fake without spinning a broker.
type kafkaTopicAdminClient interface {
	CreateTopics(ctx context.Context, req *kafka.CreateTopicsRequest) (*kafka.CreateTopicsResponse, error)
	DeleteTopics(ctx context.Context, req *kafka.DeleteTopicsRequest) (*kafka.DeleteTopicsResponse, error)
}

func (a *HTTPKafkaAdmin) ProvisionTopic(ctx context.Context, spec KafkaTopicSpec) error {
	if a.useDirectKafka() {
		return a.kafkaEnsureTopic(ctx, spec)
	}
	return a.do(ctx, http.MethodPost, "/topics", spec, nil)
}
func (a *HTTPKafkaAdmin) UpdateTopic(ctx context.Context, spec KafkaTopicSpec) error {
	if a.useDirectKafka() {
		return a.kafkaEnsureTopic(ctx, spec)
	}
	return a.do(ctx, http.MethodPut, "/topics/"+spec.Topic, spec, nil)
}
func (a *HTTPKafkaAdmin) DeleteTopic(ctx context.Context, topic string) error {
	if a.useDirectKafka() {
		return a.kafkaDeleteTopic(ctx, topic)
	}
	return a.do(ctx, http.MethodDelete, "/topics/"+topic, nil, nil)
}
func (a *HTTPKafkaAdmin) RegisterCDCSource(ctx context.Context, spec CdcRegistrationSpec) (*CdcRegistrationResult, error) {
	var out CdcRegistrationResult
	if err := a.do(ctx, http.MethodPost, "/cdc/sources", spec, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *HTTPKafkaAdmin) useDirectKafka() bool {
	return a.adminClient != nil || len(a.BootstrapServers) > 0
}

func (a *HTTPKafkaAdmin) topicAdmin() kafkaTopicAdminClient {
	if a.adminClient != nil {
		return a.adminClient
	}
	return &kafka.Client{
		Addr:      kafka.TCP(a.BootstrapServers...),
		Timeout:   a.kafkaRequestTimeout(),
		Transport: a.Transport,
	}
}

func (a *HTTPKafkaAdmin) kafkaRequestTimeout() time.Duration {
	if a.RequestTimeout > 0 {
		return a.RequestTimeout
	}
	return 15 * time.Second
}

func (a *HTTPKafkaAdmin) kafkaReplicationFactor() int {
	if a.ReplicationFactor > 0 {
		return a.ReplicationFactor
	}
	return 1
}

// kafkaEnsureTopic is the idempotent topic-create path used by both
// ProvisionTopic and UpdateTopic. Mirrors KafkaHotBuffer::ensure_topic
// in the Rust source: TopicAlreadyExists is the happy path so repeat
// calls are no-ops.
func (a *HTTPKafkaAdmin) kafkaEnsureTopic(ctx context.Context, spec KafkaTopicSpec) error {
	topic := strings.TrimSpace(spec.Topic)
	if topic == "" {
		return runtimeErr(RuntimeValidation, "kafka topic name is required")
	}
	if spec.Partitions < 1 {
		return runtimeErr(RuntimeValidation, "kafka partitions must be >= 1, got %d", spec.Partitions)
	}
	cfg := kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     int(spec.Partitions),
		ReplicationFactor: a.kafkaReplicationFactor(),
		ConfigEntries:     buildKafkaTopicConfigs(spec),
	}
	reqCtx, cancel := context.WithTimeout(ctx, a.kafkaRequestTimeout())
	defer cancel()
	resp, err := a.topicAdmin().CreateTopics(reqCtx, &kafka.CreateTopicsRequest{
		Topics: []kafka.TopicConfig{cfg},
	})
	if err != nil {
		return runtimeErr(RuntimeUpstream, "kafka create_topics(%s): %v", topic, err)
	}
	if topicErr, ok := resp.Errors[topic]; ok && topicErr != nil {
		if errors.Is(topicErr, kafka.TopicAlreadyExists) {
			return nil
		}
		return mapKafkaTopicError("create_topics", topic, topicErr)
	}
	return nil
}

// kafkaDeleteTopic deletes the topic via DeleteTopics and treats
// UnknownTopicOrPartition as success so deprovision is idempotent.
func (a *HTTPKafkaAdmin) kafkaDeleteTopic(ctx context.Context, topic string) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return runtimeErr(RuntimeValidation, "kafka topic name is required")
	}
	reqCtx, cancel := context.WithTimeout(ctx, a.kafkaRequestTimeout())
	defer cancel()
	resp, err := a.topicAdmin().DeleteTopics(reqCtx, &kafka.DeleteTopicsRequest{
		Topics: []string{topic},
	})
	if err != nil {
		return runtimeErr(RuntimeUpstream, "kafka delete_topics(%s): %v", topic, err)
	}
	if topicErr, ok := resp.Errors[topic]; ok && topicErr != nil {
		if errors.Is(topicErr, kafka.UnknownTopicOrPartition) {
			return nil
		}
		return mapKafkaTopicError("delete_topics", topic, topicErr)
	}
	return nil
}

func buildKafkaTopicConfigs(spec KafkaTopicSpec) []kafka.ConfigEntry {
	var entries []kafka.ConfigEntry
	if spec.RetentionHours > 0 {
		ms := int64(spec.RetentionHours) * int64(time.Hour/time.Millisecond)
		entries = append(entries, kafka.ConfigEntry{
			ConfigName:  "retention.ms",
			ConfigValue: strconv.FormatInt(ms, 10),
		})
	}
	if spec.Compression {
		entries = append(entries, kafka.ConfigEntry{
			ConfigName:  "compression.type",
			ConfigValue: "lz4",
		})
	}
	return entries
}

func mapKafkaTopicError(op, topic string, err error) error {
	var kerr kafka.Error
	if errors.As(err, &kerr) {
		switch kerr {
		case kafka.InvalidTopic, kafka.InvalidConfiguration:
			return runtimeErr(RuntimeValidation, "kafka %s(%s): %v", op, topic, err)
		case kafka.TopicAuthorizationFailed, kafka.ClusterAuthorizationFailed:
			return runtimeErr(RuntimeUpstream, "kafka %s(%s): %v", op, topic, err)
		}
	}
	return runtimeErr(RuntimeUpstream, "kafka %s(%s): %v", op, topic, err)
}

func (a *HTTPKafkaAdmin) do(ctx context.Context, method, path string, in any, out any) error {
	base := strings.TrimRight(a.BaseURL, "/")
	if base == "" {
		return runtimeErr(RuntimeUnavailable, "KAFKA_RUNTIME_URL is not configured")
	}
	return doRuntimeJSON(ctx, a.Client, method, base+path, in, out)
}

// HTTPFlinkDeployer is the production adapter for the Flink deployer service.
type HTTPFlinkDeployer struct {
	BaseURL string
	Client  *http.Client
}

func (d *HTTPFlinkDeployer) DeployStream(ctx context.Context, spec FlinkJobSpec) error {
	return d.do(ctx, http.MethodPost, "/jobs", spec, nil)
}
func (d *HTTPFlinkDeployer) UpdateStream(ctx context.Context, spec FlinkJobSpec) error {
	return d.do(ctx, http.MethodPut, "/jobs/"+spec.JobName, spec, nil)
}
func (d *HTTPFlinkDeployer) RegisterCDCJob(ctx context.Context, spec CdcRegistrationSpec) (*CdcRegistrationResult, error) {
	var out CdcRegistrationResult
	if err := d.do(ctx, http.MethodPost, "/cdc/jobs", spec, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (d *HTTPFlinkDeployer) do(ctx context.Context, method, path string, in any, out any) error {
	base := strings.TrimRight(d.BaseURL, "/")
	if base == "" {
		return runtimeErr(RuntimeUnavailable, "FLINK_RUNTIME_URL is not configured")
	}
	return doRuntimeJSON(ctx, d.Client, method, base+path, in, out)
}

func doRuntimeJSON(ctx context.Context, client *http.Client, method, url string, in any, out any) error {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return runtimeErr(RuntimeValidation, "marshal runtime request: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return runtimeErr(RuntimeValidation, "build runtime request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return runtimeErr(RuntimeUnavailable, "runtime request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return runtimeErr(RuntimeConflict, "runtime returned HTTP 409")
	}
	if resp.StatusCode == http.StatusBadRequest {
		return runtimeErr(RuntimeValidation, "runtime returned HTTP 400")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return runtimeErr(RuntimeUpstream, "runtime returned HTTP %d", resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return runtimeErr(RuntimeUpstream, "decode runtime response: %v", err)
		}
	}
	return nil
}
