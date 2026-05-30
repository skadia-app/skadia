package lifts

// AutoConfirmHeuristic controls whether heuristic lift candidates count before
// user confirmation. Keep false until false-positive rates are known.
//
// Declared as `var` rather than `const` so the gate in recomputeLiftStats does
// not collapse into an unreachable branch at compile time — a Go const here
// would let staticcheck/unparam flag the SQL-filter switch as dead code, and
// flipping the gate today still requires recompile+redeploy regardless. A
// runtime variable preserves the same on-paper toggle ergonomics without the
// linter friction.
//
// Future: this should be lifted to a system-wide setting key (e.g.
// "lifts.auto_confirm_heuristic" in services/api/internal/settings) so an
// admin can flip it without a rebuild. That requires plumbing the
// SettingsStore into database.DB (currently held only by the handler layer)
// and is intentionally deferred until the setting has a proper runtime owner.
//
// When the toggle is flipped to true a DB backfill is required: existing
// heuristic candidates with confirmed=false would suddenly count toward
// activities.lift_*. Trigger re-enrichment of affected activities through the
// regular enrichment pipeline (e.g. by re-queuing them) before the change
// becomes user-visible. /lifts/recalculate-all only resets soft-reject flags
// and does not recompute aggregates.
var AutoConfirmHeuristic = false
