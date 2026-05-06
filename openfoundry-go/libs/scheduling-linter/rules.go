package schedulinglinter

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	schedulingcron "github.com/openfoundry/openfoundry-go/libs/scheduling-cron"
)

// ApplySch001 — schedule with no runs in the last 90 days.
func ApplySch001(input *SweepInput) []Finding {
	cutoff := input.Now.Add(-90 * 24 * time.Hour)
	out := make([]Finding, 0)
	for _, s := range input.Schedules {
		idle := true
		for _, r := range s.RecentRuns {
			if !r.TriggeredAt.Before(cutoff) {
				idle = false
				break
			}
		}
		if !idle {
			continue
		}
		out = append(out, Finding{
			ID:          newFindingID(),
			RuleID:      Sch001InactiveLastNinety,
			Severity:    SeverityWarning,
			ScheduleRID: s.RID,
			ProjectRID:  s.ProjectRID,
			Message: "Schedule has not run in the last 90 days." +
				" Consider pausing or archiving.",
			RecommendedAction: ActionPause,
		})
	}
	return out
}

// ApplySch002 — schedule paused longer than 30 days.
func ApplySch002(input *SweepInput) []Finding {
	cutoff := input.Now.Add(-30 * 24 * time.Hour)
	out := make([]Finding, 0)
	for _, s := range input.Schedules {
		if !s.Paused || s.PausedAt == nil {
			continue
		}
		if !s.PausedAt.Before(cutoff) {
			continue
		}
		out = append(out, Finding{
			ID:                newFindingID(),
			RuleID:            Sch002PausedLongerThanThirty,
			Severity:          SeverityWarning,
			ScheduleRID:       s.RID,
			ProjectRID:        s.ProjectRID,
			Message:           "Schedule has been paused for more than 30 days.",
			RecommendedAction: ActionArchive,
		})
	}
	return out
}

// ApplySch003 — failure rate > 50% over the last 30 days.
func ApplySch003(input *SweepInput) []Finding {
	windowStart := input.Now.Add(-30 * 24 * time.Hour)
	out := make([]Finding, 0)
	for _, s := range input.Schedules {
		var total, failures int
		for _, r := range s.RecentRuns {
			if r.TriggeredAt.Before(windowStart) {
				continue
			}
			total++
			if r.Outcome == "FAILED" {
				failures++
			}
		}
		if total == 0 {
			continue
		}
		// Strict majority — `> 50 %`.
		if failures*2 <= total {
			continue
		}
		out = append(out, Finding{
			ID:          newFindingID(),
			RuleID:      Sch003HighFailureRate,
			Severity:    SeverityError,
			ScheduleRID: s.RID,
			ProjectRID:  s.ProjectRID,
			Message: fmt.Sprintf(
				"%d of %d runs in the last 30 days failed.",
				failures, total,
			),
			RecommendedAction: ActionNotify,
		})
	}
	return out
}

// ApplySch004 — owner deactivated.
func ApplySch004(input *SweepInput) []Finding {
	out := make([]Finding, 0)
	for _, s := range input.Schedules {
		owner := s.RunAsUser
		if owner == nil || owner.Active {
			continue
		}
		out = append(out, Finding{
			ID:          newFindingID(),
			RuleID:      Sch004OwnerInactive,
			Severity:    SeverityError,
			ScheduleRID: s.RID,
			ProjectRID:  s.ProjectRID,
			Message: fmt.Sprintf(
				"Owner '%s' is deactivated; schedule cannot run as that user.",
				owner.DisplayName,
			),
			RecommendedAction: ActionDelete,
		})
	}
	return out
}

// ApplySch005 — USER-mode schedule whose owner has not logged in
// for > 30 days.
func ApplySch005(input *SweepInput) []Finding {
	cutoff := input.Now.Add(-30 * 24 * time.Hour)
	out := make([]Finding, 0)
	for _, s := range input.Schedules {
		if s.ScopeKind != "USER" {
			continue
		}
		owner := s.RunAsUser
		if owner == nil || owner.LastLoginAt == nil {
			continue
		}
		if !owner.LastLoginAt.Before(cutoff) {
			continue
		}
		out = append(out, Finding{
			ID:          newFindingID(),
			RuleID:      Sch005UserScopeOwnerStale,
			Severity:    SeverityWarning,
			ScheduleRID: s.RID,
			ProjectRID:  s.ProjectRID,
			Message: fmt.Sprintf(
				"Owner '%s' has not signed in for over 30 days. Consider converting the schedule to PROJECT_SCOPED.",
				owner.DisplayName,
			),
			RecommendedAction: ActionNotify,
		})
	}
	return out
}

// ApplySch006 — production schedule firing more often than every
// 5 minutes.
func ApplySch006(input *SweepInput) []Finding {
	if !input.Production {
		return nil
	}
	out := make([]Finding, 0)
	for _, s := range input.Schedules {
		for _, leaf := range s.Trigger.Leaves() {
			if leaf.Kind != TriggerKindTime {
				continue
			}
			if !intervalUnder5Minutes(leaf.Cron, leaf.TimeZone, leaf.Flavor) {
				continue
			}
			out = append(out, Finding{
				ID:          newFindingID(),
				RuleID:      Sch006HighFrequencyCron,
				Severity:    SeverityWarning,
				ScheduleRID: s.RID,
				ProjectRID:  s.ProjectRID,
				Message: fmt.Sprintf(
					"Production schedule fires more often than every 5 minutes (cron `%s`).",
					leaf.Cron,
				),
				RecommendedAction: ActionNotify,
			})
		}
	}
	return out
}

// ApplySch007 — Event-trigger leaf with no branch_filter.
func ApplySch007(input *SweepInput) []Finding {
	out := make([]Finding, 0)
	for _, s := range input.Schedules {
		for _, leaf := range s.Trigger.Leaves() {
			if leaf.Kind != TriggerKindEvent {
				continue
			}
			if len(leaf.BranchFilter) > 0 {
				continue
			}
			out = append(out, Finding{
				ID:          newFindingID(),
				RuleID:      Sch007EventTriggerWithoutBranchFilter,
				Severity:    SeverityInfo,
				ScheduleRID: s.RID,
				ProjectRID:  s.ProjectRID,
				Message: fmt.Sprintf(
					"Event trigger on `%s` has no branch_filter; will fire on every branch.",
					leaf.TargetRID,
				),
				RecommendedAction: ActionNotify,
			})
		}
	}
	return out
}

// intervalUnder5Minutes reports whether `cron` fires more than once
// per 5-minute window somewhere in the next hour. Two consecutive
// fires < 5 minutes apart is sufficient evidence; we don't need to
// enumerate the full schedule.
func intervalUnder5Minutes(cron, tz string, flavor TriggerCronFlavor) bool {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return false
	}
	parserFlavor := schedulingcron.Unix5
	if flavor == TriggerCronQuartz6 {
		parserFlavor = schedulingcron.Quartz6
	}
	parsed, err := schedulingcron.ParseCron(cron, parserFlavor, loc)
	if err != nil {
		return false
	}
	now := time.Now().UTC()
	first, ok := schedulingcron.NextFireAfter(&parsed, now)
	if !ok {
		return false
	}
	second, ok := schedulingcron.NextFireAfter(&parsed, first)
	if !ok {
		return false
	}
	return second.Sub(first) < 5*time.Minute
}

// newFindingID — wraps uuid.NewV7 with a fallback to NewRandom on
// the (vanishingly rare) v7 generation error so callers never see
// a zero UUID. Mirrors the Rust `Uuid::now_v7()` infallible API.
func newFindingID() uuid.UUID {
	if id, err := uuid.NewV7(); err == nil {
		return id
	}
	return uuid.New()
}
