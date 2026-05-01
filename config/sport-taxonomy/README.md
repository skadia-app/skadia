# Sport Taxonomy

This directory contains build-time product data for Skadia's canonical sport taxonomy.

Edit workflow:

1. Edit `sports.yaml`.
2. Run `scripts/generate-sport-taxonomy.sh`.
3. Run `scripts/generate-sport-taxonomy.sh --check`.
4. Run the backend and frontend taxonomy tests named in the implementation plan.

Plan A owns canonical category and sport registry data only. Alias tables, UI filter projections, segment eligibility, rule tuple controls, and DB predicate SQL are separate plans.
