package schedulinglinter

import (
	"sort"

	"github.com/google/uuid"
)

// SweepReport is the output of [RunSweep].
type SweepReport struct {
	Findings []Finding `json:"findings"`
}

// RuleGroup is one bucket of [SweepReport.GroupByRule].
type RuleGroup struct {
	// Code is the canonical SCH-NNN code (e.g. "SCH-001").
	Code string
	// Findings are the findings emitted by that rule, in the
	// order they appear in [SweepReport.Findings].
	Findings []*Finding
}

// GroupByRule returns the findings bucketed by their canonical
// SCH-NNN code. The slice is sorted alphabetically by code,
// mirroring the Rust BTreeMap iteration order so the host UI gets
// a stable rendering between runs.
func (r *SweepReport) GroupByRule() []RuleGroup {
	buckets := make(map[string][]*Finding)
	for i := range r.Findings {
		f := &r.Findings[i]
		code := f.RuleID.Code()
		buckets[code] = append(buckets[code], f)
	}
	codes := make([]string, 0, len(buckets))
	for code := range buckets {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := make([]RuleGroup, 0, len(codes))
	for _, code := range codes {
		out = append(out, RuleGroup{Code: code, Findings: buckets[code]})
	}
	return out
}

// AppliedAction is one element of a sweep:apply plan.
type AppliedAction struct {
	FindingID   uuid.UUID `json:"finding_id"`
	ScheduleRID string    `json:"schedule_rid"`
	Action      Action    `json:"action"`
}

// PlanApply filters findings by rule + finding ids, returning the
// actions the apply pass must execute. Each pair carries enough
// context for the host service to call its own pause / archive /
// delete primitive.
//
// Empty `ruleIDs` matches every rule; empty `findingIDs` matches
// every finding.
func (r *SweepReport) PlanApply(ruleIDs []RuleID, findingIDs []uuid.UUID) []AppliedAction {
	out := make([]AppliedAction, 0)
	ruleSet := make(map[RuleID]struct{}, len(ruleIDs))
	for _, id := range ruleIDs {
		ruleSet[id] = struct{}{}
	}
	idSet := make(map[uuid.UUID]struct{}, len(findingIDs))
	for _, id := range findingIDs {
		idSet[id] = struct{}{}
	}
	for _, f := range r.Findings {
		if len(ruleSet) > 0 {
			if _, ok := ruleSet[f.RuleID]; !ok {
				continue
			}
		}
		if len(idSet) > 0 {
			if _, ok := idSet[f.ID]; !ok {
				continue
			}
		}
		out = append(out, AppliedAction{
			FindingID:   f.ID,
			ScheduleRID: f.ScheduleRID,
			Action:      f.RecommendedAction,
		})
	}
	return out
}

// RunSweep runs every rule against `input`. The order of findings
// reflects the rule ordering in the catalogue (SCH-001 through
// SCH-007) so the UI can render a stable, deterministic table.
func RunSweep(input *SweepInput) SweepReport {
	findings := make([]Finding, 0)
	findings = append(findings, ApplySch001(input)...)
	findings = append(findings, ApplySch002(input)...)
	findings = append(findings, ApplySch003(input)...)
	findings = append(findings, ApplySch004(input)...)
	findings = append(findings, ApplySch005(input)...)
	findings = append(findings, ApplySch006(input)...)
	findings = append(findings, ApplySch007(input)...)
	return SweepReport{Findings: findings}
}
