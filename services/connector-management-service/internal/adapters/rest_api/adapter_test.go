package rest_api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

func TestValidateConfigRequiresBaseURLOrDomain(t *testing.T) {
	require.Error(t, ValidateConfig(json.RawMessage(`{}`)))
	require.NoError(t, ValidateConfig(json.RawMessage(`{"base_url":"https://example.com"}`)))
	require.NoError(t, ValidateConfig(json.RawMessage(`{"domain":"api.open-meteo.com","auth":{"type":"none"}}`)))
}

// TestNormalizesCommonRESTWrappers mirrors Rust's
// `normalizes_common_rest_wrappers`.
func TestNormalizesCommonRESTWrappers(t *testing.T) {
	t.Run("data wrapper", func(t *testing.T) {
		var v any
		require.NoError(t, json.Unmarshal([]byte(`{"data":[{"id":1},{"id":2}]}`), &v))
		rows := normalizeRecords(v)
		require.Len(t, rows, 2)
	})
	t.Run("non-list object wraps to single row", func(t *testing.T) {
		var v any
		require.NoError(t, json.Unmarshal([]byte(`{"status":"ok"}`), &v))
		rows := normalizeRecords(v)
		require.Len(t, rows, 1)
		require.Equal(t, map[string]any{"status": "ok"}, rows[0])
	})
	t.Run("value wrapper", func(t *testing.T) {
		var v any
		require.NoError(t, json.Unmarshal([]byte(`{"value":[{"asset":"pump-01"}]}`), &v))
		rows := normalizeRecords(v)
		require.Len(t, rows, 1)
	})
	t.Run("array passthrough", func(t *testing.T) {
		var v any
		require.NoError(t, json.Unmarshal([]byte(`[{"a":1},{"a":2}]`), &v))
		rows := normalizeRecords(v)
		require.Len(t, rows, 2)
	})
}

func TestDiscoverSourcesFallbackWhenNoCatalog(t *testing.T) {
	a := New()
	conn := &models.Connection{Config: json.RawMessage(`{
		"base_url":"https://api.example.com",
		"resource_path":"/widgets",
		"resource_name":"Widgets"
	}`)}
	sources, err := a.DiscoverSources(context.Background(), conn, "")
	require.NoError(t, err)
	require.Len(t, sources, 1)
	require.Equal(t, "/widgets", sources[0].Selector)
	require.Equal(t, "Widgets", sources[0].DisplayName)
	require.Equal(t, "rest_resource", sources[0].SourceKind)
}

func TestDiscoverSourcesUsesInlineResources(t *testing.T) {
	a := New()
	conn := &models.Connection{Config: json.RawMessage(`{
		"base_url":"https://api.example.com",
		"resources":[
			{"selector":"/widgets","display_name":"Widgets"},
			{"path":"/sprockets","name":"Sprockets"}
		]
	}`)}
	sources, err := a.DiscoverSources(context.Background(), conn, "")
	require.NoError(t, err)
	require.Len(t, sources, 2)
	require.Equal(t, "/widgets", sources[0].Selector)
	require.Equal(t, "Widgets", sources[0].DisplayName)
	require.Equal(t, "/sprockets", sources[1].Selector)
	require.Equal(t, "Sprockets", sources[1].DisplayName)
}

func TestDiscoverSourcesAgainstCatalogPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/catalog", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"selector":"/a","name":"A"},{"selector":"/b"}]}`))
	}))
	defer srv.Close()

	a := New()
	a.SetHTTPClient(srv.Client())
	conn := &models.Connection{Config: json.RawMessage(`{
		"base_url":"` + srv.URL + `",
		"catalog_path":"/api/catalog"
	}`)}
	sources, err := a.DiscoverSources(context.Background(), conn, "")
	require.NoError(t, err)
	require.Len(t, sources, 2)
	require.Equal(t, "/a", sources[0].Selector)
	require.Equal(t, "/b", sources[1].Selector)
}

func TestQueryVirtualTableAgainstFakeServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/widgets", r.URL.Path)
		require.Equal(t, "Bearer top-secret", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"records":[{"id":1,"name":"a"},{"id":2,"name":"b"},{"id":3,"name":"c"}]}`))
	}))
	defer srv.Close()

	a := New()
	a.SetHTTPClient(srv.Client())
	conn := &models.Connection{Config: json.RawMessage(`{
		"base_url":"` + srv.URL + `",
		"bearer_token":"top-secret"
	}`)}
	limit := 2
	res, err := a.QueryVirtualTable(context.Background(), conn, &adapters.Query{Selector: "/widgets", Limit: &limit}, "")
	require.NoError(t, err)
	require.Equal(t, "/widgets", res.Selector)
	require.Equal(t, "zero_copy", res.Mode)
	require.Equal(t, 2, res.RowCount)
	require.Len(t, res.Rows, 2)
}

func TestQueryVirtualTableUsesModeledRESTAPISourceAuthAndQueryDefaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/forecast", r.URL.Path)
		require.Equal(t, "hourly", r.URL.Query().Get("model"))
		require.Equal(t, "dev-key", r.URL.Query().Get("api_key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"records":[{"temperature":84,"wind_speed":4.8}]}`))
	}))
	defer srv.Close()

	a := New()
	a.SetHTTPClient(srv.Client())
	conn := &models.Connection{Config: json.RawMessage(`{
		"base_url":"` + srv.URL + `",
		"query_params":{"model":"hourly"},
		"auth":{"type":"api_key","query_param":"api_key","value":"dev-key"},
		"runtime":{"worker":"foundry","timeout_ms":5000},
		"permissions":{"discoverable":true,"syncable":true,"invokable":true,"usable_in_code":true}
	}`)}
	res, err := a.QueryVirtualTable(context.Background(), conn, &adapters.Query{Selector: "/forecast"}, "")
	require.NoError(t, err)
	require.Equal(t, 1, res.RowCount)
	require.Equal(t, []string{"temperature", "wind_speed"}, res.Columns)
}

func TestConnectionAgainstFakeRESTAPISource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/forecast", r.URL.Path)
		require.Equal(t, "weather-key", r.Header.Get("X-Weather-Key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"current":{"temperature_2m":84}}`))
	}))
	defer srv.Close()

	a := New()
	a.SetHTTPClient(srv.Client())
	result, err := a.TestConnection(context.Background(), json.RawMessage(`{
		"base_url":"`+srv.URL+`",
		"health_path":"/v1/forecast",
		"auth":{"type":"api_key","header_name":"X-Weather-Key","value":"weather-key"},
		"runtime":{"worker":"foundry","timeout_ms":5000,"allowed_methods":["GET"]},
		"permissions":{"invokable":true,"allowed_egress_hosts":["api.open-meteo.com"]}
	}`))
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "validated REST API source", result.Message)
	var details map[string]any
	require.NoError(t, json.Unmarshal(result.Details, &details))
	require.Equal(t, "api_key", details["auth_type"])
	require.Equal(t, "foundry", details["worker"])
}

func TestStreamArrowReturnsErrNotImplemented(t *testing.T) {
	a := New()
	_, err := a.StreamArrow(context.Background(), &models.Connection{}, &adapters.Query{}, "")
	require.ErrorIs(t, err, adapters.ErrNotImplemented)
}

func TestBuildIngestSpec(t *testing.T) {
	a := New()
	conn := &models.Connection{Name: "rest-prod", Config: json.RawMessage(`{"base_url":"https://api.example.com"}`)}
	spec, err := a.BuildIngestSpec(context.Background(), conn, &adapters.Source{Selector: "/widgets"})
	require.NoError(t, err)
	require.Equal(t, "rest-prod", spec.Name)
	require.Equal(t, "rest_api", spec.Source)
	var cfg map[string]any
	require.NoError(t, json.Unmarshal(spec.Config, &cfg))
	require.Equal(t, "https://api.example.com", cfg["base_url"])
	require.Equal(t, "/widgets", cfg["resource_path"])
}

func TestUpstreamErrorIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	a := New()
	a.SetHTTPClient(srv.Client())
	conn := &models.Connection{Config: json.RawMessage(`{"base_url":"` + srv.URL + `"}`)}
	_, err := a.QueryVirtualTable(context.Background(), conn, &adapters.Query{Selector: "/widgets"}, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "REST source returned HTTP 503")
}
