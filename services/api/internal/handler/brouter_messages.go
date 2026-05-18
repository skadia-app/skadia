package handler

import "github.com/skadia-app/skadia/services/api/internal/routing/brouterenrich"

func injectBRouterWayTagEnrichment(gj map[string]any) {
	brouterenrich.InjectWayTagEnrichment(gj)
}
