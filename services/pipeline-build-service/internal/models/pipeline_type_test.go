package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizePipelineTypeAliases(t *testing.T) {
	tests := map[string]string{
		"":                  PipelineTypeBatch,
		"standard":          PipelineTypeBatch,
		"batch":             PipelineTypeBatch,
		"faster":            PipelineTypeFaster,
		"lightweight":       PipelineTypeFaster,
		"lightweight-batch": PipelineTypeFaster,
		"local_table":       PipelineTypeFaster,
		"incremental":       PipelineTypeIncremental,
		"spark":             PipelineTypeDistributed,
		"flink":             PipelineTypeDistributed,
		"distributed":       PipelineTypeDistributed,
	}
	for raw, expected := range tests {
		t.Run(raw, func(t *testing.T) {
			require.Equal(t, expected, NormalizePipelineType(raw))
			require.NoError(t, ValidatePipelineType(raw))
		})
	}
	require.Error(t, ValidatePipelineType("gpu_magic"))
	require.True(t, IsLightweightPipelineType("lightweight"))
	require.True(t, IsLightweightPipelineType("FASTER"))
	require.False(t, IsLightweightPipelineType("batch"))
}
