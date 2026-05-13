package sport

import "testing"

func TestResolveNaturalSportQueryUsesGeneratedAliases(t *testing.T) {
	tests := []struct {
		query     string
		sportType string
		subType   string
	}{
		{"When was my last swim?", "swimming", ""},
		{"letzte Freiwasserschwimmen Aktivität", "swimming", "open_water"},
		{"wie viele Gravel Ride Kilometer", "cycling", "gravel_cycling"},
		{"MTB dieses Jahr", "cycling", "mountain"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got, ok := ResolveNaturalSportQuery(tt.query)
			if !ok {
				t.Fatalf("ResolveNaturalSportQuery(%q) returned no match", tt.query)
			}
			if got.Tuple.SportType != tt.sportType || got.Tuple.SubType != tt.subType {
				t.Fatalf("tuple = %#v, want %s/%s", got.Tuple, tt.sportType, tt.subType)
			}
		})
	}
}

func TestQueryContainsKnownSport(t *testing.T) {
	if !QueryContainsKnownSport("Can you tell me about my last swim?") {
		t.Fatal("QueryContainsKnownSport returned false for swim")
	}
	if QueryContainsKnownSport("Was ist heute los?") {
		t.Fatal("QueryContainsKnownSport returned true for query without sport")
	}
}
