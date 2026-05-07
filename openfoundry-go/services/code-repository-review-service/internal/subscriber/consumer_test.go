package subscriber

import (
	"context"
	"encoding/json"
	"testing"

	kafka "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeReader struct {
	messages  []kafka.Message
	committed []kafka.Message
}

func (r *fakeReader) FetchMessage(context.Context) (kafka.Message, error) {
	if len(r.messages) == 0 {
		return kafka.Message{}, context.Canceled
	}
	m := r.messages[0]
	r.messages = r.messages[1:]
	return m, nil
}

func (r *fakeReader) CommitMessages(_ context.Context, msgs ...kafka.Message) error {
	r.committed = append(r.committed, msgs...)
	return nil
}

func (r *fakeReader) Close() error { return nil }

type recordingPort struct {
	called int
	events []json.RawMessage
}

func (p *recordingPort) Handle(_ context.Context, event json.RawMessage) error {
	p.called++
	p.events = append(p.events, append(json.RawMessage(nil), event...))
	return nil
}

func TestConsumerDecodesEventAndCommits(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{messages: []kafka.Message{{Topic: Topic, Partition: 0, Offset: 7, Value: []byte(`{"event_type":"dataset.branch.restored.v1","branch_rid":"ri.branch.1"}`)}}}
	port := &recordingPort{}
	consumer := &Consumer{Reader: reader, Port: port}

	require.NoError(t, consumer.RunOnce(context.Background()))
	assert.Equal(t, 1, port.called)
	require.Len(t, port.events, 1)
	assert.JSONEq(t, `{"event_type":"dataset.branch.restored.v1","branch_rid":"ri.branch.1"}`, string(port.events[0]))
	require.Len(t, reader.committed, 1)
	assert.Equal(t, int64(7), reader.committed[0].Offset)
}

func TestConsumerCallsSubscriberPort(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{messages: []kafka.Message{{Topic: Topic, Value: []byte(`{"event_type":"dataset.branch.archived.v1","branch_rid":"ri.branch.2"}`)}}}
	port := &recordingPort{}
	consumer := &Consumer{Reader: reader, Port: port}

	require.NoError(t, consumer.RunOnce(context.Background()))
	assert.Equal(t, 1, port.called)
}

func TestConsumerCommitsMalformedEventWithoutCallingPort(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{messages: []kafka.Message{{Topic: Topic, Offset: 9, Value: []byte(`{"event_type":`)}}}
	port := &recordingPort{}
	consumer := &Consumer{Reader: reader, Port: port}

	require.NoError(t, consumer.RunOnce(context.Background()))
	assert.Equal(t, 0, port.called)
	require.Len(t, reader.committed, 1)
	assert.Equal(t, int64(9), reader.committed[0].Offset)
}
