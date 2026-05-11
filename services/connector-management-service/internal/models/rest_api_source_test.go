package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRESTAPISourceConfigFromDomain(t *testing.T) {
	raw := json.RawMessage(`{
		"domain": "api.open-meteo.com",
		"auth": {"type": "None"},
		"runtime": {"worker": "foundry", "allowed_methods": ["post", "GET", "GET"]},
		"permissions": {"invokable": true},
		"webhook": {"path": "/v1/forecast"}
	}`)

	normalized, err := NormalizeRESTAPISourceConfig(raw)
	require.NoError(t, err)
	var cfg RESTAPISourceConfig
	require.NoError(t, json.Unmarshal(normalized, &cfg))
	require.Equal(t, "rest_api", cfg.SourceKind)
	require.Equal(t, "api.open-meteo.com", cfg.Domain)
	require.Equal(t, "https://api.open-meteo.com", cfg.BaseURL)
	require.Equal(t, RESTAPIAuthNone, cfg.Auth.Type)
	require.Equal(t, RESTAPIWorkerFoundry, cfg.Runtime.Worker)
	require.Equal(t, 10000, cfg.Runtime.TimeoutMS)
	require.Equal(t, []string{"GET", "POST"}, cfg.Runtime.AllowedMethods)
	require.True(t, cfg.Permissions.Invokable)
	require.Equal(t, []string{"api.open-meteo.com"}, cfg.Permissions.AllowedEgressHosts)
	require.NotNil(t, cfg.Webhook)
	require.Equal(t, "GET", cfg.Webhook.Method)
	require.Equal(t, "/v1/forecast", cfg.Webhook.Path)
}

func TestNormalizeRESTAPISourceConfigPreservesAuthAndRuntimePolicy(t *testing.T) {
	raw := json.RawMessage(`{
		"base_url": "https://api.example.com:8443/root/",
		"auth": {
			"type": "api key",
			"query_param": "api_key",
			"secret_ref": "secret://rest-api/open-meteo"
		},
		"secrets": {"api_key": "secret://rest-api/open-meteo"},
		"runtime": {
			"worker": "agent",
			"timeout_ms": 45000,
			"retry_count": 2,
			"allowed_methods": ["GET", "POST", "PATCH"],
			"allow_private_networks": true
		},
		"permissions": {
			"discoverable": true,
			"syncable": false,
			"invokable": true,
			"usable_in_code": true,
			"allowed_egress_hosts": ["api.example.com:8443", "api.example.com:8443"]
		}
	}`)

	cfg, err := ParseRESTAPISourceConfig(raw)
	require.NoError(t, err)
	require.Equal(t, "api.example.com:8443", cfg.Domain)
	require.Equal(t, "https://api.example.com:8443/root", cfg.BaseURL)
	require.Equal(t, RESTAPIAuthAPIKey, cfg.Auth.Type)
	require.Equal(t, "api_key", cfg.Auth.QueryParam)
	require.Equal(t, "secret://rest-api/open-meteo", cfg.Auth.SecretRef)
	require.Equal(t, RESTAPIWorkerAgent, cfg.Runtime.Worker)
	require.Equal(t, 45000, cfg.Runtime.TimeoutMS)
	require.True(t, cfg.Runtime.AllowPrivateNetworks)
	require.Equal(t, []string{"api.example.com:8443"}, cfg.Permissions.AllowedEgressHosts)
}

func TestNormalizeRESTAPISourceConfigRejectsInvalidSource(t *testing.T) {
	_, err := NormalizeRESTAPISourceConfig(json.RawMessage(`{"base_url":"ftp://example.com"}`))
	require.ErrorContains(t, err, "http or https")

	_, err = NormalizeRESTAPISourceConfig(json.RawMessage(`{"domain":"api.example.com","auth":{"type":"digest"}}`))
	require.ErrorContains(t, err, "auth.type")

	_, err = NormalizeRESTAPISourceConfig(json.RawMessage(`{"domain":"api.example.com","runtime":{"worker":"carrier"}}`))
	require.ErrorContains(t, err, "runtime.worker")
}
