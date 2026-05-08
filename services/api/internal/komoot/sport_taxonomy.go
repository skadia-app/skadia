package komoot

import (
	"log/slog"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/importmetrics"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func komootRouteSportTuple(komootSport string, logAttrs ...any) (string, string, bool) {
	return komootRouteSportTupleWithRecorder(komootSport, importmetrics.RecordUnmappedSport, logAttrs...)
}

func komootRouteSportTupleWithRecorder(komootSport string, recordUnmappedSport func(importmetrics.Source), logAttrs ...any) (string, string, bool) {
	resolved, err := sport.ResolveSport(sport.ResolveInput{Source: sport.SourceKomoot, RawSport: komootSport})
	if err != nil || !resolved.Resolved {
		attrs := []any{"source", "komoot_route", "raw_sport", strings.TrimSpace(komootSport)}
		attrs = append(attrs, logAttrs...)
		slog.Warn("komoot route: unmapped sport type", attrs...)
		recordUnmappedSport(importmetrics.SourceKomootRoute)
		return "unknown", "", false
	}
	modifiers := sport.ApplyModifierPatch(sport.ModifierValues{}, resolved.Patch)
	return resolved.Tuple.SportType, resolved.Tuple.SubType, modifiers.IsElectric
}
