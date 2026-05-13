package sport

import (
	"strings"
	"sync"
	"unicode"
)

type NaturalSportMatch struct {
	Query        string
	MatchedTerm  string
	Tuple        Tuple
	Patch        ModifierPatch
	AliasMatched bool
}

var (
	naturalSportOnce  sync.Once
	naturalSportTerms []naturalSportTerm
)

type naturalSportTerm struct {
	term         string
	tuple        Tuple
	patch        ModifierPatch
	aliasMatched bool
}

func ResolveNaturalSportQuery(query string) (NaturalSportMatch, bool) {
	normalizedQuery := normalizeSportSearchText(query)
	if normalizedQuery == "" {
		return NaturalSportMatch{}, false
	}
	naturalSportOnce.Do(buildNaturalSportTerms)
	for _, term := range naturalSportTerms {
		if containsSportSearchTerm(normalizedQuery, term.term) {
			return NaturalSportMatch{
				Query:        query,
				MatchedTerm:  term.term,
				Tuple:        term.tuple,
				Patch:        term.patch,
				AliasMatched: term.aliasMatched,
			}, true
		}
	}
	return NaturalSportMatch{}, false
}

func QueryContainsKnownSport(query string) bool {
	_, ok := ResolveNaturalSportQuery(query)
	return ok
}

func buildNaturalSportTerms() {
	seen := map[string]bool{}
	add := func(raw string, tuple Tuple, patch ModifierPatch, aliasMatched bool) {
		term := normalizeSportSearchText(raw)
		if term == "" || seen[term] {
			return
		}
		seen[term] = true
		naturalSportTerms = append(naturalSportTerms, naturalSportTerm{
			term:         term,
			tuple:        tuple,
			patch:        patch,
			aliasMatched: aliasMatched,
		})
	}

	for _, def := range generatedSportDefs {
		tuple := Tuple{SportType: def.SportType}
		add(def.SportType, tuple, ModifierPatch{}, false)
		add(def.LabelDE, tuple, ModifierPatch{}, false)
		add(def.AutoTitleDE, tuple, ModifierPatch{}, false)
		for _, subType := range def.SubTypeOrder {
			st := def.SubTypes[subType]
			subTuple := Tuple{SportType: def.SportType, SubType: st.SubType}
			add(st.SubType, subTuple, ModifierPatch{}, false)
			add(st.LabelDE, subTuple, ModifierPatch{}, false)
			add(st.DetailDE, subTuple, ModifierPatch{}, false)
			add(st.AutoTitleDE, subTuple, ModifierPatch{}, false)
		}
	}
	for _, aliases := range generatedSourceAliases {
		for _, alias := range aliases {
			add(alias.RawSport, alias.Tuple, alias.Patch, true)
			if alias.RawSub != "" {
				add(alias.RawSub, alias.Tuple, alias.Patch, true)
				add(alias.RawSport+" "+alias.RawSub, alias.Tuple, alias.Patch, true)
			}
		}
	}

	sortNaturalSportTerms()
}

func sortNaturalSportTerms() {
	for i := 1; i < len(naturalSportTerms); i++ {
		current := naturalSportTerms[i]
		j := i - 1
		for j >= 0 && len([]rune(naturalSportTerms[j].term)) < len([]rune(current.term)) {
			naturalSportTerms[j+1] = naturalSportTerms[j]
			j--
		}
		naturalSportTerms[j+1] = current
	}
}

func normalizeSportSearchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"ä", "ae",
		"ö", "oe",
		"ü", "ue",
		"ß", "ss",
		"_", " ",
		"-", " ",
		"/", " ",
	)
	value = replacer.Replace(value)
	var b strings.Builder
	lastSpace := true
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func containsSportSearchTerm(query, term string) bool {
	if query == term {
		return true
	}
	return strings.Contains(" "+query+" ", " "+term+" ")
}
