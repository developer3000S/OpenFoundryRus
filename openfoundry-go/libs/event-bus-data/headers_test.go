package databus_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	databus "github.com/openfoundry/openfoundry-go/libs/event-bus-data"
)

func TestOpenLineageHeadersRoundTrip(t *testing.T) {
	t.Parallel()
	when := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	original := databus.NewOpenLineageHeaders(
		"of://datasets",
		"etl.daily_orders",
		"01HXY-run",
		"https://github.com/openfoundry/openfoundry-go",
	).
		WithEventTime(when).
		WithSchemaURL("https://schemas.openfoundry.dev/orders/v1")

	parsed, ok := databus.OpenLineageHeadersFromKafka(original.ToKafkaHeaders())
	require.True(t, ok)
	assert.Equal(t, original.Namespace, parsed.Namespace)
	assert.Equal(t, original.JobName, parsed.JobName)
	assert.Equal(t, original.RunID, parsed.RunID)
	assert.True(t, original.EventTime.Equal(parsed.EventTime))
	assert.Equal(t, original.Producer, parsed.Producer)
	assert.Equal(t, original.SchemaURL, parsed.SchemaURL)
}

func TestOpenLineageHeadersMissingFieldYieldsFalse(t *testing.T) {
	t.Parallel()
	hdrs := databus.NewOpenLineageHeaders("ns", "job", "run", "prod").ToKafkaHeaders()
	// Strip a required header.
	for i, h := range hdrs {
		if h.Key == databus.HeaderRunID {
			hdrs = append(hdrs[:i], hdrs[i+1:]...)
			break
		}
	}
	_, ok := databus.OpenLineageHeadersFromKafka(hdrs)
	assert.False(t, ok)
}
