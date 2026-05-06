package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/ml-kernel-go/models"
)

// Proof point: this binary depends on libs/ml-kernel-go/models for
// its wire-format DTO surface. The test below exercises a round-trip
// on ModelDeployment so a regression in the kernel models surfaces
// here immediately. The follow-up slice that wires
// `/api/v1/model-deployment/deployments` against this same type
// does not have to re-pin the wire shape.
func TestModelDeploymentRoundTripsViaMlKernelGo(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	d := models.ModelDeployment{
		ID:               uuid.New(),
		ModelID:          uuid.New(),
		Name:             "fraud-detector-v3",
		Status:           "active",
		StrategyType:     models.DefaultDeploymentStrategyType,
		EndpointPath:     "/predict/fraud",
		MonitoringWindow: models.DefaultDeploymentMonitoringWindow,
		TrafficSplit: []models.TrafficSplitEntry{
			{ModelVersionID: uuid.New(), Label: "champion", Allocation: 80},
			{ModelVersionID: uuid.New(), Label: "challenger", Allocation: 20},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	b, err := json.Marshal(d)
	require.NoError(t, err)

	var got models.ModelDeployment
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, d.Name, got.Name)
	assert.Equal(t, "single", got.StrategyType)
	assert.Equal(t, "24h", got.MonitoringWindow)
	assert.Len(t, got.TrafficSplit, 2)
	assert.Equal(t, uint8(80), got.TrafficSplit[0].Allocation)
}

func TestDriftReportRoundTrip(t *testing.T) {
	t.Parallel()
	r := models.DriftReport{
		GeneratedAt: time.Now().UTC(),
		DatasetMetrics: []models.DriftMetric{
			{Name: "psi", Score: 0.12, Threshold: 0.2, Status: "ok"},
		},
		ConceptMetrics:      []models.DriftMetric{},
		RecommendRetraining: false,
		Notes:               "stable",
	}
	b, err := json.Marshal(r)
	require.NoError(t, err)
	var got models.DriftReport
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "psi", got.DatasetMetrics[0].Name)
	assert.Equal(t, "ok", got.DatasetMetrics[0].Status)
}
