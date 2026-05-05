package databus

import (
	"time"

	kafka "github.com/segmentio/kafka-go"
)

// Header keys (lowercase, hyphenated) — matched verbatim with the Rust impl.
const (
	HeaderNamespace = "ol-namespace"
	HeaderJobName   = "ol-job-name"
	HeaderRunID     = "ol-run-id"
	HeaderEventTime = "ol-event-time"
	HeaderProducer  = "ol-producer"
	HeaderSchemaURL = "ol-schema-url"
)

// OpenLineageHeaders is the in-memory representation of the
// OpenLineage facets we propagate through the data plane.
type OpenLineageHeaders struct {
	Namespace string
	JobName   string
	RunID     string
	EventTime time.Time
	Producer  string
	SchemaURL string // optional
}

// NewOpenLineageHeaders constructs the minimal facet set with
// EventTime defaulted to "now" (UTC), matching the Rust constructor.
func NewOpenLineageHeaders(namespace, jobName, runID, producer string) OpenLineageHeaders {
	return OpenLineageHeaders{
		Namespace: namespace,
		JobName:   jobName,
		RunID:     runID,
		EventTime: time.Now().UTC(),
		Producer:  producer,
	}
}

// WithSchemaURL adds the optional schema registry URL.
func (h OpenLineageHeaders) WithSchemaURL(url string) OpenLineageHeaders {
	h.SchemaURL = url
	return h
}

// WithEventTime overrides EventTime.
func (h OpenLineageHeaders) WithEventTime(t time.Time) OpenLineageHeaders {
	h.EventTime = t
	return h
}

// ToKafkaHeaders converts to the segmentio/kafka-go header slice for
// attachment to a Kafka record. RFC3339 encoding for EventTime matches
// the Rust impl byte-for-byte.
func (h OpenLineageHeaders) ToKafkaHeaders() []kafka.Header {
	out := []kafka.Header{
		{Key: HeaderNamespace, Value: []byte(h.Namespace)},
		{Key: HeaderJobName, Value: []byte(h.JobName)},
		{Key: HeaderRunID, Value: []byte(h.RunID)},
		{Key: HeaderEventTime, Value: []byte(h.EventTime.Format(time.RFC3339Nano))},
		{Key: HeaderProducer, Value: []byte(h.Producer)},
	}
	if h.SchemaURL != "" {
		out = append(out, kafka.Header{Key: HeaderSchemaURL, Value: []byte(h.SchemaURL)})
	}
	return out
}

// OpenLineageHeadersFromKafka extracts the facets from a Kafka header
// slice. Returns ok=false when any required field is missing or the
// EventTime cannot be parsed — same all-or-nothing semantics as the
// Rust `from_kafka_headers`.
func OpenLineageHeadersFromKafka(hs []kafka.Header) (OpenLineageHeaders, bool) {
	var (
		out  OpenLineageHeaders
		hasN, hasJ, hasR, hasE, hasP bool
	)
	for _, h := range hs {
		v := string(h.Value)
		switch h.Key {
		case HeaderNamespace:
			out.Namespace = v
			hasN = true
		case HeaderJobName:
			out.JobName = v
			hasJ = true
		case HeaderRunID:
			out.RunID = v
			hasR = true
		case HeaderEventTime:
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				if t, err = time.Parse(time.RFC3339, v); err != nil {
					return OpenLineageHeaders{}, false
				}
			}
			out.EventTime = t.UTC()
			hasE = true
		case HeaderProducer:
			out.Producer = v
			hasP = true
		case HeaderSchemaURL:
			out.SchemaURL = v
		}
	}
	if !(hasN && hasJ && hasR && hasE && hasP) {
		return OpenLineageHeaders{}, false
	}
	return out, true
}
