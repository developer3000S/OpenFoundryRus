package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kafka "github.com/segmentio/kafka-go"
)

type fakeTopicAdmin struct {
	createReq  *kafka.CreateTopicsRequest
	deleteReq  *kafka.DeleteTopicsRequest
	createResp *kafka.CreateTopicsResponse
	deleteResp *kafka.DeleteTopicsResponse
	createErr  error
	deleteErr  error
}

func (f *fakeTopicAdmin) CreateTopics(_ context.Context, req *kafka.CreateTopicsRequest) (*kafka.CreateTopicsResponse, error) {
	f.createReq = req
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createResp != nil {
		return f.createResp, nil
	}
	return &kafka.CreateTopicsResponse{Errors: map[string]error{}}, nil
}
func (f *fakeTopicAdmin) DeleteTopics(_ context.Context, req *kafka.DeleteTopicsRequest) (*kafka.DeleteTopicsResponse, error) {
	f.deleteReq = req
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	if f.deleteResp != nil {
		return f.deleteResp, nil
	}
	return &kafka.DeleteTopicsResponse{Errors: map[string]error{}}, nil
}

func newKafkaAdminWithFake(fake kafkaTopicAdminClient) *HTTPKafkaAdmin {
	return &HTTPKafkaAdmin{adminClient: fake}
}

func TestHTTPKafkaAdminProvisionTopicCreatesViaKafka(t *testing.T) {
	t.Parallel()
	fake := &fakeTopicAdmin{}
	a := newKafkaAdminWithFake(fake)
	err := a.ProvisionTopic(context.Background(), KafkaTopicSpec{
		Topic:          "stream.orders.abc",
		Partitions:     6,
		RetentionHours: 24,
		Compression:    true,
	})
	require.NoError(t, err)
	require.NotNil(t, fake.createReq)
	require.Len(t, fake.createReq.Topics, 1)
	tc := fake.createReq.Topics[0]
	assert.Equal(t, "stream.orders.abc", tc.Topic)
	assert.Equal(t, 6, tc.NumPartitions)
	assert.Equal(t, 1, tc.ReplicationFactor)
	configs := map[string]string{}
	for _, e := range tc.ConfigEntries {
		configs[e.ConfigName] = e.ConfigValue
	}
	assert.Equal(t, "86400000", configs["retention.ms"])
	assert.Equal(t, "lz4", configs["compression.type"])
}

func TestHTTPKafkaAdminProvisionTopicTreatsAlreadyExistsAsSuccess(t *testing.T) {
	t.Parallel()
	fake := &fakeTopicAdmin{
		createResp: &kafka.CreateTopicsResponse{
			Errors: map[string]error{"t": kafka.TopicAlreadyExists},
		},
	}
	a := newKafkaAdminWithFake(fake)
	err := a.ProvisionTopic(context.Background(), KafkaTopicSpec{Topic: "t", Partitions: 1})
	require.NoError(t, err)
}

func TestHTTPKafkaAdminUpdateTopicUsesEnsureTopicPath(t *testing.T) {
	t.Parallel()
	fake := &fakeTopicAdmin{}
	a := newKafkaAdminWithFake(fake)
	err := a.UpdateTopic(context.Background(), KafkaTopicSpec{Topic: "t", Partitions: 3})
	require.NoError(t, err)
	require.NotNil(t, fake.createReq)
	assert.Nil(t, fake.deleteReq)
}

func TestHTTPKafkaAdminProvisionTopicRejectsBadPartitions(t *testing.T) {
	t.Parallel()
	a := newKafkaAdminWithFake(&fakeTopicAdmin{})
	for _, n := range []int32{0, -1} {
		err := a.ProvisionTopic(context.Background(), KafkaTopicSpec{Topic: "t", Partitions: n})
		require.Error(t, err)
		var rt *RuntimeError
		require.True(t, errors.As(err, &rt))
		assert.Equal(t, RuntimeValidation, rt.Kind)
	}
}

func TestHTTPKafkaAdminProvisionTopicRejectsEmptyTopic(t *testing.T) {
	t.Parallel()
	a := newKafkaAdminWithFake(&fakeTopicAdmin{})
	err := a.ProvisionTopic(context.Background(), KafkaTopicSpec{Topic: "  ", Partitions: 3})
	require.Error(t, err)
	var rt *RuntimeError
	require.True(t, errors.As(err, &rt))
	assert.Equal(t, RuntimeValidation, rt.Kind)
}

func TestHTTPKafkaAdminProvisionTopicMapsBrokerErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		brokerE error
		want    RuntimeErrorKind
	}{
		{"invalid topic", kafka.InvalidTopic, RuntimeValidation},
		{"invalid config", kafka.InvalidConfiguration, RuntimeValidation},
		{"topic auth", kafka.TopicAuthorizationFailed, RuntimeUpstream},
		{"cluster auth", kafka.ClusterAuthorizationFailed, RuntimeUpstream},
		{"unknown", kafka.Unknown, RuntimeUpstream},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeTopicAdmin{
				createResp: &kafka.CreateTopicsResponse{Errors: map[string]error{"t": tc.brokerE}},
			}
			a := newKafkaAdminWithFake(fake)
			err := a.ProvisionTopic(context.Background(), KafkaTopicSpec{Topic: "t", Partitions: 1})
			require.Error(t, err)
			var rt *RuntimeError
			require.True(t, errors.As(err, &rt))
			assert.Equal(t, tc.want, rt.Kind)
		})
	}
}

func TestHTTPKafkaAdminDeleteTopicCallsKafka(t *testing.T) {
	t.Parallel()
	fake := &fakeTopicAdmin{}
	a := newKafkaAdminWithFake(fake)
	err := a.DeleteTopic(context.Background(), "stream.orders.abc")
	require.NoError(t, err)
	require.NotNil(t, fake.deleteReq)
	assert.Equal(t, []string{"stream.orders.abc"}, fake.deleteReq.Topics)
}

func TestHTTPKafkaAdminDeleteTopicTreatsUnknownTopicAsSuccess(t *testing.T) {
	t.Parallel()
	fake := &fakeTopicAdmin{
		deleteResp: &kafka.DeleteTopicsResponse{
			Errors: map[string]error{"t": kafka.UnknownTopicOrPartition},
		},
	}
	a := newKafkaAdminWithFake(fake)
	err := a.DeleteTopic(context.Background(), "t")
	require.NoError(t, err)
}

func TestHTTPKafkaAdminDeleteTopicRejectsEmpty(t *testing.T) {
	t.Parallel()
	a := newKafkaAdminWithFake(&fakeTopicAdmin{})
	err := a.DeleteTopic(context.Background(), "  ")
	require.Error(t, err)
	var rt *RuntimeError
	require.True(t, errors.As(err, &rt))
	assert.Equal(t, RuntimeValidation, rt.Kind)
}

func TestHTTPKafkaAdminFallsBackToHTTPWhenNoBootstrap(t *testing.T) {
	t.Parallel()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	a := &HTTPKafkaAdmin{BaseURL: srv.URL}
	require.NoError(t, a.ProvisionTopic(context.Background(), KafkaTopicSpec{Topic: "t", Partitions: 1}))
	require.NoError(t, a.UpdateTopic(context.Background(), KafkaTopicSpec{Topic: "t", Partitions: 1}))
	require.NoError(t, a.DeleteTopic(context.Background(), "t"))
	assert.Equal(t, []string{
		"POST /topics",
		"PUT /topics/t",
		"DELETE /topics/t",
	}, seen)
}

func TestHTTPKafkaAdminCDCRegistrationStaysHTTP(t *testing.T) {
	t.Parallel()
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			_, _ = r.Body.Read(buf)
		}
		bodies = append(bodies, r.Method+" "+r.URL.Path+" "+string(buf))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	a := &HTTPKafkaAdmin{
		BaseURL:          srv.URL,
		BootstrapServers: []string{"unused:9092"},
		adminClient:      &fakeTopicAdmin{},
	}
	_, err := a.RegisterCDCSource(context.Background(), CdcRegistrationSpec{Slug: "x"})
	require.NoError(t, err)
	require.Len(t, bodies, 1)
	assert.True(t, strings.HasPrefix(bodies[0], "POST /cdc/sources "))
}
