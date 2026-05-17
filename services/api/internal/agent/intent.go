package agent

import "strings"

type IntentOperation string

const (
	IntentRead       IntentOperation = "read"
	IntentCreate     IntentOperation = "create"
	IntentUpdate     IntentOperation = "update"
	IntentDelete     IntentOperation = "delete"
	IntentBulkUpdate IntentOperation = "bulk_update"
	IntentPlan       IntentOperation = "plan"
	IntentSave       IntentOperation = "save"
)

type IntentEntity string

const (
	EntityActivity IntentEntity = "activity"
	EntityGear     IntentEntity = "gear"
	EntityTag      IntentEntity = "tag"
	EntityRule     IntentEntity = "automation_rule"
	EntitySegment  IntentEntity = "segment"
	EntityRoute    IntentEntity = "route"
	EntityWeather  IntentEntity = "weather"
	EntityDocument IntentEntity = "document"
	EntityPhoto    IntentEntity = "photo"
	EntityUnknown  IntentEntity = "unknown"
)

type AgentIntent struct {
	Operation      IntentOperation
	Domains        []string
	Entities       []IntentEntity
	WriteRequested bool
	ExpansionLevel int
	Reason         string
}

func (intent AgentIntent) DebugLabel() string {
	parts := []string{string(intent.Operation)}
	if len(intent.Domains) > 0 {
		parts = append(parts, strings.Join(intent.Domains, "+"))
	}
	if len(intent.Entities) > 0 {
		entities := make([]string, 0, len(intent.Entities))
		for _, entity := range intent.Entities {
			entities = append(entities, string(entity))
		}
		parts = append(parts, strings.Join(entities, "+"))
	}
	return strings.Join(parts, ":")
}

func ClassifyAgentIntent(query string, profile *ModelProfile) AgentIntent {
	routeProfile := routerProfile(profile)
	route := RouteQuery(query, routeProfile)
	domains := route.Domains
	if len(domains) == 0 {
		domains = []string{"activities"}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	entities := inferIntentEntities(q, domains)
	op := IntentRead
	write := false

	switch {
	case hasBulkWriteIntent(q):
		op, write = IntentBulkUpdate, true
	case containsAny(q, "loesche", "lösche", "delete", "remove", "entferne"):
		op, write = IntentDelete, true
	case containsAny(q, "lege ", "erstelle", "create", "anlegen", "füge ", "fuege "):
		op, write = IntentCreate, true
	case containsAny(q, "tagge", "setze", "ändere", "aendere", "update", "rename", "benenne"):
		op, write = IntentUpdate, true
	case containsAny(q, "speichere", "save") && containsAny(q, "route"):
		op, write = IntentSave, true
	case containsAny(q, "plane", "planen", "route", "rundtour", "rundroute"):
		op = IntentPlan
	}

	return AgentIntent{
		Operation:      op,
		Domains:        domains,
		Entities:       entities,
		WriteRequested: write,
		Reason:         "intent-router",
	}
}

func routerProfile(profile *ModelProfile) *ModelProfile {
	if profile == nil {
		p := GetProfile("small", "", nil)
		return &p
	}
	p := *profile
	p.UseRouter = true
	return &p
}

func hasBulkWriteIntent(q string) bool {
	hasWriteVerb := containsAny(q, "tagge", "setze", "ändere", "aendere", "update", "lösche", "loesche", "delete", "remove", "entferne", "füge", "fuege")
	hasBulkScope := containsAny(q, " alle ", " allen ", " jede ", " jeden ", "all ", "bulk", "rueckwirkend", "rückwirkend")
	hasTarget := containsAny(q, "tag", "aktivität", "aktivitaet", "activity", "regel", "rule")
	return hasWriteVerb && hasBulkScope && hasTarget
}

func inferIntentEntities(q string, domains []string) []IntentEntity {
	seen := map[IntentEntity]bool{}
	add := func(entity IntentEntity) {
		if !seen[entity] {
			seen[entity] = true
		}
	}
	if containsAny(q, "tag") {
		add(EntityTag)
	}
	if containsAny(q, "regel", "rule", "automation") {
		add(EntityRule)
	}
	if containsAny(q, "gear", "fahrrad", "rad", "ausrüstung", "ausruestung", "schuh", "kette", "reifen") {
		add(EntityGear)
	}
	if containsAny(q, "segment", "bestzeit", "pr", "rekord") {
		add(EntitySegment)
	}
	if containsAny(q, "route", "routenprofil", "routingprofil", "routing profile", "rundtour", "rundroute", "strecke", "luftlinie", "ort", "wetter") {
		add(EntityRoute)
	}
	if containsAny(q, "wetter", "weather", "regen", "wind", "temperatur") {
		add(EntityWeather)
	}
	if containsAny(q, "dokument", "rechnung", "beleg", "ocr", "markdown", "pdf") {
		add(EntityDocument)
	}
	if containsAny(q, "foto", "bild", "photo") {
		add(EntityPhoto)
	}
	for _, domain := range domains {
		switch domain {
		case "activities":
			if len(seen) == 0 || containsActivityEntityHint(q) {
				add(EntityActivity)
			}
		case "training", "geo":
			add(EntityActivity)
		case "gear":
			add(EntityGear)
		case "segments":
			add(EntitySegment)
		case "routing":
			add(EntityRoute)
		case "documents":
			add(EntityDocument)
		case "photos":
			add(EntityPhoto)
		case "health":
			add(EntityActivity)
		}
	}
	if len(seen) == 0 {
		return []IntentEntity{EntityUnknown}
	}
	out := make([]IntentEntity, 0, len(seen))
	for _, entity := range []IntentEntity{EntityRoute, EntitySegment, EntityActivity, EntityGear, EntityTag, EntityRule, EntityWeather, EntityDocument, EntityPhoto, EntityUnknown} {
		if seen[entity] {
			out = append(out, entity)
		}
	}
	return out
}

func containsActivityEntityHint(q string) bool {
	return containsAny(q,
		"aktivität", "aktivitaet", "activity",
		"training", "trainings",
		"lauf", "laufen", "run",
		"radfahrt", "radfahrten", "radtour", "radtouren", "ride",
		"schwimm", "swim",
		"wanderung", "hike",
	)
}
