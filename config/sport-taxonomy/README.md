# Sport Taxonomy

This directory contains build-time product data for Skadia's canonical sport taxonomy.

`sports.yaml` is the canonical source for sport categories, sport tuples, labels,
modifier applicability, and stable trait metadata. Traits classify durable sport
identity dimensions such as environment, movement, equipment, activity form, and
propulsion. Modifiers describe optional per-activity execution states such as
indoor, virtual, electric, or commute. Projection files decide where those
tuples appear in concrete UI, filter, rule, segment, or importer surfaces.

Edit workflow:

1. Edit `sports.yaml`.
2. Run `scripts/generate-sport-taxonomy.sh`.
3. Run `scripts/generate-sport-taxonomy.sh --check`.
4. Run the backend and frontend taxonomy tests for the touched surfaces.
5. Run the final implementation Critic and fix accepted findings.
6. Run the required documentation sync for any reader-facing or developer-facing
   surfaces that changed.

Do not review taxonomy changes only in generated source. The source YAML is the
canonical input; generated tests and projections are the review surfaces for
concrete consumers.
