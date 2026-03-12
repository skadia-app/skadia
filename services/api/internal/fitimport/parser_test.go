package fitimport

import (
	"testing"

	"github.com/muktihari/fit/profile/typedef"
)

// TestAllSameSport_True — alle gleiche Sportart
func TestAllSameSport_True(t *testing.T) {
	sports := []typedef.Sport{typedef.SportCycling, typedef.SportCycling}
	if !allSameSport(sports) {
		t.Error("expected true for two cycling sessions")
	}
}

// TestAllSameSport_False — unterschiedliche Sportarten
func TestAllSameSport_False(t *testing.T) {
	sports := []typedef.Sport{typedef.SportCycling, typedef.SportRunning}
	if allSameSport(sports) {
		t.Error("expected false for cycling + running")
	}
}

// TestAllSameSport_Empty — leere Liste → false
func TestAllSameSport_Empty(t *testing.T) {
	if allSameSport(nil) {
		t.Error("expected false for empty slice")
	}
}

// TestAllSameSport_SingleElement — Einzelelement → true
func TestAllSameSport_Single(t *testing.T) {
	sports := []typedef.Sport{typedef.SportRunning}
	if !allSameSport(sports) {
		t.Error("expected true for single element")
	}
}

// TestAllSameSport_TriathlonSports — Triathlon-Mix → false
func TestAllSameSport_TriathlonSports(t *testing.T) {
	sports := []typedef.Sport{
		typedef.SportSwimming,
		typedef.SportTransition,
		typedef.SportCycling,
		typedef.SportTransition,
		typedef.SportRunning,
	}
	if allSameSport(sports) {
		t.Error("expected false for triathlon mix")
	}
}
