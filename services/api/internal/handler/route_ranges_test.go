package handler

import (
	"reflect"
	"testing"
)

func TestContiguousRangesFromMatch_Empty(t *testing.T) {
	got := contiguousRangesFromMatch([]bool{}, []int{}, 0)
	if len(got) != 0 {
		t.Fatalf("empty → got %v, want empty", got)
	}
}

func TestContiguousRangesFromMatch_NoMatches(t *testing.T) {
	got := contiguousRangesFromMatch(
		[]bool{false, false, false},
		[]int{0, 5, 10},
		14,
	)
	if len(got) != 0 {
		t.Fatalf("no matches → got %v, want empty", got)
	}
}

func TestContiguousRangesFromMatch_AllMatch(t *testing.T) {
	got := contiguousRangesFromMatch(
		[]bool{true, true, true},
		[]int{0, 5, 10},
		14,
	)
	want := []CoordRange{{From: 0, To: 15}} // last edge: lastCoordIdx+1
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestContiguousRangesFromMatch_Alternating(t *testing.T) {
	// match-nomatch-match → two separate ranges
	got := contiguousRangesFromMatch(
		[]bool{true, false, true},
		[]int{0, 5, 10},
		14,
	)
	want := []CoordRange{{From: 0, To: 5}, {From: 10, To: 15}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestContiguousRangesFromMatch_ContiguousMerges(t *testing.T) {
	got := contiguousRangesFromMatch(
		[]bool{true, true, false, true},
		[]int{0, 5, 10, 15},
		19,
	)
	want := []CoordRange{{From: 0, To: 10}, {From: 15, To: 20}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
