package conditionconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	databus "github.com/openfoundry/openfoundry-go/libs/event-bus-data"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/event"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/topics"
)

func TestSubscribeTopicsPinned(t *testing.T) {
	if len(SubscribeTopics) != 1 || SubscribeTopics[0] != topics.AutomateConditionV1 {
		t.Fatalf("SubscribeTopics = %#v, want [%q]", SubscribeTopics, topics.AutomateConditionV1)
	}
}

func TestConsumerGroupPinned(t *testing.T) {
	if ConsumerGroup != "workflow-automation-service" {
		t.Fatalf("ConsumerGroup = %q", ConsumerGroup)
	}
}

func TestDecodeConditionRoundTripsMinimalPayload(t *testing.T) {
	definitionID := uuid.New()
	correlationID := uuid.New()
	payload := map[string]any{
		"definition_id":   definitionID,
		"tenant_id":       "acme",
		"correlation_id":  correlationID,
		"triggered_by":    "user-1",
		"trigger_type":    "manual",
		"trigger_payload": map[string]any{"action_id": "promote"},
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	decoded, err := DecodeCondition(bytes)
	if err != nil {
		t.Fatalf("DecodeCondition returned error: %v", err)
	}
	if decoded.DefinitionID != definitionID {
		t.Fatalf("DefinitionID = %s, want %s", decoded.DefinitionID, definitionID)
	}
	if decoded.TenantID != "acme" {
		t.Fatalf("TenantID = %q, want acme", decoded.TenantID)
	}
	var trigger map[string]string
	if err := json.Unmarshal(decoded.TriggerPayload, &trigger); err != nil {
		t.Fatalf("decode trigger payload: %v", err)
	}
	if trigger["action_id"] != "promote" {
		t.Fatalf("trigger action_id = %q, want promote", trigger["action_id"])
	}
}

func TestDecodeConditionSurfacesMalformedInput(t *testing.T) {
	if _, err := DecodeCondition([]byte("{ not json")); err == nil {
		t.Fatal("DecodeCondition returned nil error for malformed input")
	}
}

func TestProcessMessageSkipsEmptyPayloadWithoutConsumer(t *testing.T) {
	label, err := processMessage(context.Background(), nil, &databus.DataMessage{
		Topic:     topics.AutomateConditionV1,
		Partition: 2,
		Offset:    41,
	})
	if err != nil {
		t.Fatalf("processMessage returned error: %v", err)
	}
	if label != "empty_payload" {
		t.Fatalf("label = %q, want empty_payload", label)
	}
}

func TestProcessMessageSkipsMalformedPayloadWithoutConsumer(t *testing.T) {
	label, err := processMessage(context.Background(), nil, &databus.DataMessage{
		Topic: topics.AutomateConditionV1,
		Value: []byte("{ not json"),
	})
	if err != nil {
		t.Fatalf("processMessage returned error: %v", err)
	}
	if label != "decode_error" {
		t.Fatalf("label = %q, want decode_error", label)
	}
}

func TestRunCommitsAfterProcessMessageSkipsMalformedPayload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := &scriptedSubscriber{
		messages: []*databus.DataMessage{{Topic: topics.AutomateConditionV1, Value: []byte("{ not json")}},
		onCommit: cancel,
	}

	err := Run(ctx, nil, sub)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if sub.commits != 1 {
		t.Fatalf("commits = %d, want 1", sub.commits)
	}
}

type scriptedSubscriber struct {
	messages []*databus.DataMessage
	commits  int
	onCommit func()
}

func (s *scriptedSubscriber) Poll(ctx context.Context) (*databus.DataMessage, error) {
	if len(s.messages) == 0 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	msg := s.messages[0]
	s.messages = s.messages[1:]
	return msg, nil
}

func (s *scriptedSubscriber) CommitMessages(_ context.Context, msgs []*databus.DataMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	s.commits++
	if s.onCommit != nil {
		s.onCommit()
	}
	return nil
}

func (s *scriptedSubscriber) CommitOffsets(context.Context) error { return nil }
func (s *scriptedSubscriber) Close() error                        { return nil }

var _ databus.Subscriber = (*scriptedSubscriber)(nil)

// Keep a compile-time touchpoint with the Rust condition payload so this
// package's tests fail if the Go event shape drifts from the runtime slice.
var _ = event.AutomateConditionV1{}
