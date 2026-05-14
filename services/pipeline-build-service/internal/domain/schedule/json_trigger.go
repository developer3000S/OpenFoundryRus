package schedule

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	cron "github.com/openfoundry/openfoundry-go/libs/scheduling-cron"
)

type JSONTriggerEvent struct {
	EventType  EventType
	TargetRID  string
	Branch     string
	OccurredAt time.Time
}

type JSONTriggerContext struct {
	Now       time.Time
	CreatedAt time.Time
	LastRunAt *time.Time
	Event     *JSONTriggerEvent
	Observed  map[string]bool
}

type JSONObservedTrigger struct {
	Path      string
	EventType EventType
	TargetRID string
}

type JSONTriggerEvaluation struct {
	Fires         bool
	NextFire      *time.Time
	TriggerType   string
	Snapshot      map[string]string
	ObservedPaths []JSONObservedTrigger
}

type jsonTriggerNode struct {
	kind string
	raw  json.RawMessage
}

// EvaluateJSONTrigger evaluates both schedule JSON shapes used in this
// service: the legacy internally-tagged `{"kind":"time","data":...}` form and
// the Foundry v2/API form `{"kind":{"time":{...}}}`.
func EvaluateJSONTrigger(raw json.RawMessage, ctx JSONTriggerContext) (JSONTriggerEvaluation, error) {
	if ctx.Now.IsZero() {
		ctx.Now = time.Now().UTC()
	}
	out, err := evaluateJSONTriggerAt(raw, ctx, "")
	if out.Snapshot == nil {
		out.Snapshot = map[string]string{}
	}
	return out, err
}

// MatchingJSONEventObservations returns the event leaves satisfied by a single
// event. Callers persist these paths so compound AND triggers can be satisfied
// over multiple observations before the schedule actually runs.
func MatchingJSONEventObservations(raw json.RawMessage, event JSONTriggerEvent) ([]JSONObservedTrigger, error) {
	evaluation, err := EvaluateJSONTrigger(raw, JSONTriggerContext{
		Now:   firstNonZeroTime(event.OccurredAt, time.Now().UTC()),
		Event: &event,
	})
	if err != nil {
		return nil, err
	}
	return evaluation.ObservedPaths, nil
}

func evaluateJSONTriggerAt(raw json.RawMessage, ctx JSONTriggerContext, path string) (JSONTriggerEvaluation, error) {
	node, err := decodeJSONTriggerNode(raw)
	if err != nil {
		return JSONTriggerEvaluation{}, err
	}
	switch node.kind {
	case "time":
		return evaluateJSONTimeTrigger(node.raw, ctx, path)
	case "event":
		return evaluateJSONEventTrigger(node.raw, ctx, path)
	case "compound":
		return evaluateJSONCompoundTrigger(node.raw, ctx, path)
	default:
		return JSONTriggerEvaluation{}, fmt.Errorf("unknown trigger kind: %s", node.kind)
	}
}

func decodeJSONTriggerNode(raw json.RawMessage) (jsonTriggerNode, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return jsonTriggerNode{}, fmt.Errorf("trigger is empty")
	}
	var apiShape struct {
		Kind map[string]json.RawMessage `json:"kind"`
	}
	if err := json.Unmarshal(raw, &apiShape); err == nil && len(apiShape.Kind) > 0 {
		for kind, data := range apiShape.Kind {
			return jsonTriggerNode{kind: normalizeKind(kind), raw: data}, nil
		}
	}
	var legacyShape struct {
		Kind string          `json:"kind"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &legacyShape); err != nil {
		return jsonTriggerNode{}, err
	}
	if legacyShape.Kind == "" {
		return jsonTriggerNode{}, fmt.Errorf("trigger kind is required")
	}
	return jsonTriggerNode{kind: normalizeKind(legacyShape.Kind), raw: legacyShape.Data}, nil
}

func evaluateJSONTimeTrigger(raw json.RawMessage, ctx JSONTriggerContext, path string) (JSONTriggerEvaluation, error) {
	var trigger TimeTrigger
	if err := json.Unmarshal(raw, &trigger); err != nil {
		return JSONTriggerEvaluation{}, err
	}
	if strings.TrimSpace(trigger.TimeZone) == "" {
		trigger.TimeZone = "UTC"
	}
	if strings.TrimSpace(trigger.Cron) == "" {
		return JSONTriggerEvaluation{}, fmt.Errorf("time trigger cron is required")
	}
	tz, err := time.LoadLocation(trigger.TimeZone)
	if err != nil {
		return JSONTriggerEvaluation{}, fmt.Errorf("invalid time_zone %q: %w", trigger.TimeZone, err)
	}
	parsed, err := cron.ParseCron(trigger.Cron, mapFlavor(normalizeCronFlavor(trigger.Flavor)), tz)
	if err != nil {
		return JSONTriggerEvaluation{}, fmt.Errorf("invalid cron expression %q: %w", trigger.Cron, err)
	}
	cursor := ctx.CreatedAt
	if ctx.LastRunAt != nil {
		cursor = *ctx.LastRunAt
	}
	next, ok := cron.NextFireAfter(&parsed, cursor)
	if !ok {
		return JSONTriggerEvaluation{TriggerType: "TIME", Snapshot: map[string]string{
			"type": "time",
			"path": printableTriggerPath(path, "time"),
			"cron": trigger.Cron,
		}}, nil
	}
	return JSONTriggerEvaluation{
		Fires:       !next.After(ctx.Now),
		NextFire:    &next,
		TriggerType: "TIME",
		Snapshot: map[string]string{
			"type":          "time",
			"path":          printableTriggerPath(path, "time"),
			"cron":          trigger.Cron,
			"time_zone":     trigger.TimeZone,
			"scheduled_for": next.Format(time.RFC3339Nano),
			"evaluated_at":  ctx.Now.Format(time.RFC3339Nano),
			"cron_flavor":   string(normalizeCronFlavor(trigger.Flavor)),
		},
	}, nil
}

func evaluateJSONEventTrigger(raw json.RawMessage, ctx JSONTriggerContext, path string) (JSONTriggerEvaluation, error) {
	var trigger EventTrigger
	if err := json.Unmarshal(raw, &trigger); err != nil {
		return JSONTriggerEvaluation{}, err
	}
	trigger.EventType = NormalizeEventType(string(trigger.EventType))
	eventPath := printableTriggerPath(path, "event")
	eventMatches := jsonEventMatches(trigger, ctx.Event)
	observed := ctx.Observed != nil && ctx.Observed[eventPath]
	out := JSONTriggerEvaluation{
		Fires:       eventMatches || observed,
		TriggerType: "EVENT",
		Snapshot: map[string]string{
			"type":       "event",
			"path":       eventPath,
			"event_type": string(trigger.EventType),
			"target_rid": trigger.TargetRID,
		},
	}
	if eventMatches {
		out.ObservedPaths = append(out.ObservedPaths, JSONObservedTrigger{
			Path:      eventPath,
			EventType: trigger.EventType,
			TargetRID: trigger.TargetRID,
		})
		if ctx.Event != nil {
			out.Snapshot["observed_at"] = ctx.Event.OccurredAt.Format(time.RFC3339Nano)
			out.Snapshot["branch"] = ctx.Event.Branch
		}
	}
	if observed {
		out.Snapshot["observed"] = "true"
	}
	return out, nil
}

func evaluateJSONCompoundTrigger(raw json.RawMessage, ctx JSONTriggerContext, path string) (JSONTriggerEvaluation, error) {
	var trigger struct {
		Op         CompoundOp        `json:"op"`
		Components []json.RawMessage `json:"components"`
	}
	if err := json.Unmarshal(raw, &trigger); err != nil {
		return JSONTriggerEvaluation{}, err
	}
	op := CompoundOp(strings.ToUpper(strings.TrimSpace(string(trigger.Op))))
	if op == "" {
		op = CompoundOpAnd
	}
	if len(trigger.Components) == 0 {
		return JSONTriggerEvaluation{TriggerType: "COMPOUND", Snapshot: map[string]string{"type": "compound", "op": string(op)}}, nil
	}

	childResults := make([]JSONTriggerEvaluation, 0, len(trigger.Components))
	observed := []JSONObservedTrigger{}
	var next *time.Time
	for i, child := range trigger.Components {
		childPath := joinTriggerPath(path, fmt.Sprintf("compound[%d]", i))
		result, err := evaluateJSONTriggerAt(child, ctx, childPath)
		if err != nil {
			return JSONTriggerEvaluation{}, err
		}
		childResults = append(childResults, result)
		observed = append(observed, result.ObservedPaths...)
		if result.NextFire != nil && (next == nil || result.NextFire.Before(*next)) {
			candidate := *result.NextFire
			next = &candidate
		}
	}

	fires := op == CompoundOpAnd
	matched := []string{}
	for _, child := range childResults {
		if child.Fires {
			matched = append(matched, child.Snapshot["path"])
		}
		switch op {
		case CompoundOpAnd:
			fires = fires && child.Fires
		case CompoundOpOr:
			fires = fires || child.Fires
		default:
			return JSONTriggerEvaluation{}, fmt.Errorf("unknown compound op: %s", op)
		}
	}
	return JSONTriggerEvaluation{
		Fires:         fires,
		NextFire:      next,
		TriggerType:   "COMPOUND",
		ObservedPaths: observed,
		Snapshot: map[string]string{
			"type":          "compound",
			"path":          printableTriggerPath(path, "compound"),
			"op":            string(op),
			"matched_paths": strings.Join(matched, ","),
			"evaluated_at":  ctx.Now.Format(time.RFC3339Nano),
		},
	}, nil
}

func jsonEventMatches(trigger EventTrigger, event *JSONTriggerEvent) bool {
	if event == nil {
		return false
	}
	if NormalizeEventType(string(event.EventType)) != trigger.EventType {
		return false
	}
	if strings.TrimSpace(trigger.TargetRID) != "" && strings.TrimSpace(event.TargetRID) != strings.TrimSpace(trigger.TargetRID) {
		return false
	}
	if len(trigger.BranchFilter) == 0 {
		return true
	}
	for _, branch := range trigger.BranchFilter {
		if strings.TrimSpace(branch) == "" || strings.TrimSpace(branch) == event.Branch {
			return true
		}
	}
	return false
}

func NormalizeEventType(value string) EventType {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "", "DATASET_UPDATED", "DATA_UPDATE":
		return EventTypeDataUpdated
	case "LOGIC_UPDATED", "LOGIC_UPDATE":
		return EventTypeNewLogic
	case "JOB_COMPLETED":
		return EventTypeJobSucceeded
	case "SCHEDULE_SUCCEEDED", "SCHEDULE_RAN_SUCCEEDED":
		return EventTypeScheduleRanSuccessfully
	default:
		return EventType(normalized)
	}
}

func normalizeCronFlavor(value CronFlavor) CronFlavor {
	normalized := strings.ToUpper(strings.TrimSpace(string(value)))
	normalized = strings.ReplaceAll(normalized, "_", "")
	switch normalized {
	case "QUARTZ6":
		return CronFlavorQuartz6
	default:
		return CronFlavorUnix5
	}
}

func normalizeKind(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "pipelineBuild":
		return "pipeline_build"
	case "datasetBuild":
		return "dataset_build"
	case "syncRun":
		return "sync_run"
	case "healthCheck":
		return "health_check"
	default:
		return strings.ToLower(normalized)
	}
}

func joinTriggerPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func printableTriggerPath(parent, kind string) string {
	if parent == "" {
		return kind
	}
	return parent + "." + kind
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}
