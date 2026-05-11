package automation

// Phase 4b.A — System-Artefakt-Architektur entkoppeln.
//
// This file is the single source of truth for system tags + automation rules
// that every user must have. Adding a new system tag or system rule is now a
// single-file Go change here (plus a SystemArtifactsVersion bump) instead of
// a SQL migration.
//
// The previous design — inline INSERTs in tags_db.SeedSystemDefaults plus an
// SQL CHECK whitelist on automation_rules.system_type — required a new
// migration for every system_type addition (000176, 000220, 000260, 000261,
// 000268, 000288 — all just to expand the whitelist or seed a row). The CHECK
// constraint was dropped in migration 000356_drop_system_type_check_for_code_validation;
// validation is now done in IsValidSystemType below.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// SystemTagSpec describes a system tag that should exist for every user.
//
// All system tags are inserted with category='system', is_system=true,
// is_auto=false. is_auto=false is intentional: tags are managed by automation
// rules (or DB triggers for the *_sync rules), not by the legacy autotag engine.
type SystemTagSpec struct {
	Name        string
	Color       string
	TextColor   string
	Description string
}

// SystemRuleSpec describes the automation rule that pairs with a SystemTagSpec.
// Conditions are stored as JSON-strings to keep the spec declarative.
//
// Action params support a single template token "{{TAG_ID}}" which is resolved
// to the per-user tag UUID at insert time (see EnsureSystemArtifacts).
type SystemRuleSpec struct {
	TagName      string           // matches a SystemTagSpec.Name
	SystemType   string           // unique per (user_id, is_system=true)
	Priority     int              // negative = system priority band
	Conditions   string           // JSON array, may be "[]"
	Actions      []map[string]any // serialised at insert time; tag_id resolved per user
	TriggerEvent string           // MUST be one of the chk_trigger_event whitelist:
	// "on_import", "on_enrichment_complete", "on_manual_apply"
	// (CHECK enforced by migration 000267).
}

// SystemTags is the single source of truth for all system tags.
// Adding a new system tag is a one-line append here.
//
// Order matches the historical order from tags_db.SeedSystemDefaults so the
// behavioural diff in Phase 4b.A is zero (verified by tags_db_test.go).
var SystemTags = []SystemTagSpec{
	{Name: "Rundtour", Color: "#10B981", TextColor: "#1a1a1a",
		Description: "Automatisch bei Aktivitaeten die als Rundtour erkannt wurden (Sync mit is_round_trip)"},
	{Name: "E-Bike", Color: "#16A34A", TextColor: "#FFFFFF",
		Description: "Automatisch bei E-Bike-Aktivitäten (Sync mit is_electric)"},
	{Name: "Pendeln", Color: "#6B7280", TextColor: "#FFFFFF",
		Description: "Automatisch bei Pendel-Aktivitäten (Sync mit is_commute)"},
	{Name: "Indoor", Color: "#3B82F6", TextColor: "#FFFFFF",
		Description: "Automatisch bei Indoor-Aktivitäten (Sync mit is_indoor)"},
	{Name: "virtuell", Color: "#8B5CF6", TextColor: "#FFFFFF",
		Description: "Automatisch bei virtuellen Aktivitäten (Sync mit is_virtual)"},
	{Name: "Frost", Color: "#38BDF8", TextColor: "#0C4A6E",
		Description: "Automatisch bei Temperaturen unter 0 Grad C"},
	{Name: "Nacht", Color: "#1E293B", TextColor: "#E2E8F0",
		Description: "Automatisch bei Aktivitaeten die in die astronomische Nacht fallen"},
	// Phase 4b.B.1 — Lift tag for activities with detected lift rides
	// (Method A: OSM-matched, auto-confirmed; Method B: heuristic, gated
	// by user confirm via the lift-candidate UI).
	{Name: "Lift", Color: "#6366F1", TextColor: "#FFFFFF",
		Description: "Automatisch bei Aktivitäten mit erkannten Liftfahrten"},
}

// SystemRules is the matching automation-rule list. Empty conditions/actions =
// fully managed by DB trigger or import pipeline (electric_sync, indoor_sync,
// virtual_sync).
//
// Intentional split: the "Pendeln" SystemTag is default-seeded for every user
// (above), but the matching "commute_auto" SystemRule is NOT in this list —
// it is created on-demand by handler/commute_rule.go when a user enables
// commute detection. Default-seeding the rule would waste a row for every
// user who never enables commute detection. See OnDemandSystemTypes below for
// the validation hook that keeps commute_auto in the IsValidSystemType
// whitelist.
//
// User-edit preservation contract: EnsureSystemArtifacts uses
// ON CONFLICT (user_id, system_type) WHERE is_system=true DO NOTHING, so
// changing a SystemRuleSpec field (priority/conditions/actions) only affects
// NEW users who haven't been seeded yet. Existing users keep whatever's in
// their row — both their is_active toggle (intentional preservation) AND any
// spec field that was updated since they were seeded (unintentional staleness).
// If a future spec change MUST reach existing users it requires a separate
// one-shot UPDATE migration; the version-bump mechanism alone won't propagate
// it. Trade-off accepted because it's the only way to preserve user
// customisations without dirty-bit tracking.
//
// TriggerEvent values reproduce the existing tags_db.go behaviour: today the
// 7 system rules omit the trigger_event column on INSERT and inherit the
// DEFAULT 'on_import' from migration 000267 — so we set "on_import" explicitly
// here. The Lift rule in 4b.B uses "on_enrichment_complete" because lift_count
// is populated during enrichment, not at import time.
var SystemRules = []SystemRuleSpec{
	{TagName: "Rundtour", SystemType: "round_trip", Priority: -99,
		Conditions: `[{"field":"start_end_distance_m","op":"lte","value":200}]`,
		Actions: []map[string]any{
			{"action": "add_tag", "params": map[string]any{"tag_id": "{{TAG_ID}}"}},
			{"action": "set_field", "params": map[string]any{"field": "is_round_trip", "value": true}},
		},
		TriggerEvent: "on_import"},
	{TagName: "E-Bike", SystemType: "electric_sync", Priority: -100,
		Conditions: `[]`, Actions: []map[string]any{}, TriggerEvent: "on_import"},
	{TagName: "Indoor", SystemType: "indoor_sync", Priority: -100,
		Conditions: `[]`, Actions: []map[string]any{}, TriggerEvent: "on_import"},
	{TagName: "virtuell", SystemType: "virtual_sync", Priority: -100,
		Conditions: `[]`, Actions: []map[string]any{}, TriggerEvent: "on_import"},
	{TagName: "Frost", SystemType: "frost", Priority: -98,
		Conditions: `[{"field":"min_temperature_c","op":"lte","value":0}]`,
		Actions: []map[string]any{
			{"action": "add_tag", "params": map[string]any{"tag_id": "{{TAG_ID}}"}},
		},
		TriggerEvent: "on_import"},
	{TagName: "Nacht", SystemType: "nacht", Priority: -97,
		Conditions: `[{"field":"sun_elevation","op":"lte","value":-6}]`,
		Actions: []map[string]any{
			{"action": "add_tag", "params": map[string]any{"tag_id": "{{TAG_ID}}"}},
		},
		TriggerEvent: "on_import"},
	// Phase 4b.B.1 — lift_detected. Trigger 'on_enrichment_complete' because
	// lift_count is populated during enrichment, not at import. Re-fired by
	// handler/lift_candidates.go after recomputeLiftStats so heuristic
	// (Method B) confirms also reach the rule (Phase 4b.B.4).
	{TagName: "Lift", SystemType: "lift_detected", Priority: -96,
		Conditions: `[{"field":"has_lift","op":"eq","value":true}]`,
		Actions: []map[string]any{
			{"action": "add_tag", "params": map[string]any{"tag_id": "{{TAG_ID}}"}},
		},
		TriggerEvent: "on_enrichment_complete"},
}

// SystemArtifactsVersion gates the EnsureSystemArtifactsForAllUsers sweep.
// Increment by +1 whenever SystemTags or SystemRules changes. Users whose
// users.system_artifacts_version equals this value are skipped on subsequent
// sweeps — O(changed users) rather than O(all users) per pod start.
//
// Numbering policy: monotonically increasing integer.
//   - Version 1 = the initial snapshot (7 system tags + 6 default-seeded rules,
//     Phase 4b.A).
//   - Version 2 = adds the "Lift" SystemTag + lift_detected SystemRule
//     (Phase 4b.B). Bumping is REQUIRED — otherwise the startup sweep would
//     skip every existing user and only freshly-registered users would receive
//     the new artefact.
//
// Future PRs that add or alter a spec MUST bump this constant in the same
// commit that changes the spec list.
const SystemArtifactsVersion = 2

// sweepAdvisoryLockKey serialises EnsureSystemArtifactsForAllUsers across
// multiple API instances via pg_try_advisory_lock. Today the deployment is
// single-compose, but the lock costs a single round-trip at pod start and
// makes the architecture safe for any future multi-instance setup — without
// the lock, N parallel instances would each run the sweep at once and
// trigger N×O(users) INSERTs.
//
// Value chosen as a random fixed int64; collision check via grep over
// services/api/internal/handler/advisory_locks.go at plan time.
const sweepAdvisoryLockKey int64 = 0x5c46c1f770b0 // "SCA L1FT" mnemonic

// IsValidSystemType replaces the SQL CHECK constraint that migration
// 000356_drop_system_type_check_for_code_validation dropped. Verified
// callsites that write automation_rules.system_type
// (grep at plan time, services/api/internal/):
//
//  1. system_artifacts.go::EnsureSystemArtifacts — iterates SystemRules whose
//     entries are validated by TestSystemTypeWhitelist_RejectsUnknown at
//     compile-test time, so the runtime check is belt-and-braces.
//  2. handler/commute_rule.go createCommuteRule + createCommuteRuleRaw —
//     the per-user "commute_auto" system rule is created on demand when
//     the user enables commute detection. Phase 4b.A.2.1b patches both
//     INSERTs to call IsValidSystemType("commute_auto") first; an invalid
//     result returns 500 with a logged invariant breach.
//
// The previous tags_db.go INSERT block (round_trip/electric_sync/indoor_sync/
// virtual_sync) is REPLACED by EnsureSystemArtifacts in Phase 4b.A.3.
func IsValidSystemType(s string) bool {
	for _, r := range SystemRules {
		if r.SystemType == s {
			return true
		}
	}
	for _, t := range OnDemandSystemTypes {
		if t == s {
			return true
		}
	}
	return false
}

// OnDemandSystemTypes lists system_types that are NOT default-seeded by
// EnsureSystemArtifacts (they're created per-user on demand) but are still
// legitimate system_type values. Keeping this list separate from SystemRules
// makes the "default-seeded vs on-demand" distinction explicit instead of
// implicit.
var OnDemandSystemTypes = []string{"commute_auto"}

// EnsureSystemArtifacts is idempotent. Safe to call multiple times per user.
// Respects user deactivation: never sets is_active back to true on existing
// rules. Runs unconditionally (no version gate) — the gate sits in the batch
// sweep, NOT here, because callers like the SeedSystemDefaults handler want
// the freshest snapshot regardless of stored version.
//
// On success the user's system_artifacts_version is bumped to
// SystemArtifactsVersion so subsequent sweeps skip the user.
func EnsureSystemArtifacts(ctx context.Context, db *database.DB, userID string) error {
	for _, t := range SystemTags {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description)
			VALUES ($1, $2, $3, $4, 'system', true, false, $5)
			ON CONFLICT (user_id, name) DO UPDATE
			SET category = 'system',
			    is_system = true,
			    is_auto = false`,
			userID, t.Name, t.Color, t.TextColor, t.Description,
		); err != nil {
			return fmt.Errorf("ensure tag %s: %w", t.Name, err)
		}
	}
	for _, r := range SystemRules {
		var tagID string
		if err := db.Pool.QueryRow(ctx,
			`SELECT id::text FROM tags WHERE user_id = $1 AND name = $2 AND is_system = true`,
			userID, r.TagName,
		).Scan(&tagID); err != nil {
			return fmt.Errorf("lookup tag %s for rule %s: %w", r.TagName, r.SystemType, err)
		}
		// Resolve {{TAG_ID}} placeholders inside action params.
		actions := make([]map[string]any, len(r.Actions))
		for i, a := range r.Actions {
			cp := map[string]any{"action": a["action"]}
			if p, ok := a["params"].(map[string]any); ok {
				resolved := map[string]any{}
				for k, v := range p {
					if vs, ok := v.(string); ok && vs == "{{TAG_ID}}" {
						resolved[k] = tagID
					} else {
						resolved[k] = v
					}
				}
				cp["params"] = resolved
			}
			actions[i] = cp
		}
		actionsJSON, err := json.Marshal(actions)
		if err != nil {
			return fmt.Errorf("marshal actions for %s: %w", r.SystemType, err)
		}
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO automation_rules (
				user_id, name, is_system, system_type, is_active, priority,
				conditions, conditions_logic, actions, trigger_event
			)
			VALUES ($1, $2, true, $3, true, $4, $5::jsonb, 'and', $6::jsonb, $7)
			ON CONFLICT (user_id, system_type) WHERE is_system = true DO NOTHING`,
			userID, r.TagName, r.SystemType, r.Priority,
			r.Conditions, string(actionsJSON), r.TriggerEvent,
		); err != nil {
			return fmt.Errorf("ensure rule %s: %w", r.SystemType, err)
		}
	}
	if err := refreshSystemArtifactAutomationState(ctx, db, userID); err != nil {
		return err
	}
	if _, err := db.Pool.Exec(ctx,
		`UPDATE users SET system_artifacts_version = $1 WHERE id = $2`,
		SystemArtifactsVersion, userID,
	); err != nil {
		return fmt.Errorf("update system_artifacts_version: %w", err)
	}
	return nil
}

func refreshSystemArtifactAutomationState(ctx context.Context, db *database.DB, userID string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE tags t SET has_automation = EXISTS (
			SELECT 1 FROM automation_rules ar
			WHERE ar.user_id = t.user_id AND ar.is_active = true
			  AND (ar.actions::text LIKE '%' || t.id::text || '%' OR COALESCE(ar.branches::text, '') LIKE '%' || t.id::text || '%')
		) WHERE t.user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("refresh tag automation state: %w", err)
	}
	return nil
}

// EnsureSystemArtifactsForAllUsers backfills users whose stored version is
// below SystemArtifactsVersion. Designed for future multi-instance startup:
//
//   - pg_try_advisory_lock serialises the sweep across pods. If another pod
//     holds the lock, this call returns nil without work (the other pod will
//     finish for everyone).
//   - users.system_artifacts_version gates per-user work — only outdated
//     users are touched, even if SystemArtifactsVersion has bumped only for
//     some specs.
//   - paginated via BTree idx_users_system_artifacts_version, batch size 1000,
//     so memory footprint is bounded regardless of total user count.
//
// Per-user failures are logged (slog.Warn) but do not abort the sweep; the
// failed user's version stays below SystemArtifactsVersion so the next sweep
// retries them.
func EnsureSystemArtifactsForAllUsers(ctx context.Context, db *database.DB) error {
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for sweep advisory lock: %w", err)
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx,
		`SELECT pg_try_advisory_lock($1)`, sweepAdvisoryLockKey,
	).Scan(&locked); err != nil {
		return fmt.Errorf("acquire sweep advisory lock: %w", err)
	}
	if !locked {
		slog.Info("system artifacts sweep skipped — another pod holds the advisory lock")
		return nil
	}
	defer func() {
		// Best-effort unlock on the same connection (advisory locks are session-scoped).
		_, _ = conn.Exec(context.Background(),
			`SELECT pg_advisory_unlock($1)`, sweepAdvisoryLockKey)
	}()

	const batchSize = 1000
	var failed, processed int
	lastID := ""
	for {
		rows, err := db.Pool.Query(ctx, `
			SELECT id::text FROM users
			WHERE system_artifacts_version < $1
			  AND ($3 = '' OR id > $3::uuid)
			ORDER BY id
			LIMIT $2`, SystemArtifactsVersion, batchSize, lastID)
		if err != nil {
			return fmt.Errorf("query stale users: %w", err)
		}
		var batch []string
		for rows.Next() {
			var uid string
			if err := rows.Scan(&uid); err != nil {
				rows.Close()
				return fmt.Errorf("scan user id: %w", err)
			}
			batch = append(batch, uid)
		}
		rows.Close()
		if len(batch) == 0 {
			break
		}
		for _, uid := range batch {
			lastID = uid
			if err := EnsureSystemArtifacts(ctx, db, uid); err != nil {
				slog.Warn("ensure system artifacts failed (non-fatal)",
					"user_id", uid, "error", err)
				failed++
				// Don't update version on failure — user gets retried next sweep.
			}
			processed++
		}
	}
	slog.Info("system artifacts sweep done", "processed", processed, "failed", failed)
	return nil
}
