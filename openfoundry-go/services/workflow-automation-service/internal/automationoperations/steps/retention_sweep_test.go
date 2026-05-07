package steps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	saga "github.com/openfoundry/openfoundry-go/libs/saga"
)

func TestRetentionSweepExecuteSuccess(t *testing.T) {
	t.Parallel()
	var got RetentionSweepInput
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, retentionSweepPath, r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"evicted":7,"older_than_days":30,"dry_run":true}`))
	}))
	t.Cleanup(srv.Close)

	step := EvictRetentionEligible{Client: NewHTTPRetentionSweepClient(srv.Client(), srv.URL, "test-token")}
	out, err := step.Execute(context.Background(), RetentionSweepInput{TenantID: "acme", OlderThanDays: 30, DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, RetentionSweepInput{TenantID: "acme", OlderThanDays: 30, DryRun: true}, got)
	assert.Equal(t, RetentionSweepOutput{Evicted: 7, OlderThanDays: 30, DryRun: true}, out)
}

func TestRetentionSweepExecuteMapsAuditCompliance4xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSONStatus(w, http.StatusBadRequest, `{"error":"tenant not found"}`)
	}))
	t.Cleanup(srv.Close)

	step := EvictRetentionEligible{Client: NewHTTPRetentionSweepClient(srv.Client(), srv.URL, "")}
	_, err := step.Execute(context.Background(), RetentionSweepInput{TenantID: "missing", OlderThanDays: 30})
	require.Error(t, err)
	assert.True(t, saga.IsStepFailure(err))
	assert.Contains(t, err.Error(), "rejected request")
	assert.Contains(t, err.Error(), "HTTP 400")
}

func TestRetentionSweepExecuteMapsAuditCompliance5xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSONStatus(w, http.StatusServiceUnavailable, `{"error":"db unavailable"}`)
	}))
	t.Cleanup(srv.Close)

	step := EvictRetentionEligible{Client: NewHTTPRetentionSweepClient(srv.Client(), srv.URL, "")}
	_, err := step.Execute(context.Background(), RetentionSweepInput{TenantID: "acme", OlderThanDays: 30})
	require.Error(t, err)
	assert.True(t, saga.IsStepFailure(err))
	assert.Contains(t, err.Error(), "unavailable")
	assert.Contains(t, err.Error(), "HTTP 503")
}

func TestRetentionSweepExecuteMapsContextCancel(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	step := EvictRetentionEligible{Client: NewHTTPRetentionSweepClient(srv.Client(), srv.URL, "")}
	_, err := step.Execute(ctx, RetentionSweepInput{TenantID: "acme", OlderThanDays: 30})
	require.Error(t, err)
	assert.True(t, saga.IsStepFailure(err))
	assert.True(t, strings.Contains(err.Error(), context.Canceled.Error()) || errors.Is(err, context.Canceled))
}

func TestRetentionSweepExecuteRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	step := EvictRetentionEligible{Client: NewHTTPRetentionSweepClient(http.DefaultClient, "http://audit", "")}
	_, err := step.Execute(context.Background(), RetentionSweepInput{OlderThanDays: 30})
	require.Error(t, err)
	assert.True(t, saga.IsStepFailure(err))
	assert.Contains(t, err.Error(), "tenant_id is required")
}

func TestRetentionSweepInputDefaults(t *testing.T) {
	t.Parallel()
	var in RetentionSweepInput
	require.NoError(t, json.Unmarshal([]byte(`{"tenant_id":"acme"}`), &in))
	assert.Equal(t, uint32(90), in.OlderThanDays)
	assert.False(t, in.DryRun)
}

func writeJSONStatus(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
