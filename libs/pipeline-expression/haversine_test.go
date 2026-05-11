package pipelineexpression_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	pe "github.com/openfoundry/openfoundry-go/libs/pipeline-expression"
)

func TestHaversineDistanceInference(t *testing.T) {
	env := pe.NewColumnEnv().
		With("start_lat", pe.DoubleType()).
		With("start_lon", pe.DoubleType()).
		With("end_lat", pe.DoubleType()).
		With("end_lon", pe.DoubleType()).
		With("unit", pe.StringType())

	assertOk(t, `haversine_distance(start_lat, start_lon, end_lat, end_lon, unit)`, env, pe.DoubleType())
	assertOk(t, `haversine_miles(start_lat, start_lon, end_lat, end_lon)`, env, pe.DoubleType())
	assertOk(t, `haversine_km(start_lat, start_lon, end_lat, end_lon)`, env, pe.DoubleType())
	assertOk(t, `haversine_meters(start_lat, start_lon, end_lat, end_lon)`, env, pe.DoubleType())
}

func TestHaversineDistanceEval(t *testing.T) {
	expr, err := pe.ParseExpr(`haversine_distance(start_lat, start_lon, end_lat, end_lon, "miles")`)
	require.NoError(t, err)

	got, err := pe.Eval(expr, pe.Row{
		"start_lat": pe.EvalDouble(0).ToJSON(),
		"start_lon": pe.EvalDouble(0).ToJSON(),
		"end_lat":   pe.EvalDouble(0).ToJSON(),
		"end_lon":   pe.EvalDouble(1).ToJSON(),
	})
	require.NoError(t, err)
	require.Equal(t, pe.EvalKindDouble, got.Kind)
	require.InDelta(t, 69.09341957563636, got.Double, 1e-9)
}

func TestHaversineDistanceEvalReturnsNullForNullableCoordinates(t *testing.T) {
	expr, err := pe.ParseExpr(`haversine_km(start_lat, start_lon, end_lat, end_lon)`)
	require.NoError(t, err)

	got, err := pe.Eval(expr, pe.Row{
		"start_lat": pe.EvalDouble(0).ToJSON(),
		"start_lon": pe.EvalDouble(0).ToJSON(),
		"end_lat":   pe.EvalNull().ToJSON(),
		"end_lon":   pe.EvalDouble(1).ToJSON(),
	})
	require.NoError(t, err)
	require.Equal(t, pe.EvalKindNull, got.Kind)
}
