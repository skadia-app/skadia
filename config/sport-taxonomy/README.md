# Sport Taxonomy

This directory contains build-time product data for Skadia's canonical sport taxonomy.

`sports.yaml` is the canonical source for sport categories, sport tuples, labels,
modifier applicability, modifier defaults, and stable trait metadata. Traits
classify durable sport identity dimensions such as environment, movement,
equipment, activity form, and propulsion. Modifiers describe optional
per-activity execution states such as indoor, virtual, electric, or commute.

`aliases/*.yaml` maps raw source/import sport labels to canonical sport tuples.
Use aliases for importer labels, legacy spellings, file-format sport strings,
route/course export labels, and source-specific modifier patches when the raw
source label itself carries that evidence. Do not use aliases to create new
canonical sports or to encode parser/device evidence that is derived from
numeric IDs or file metadata.

Source aliases also feed the generated gear-default projection where a current
gear importer has verified raw default-sport labels. The projection source list
lives in `projections/gear_defaults.yaml`; add a source there only after its
gear-default labels have been reviewed against the canonical aliases. Gear
defaults store both the broad `sport_type` compatibility value and predicate
defaults that may include a `sub_type` or modifier filters.

`projections/*.yaml` defines consumer-specific groupings of canonical tuples.
`projections/activity_filters.yaml` owns activity and map filter placement.
`projections/segments.yaml` defines which sport tuples are eligible for segment
creation, which route-planner sport options are exposed, and which matcher
segment group each eligible tuple uses.
`projections/rules.yaml` defines automation-rule tuple selectors, tuple-scoped
modifier controls, and rule-visible trait condition fields. Projections do not
own taxonomy identity and should not duplicate source/import alias rules.
`projections/gear_defaults.yaml` defines which source alias namespaces are
allowed to contribute gear-default projections. It does not define sports
itself; it only selects already validated alias namespaces.
If a non-selectable broad fallback must remain matchable for compatibility, mark
that row with `eligibility_role: legacy_fallback_coverage` and cover it with a
loader test; do not add hidden generator exceptions.

Generated Go artifacts are committed. Review the YAML source first, then use
generated diffs and tests to verify the runtime contract.
The JSON schemas in `schema/*.schema.json` are editor guidance and test
fixtures. Loader code remains authoritative, and the schema-validation tests
compare the repository YAML against the schemas so schema drift fails in the
normal taxonomy test suite.

Edit workflow:

1. Edit the relevant YAML source: `sports.yaml`, `aliases/*.yaml`,
   `modifiers.yaml`, or `projections/*.yaml`.
2. Run `scripts/generate-sport-taxonomy.sh`.
3. Run `scripts/generate-sport-taxonomy.sh --check`.
4. Run the backend and frontend taxonomy tests for the touched surfaces. The
   backend taxonomy-generator tests validate the YAML/schema pairs and compare
   committed generated artifacts against freshly rendered output.
5. Run the required documentation sync for any reader-facing or developer-facing
   surfaces that changed.

Adding a top-level sport:

1. Add the sport once in `sports.yaml` with labels, auto title, category,
   segment group, GPS behavior, lifecycle/selectability, modifier applicability,
   traits, and any sub-types.
2. Add import/source aliases in `aliases/*.yaml` only for raw external or legacy
   labels that should normalize to the new tuple.
3. Add user-facing filter placement in `projections/activity_filters.yaml`.
   Frontend `SPORT_DEFS` display order is derived from the `activity_all`
   projection, not from a hand-maintained TypeScript list.
4. Add route-planner exposure in `projections/segments.yaml` `route_options`
   only if the tuple should be selectable for routes; add segment eligibility in
   `eligible` only if it should participate in segment matching.
5. Add records, rules, or segment projections only when those consumers need the
   tuple. Do not add empty projection rows just to mirror `sports.yaml`.
6. Regenerate and review generated Go/TypeScript diffs instead of editing them
   by hand.

Adding a modifier:

1. Add the modifier to `modifiers.yaml` with stable key, German label, Go field,
   TypeScript field, JSON key, activity column, action names, ShadowFS token,
   capability flags, and optional tag-sync metadata. Applicability source names
   are derived from the modifier key; do not duplicate them in YAML.
2. If the modifier is stored on activities, add the storage migration and update
   all SQL contracts for the enabled capabilities. The parity tests cover
   modifier filters, gear-default predicates, heatmap filters, and tag-sync SQL.
3. Decide every capability explicitly. Do not enable a surface just because a
   neighboring modifier has it; each consumer has different semantics.
4. Add or update `modifier_applicability` in `sports.yaml` for every top-level
   sport. The generator validates that every registered modifier key is present,
   so adding a modifier is an explicit taxonomy decision for each sport.
5. Add `modifier_defaults` only for sports where the modifier has
   `taxonomy_default: true`. This keeps automatic tuple defaults narrow: a
   modifier can be stored or filtered without being eligible for implicit
   defaulting.
6. Update source aliases only when a raw source label itself carries modifier
   evidence. Parser/device evidence belongs in importer code, not aliases.
7. Regenerate with `scripts/generate-sport-taxonomy.sh`, then run
   `scripts/generate-sport-taxonomy.sh --check`.
8. Run the modifier parity matrix for generated Go/TypeScript metadata,
   SQL validators, rule projections, route filters, records, ShadowFS, sidecar
   metadata, import metadata, and frontend modifier consumers.
9. Check importer precedence: source evidence must not be overwritten by
   taxonomy defaults, explicit false patches must remain possible, and tag sync
   must keep context-specific labels such as `E-Bike` scoped to their sport.

The modifier registry is strict build-time input. Deploy the generator/code that
knows a new capability before adding that capability to `modifiers.yaml`.
`rule_condition` and `rule_action` are intentionally symmetrical today: rule
modifiers are exposed as condition and action pairs, not condition-only or
action-only controls.

Stored activity modifiers still require explicit storage and importer work:
adding a stored modifier means adding the activity column/migration, Go model
fields, sidecar/manual-file fields, and importer assignment tests. The registry
and parity tests make those checkpoints fail loudly; they do not create storage
columns dynamically. `is_virtual` also forces heatmap exclusion in the quick-edit
UI; that UX coupling is hand-coded and must be reviewed if virtual semantics or
heatmap behavior change.

Do not review taxonomy changes only in generated source. The source YAML is the
canonical input; generated tests and projections are the review surfaces for
concrete consumers.
