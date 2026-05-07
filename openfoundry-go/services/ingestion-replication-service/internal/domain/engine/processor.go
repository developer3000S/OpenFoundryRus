// Package engine holds the pure, side-effect-free helpers that drive the
// in-process topology DAG executor. The runtime engine in
// internal/engine composes these helpers with the runtime store and sink
// uploaders. Mirrors the Rust helpers under
// services/ingestion-replication-service/src/event_streaming/domain/engine/processor.rs.
package engine

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/domain"
)

// ProcessedEvent mirrors the Rust private struct of the same name. Public
// here so the runtime engine + tests can build event slices to feed into
// the helpers without going through the runtime store.
type ProcessedEvent struct {
	ID             uuid.UUID
	StreamID       uuid.UUID
	StreamName     string
	ConnectorType  string
	Payload        json.RawMessage
	EventTime      time.Time
	ProcessingTime time.Time
	SequenceNo     int64
}

// ToProcessedEvent mirrors processor::to_processed_event.
func ToProcessedEvent(stream *domain.DomainStreamDefinition, row domain.RuntimeEvent, defaultProcessingTime time.Time) ProcessedEvent {
	processing := defaultProcessingTime
	if row.ProcessedAt != nil {
		processing = *row.ProcessedAt
	}
	return ProcessedEvent{
		ID:             row.ID,
		StreamID:       row.StreamID,
		StreamName:     stream.Name,
		ConnectorType:  stream.SourceBinding.ConnectorType,
		Payload:        row.Payload,
		EventTime:      row.EventTime,
		ProcessingTime: processing,
		SequenceNo:     row.SequenceNo,
	}
}

// ToLiveTailEvent mirrors processor::to_live_tail_event.
func ToLiveTailEvent(topologyID uuid.UUID, event ProcessedEvent) domain.LiveTailEvent {
	id := fmt.Sprintf("%s-%d", strings.ReplaceAll(strings.ToLower(event.StreamName), " ", "-"), event.SequenceNo)
	return domain.LiveTailEvent{
		ID:             id,
		TopologyID:     topologyID,
		StreamName:     event.StreamName,
		ConnectorType:  event.ConnectorType,
		Payload:        event.Payload,
		EventTime:      event.EventTime,
		ProcessingTime: event.ProcessingTime,
		Tags:           []string{fmt.Sprintf("stream:%s", event.StreamID)},
	}
}

// BuildJoinedEvents mirrors processor::build_joined_events.
//
// For every (left, right) source-event pair whose configured key fields
// match within the join window, emits a synthesised merged event so the
// downstream sink and CEP detection can reason over the join output.
func BuildJoinedEvents(topology *domain.TopologyDefinition, sourceEvents []ProcessedEvent) []ProcessedEvent {
	if topology.JoinDefinition == nil {
		return nil
	}
	join := topology.JoinDefinition
	var leftEvents, rightEvents []ProcessedEvent
	for _, evt := range sourceEvents {
		if evt.StreamID == join.LeftStreamID {
			leftEvents = append(leftEvents, evt)
		}
		if evt.StreamID == join.RightStreamID {
			rightEvents = append(rightEvents, evt)
		}
	}
	var joined []ProcessedEvent
	for _, left := range leftEvents {
		for _, right := range rightEvents {
			if !sameKeys(left.Payload, right.Payload, join.KeyFields) {
				continue
			}
			delta := absDuration(left.EventTime.Sub(right.EventTime))
			if int32(delta/time.Second) > join.WindowSeconds {
				continue
			}
			merged := mergeJoinPayload(join.JoinType, left, right)
			eventTime := left.EventTime
			if right.EventTime.After(eventTime) {
				eventTime = right.EventTime
			}
			processingTime := left.ProcessingTime
			if right.ProcessingTime.After(processingTime) {
				processingTime = right.ProcessingTime
			}
			seq := left.SequenceNo
			if right.SequenceNo > seq {
				seq = right.SequenceNo
			}
			joined = append(joined, ProcessedEvent{
				ID:             uuid.New(),
				StreamID:       left.StreamID,
				StreamName:     fmt.Sprintf("%s-joined", topology.Name),
				ConnectorType:  "join",
				Payload:        merged,
				EventTime:      eventTime,
				ProcessingTime: processingTime,
				SequenceNo:     seq,
			})
		}
	}
	return joined
}

func sameKeys(left, right json.RawMessage, fields []string) bool {
	if len(fields) == 0 {
		return true
	}
	leftObj, _ := decodeObject(left)
	rightObj, _ := decodeObject(right)
	for _, field := range fields {
		l, lok := leftObj[field]
		r, rok := rightObj[field]
		if !lok && !rok {
			continue
		}
		if string(l) != string(r) {
			return false
		}
	}
	return true
}

func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	obj := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	return obj, true
}

func mergeJoinPayload(joinType string, left, right ProcessedEvent) json.RawMessage {
	merged := map[string]any{}
	merged["_join"] = joinType
	merged["left_stream"] = left.StreamName
	merged["right_stream"] = right.StreamName
	if leftObj, ok := decodeObject(left.Payload); ok {
		for k, v := range leftObj {
			merged[k] = json.RawMessage(v)
		}
	}
	if rightObj, ok := decodeObject(right.Payload); ok {
		for k, v := range rightObj {
			merged["right_"+k] = json.RawMessage(v)
		}
	}
	out, _ := json.Marshal(merged)
	return out
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// BuildWindowAggregates mirrors processor::build_window_aggregates.
//
// Walks every node that has a window_id, finds the matching window, and
// buckets event payloads by (bucket_start, group_key, measure_name). When
// the window has no aggregation_keys the group key collapses to "all";
// when it has no measure_fields a synthetic events_per_window measure of
// 1.0 per event is emitted so the bucket is still surfaced.
func BuildWindowAggregates(topology *domain.TopologyDefinition, windows []domain.WindowDefinition, events []ProcessedEvent) []domain.WindowAggregate {
	type bucketKey struct {
		bucket  time.Time
		group   string
		measure string
	}
	var aggregates []domain.WindowAggregate
	for _, node := range topology.Nodes {
		if node.WindowID == nil {
			continue
		}
		var window *domain.WindowDefinition
		for i := range windows {
			if windows[i].ID == *node.WindowID {
				window = &windows[i]
				break
			}
		}
		if window == nil {
			continue
		}
		grouped := map[bucketKey]float64{}
		duration := int64(window.DurationSeconds)
		if duration < 1 {
			duration = 1
		}
		for _, evt := range events {
			bucket := bucketStart(evt.EventTime, duration)
			groupKey := groupKeyFor(window, evt.Payload)
			measures := measuresFor(window, evt.Payload)
			for _, m := range measures {
				grouped[bucketKey{bucket: bucket, group: groupKey, measure: m.name}] += m.value
			}
		}
		for k, v := range grouped {
			aggregates = append(aggregates, domain.WindowAggregate{
				WindowName:  window.Name,
				WindowType:  window.WindowType,
				BucketStart: k.bucket,
				BucketEnd:   k.bucket.Add(time.Duration(window.DurationSeconds) * time.Second),
				GroupKey:    k.group,
				MeasureName: k.measure,
				Value:       v,
			})
		}
	}
	sort.SliceStable(aggregates, func(i, j int) bool { return aggregates[i].BucketStart.Before(aggregates[j].BucketStart) })
	return aggregates
}

type measure struct {
	name  string
	value float64
}

func groupKeyFor(window *domain.WindowDefinition, payload json.RawMessage) string {
	if len(window.AggregationKeys) == 0 {
		return "all"
	}
	obj, _ := decodeObject(payload)
	parts := make([]string, len(window.AggregationKeys))
	for i, key := range window.AggregationKeys {
		raw, ok := obj[key]
		if !ok {
			parts[i] = fmt.Sprintf("%s:%s", key, stringifyJSON(json.RawMessage(`null`)))
			continue
		}
		parts[i] = fmt.Sprintf("%s:%s", key, stringifyJSON(raw))
	}
	return strings.Join(parts, "|")
}

func measuresFor(window *domain.WindowDefinition, payload json.RawMessage) []measure {
	if len(window.MeasureFields) == 0 {
		return []measure{{name: "events_per_window", value: 1.0}}
	}
	obj, _ := decodeObject(payload)
	out := make([]measure, 0, len(window.MeasureFields))
	for _, field := range window.MeasureFields {
		var v float64
		if raw, ok := obj[field]; ok {
			_ = json.Unmarshal(raw, &v)
		}
		out = append(out, measure{name: field, value: v})
	}
	return out
}

func stringifyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	trimmed := strings.TrimSpace(string(raw))
	switch trimmed {
	case "null":
		return "null"
	case "true", "false":
		return trimmed
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return trimmed
}

func bucketStart(eventTime time.Time, durationSeconds int64) time.Time {
	timestamp := eventTime.Unix()
	rem := timestamp % durationSeconds
	if rem < 0 {
		rem += durationSeconds
	}
	return time.Unix(timestamp-rem, 0).UTC()
}

// DetectCepMatches mirrors processor::detect_cep_matches.
func DetectCepMatches(topology *domain.TopologyDefinition, events []ProcessedEvent) []domain.CepMatch {
	cep := topology.CepDefinition
	if cep == nil {
		return nil
	}
	sorted := append([]ProcessedEvent(nil), events...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].EventTime.Before(sorted[j].EventTime) })
	var matches []domain.CepMatch
	for startIdx := range sorted {
		cursor := startIdx
		startTime := sorted[startIdx].EventTime
		matched := make([]string, 0, len(cep.Sequence))
		for _, expected := range cep.Sequence {
			for cursor < len(sorted) {
				label := eventLabel(sorted[cursor])
				if strings.EqualFold(label, expected) {
					matched = append(matched, expected)
					cursor++
					break
				}
				cursor++
			}
		}
		if len(matched) == len(cep.Sequence) {
			lastIdx := cursor - 1
			if lastIdx < 0 {
				lastIdx = 0
			}
			elapsed := sorted[lastIdx].EventTime.Sub(startTime)
			if int32(elapsed/time.Second) <= cep.WithinSeconds {
				matches = append(matches, domain.CepMatch{
					PatternName:     cep.PatternName,
					MatchedSequence: matched,
					Confidence:      0.95,
					DetectedAt:      sorted[lastIdx].EventTime,
				})
			}
		}
	}
	return matches
}

func eventLabel(event ProcessedEvent) string {
	obj, _ := decodeObject(event.Payload)
	for _, key := range []string{"status", "event_type", "state"} {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
	}
	return "event"
}

// MaterializationRows mirrors processor::materialization_rows.
//
// Builds the JSON rows uploaded to the dataset sink based on what the
// engine produced (windowed aggregates, joined events, or CEP matches).
func MaterializationRows(events []ProcessedEvent, aggregates []domain.WindowAggregate, cepMatches []domain.CepMatch) []json.RawMessage {
	if len(aggregates) > 0 {
		out := make([]json.RawMessage, 0, len(aggregates))
		for _, a := range aggregates {
			row, _ := json.Marshal(map[string]any{
				"window_name":  a.WindowName,
				"window_type":  a.WindowType,
				"bucket_start": a.BucketStart,
				"bucket_end":   a.BucketEnd,
				"group_key":    a.GroupKey,
				"measure_name": a.MeasureName,
				"value":        a.Value,
			})
			out = append(out, row)
		}
		return out
	}
	if len(events) > 0 {
		out := make([]json.RawMessage, 0, len(events))
		for _, e := range events {
			obj := map[string]any{}
			if decoded, ok := decodeObject(e.Payload); ok {
				for k, v := range decoded {
					obj[k] = json.RawMessage(v)
				}
			}
			obj["stream_name"] = e.StreamName
			obj["event_time"] = e.EventTime
			row, _ := json.Marshal(obj)
			out = append(out, row)
		}
		return out
	}
	out := make([]json.RawMessage, 0, len(cepMatches))
	for _, m := range cepMatches {
		row, _ := json.Marshal(map[string]any{
			"pattern_name":     m.PatternName,
			"matched_sequence": m.MatchedSequence,
			"confidence":       m.Confidence,
			"detected_at":      m.DetectedAt,
		})
		out = append(out, row)
	}
	return out
}

// EffectiveOutputCount mirrors processor::effective_output_count.
func EffectiveOutputCount(events []ProcessedEvent, aggregates []domain.WindowAggregate) int {
	if len(aggregates) > 0 {
		return len(aggregates)
	}
	return len(events)
}

// BuildRunMetrics mirrors processor::build_run_metrics.
func BuildRunMetrics(
	sourceEvents, materializationEvents []ProcessedEvent,
	aggregateWindows []domain.WindowAggregate,
	cepMatches []domain.CepMatch,
	completedAt time.Time,
) domain.TopologyRunMetrics {
	inputEvents := int32(len(sourceEvents))
	outputEvents := int32(EffectiveOutputCount(materializationEvents, aggregateWindows))
	var totalLatencyMS int64
	var p95LatencyMS int32
	for _, evt := range sourceEvents {
		latency := completedAt.Sub(evt.EventTime).Milliseconds()
		if latency < 0 {
			latency = 0
		}
		totalLatencyMS += latency
		if int32(latency) > p95LatencyMS {
			p95LatencyMS = int32(latency)
		}
	}
	var avgLatencyMS int32
	if inputEvents > 0 {
		avgLatencyMS = int32(totalLatencyMS / int64(inputEvents))
	}
	throughput := CalculateThroughputPerSecond(sourceEvents)
	var joinOutputRows int32
	for _, evt := range materializationEvents {
		obj, _ := decodeObject(evt.Payload)
		if _, ok := obj["_join"]; ok {
			joinOutputRows++
		}
	}
	return domain.TopologyRunMetrics{
		InputEvents:         inputEvents,
		OutputEvents:        outputEvents,
		AvgLatencyMS:        avgLatencyMS,
		P95LatencyMS:        p95LatencyMS,
		ThroughputPerSecond: throughput,
		DroppedEvents:       0,
		BackpressureRatio:   0.0,
		JoinOutputRows:      joinOutputRows,
		CepMatchCount:       int32(len(cepMatches)),
		StateEntries:        0,
	}
}

// BuildPreviewMetrics mirrors processor::build_preview_metrics.
func BuildPreviewMetrics(
	pendingEvents, materializationEvents []ProcessedEvent,
	aggregateWindows []domain.WindowAggregate,
	cepMatches []domain.CepMatch,
	recentEvents []ProcessedEvent,
	bp domain.BackpressureSnapshot,
) domain.TopologyRunMetrics {
	latest := time.Now().UTC()
	for _, evt := range pendingEvents {
		if evt.ProcessingTime.After(latest) {
			latest = evt.ProcessingTime
		}
	}
	if len(pendingEvents) == 0 {
		latest = time.Now().UTC()
	}
	metrics := BuildRunMetrics(pendingEvents, materializationEvents, aggregateWindows, cepMatches, latest)
	metrics.ThroughputPerSecond = CalculateThroughputPerSecond(recentEvents)
	metrics.BackpressureRatio = BackpressureRatio(bp)
	return metrics
}

// BuildStateSnapshot mirrors processor::build_state_snapshot.
func BuildStateSnapshot(topology *domain.TopologyDefinition, keyCount, checkpointCount int32, lastCheckpointAt time.Time) domain.StateStoreSnapshot {
	disk := keyCount
	if disk < 1 {
		disk = 1
	}
	cps := checkpointCount
	if cps < 1 {
		cps = 1
	}
	usage := disk + cps*2
	if usage < 1 {
		usage = 1
	}
	return domain.StateStoreSnapshot{
		Backend:          topology.StateBackend,
		Namespace:        strings.ReplaceAll(strings.ToLower(topology.Name), " ", "-"),
		KeyCount:         keyCount,
		DiskUsageMB:      usage,
		CheckpointCount:  checkpointCount,
		LastCheckpointAt: lastCheckpointAt,
	}
}

// GroupEventCountByStream mirrors processor::group_event_count_by_stream.
func GroupEventCountByStream(events []ProcessedEvent) map[uuid.UUID]int32 {
	counts := map[uuid.UUID]int32{}
	for _, evt := range events {
		counts[evt.StreamID]++
	}
	return counts
}

// GroupThroughputByStream mirrors processor::group_throughput_by_stream.
func GroupThroughputByStream(events []ProcessedEvent) map[uuid.UUID]float32 {
	grouped := map[uuid.UUID][]ProcessedEvent{}
	for _, evt := range events {
		grouped[evt.StreamID] = append(grouped[evt.StreamID], evt)
	}
	out := make(map[uuid.UUID]float32, len(grouped))
	for streamID, items := range grouped {
		out[streamID] = CalculateThroughputPerSecond(items)
	}
	return out
}

// CalculateThroughputPerSecond mirrors processor::calculate_throughput_per_second.
func CalculateThroughputPerSecond(events []ProcessedEvent) float32 {
	if len(events) == 0 {
		return 0.0
	}
	if len(events) == 1 {
		return 1.0
	}
	ordered := append([]ProcessedEvent(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].EventTime.Before(ordered[j].EventTime) })
	first := ordered[0].EventTime
	last := ordered[len(ordered)-1].EventTime
	var elapsed float32
	if last.After(first) {
		elapsed = float32(last.Sub(first).Seconds())
		if elapsed < 1.0 {
			elapsed = 1.0
		}
	} else {
		elapsed = 60.0
	}
	return float32(len(events)) / elapsed
}

// BackpressureRatio mirrors processor::backpressure_ratio.
func BackpressureRatio(snapshot domain.BackpressureSnapshot) float32 {
	if snapshot.QueueCapacity <= 0 {
		return 0.0
	}
	return float32(snapshot.QueueDepth) / float32(snapshot.QueueCapacity)
}
