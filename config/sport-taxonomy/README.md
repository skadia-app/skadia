# Sport Taxonomy

This directory contains build-time product data for Skadia's canonical sport taxonomy.

`sports.yaml` is the canonical source for sport categories, sport tuples, labels,
modifier applicability, modifier defaults, and stable trait metadata. Traits
classify durable sport identity dimensions such as environment, movement,
equipment, activity form, and propulsion. Modifiers describe optional
per-activity execution states such as indoor, virtual, electric, or commute.

`aliases/*.yaml` maps raw source/import sport labels to canonical sport tuples.
Use aliases for importer labels, legacy spellings, file-format sport strings,
and source-specific modifier patches when the raw source label itself carries
that evidence. Do not use aliases to create new canonical sports or to encode
parser/device evidence that is derived from numeric IDs or file metadata.

`projections/*.yaml` defines consumer-specific groupings of canonical tuples.
`projections/activity_filters.yaml` owns activity and map filter placement.
`projections/segments.yaml` defines which sport tuples are eligible for segment
creation and which matcher segment group each eligible tuple uses.
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

Do not review taxonomy changes only in generated source. The source YAML is the
canonical input; generated tests and projections are the review surfaces for
concrete consumers.
