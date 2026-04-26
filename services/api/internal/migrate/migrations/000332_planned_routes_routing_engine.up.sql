ALTER TABLE planned_routes
    ADD COLUMN routing_engine TEXT
        CHECK (routing_engine IS NULL OR routing_engine IN ('badrouter', 'brouter'));

COMMENT ON COLUMN planned_routes.routing_engine IS
    'Engine that produced the saved geometry (badrouter|brouter). NULL for legacy/unknown routes — frontend falls back to current state.routingEngine on load. Set by saveRoute when editingRouteId is null (create) or via PATCH /routes/{id} on update.';
