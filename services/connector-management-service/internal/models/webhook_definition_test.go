package models

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildWeatherWebhookRequestAndExtractOutputs(t *testing.T) {
	source, err := ParseRESTAPISourceConfig(json.RawMessage(`{
		"domain":"api.open-meteo.com",
		"auth":{"type":"none"},
		"runtime":{"worker":"foundry"},
		"permissions":{"invokable":true},
		"webhook":{
			"method":"GET",
			"path":"/v1/forecast",
			"inputs":[
				{"id":"latitude","type":"number","required":true},
				{"id":"longitude","type":"number","required":true}
			],
			"calls":[{
				"id":"weather",
				"method":"GET",
				"path":"/v1/forecast",
				"query_params":{
					"latitude":"{{latitude}}",
					"longitude":"{{longitude}}",
					"current":"temperature_2m,wind_speed_10m,relative_humidity_2m",
					"temperature_unit":"fahrenheit",
					"wind_speed_unit":"mph"
				}
			}],
			"outputs":[
				{"id":"temperature","type":"number","extractor":{"from_call":"weather","path":"/current/temperature_2m"}},
				{"id":"wind_speed","type":"number","extractor":{"from_call":"weather","path":"/current/wind_speed_10m"}},
				{"id":"humidity","type":"number","extractor":{"from_call":"weather","path":"/current/relative_humidity_2m"}}
			]
		}
	}`))
	require.NoError(t, err)
	def, err := WebhookDefinitionFromRESTAPISource(source)
	require.NoError(t, err)
	require.Equal(t, 10000, def.TimeoutMS)
	require.True(t, def.History.Enabled)

	built, err := BuildWebhookRequest(def, source, def.Calls[0], json.RawMessage(`{"latitude":40.016353,"longitude":-105.34458}`), nil)
	require.NoError(t, err)
	require.Equal(t, "GET", built.Method)
	parsed, err := url.Parse(built.URL)
	require.NoError(t, err)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "api.open-meteo.com", parsed.Host)
	require.Equal(t, "/v1/forecast", parsed.Path)
	require.Equal(t, "40.016353", parsed.Query().Get("latitude"))
	require.Equal(t, "-105.34458", parsed.Query().Get("longitude"))
	require.Equal(t, "temperature_2m,wind_speed_10m,relative_humidity_2m", parsed.Query().Get("current"))

	outputs, err := ExtractWebhookOutputs(def, []WebhookCallResult{{
		CallID: "weather",
		Status: 200,
		Response: json.RawMessage(`{
			"current":{
				"temperature_2m":84,
				"wind_speed_10m":4.8,
				"relative_humidity_2m":62
			}
		}`),
	}})
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(outputs, &decoded))
	require.Equal(t, float64(84), decoded["temperature"])
	require.Equal(t, float64(4.8), decoded["wind_speed"])
	require.Equal(t, float64(62), decoded["humidity"])
}

func TestWebhookCallChainingUsesExtractedValues(t *testing.T) {
	def, err := NormalizeWebhookDefinition(json.RawMessage(`{
		"url":"https://api.example.com",
		"calls":[
			{
				"id":"token",
				"method":"POST",
				"path":"/oauth/token",
				"extractors":[{"name":"access_token","path":"/access_token","type":"string"}]
			},
			{
				"id":"forecast",
				"method":"GET",
				"path":"/v1/forecast",
				"headers":{"Authorization":"Bearer {{access_token}}"},
				"query_params":{"latitude":"{{inputs.latitude}}"}
			}
		],
		"outputs":[{"id":"temperature","type":"number","extractor":{"from_call":"forecast","path":"current.temperature_2m"}}]
	}`))
	require.NoError(t, err)
	state := map[string]any{}
	require.NoError(t, CaptureWebhookCallResult(def, def.Calls[0], WebhookCallResult{
		CallID:   "token",
		Status:   200,
		Response: json.RawMessage(`{"access_token":"abc123"}`),
	}, state))
	built, err := BuildWebhookRequest(def, nil, def.Calls[1], json.RawMessage(`{"latitude":40.0}`), state)
	require.NoError(t, err)
	require.Equal(t, "Bearer abc123", built.Headers["Authorization"])
	parsed, err := url.Parse(built.URL)
	require.NoError(t, err)
	require.Equal(t, "40", parsed.Query().Get("latitude"))
}

func TestNormalizeWebhookDefinitionRejectsInvalidLimitsAndOutputs(t *testing.T) {
	_, err := NormalizeWebhookDefinition(json.RawMessage(`{"url":"https://api.example.com","timeout_ms":10}`))
	require.ErrorContains(t, err, "timeout_ms")

	_, err = NormalizeWebhookDefinition(json.RawMessage(`{"url":"https://api.example.com","outputs":[{"type":"number"}]}`))
	require.ErrorContains(t, err, "output")
}

func TestValidateWebhookInvocationEnforcesInputsAndLimits(t *testing.T) {
	def, err := NormalizeWebhookDefinition(json.RawMessage(`{
		"url":"https://api.example.com",
		"inputs":[{"id":"latitude","type":"number","required":true}],
		"calls":[{"id":"a","path":"/a"},{"id":"b","path":"/b"}],
		"limits":{"max_calls":1,"max_input_bytes":32}
	}`))
	require.NoError(t, err)

	err = ValidateWebhookInvocation(def, json.RawMessage(`{"latitude":40.0}`))
	require.ErrorContains(t, err, "max_calls")

	def.Limits.MaxCalls = 2
	err = ValidateWebhookInvocation(def, json.RawMessage(`{"longitude":-105.0}`))
	require.ErrorContains(t, err, "latitude")

	err = ValidateWebhookInvocation(def, json.RawMessage(`{"latitude":"north"}`))
	require.ErrorContains(t, err, "numeric")

	err = ValidateWebhookInvocation(def, json.RawMessage(`{"latitude":40.0}`))
	require.NoError(t, err)
}

func TestSanitizeWebhookDiagnosticRedactsSecrets(t *testing.T) {
	msg := `Get "https://api.example.com/v1?api_key=super-secret&latitude=40": Authorization: Bearer top-secret token=abc password=hunter2`
	sanitized := SanitizeWebhookDiagnostic(msg)
	require.NotContains(t, sanitized, "super-secret")
	require.NotContains(t, sanitized, "top-secret")
	require.NotContains(t, sanitized, "hunter2")
	require.Contains(t, sanitized, "latitude=40")
	require.Contains(t, sanitized, "api_key=REDACTED")
}
