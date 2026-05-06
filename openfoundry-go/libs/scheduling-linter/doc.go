// Package schedulinglinter is the Foundry-parity "Sweep schedules"
// linter. Mirrors libs/scheduling-linter from the Rust workspace
// verbatim — same rule catalogue, same severities, same recommended
// actions.
//
// Rules implemented (see the docs_original_palantir_foundry/.../
// Linter/Sweep schedules.md catalogue):
//
//   - SCH-001 — schedule with no runs in the last 90 days.
//   - SCH-002 — schedule paused for more than 30 days.
//   - SCH-003 — schedule whose run failure rate over the last 30
//     days exceeds 50 %.
//   - SCH-004 — schedule whose owner is no longer active.
//   - SCH-005 — USER-scope schedule whose owner has not logged in
//     for more than 30 days.
//   - SCH-006 — production schedule running more often than every
//     5 minutes (cron parsed via the schedulingcron lib).
//   - SCH-007 — Event-trigger schedule with no branch_filter.
//
// The package is pure-logic: rules consume an immutable [SweepInput]
// snapshot built by the host service from its own database. Each
// rule returns zero-or-more [Finding]s; [RunSweep] sums them up and
// returns a [SweepReport] for the handler / UI to consume.
package schedulinglinter

import _ "time/tzdata"
