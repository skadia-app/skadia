package sport

import "testing"

func TestResolveSportPrefersExactRawSubAlias(t *testing.T) {
	got, err := ResolveSport(ResolveInput{
		Source:   SourceLegacy,
		RawSport: "wheelchair_push_walk",
		RawSub:   "indoor_wheelchair_walk",
	})
	if err != nil {
		t.Fatalf("ResolveSport() error = %v", err)
	}
	if !got.AliasMatched || !got.Resolved {
		t.Fatalf("matched/resolved = %v/%v, want true/true", got.AliasMatched, got.Resolved)
	}
	if got.Tuple != (Tuple{SportType: "wheelchair_push", SubType: "walk_speed"}) {
		t.Fatalf("tuple = %#v, want wheelchair_push/walk_speed", got.Tuple)
	}
	if got.Patch.IsIndoor == nil || !*got.Patch.IsIndoor {
		t.Fatalf("IsIndoor patch = %#v, want true", got.Patch.IsIndoor)
	}
}

func TestResolveSportFallsBackToRawOnlyAliasAfterExactMiss(t *testing.T) {
	got, err := ResolveSport(ResolveInput{
		Source:   SourceLegacy,
		RawSport: "bike",
		RawSub:   "road",
	})
	if err != nil {
		t.Fatalf("ResolveSport() error = %v", err)
	}
	if got.Tuple != (Tuple{SportType: "cycling", SubType: "road"}) {
		t.Fatalf("tuple = %#v, want cycling/road", got.Tuple)
	}
}

func TestResolveSportLegacyRawOnlyAliasCarriesCanonicalSubType(t *testing.T) {
	got, err := ResolveSport(ResolveInput{
		Source:   SourceLegacy,
		RawSport: "cycling",
		RawSub:   "mountain",
	})
	if err != nil {
		t.Fatalf("ResolveSport() error = %v", err)
	}
	if got.Tuple != (Tuple{SportType: "cycling", SubType: "mountain"}) {
		t.Fatalf("tuple = %#v, want cycling/mountain", got.Tuple)
	}
}

func TestResolveSportNoMatchForStrictSourceIsUnresolved(t *testing.T) {
	got, err := ResolveSport(ResolveInput{Source: SourceStrava, RawSport: "Not A Known Label"})
	if err != nil {
		t.Fatalf("ResolveSport() error = %v", err)
	}
	if got.AliasMatched || got.Resolved {
		t.Fatalf("matched/resolved = %v/%v, want false/false", got.AliasMatched, got.Resolved)
	}
}
