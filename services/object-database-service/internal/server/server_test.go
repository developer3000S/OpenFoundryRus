package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/storage"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := &config.Config{}
	cfg.Service.Name = "object-database-service"
	cfg.Service.Version = "test"
	h := &handlers.Handlers{
		Objects: storage.NewInMemoryObjectStore(),
		Links:   storage.NewInMemoryLinkStore(),
		Backend: config.BackendInMemory,
	}
	return httptest.NewServer(BuildRouter(cfg, h, observability.NewMetrics()))
}

func TestStatusReportsBackend(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/status")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, true, body["ready"])
	assert.Equal(t, "in_memory", body["backend"], "wire token must match Rust BackendMode::InMemory")
	assert.Equal(t, "object-database-service", body["service"])
}

func TestHealthAndHealthzCoexist(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	// Plain text /health (Rust legacy probe)
	resp, err := http.Get(srv.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ok", string(body))

	// JSON /healthz (openfoundry-go convention)
	resp2, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp2.Body.Close()
	var hz map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&hz))
	assert.Equal(t, "object-database-service", hz["service"])
}

func TestObjectQuerySupportsWorkshopObjectSetOperators(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	seed := func(body string) {
		t.Helper()
		resp, err := http.Post(
			srv.URL+"/api/v1/ontology/types/Trail/objects",
			"application/json",
			strings.NewReader(body),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}
	seed(`{"properties":{"name":"Mesa Trail","difficulty":"hard","gain_ft":800}}`)
	seed(`{"properties":{"name":"Valley Trail","difficulty":"easy","gain_ft":200}}`)

	resp, err := http.Post(
		srv.URL+"/api/v1/ontology/types/Trail/objects/query",
		"application/json",
		strings.NewReader(`{
			"filters": [
				{"property_name":"name","operator":"contains","value":"trail"},
				{"property_name":"gain_ft","operator":"gte","value":500},
				{"property_name":"difficulty","operator":"in","value":["hard","moderate"]}
			],
			"per_page": 25
		}`),
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, 1, body.Total)
	require.Len(t, body.Data, 1)
	props, ok := body.Data[0]["properties"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Mesa Trail", props["name"])
}

func TestObjectQuerySupportsWorkshopSortCountAggregationsAndSelectedIDs(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	seed := func(body string) string {
		t.Helper()
		resp, err := http.Post(
			srv.URL+"/api/v1/ontology/types/Trail/objects",
			"application/json",
			strings.NewReader(body),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		var created map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
		id, _ := created["id"].(string)
		require.NotEmpty(t, id)
		return id
	}
	low := seed(`{"properties":{"name":"Valley Trail","difficulty":"easy","gain_ft":200}}`)
	high := seed(`{"properties":{"name":"Mesa Trail","difficulty":"hard","gain_ft":800}}`)
	seed(`{"properties":{"name":"Road Walk","difficulty":"easy","gain_ft":50}}`)

	resp, err := http.Post(
		srv.URL+"/api/v1/ontology/types/Trail/objects/query",
		"application/json",
		strings.NewReader(`{
			"selected_object_ids": [`+strconv.Quote(low)+`, `+strconv.Quote(high)+`],
			"sort": [{"property_name":"gain_ft","direction":"desc"}],
			"aggregations": [
				{"id":"trail_count","function":"count"},
				{"id":"gain_sum","function":"sum","property_name":"gain_ft"},
				{"id":"avg_gain","function":"avg","property_name":"gain_ft"}
			],
			"per_page": 10,
			"include_count": true
		}`),
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Data []struct {
			ID         string         `json:"id"`
			Properties map[string]any `json:"properties"`
		} `json:"data"`
		Total        int `json:"total"`
		Count        int `json:"count"`
		Aggregations []struct {
			ID    string `json:"id"`
			Value any    `json:"value"`
			Count int    `json:"count"`
		} `json:"aggregations"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, 2, body.Total)
	require.Equal(t, 2, body.Count)
	require.Len(t, body.Data, 2)
	assert.Equal(t, "Mesa Trail", body.Data[0].Properties["name"])
	require.Len(t, body.Aggregations, 3)
	assert.Equal(t, "trail_count", body.Aggregations[0].ID)
	assert.Equal(t, float64(2), body.Aggregations[0].Value)
	assert.Equal(t, "gain_sum", body.Aggregations[1].ID)
	assert.Equal(t, float64(1000), body.Aggregations[1].Value)
	assert.Equal(t, "avg_gain", body.Aggregations[2].ID)
	assert.Equal(t, float64(500), body.Aggregations[2].Value)
}

func TestObjectQuerySearchAroundTraversesLinks(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	seed := func(typeID string, body string) string {
		t.Helper()
		resp, err := http.Post(
			srv.URL+"/api/v1/ontology/types/"+typeID+"/objects",
			"application/json",
			strings.NewReader(body),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		var created map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
		id, _ := created["id"].(string)
		require.NotEmpty(t, id)
		return id
	}
	trailID := seed("Trail", `{"properties":{"name":"Mesa Trail","lat":40.0,"lon":-105.0}}`)
	nearCoffeeID := seed("CoffeeShop", `{"properties":{"name":"Boxcar Coffee","lat":40.01,"lon":-105.01}}`)
	seed("CoffeeShop", `{"properties":{"name":"Far Coffee","lat":41.0,"lon":-106.0}}`)

	linkBody := `{"from":` + strconv.Quote(trailID) + `,"to":` + strconv.Quote(nearCoffeeID) + `,"payload":{"distance_miles":0.8},"created_at_ms":3}`
	linkReq, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/api/v1/object-database/links/default/trail_near_coffee", strings.NewReader(linkBody))
	linkResp, err := http.DefaultClient.Do(linkReq)
	require.NoError(t, err)
	defer linkResp.Body.Close()
	require.Equal(t, http.StatusCreated, linkResp.StatusCode)

	resp, err := http.Post(
		srv.URL+"/api/v1/ontology/types/CoffeeShop/objects/query",
		"application/json",
		strings.NewReader(`{
			"search_around": {
				"source_object_ids": [`+strconv.Quote(trailID)+`],
				"link_type_ids": ["trail_near_coffee"],
				"direction": "outgoing",
				"depth": 1,
				"target_object_type_id": "CoffeeShop"
			},
			"per_page": 10
		}`),
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Data []struct {
			ID         string         `json:"id"`
			Properties map[string]any `json:"properties"`
		} `json:"data"`
		LinkedEdges []struct {
			LinkTypeID     string         `json:"link_type_id"`
			SourceObjectID string         `json:"source_object_id"`
			TargetObjectID string         `json:"target_object_id"`
			Direction      string         `json:"direction"`
			Depth          int            `json:"depth"`
			Properties     map[string]any `json:"properties"`
		} `json:"linked_edges"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, nearCoffeeID, body.Data[0].ID)
	assert.Equal(t, "Boxcar Coffee", body.Data[0].Properties["name"])
	require.Len(t, body.LinkedEdges, 1)
	assert.Equal(t, "trail_near_coffee", body.LinkedEdges[0].LinkTypeID)
	assert.Equal(t, trailID, body.LinkedEdges[0].SourceObjectID)
	assert.Equal(t, nearCoffeeID, body.LinkedEdges[0].TargetObjectID)
	assert.Equal(t, "outgoing", body.LinkedEdges[0].Direction)
	assert.Equal(t, 1, body.LinkedEdges[0].Depth)
	assert.Equal(t, float64(0.8), body.LinkedEdges[0].Properties["distance_miles"])
}

func TestObjectQueryKNNRanksVectorProperties(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	seed := func(body string) {
		t.Helper()
		resp, err := http.Post(
			srv.URL+"/api/v1/ontology/types/Run/objects",
			"application/json",
			strings.NewReader(body),
		)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}
	seed(`{"properties":{"name":"Mesa effort","trail_vector":[5,100],"activity_type":"trail"}}`)
	seed(`{"properties":{"name":"Long climb","trail_vector":[20,1200],"activity_type":"trail"}}`)
	seed(`{"properties":{"name":"Road recovery","trail_vector":[8,180],"activity_type":"road"}}`)
	seed(`{"properties":{"name":"Malformed vector","trail_vector":[6],"activity_type":"trail"}}`)

	resp, err := http.Post(
		srv.URL+"/api/v1/ontology/types/Run/objects/query",
		"application/json",
		strings.NewReader(`{
			"filters": [{"property_name":"activity_type","operator":"in","value":["trail","road"]}],
			"knn": {
				"property_name": "trail_vector",
				"vector": [6, 120],
				"k": 2,
				"metric": "euclidean"
			},
			"per_page": 10,
			"include_count": true
		}`),
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Data []struct {
			ID         string         `json:"id"`
			Properties map[string]any `json:"properties"`
		} `json:"data"`
		Total      int `json:"total"`
		KNNResults []struct {
			ObjectID     string  `json:"object_id"`
			Rank         int     `json:"rank"`
			Score        float64 `json:"score"`
			Distance     float64 `json:"distance"`
			Metric       string  `json:"metric"`
			PropertyName string  `json:"property_name"`
		} `json:"knn_results"`
		ObjectSet struct {
			KNN map[string]any `json:"knn"`
		} `json:"object_set"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, 2, body.Total)
	require.Len(t, body.Data, 2)
	assert.Equal(t, "Mesa effort", body.Data[0].Properties["name"])
	assert.Equal(t, "Road recovery", body.Data[1].Properties["name"])
	assert.Equal(t, float64(1), body.Data[0].Properties["__of_knn_rank"])
	assert.Equal(t, "euclidean", body.Data[0].Properties["__of_knn_metric"])
	require.Len(t, body.KNNResults, 2)
	assert.Equal(t, body.Data[0].ID, body.KNNResults[0].ObjectID)
	assert.Equal(t, 1, body.KNNResults[0].Rank)
	assert.Equal(t, "trail_vector", body.KNNResults[0].PropertyName)
	assert.Equal(t, "euclidean", body.KNNResults[0].Metric)
	assert.InDelta(t, 20.024984, body.KNNResults[0].Distance, 0.0001)
	assert.Greater(t, body.KNNResults[0].Score, body.KNNResults[1].Score)
	assert.Equal(t, "trail_vector", body.ObjectSet.KNN["property_name"])
	assert.Equal(t, float64(2), body.ObjectSet.KNN["k"])
	assert.Equal(t, float64(2), body.ObjectSet.KNN["query_vector_dimension"])
}

func TestPutGetDeleteRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	body := `{
		"type_id": "aircraft",
		"version": 0,
		"payload": {"tail":"N123OF"},
		"owner": "owner-1",
		"markings": ["public"],
		"created_at_ms": 1,
		"updated_at_ms": 2
	}`
	req, _ := http.NewRequestWithContext(context.Background(),
		http.MethodPut, srv.URL+"/api/v1/object-database/objects/tenant-a/object-1",
		strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var put map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&put))
	assert.Equal(t, "inserted", put["outcome"])

	// GET
	resp2, err := http.Get(srv.URL + "/api/v1/object-database/objects/tenant-a/object-1")
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&got))
	assert.Equal(t, "tenant-a", got["tenant"])
	assert.Equal(t, "object-1", got["id"])
	assert.Equal(t, "aircraft", got["type_id"])
	assert.Equal(t, float64(1), got["version"])

	// Version conflict
	conflict := `{"type_id":"aircraft","version":0,"payload":{},"expected_version":99,"markings":[]}`
	req2, _ := http.NewRequest(http.MethodPut,
		srv.URL+"/api/v1/object-database/objects/tenant-a/object-1",
		strings.NewReader(conflict))
	resp3, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	defer resp3.Body.Close()
	var conflictBody map[string]any
	require.NoError(t, json.NewDecoder(resp3.Body).Decode(&conflictBody))
	assert.Equal(t, "version_conflict", conflictBody["outcome"])
	assert.Equal(t, float64(99), conflictBody["expected_version"])
	assert.Equal(t, float64(1), conflictBody["actual_version"])

	// DELETE
	delReq, _ := http.NewRequest(http.MethodDelete,
		srv.URL+"/api/v1/object-database/objects/tenant-a/object-1", nil)
	delResp, err := http.DefaultClient.Do(delReq)
	require.NoError(t, err)
	defer delResp.Body.Close()
	assert.Equal(t, http.StatusNoContent, delResp.StatusCode)

	// 404 after delete
	resp4, err := http.Get(srv.URL + "/api/v1/object-database/objects/tenant-a/object-1")
	require.NoError(t, err)
	defer resp4.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp4.StatusCode)
}

func TestListByOwnerAndMarkingAndLinks(t *testing.T) {
	srv := newTestServer(t)
	t.Cleanup(srv.Close)

	// seed two objects
	put := func(id, owner, marking string) {
		body := `{"type_id":"aircraft","version":0,"payload":{},"owner":"` + owner +
			`","markings":["` + marking + `"],"updated_at_ms":1}`
		req, _ := http.NewRequest(http.MethodPut,
			srv.URL+"/api/v1/object-database/objects/tenant-a/"+id, strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}
	put("obj-1", "owner-1", "public")
	put("obj-2", "owner-2", "secret")

	resp, err := http.Get(srv.URL + "/api/v1/object-database/objects/tenant-a/by-owner/owner-1?size=10")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var byOwner map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&byOwner))
	items, _ := byOwner["items"].([]any)
	assert.Len(t, items, 1)

	resp2, err := http.Get(srv.URL + "/api/v1/object-database/objects/tenant-a/by-marking/secret?size=10")
	require.NoError(t, err)
	defer resp2.Body.Close()
	var byMark map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&byMark))
	items2, _ := byMark["items"].([]any)
	assert.Len(t, items2, 1)

	linkBody := `{"from":"obj-1","to":"obj-2","payload":{"kind":"primary"},"created_at_ms":3}`
	linkReq, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/api/v1/object-database/links/tenant-a/related_to", strings.NewReader(linkBody))
	linkResp, err := http.DefaultClient.Do(linkReq)
	require.NoError(t, err)
	defer linkResp.Body.Close()
	assert.Equal(t, http.StatusCreated, linkResp.StatusCode)

	outgoing, err := http.Get(srv.URL + "/api/v1/object-database/links/tenant-a/related_to/outgoing/obj-1?size=10")
	require.NoError(t, err)
	defer outgoing.Body.Close()
	assert.Equal(t, http.StatusOK, outgoing.StatusCode)
	var links map[string]any
	require.NoError(t, json.NewDecoder(outgoing.Body).Decode(&links))
	linkItems, _ := links["items"].([]any)
	assert.Len(t, linkItems, 1)
	assert.Equal(t, "obj-2", linkItems[0].(map[string]any)["to"])
}
