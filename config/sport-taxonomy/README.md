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

Source aliases also feed the generated top-level gear-default projection where a
current gear importer has verified raw default-sport labels. Gear defaults remain
stored as top-level `sport_type` strings. They do not store `sub_type` or
modifier predicates until a dedicated gear predicate schema exists.

`projections/*.yaml` defines consumer-specific groupings of canonical tuples.
`projections/activity_filters.yaml` owns activity and map filter placement.
`projections/segments.yaml` defines which sport tuples are eligible for segment
creation, which route-planner sport options are exposed, and which matcher
segment group each eligible tuple uses.
`projections/rules.yaml` defines automation-rule tuple selectors, tuple-scoped
modifier controls, and rule-visible trait condition fields. Projections do not
own taxonomy identity and should not duplicate source/import alias rules.
If a non-selectable broad fallback must remain matchable for compatibility, mark
that row with `eligibility_role: legacy_fallback_coverage` and cover it with a
loader test; do not add hidden generator exceptions.

Generated Go artifacts are committed. Review the YAML source first, then use
generated diffs and tests to verify the runtime contract.

Edit workflow:

1. Edit the relevant YAML source: `sports.yaml`, `aliases/*.yaml`, or
   `projections/*.yaml`.
2. Run `scripts/generate-sport-taxonomy.sh`.
3. Run `scripts/generate-sport-taxonomy.sh --check`.
4. Run the backend and frontend taxonomy tests for the touched surfaces.
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
   capability flags, and optional tag-sync metadata.
2. If the modifier is stored on activities, add the storage migration and update
   all SQL contracts for the enabled capabilities. The parity tests cover
   modifier filters, gear-default predicates, heatmap filters, and tag-sync SQL.
3. Decide every capability explicitly. Do not enable a surface just because a
   neighboring modifier has it; each consumer has different semantics.
4. Add or update `modifier_applicability` and `modifier_defaults` in
   `sports.yaml` only for sports where the modifier can actually apply.
5. Update source aliases only when a raw source label itself carries modifier
   evidence. Parser/device evidence belongs in importer code, not aliases.
6. Regenerate with `scripts/generate-sport-taxonomy.sh`, then run
   `scripts/generate-sport-taxonomy.sh --check`.
7. Run the modifier parity matrix for generated Go/TypeScript metadata,
   SQL validators, rule projections, route filters, records, ShadowFS, sidecar
   metadata, import metadata, and frontend modifier consumers.
8. Check importer precedence: source evidence must not be overwritten by
   taxonomy defaults, explicit false patches must remain possible, and tag sync
   must keep context-specific labels such as `E-Bike` scoped to their sport.

Do not review taxonomy changes only in generated source. The source YAML is the
canonical input; generated tests and projections are the review surfaces for
concrete consumers.
