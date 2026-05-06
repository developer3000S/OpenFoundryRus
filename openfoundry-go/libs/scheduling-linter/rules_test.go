package schedulinglinter_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	linter "github.com/openfoundry/openfoundry-go/libs/scheduling-linter"
)

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid v7: %v", err)
	}
	return id
}

func ymdhms(year int, month time.Month, day, hour, min, sec int) time.Time {
	return time.Date(year, month, day, hour, min, sec, 0, time.UTC)
}

func scheduleFixture(t *testing.T, name string) linter.InventorySchedule {
	t.Helper()
	owner := linter.InventoryUser{
		ID:          mustUUID(t),
		DisplayName: "alice",
		Active:      true,
		LastLoginAt: nil,
	}
	return linter.InventorySchedule{
		ID:         mustUUID(t),
		RID:        "ri.foundry.main.schedule." + name,
		ProjectRID: "ri.foundry.main.project.t",
		Name:       name,
		Paused:     false,
		PausedAt:   nil,
		ScopeKind:  "USER",
		RunAsUser:  &owner,
		Trigger: linter.InventoryTrigger{
			Kind:     linter.TriggerKindTime,
			Cron:     "0 9 * * *",
			TimeZone: "UTC",
			Flavor:   linter.TriggerCronUnix5,
		},
		RecentRuns: nil,
	}
}

func inputAt(now time.Time, schedules ...linter.InventorySchedule) *linter.SweepInput {
	return &linter.SweepInput{
		Schedules:  schedules,
		Now:        now,
		Production: true,
	}
}

func TestSch001FlagsScheduleWithNoRecentRuns(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleFixture(t, "idle")
	findings := linter.ApplySch001(inputAt(now, s))
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleID != linter.Sch001InactiveLastNinety {
		t.Fatalf("unexpected rule: %v", findings[0].RuleID)
	}
	if findings[0].RecommendedAction != linter.ActionPause {
		t.Fatalf("unexpected action: %v", findings[0].RecommendedAction)
	}
}

func TestSch001SkipsScheduleWithRecentRun(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleFixture(t, "active")
	s.RecentRuns = []linter.InventoryRun{
		{TriggeredAt: now.Add(-24 * time.Hour), Outcome: "SUCCEEDED"},
	}
	findings := linter.ApplySch001(inputAt(now, s))
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestSch002FlagsSchedulePausedMoreThan30Days(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	pausedAt := now.Add(-45 * 24 * time.Hour)
	s := scheduleFixture(t, "p")
	s.Paused = true
	s.PausedAt = &pausedAt
	findings := linter.ApplySch002(inputAt(now, s))
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RecommendedAction != linter.ActionArchive {
		t.Fatalf("unexpected action: %v", findings[0].RecommendedAction)
	}
}

func TestSch003FlagsHighFailureRate(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleFixture(t, "flaky")
	s.RecentRuns = []linter.InventoryRun{
		{TriggeredAt: now.Add(-2 * 24 * time.Hour), Outcome: "FAILED"},
		{TriggeredAt: now.Add(-3 * 24 * time.Hour), Outcome: "FAILED"},
		{TriggeredAt: now.Add(-4 * 24 * time.Hour), Outcome: "SUCCEEDED"},
	}
	findings := linter.ApplySch003(inputAt(now, s))
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != linter.SeverityError {
		t.Fatalf("unexpected severity: %v", findings[0].Severity)
	}
}

func TestSch003DoesNotFlagScheduleUnderThreshold(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleFixture(t, "ok")
	s.RecentRuns = []linter.InventoryRun{
		{TriggeredAt: now.Add(-2 * 24 * time.Hour), Outcome: "FAILED"},
		{TriggeredAt: now.Add(-3 * 24 * time.Hour), Outcome: "SUCCEEDED"},
	}
	findings := linter.ApplySch003(inputAt(now, s))
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestSch004FlagsOrphanedOwner(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleFixture(t, "orphan")
	s.RunAsUser.Active = false
	findings := linter.ApplySch004(inputAt(now, s))
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RecommendedAction != linter.ActionDelete {
		t.Fatalf("unexpected action: %v", findings[0].RecommendedAction)
	}
}

func TestSch005FlagsUserScopeWithStaleOwner(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleFixture(t, "stale")
	last := now.Add(-60 * 24 * time.Hour)
	s.RunAsUser.LastLoginAt = &last
	findings := linter.ApplySch005(inputAt(now, s))
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestSch005SkipsProjectScopedSchedules(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleFixture(t, "ps")
	s.ScopeKind = "PROJECT_SCOPED"
	last := now.Add(-60 * 24 * time.Hour)
	s.RunAsUser.LastLoginAt = &last
	findings := linter.ApplySch005(inputAt(now, s))
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestSch006FlagsHighFrequencyCronInProduction(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleFixture(t, "freq")
	s.Trigger = linter.InventoryTrigger{
		Kind:     linter.TriggerKindTime,
		Cron:     "*/2 * * * *",
		TimeZone: "UTC",
		Flavor:   linter.TriggerCronUnix5,
	}
	findings := linter.ApplySch006(inputAt(now, s))
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestSch006SkipsWhenNotProduction(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleFixture(t, "dev")
	s.Trigger = linter.InventoryTrigger{
		Kind:     linter.TriggerKindTime,
		Cron:     "*/2 * * * *",
		TimeZone: "UTC",
		Flavor:   linter.TriggerCronUnix5,
	}
	in := inputAt(now, s)
	in.Production = false
	findings := linter.ApplySch006(in)
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestSch007FlagsEventTriggerWithoutBranchFilter(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleFixture(t, "ev")
	s.Trigger = linter.InventoryTrigger{
		Kind:         linter.TriggerKindEvent,
		TargetRID:    "ri.foundry.main.dataset.x",
		BranchFilter: nil,
	}
	findings := linter.ApplySch007(inputAt(now, s))
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestSch007SkipsEventTriggerWithBranchFilter(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleFixture(t, "ev-filtered")
	s.Trigger = linter.InventoryTrigger{
		Kind:         linter.TriggerKindEvent,
		TargetRID:    "ri.foundry.main.dataset.x",
		BranchFilter: []string{"master"},
	}
	findings := linter.ApplySch007(inputAt(now, s))
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestRunSweepCollectsFindingsInRuleOrder(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	pausedAt := now.Add(-45 * 24 * time.Hour)
	last := now.Add(-45 * 24 * time.Hour)
	owner := linter.InventoryUser{
		ID:          mustUUID(t),
		DisplayName: "alice",
		Active:      true,
		LastLoginAt: &last,
	}
	s := linter.InventorySchedule{
		ID:         mustUUID(t),
		RID:        "ri.s.1",
		ProjectRID: "ri.p.1",
		Name:       "noisy",
		Paused:     true,
		PausedAt:   &pausedAt,
		ScopeKind:  "USER",
		RunAsUser:  &owner,
		Trigger: linter.InventoryTrigger{
			Kind:         linter.TriggerKindEvent,
			TargetRID:    "ri.x",
			BranchFilter: nil,
		},
		RecentRuns: nil,
	}
	report := linter.RunSweep(inputAt(now, s))
	codes := make(map[string]bool)
	for _, f := range report.Findings {
		codes[f.RuleID.Code()] = true
	}
	for _, want := range []string{"SCH-001", "SCH-002", "SCH-005", "SCH-007"} {
		if !codes[want] {
			t.Fatalf("expected code %s in findings, got %v", want, codes)
		}
	}
}

func TestGroupByRuleReturnsAlphabeticalOrder(t *testing.T) {
	// Mix findings from SCH-007, SCH-001, SCH-003 in non-sorted
	// insertion order; GroupByRule must emit them in canonical
	// code order (SCH-001, SCH-003, SCH-007).
	report := linter.SweepReport{
		Findings: []linter.Finding{
			{ID: uuid.MustParse("00000000-0000-0000-0000-000000000003"), RuleID: linter.Sch007EventTriggerWithoutBranchFilter},
			{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), RuleID: linter.Sch001InactiveLastNinety},
			{ID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), RuleID: linter.Sch003HighFailureRate},
		},
	}
	groups := report.GroupByRule()
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	want := []string{"SCH-001", "SCH-003", "SCH-007"}
	for i, g := range groups {
		if g.Code != want[i] {
			t.Fatalf("group[%d].Code = %s, want %s", i, g.Code, want[i])
		}
	}
}

func TestPlanApplyFiltersByFindingID(t *testing.T) {
	report := linter.SweepReport{
		Findings: []linter.Finding{
			{
				ID:                uuid.Nil,
				RuleID:            linter.Sch001InactiveLastNinety,
				Severity:          linter.SeverityWarning,
				ScheduleRID:       "ri.s.1",
				ProjectRID:        "ri.p.1",
				RecommendedAction: linter.ActionPause,
			},
			{
				ID:                uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				RuleID:            linter.Sch003HighFailureRate,
				Severity:          linter.SeverityError,
				ScheduleRID:       "ri.s.2",
				ProjectRID:        "ri.p.1",
				RecommendedAction: linter.ActionNotify,
			},
		},
	}
	plan := report.PlanApply(nil, []uuid.UUID{uuid.Nil})
	if len(plan) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan))
	}
	if plan[0].Action != linter.ActionPause {
		t.Fatalf("unexpected action: %v", plan[0].Action)
	}
}

// SCH-001 integration coverage mirroring tests/sch001_inactive_schedule.rs.

func scheduleWithLastRun(t *testing.T, daysAgo int, name string) linter.InventorySchedule {
	t.Helper()
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	last := now.Add(-24 * time.Hour)
	owner := linter.InventoryUser{
		ID:          mustUUID(t),
		DisplayName: "alice",
		Active:      true,
		LastLoginAt: &last,
	}
	return linter.InventorySchedule{
		ID:         mustUUID(t),
		RID:        "ri.foundry.main.schedule." + name,
		ProjectRID: "ri.foundry.main.project.t",
		Name:       name,
		Paused:     false,
		PausedAt:   nil,
		ScopeKind:  "USER",
		RunAsUser:  &owner,
		Trigger: linter.InventoryTrigger{
			Kind:     linter.TriggerKindTime,
			Cron:     "0 9 * * *",
			TimeZone: "UTC",
			Flavor:   linter.TriggerCronUnix5,
		},
		RecentRuns: []linter.InventoryRun{
			{TriggeredAt: now.Add(-time.Duration(daysAgo) * 24 * time.Hour), Outcome: "SUCCEEDED"},
		},
	}
}

func TestSch001IntegrationDoesNotFlagFreshSchedule(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleWithLastRun(t, 45, "fresh")
	report := linter.RunSweep(&linter.SweepInput{
		Schedules:  []linter.InventorySchedule{s},
		Now:        now,
		Production: false,
	})
	for _, f := range report.Findings {
		if f.RuleID == linter.Sch001InactiveLastNinety {
			t.Fatalf("did not expect SCH-001, got %+v", f)
		}
	}
}

func TestSch001IntegrationFlagsStaleSchedule(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleWithLastRun(t, 120, "stale")
	report := linter.RunSweep(&linter.SweepInput{
		Schedules:  []linter.InventorySchedule{s},
		Now:        now,
		Production: false,
	})
	count := 0
	for _, f := range report.Findings {
		if f.RuleID == linter.Sch001InactiveLastNinety {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 SCH-001 finding, got %d", count)
	}
}

func TestSch001IntegrationFlagsNeverRunSchedule(t *testing.T) {
	now := ymdhms(2026, 5, 1, 0, 0, 0)
	s := scheduleWithLastRun(t, 45, "never")
	s.RecentRuns = nil
	report := linter.RunSweep(&linter.SweepInput{
		Schedules:  []linter.InventorySchedule{s},
		Now:        now,
		Production: false,
	})
	count := 0
	for _, f := range report.Findings {
		if f.RuleID == linter.Sch001InactiveLastNinety {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 SCH-001 finding, got %d", count)
	}
}
