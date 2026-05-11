package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

func TestOntologyObjectOutputCommitterDeploysTypePropertiesAndObjects(t *testing.T) {
	datasetID := uuid.New()
	objectTypeID := uuid.New()
	ontologyCalls := []string{}
	objectCalls := []string{}

	ontology := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ontologyCalls = append(ontologyCalls, r.Method+" "+r.URL.Path)
		require.Equal(t, "Bearer ontology-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/ontology/types/"+objectTypeID.String():
			http.NotFound(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/ontology/types":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ontology/types":
			var body ontologyObjectTypeWire
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, objectTypeID.String(), body.ID)
			require.Equal(t, "TrailEstimate", body.Name)
			require.Equal(t, "Trail Estimate", body.DisplayName)
			require.NotNil(t, body.PrimaryKeyProperty)
			require.Equal(t, "trailId", *body.PrimaryKeyProperty)
			require.NotNil(t, body.Editable)
			require.True(t, *body.Editable)
			require.NotNil(t, body.BackingDatasetRID)
			require.Equal(t, "ri.foundry.main.dataset."+datasetID.String(), *body.BackingDatasetRID)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": objectTypeID.String(), "name": body.Name, "display_name": body.DisplayName})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/ontology/types/"+objectTypeID.String()+"/properties":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ontology/types/"+objectTypeID.String()+"/properties":
			var body ontologyPropertyWire
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Contains(t, []string{"trailId", "effortScore"}, body.Name)
			if body.Name == "trailId" {
				require.True(t, body.Required)
				require.True(t, body.UniqueConstraint)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(body)
		default:
			t.Fatalf("unexpected ontology call: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer ontology.Close()

	objectDB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		objectCalls = append(objectCalls, r.Method+" "+r.URL.Path)
		require.Equal(t, "Bearer object-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/objects/query"):
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/objects/"):
			var body struct {
				Properties map[string]any `json:"properties"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.NotEmpty(t, body.Properties["trailId"])
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": uuid.NewString(), "properties": body.Properties})
		default:
			t.Fatalf("unexpected object-db call: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer objectDB.Close()

	datasetCommitter := &recordingCommitter{}
	metadataCommitter := &recordingCommitter{}
	committer := OntologyObjectOutputCommitter{
		Dataset:                  datasetCommitter,
		Metadata:                 metadataCommitter,
		OntologyDefinitionURL:    ontology.URL,
		OntologyDefinitionBearer: "ontology-token",
		ObjectDatabaseURL:        objectDB.URL,
		ObjectDatabaseBearer:     "object-token",
		Client:                   ontology.Client(),
	}
	result := executor.NodeResult{
		OutputContentHash: "sha256:object-rows",
		Metadata: map[string]any{
			"columns": []string{"trail_id", "effort_score"},
			"data_rows": []map[string]json.RawMessage{
				{"trail_id": json.RawMessage(`"mesa"`), "effort_score": json.RawMessage(`172`)},
				{"trail_id": json.RawMessage(`"green"`), "effort_score": json.RawMessage(`158`)},
			},
		},
	}
	err := committer.Commit(context.Background(), executor.OutputTransaction{
		DatasetRID:            datasetID.String(),
		TransactionRID:        "pipeline-run:one:object",
		OutputKind:            "object_type",
		OutputNodeID:          "output",
		DatasetName:           "Trail Estimate backing",
		ObjectTypeID:          objectTypeID.String(),
		ObjectTypeName:        "TrailEstimate",
		ObjectTypeDisplayName: "Trail Estimate",
		ObjectTypePrimaryKey:  "trailId",
		ObjectTypeIcon:        "walk",
		ObjectTypeColor:       "#2d72d2",
		ObjectTypeEditable:    true,
		ObjectPropertyMappings: []executor.OutputPropertyMapping{
			{SourceField: "trail_id", TargetProperty: "trailId", PropertyType: "string", DisplayName: "Trail ID", Required: true, UniqueConstraint: true},
			{SourceField: "effort_score", TargetProperty: "effortScore", PropertyType: "integer", DisplayName: "Effort Score"},
		},
		CreateIfMissing: true,
	}, result)
	require.NoError(t, err)
	require.Equal(t, []string{datasetID.String()}, datasetCommitter.datasets)
	require.Equal(t, []string{datasetID.String()}, metadataCommitter.datasets)
	require.Contains(t, ontologyCalls, "POST /api/v1/ontology/types")
	require.Contains(t, objectCalls, "POST /api/v1/ontology/types/"+objectTypeID.String()+"/objects/query")
	require.Len(t, objectCalls, 4)
}

func TestOntologyObjectOutputCommitterDeploysLinkTypeAndMaterializesLinks(t *testing.T) {
	datasetID := uuid.New()
	linkTypeID := uuid.New()
	sourceTypeID := uuid.New()
	targetTypeID := uuid.New()
	sourceObjectID := uuid.NewString()
	targetObjectID := uuid.NewString()
	createdLinks := []map[string]any{}

	ontology := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer ontology-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/ontology/links/"+linkTypeID.String():
			http.NotFound(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/ontology/links":
			require.Equal(t, sourceTypeID.String(), r.URL.Query().Get("object_type_id"))
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/ontology/links":
			var body ontologyLinkTypeWire
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, linkTypeID.String(), body.ID)
			require.Equal(t, "TrailToCoffee", body.Name)
			require.Equal(t, "Trail to Coffee", body.DisplayName)
			require.Equal(t, sourceTypeID.String(), body.SourceTypeID)
			require.Equal(t, targetTypeID.String(), body.TargetTypeID)
			require.Equal(t, "one_to_many", body.Cardinality)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(body)
		default:
			t.Fatalf("unexpected ontology call: %s %s", r.Method, r.URL.String())
		}
	}))
	defer ontology.Close()

	objectDB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer object-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/objects/query"):
			var body struct {
				Equals map[string]any `json:"equals"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			id := ""
			switch {
			case strings.Contains(r.URL.Path, sourceTypeID.String()) && body.Equals["trailId"] == "mesa":
				id = sourceObjectID
			case strings.Contains(r.URL.Path, targetTypeID.String()) && body.Equals["coffeeId"] == "trident":
				id = targetObjectID
			}
			if id == "" {
				_, _ = w.Write([]byte(`{"data":[]}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": id}}})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/object-database/links/default/"+linkTypeID.String()):
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, sourceObjectID, body["from"])
			require.Equal(t, targetObjectID, body["to"])
			createdLinks = append(createdLinks, body)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(body)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/outgoing/"+sourceObjectID):
			_ = json.NewEncoder(w).Encode(map[string]any{"items": createdLinks})
		default:
			t.Fatalf("unexpected object-db call: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer objectDB.Close()

	datasetCommitter := &recordingCommitter{}
	metadataCommitter := &recordingCommitter{}
	committer := OntologyObjectOutputCommitter{
		Dataset:                  datasetCommitter,
		Metadata:                 metadataCommitter,
		OntologyDefinitionURL:    ontology.URL,
		OntologyDefinitionBearer: "ontology-token",
		ObjectDatabaseURL:        objectDB.URL,
		ObjectDatabaseBearer:     "object-token",
		Client:                   ontology.Client(),
	}
	result := executor.NodeResult{
		OutputContentHash: "sha256:link-rows",
		Metadata: map[string]any{
			"columns": []string{"trail_id", "coffee_id"},
			"data_rows": []map[string]json.RawMessage{
				{"trail_id": json.RawMessage(`"mesa"`), "coffee_id": json.RawMessage(`"trident"`)},
			},
		},
	}
	err := committer.Commit(context.Background(), executor.OutputTransaction{
		DatasetRID:             datasetID.String(),
		TransactionRID:         "pipeline-run:one:link",
		OutputKind:             "link_type",
		OutputNodeID:           "trail_to_coffee",
		LinkTypeID:             linkTypeID.String(),
		LinkTypeName:           "TrailToCoffee",
		LinkTypeDisplayName:    "Trail to Coffee",
		LinkTypeCardinality:    "one_to_many",
		LinkSourceObjectTypeID: sourceTypeID.String(),
		LinkTargetObjectTypeID: targetTypeID.String(),
		LinkSourcePrimaryKey:   "trailId",
		LinkTargetPrimaryKey:   "coffeeId",
		LinkSourceKeyColumn:    "trail_id",
		LinkTargetKeyColumn:    "coffee_id",
		LinkTenant:             "default",
		CreateIfMissing:        true,
	}, result)
	require.NoError(t, err)
	require.Equal(t, []string{datasetID.String()}, datasetCommitter.datasets)
	require.Equal(t, []string{datasetID.String()}, metadataCommitter.datasets)
	require.Len(t, createdLinks, 1)

	req, _ := http.NewRequest(http.MethodGet, objectDB.URL+"/api/v1/object-database/links/default/"+linkTypeID.String()+"/outgoing/"+sourceObjectID, nil)
	req.Header.Set("Authorization", "Bearer object-token")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var linked map[string][]map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&linked))
	require.Len(t, linked["items"], 1)
}
