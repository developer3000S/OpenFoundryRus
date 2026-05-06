package schedulinglinter

import (
	"time"

	"github.com/google/uuid"
)

// Severity classifies a [Finding].
type Severity uint8

const (
	// SeverityInfo — informational signal, no action required.
	SeverityInfo Severity = iota
	// SeverityWarning — schedule should be reviewed.
	SeverityWarning
	// SeverityError — schedule is broken or about to misfire.
	SeverityError
)

// String returns the JSON name used in the wire protocol.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "Info"
	case SeverityWarning:
		return "Warning"
	case SeverityError:
		return "Error"
	default:
		return "Info"
	}
}

// MarshalJSON emits the variant name (matching the Rust
// `serde::Serialize` default for unit enums).
func (s Severity) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// UnmarshalJSON parses the variant name back to a Severity.
func (s *Severity) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case `"Info"`:
		*s = SeverityInfo
	case `"Warning"`:
		*s = SeverityWarning
	case `"Error"`:
		*s = SeverityError
	default:
		*s = SeverityInfo
	}
	return nil
}

// RuleID identifies which rule produced a [Finding].
type RuleID uint8

const (
	// Sch001InactiveLastNinety — SCH-001.
	Sch001InactiveLastNinety RuleID = iota
	// Sch002PausedLongerThanThirty — SCH-002.
	Sch002PausedLongerThanThirty
	// Sch003HighFailureRate — SCH-003.
	Sch003HighFailureRate
	// Sch004OwnerInactive — SCH-004.
	Sch004OwnerInactive
	// Sch005UserScopeOwnerStale — SCH-005.
	Sch005UserScopeOwnerStale
	// Sch006HighFrequencyCron — SCH-006.
	Sch006HighFrequencyCron
	// Sch007EventTriggerWithoutBranchFilter — SCH-007.
	Sch007EventTriggerWithoutBranchFilter
)

// Code returns the canonical SCH-NNN code for this rule.
func (r RuleID) Code() string {
	switch r {
	case Sch001InactiveLastNinety:
		return "SCH-001"
	case Sch002PausedLongerThanThirty:
		return "SCH-002"
	case Sch003HighFailureRate:
		return "SCH-003"
	case Sch004OwnerInactive:
		return "SCH-004"
	case Sch005UserScopeOwnerStale:
		return "SCH-005"
	case Sch006HighFrequencyCron:
		return "SCH-006"
	case Sch007EventTriggerWithoutBranchFilter:
		return "SCH-007"
	default:
		return ""
	}
}

// Name returns the variant name (matching the Rust enum tag used
// by serde when serialising findings to JSON).
func (r RuleID) Name() string {
	switch r {
	case Sch001InactiveLastNinety:
		return "Sch001InactiveLastNinety"
	case Sch002PausedLongerThanThirty:
		return "Sch002PausedLongerThanThirty"
	case Sch003HighFailureRate:
		return "Sch003HighFailureRate"
	case Sch004OwnerInactive:
		return "Sch004OwnerInactive"
	case Sch005UserScopeOwnerStale:
		return "Sch005UserScopeOwnerStale"
	case Sch006HighFrequencyCron:
		return "Sch006HighFrequencyCron"
	case Sch007EventTriggerWithoutBranchFilter:
		return "Sch007EventTriggerWithoutBranchFilter"
	default:
		return ""
	}
}

// MarshalJSON emits the variant name.
func (r RuleID) MarshalJSON() ([]byte, error) {
	return []byte(`"` + r.Name() + `"`), nil
}

// UnmarshalJSON parses the variant name back to a RuleID.
func (r *RuleID) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case `"Sch001InactiveLastNinety"`:
		*r = Sch001InactiveLastNinety
	case `"Sch002PausedLongerThanThirty"`:
		*r = Sch002PausedLongerThanThirty
	case `"Sch003HighFailureRate"`:
		*r = Sch003HighFailureRate
	case `"Sch004OwnerInactive"`:
		*r = Sch004OwnerInactive
	case `"Sch005UserScopeOwnerStale"`:
		*r = Sch005UserScopeOwnerStale
	case `"Sch006HighFrequencyCron"`:
		*r = Sch006HighFrequencyCron
	case `"Sch007EventTriggerWithoutBranchFilter"`:
		*r = Sch007EventTriggerWithoutBranchFilter
	}
	return nil
}

// Action is the recommended remediation attached to a Finding. The
// sweep:apply endpoint maps these onto concrete schedule-store
// calls.
type Action uint8

const (
	// ActionNotify — no automatic remediation; alert the owner only.
	ActionNotify Action = iota
	// ActionPause — pause the schedule (preserving the row). Used
	// by SCH-001 / 002.
	ActionPause
	// ActionDelete — hard delete the schedule. Used by SCH-004
	// (orphaned owner).
	ActionDelete
	// ActionArchive — archive the schedule (paused + archived
	// flag). For schedules the operator wants kept around as
	// audit history.
	ActionArchive
)

// String returns the JSON name used in the wire protocol.
func (a Action) String() string {
	switch a {
	case ActionNotify:
		return "Notify"
	case ActionPause:
		return "Pause"
	case ActionDelete:
		return "Delete"
	case ActionArchive:
		return "Archive"
	default:
		return "Notify"
	}
}

// MarshalJSON emits the variant name.
func (a Action) MarshalJSON() ([]byte, error) {
	return []byte(`"` + a.String() + `"`), nil
}

// UnmarshalJSON parses the variant name back to an Action.
func (a *Action) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case `"Notify"`:
		*a = ActionNotify
	case `"Pause"`:
		*a = ActionPause
	case `"Delete"`:
		*a = ActionDelete
	case `"Archive"`:
		*a = ActionArchive
	default:
		*a = ActionNotify
	}
	return nil
}

// Finding is a single rule-violation report.
type Finding struct {
	ID                uuid.UUID `json:"id"`
	RuleID            RuleID    `json:"rule_id"`
	Severity          Severity  `json:"severity"`
	ScheduleRID       string    `json:"schedule_rid"`
	ProjectRID        string    `json:"project_rid"`
	Message           string    `json:"message"`
	RecommendedAction Action    `json:"recommended_action"`
}

// SweepInput is the full inventory the rule engine inspects.
// Pure-data; no DB / network.
type SweepInput struct {
	Schedules []InventorySchedule
	Now       time.Time
	// Production gates SCH-006: the rule only fires when the host
	// is the production environment.
	Production bool
}

// InventorySchedule is the rule-engine's view of one schedule.
type InventorySchedule struct {
	ID          uuid.UUID
	RID         string
	ProjectRID  string
	Name        string
	Paused      bool
	PausedAt    *time.Time
	ScopeKind   string
	RunAsUser   *InventoryUser
	Trigger     InventoryTrigger
	RecentRuns  []InventoryRun
}

// TriggerCronFlavor selects the cron grammar used by an
// InventoryTriggerTime.
type TriggerCronFlavor uint8

const (
	// TriggerCronUnix5 — 5-field expression (min hour dom month dow).
	TriggerCronUnix5 TriggerCronFlavor = iota
	// TriggerCronQuartz6 — 6-field expression (sec min hour dom month dow).
	TriggerCronQuartz6
)

// InventoryTriggerKind discriminates the InventoryTrigger
// tagged-union variants.
type InventoryTriggerKind uint8

const (
	// TriggerKindTime — wall-clock cron trigger.
	TriggerKindTime InventoryTriggerKind = iota
	// TriggerKindEvent — event-driven trigger on another resource.
	TriggerKindEvent
	// TriggerKindCompound — composition of children triggers.
	TriggerKindCompound
)

// InventoryTrigger is the linter's materialised view of a
// schedule's trigger surface. Only the fields the linter inspects
// are kept here; the host service owns the canonical shape.
//
// The struct is a tagged union: Kind selects which fields are
// populated. It is intentionally not modelled as Go interfaces so
// the linter can stay independent of the canonical
// pipeline_schedule_service trigger model and still survive a
// round-trip through JSON.
type InventoryTrigger struct {
	Kind InventoryTriggerKind

	// Time fields (Kind == TriggerKindTime).
	Cron     string
	TimeZone string
	Flavor   TriggerCronFlavor

	// Event fields (Kind == TriggerKindEvent).
	TargetRID    string
	BranchFilter []string

	// Compound fields (Kind == TriggerKindCompound).
	Children []InventoryTrigger
}

// Leaves yields every leaf trigger under this node, depth-first.
// Compound triggers fan out into individual leaves; Time and Event
// nodes are returned as-is.
func (t InventoryTrigger) Leaves() []InventoryTrigger {
	var out []InventoryTrigger
	var recurse func(n InventoryTrigger)
	recurse = func(n InventoryTrigger) {
		if n.Kind == TriggerKindCompound {
			for _, c := range n.Children {
				recurse(c)
			}
			return
		}
		out = append(out, n)
	}
	recurse(t)
	return out
}

// InventoryRun is one row of a schedule's recent-runs window.
//
// Outcome is one of "SUCCEEDED" | "IGNORED" | "FAILED" — kept as a
// plain string so the linter's wire model stays independent of any
// concrete enum the host service might use.
type InventoryRun struct {
	TriggeredAt time.Time
	Outcome     string
}

// InventoryUser is the rule-engine's view of an owner.
//
// LastLoginAt is nil when the user has never logged in (e.g.
// service identities).
type InventoryUser struct {
	ID          uuid.UUID
	DisplayName string
	Active      bool
	LastLoginAt *time.Time
}
