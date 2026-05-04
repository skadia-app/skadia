package komoot

import "github.com/skadia-app/skadia/services/api/internal/sport"

func komootRouteSportTuple(komootSport string) (string, string, bool) {
	resolved, err := sport.ResolveSport(sport.ResolveInput{Source: sport.SourceKomoot, RawSport: komootSport})
	if err != nil || !resolved.Resolved {
		return "unknown", "", false
	}
	modifiers := sport.ApplyModifierPatch(sport.ModifierValues{}, resolved.Patch)
	return resolved.Tuple.SportType, resolved.Tuple.SubType, modifiers.IsElectric
}
