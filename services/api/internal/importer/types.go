package importer

import "slices"

import "time"

// ConflictStrategy steuert was bei Duplikat-Erkennung passiert.
type ConflictStrategy string

const (
	StrategyEnrichMissing ConflictStrategy = "enrich_missing" // Default
	StrategyReplaceAll    ConflictStrategy = "replace_all"
	StrategySkip          ConflictStrategy = "skip"
)

// DuplicateMatch ist ein sicheres Duplikat (Tier-1, hohe Konfidenz).
type DuplicateMatch struct {
	ExistingActivityID string
	Confidence         string   // "hash" | "platform_id" | "temporal"
	MissingFields      []string // Felder die in der bestehenden Aktivität fehlen/0 sind
}

// FusionCandidate ist ein unsicheres Duplikat (Tier-2, zwei verschiedene Geräte).
type FusionCandidate struct {
	ActivityIDA    string
	ActivityIDB    string
	OverlapPercent float64
	FieldRichnessA int
	FieldRichnessB int
	TimeDiffSec    float64
}

// ImportOptions steuert den selektiven Import (Kategorien, Datumsbereich, Strategie).
type ImportOptions struct {
	Categories       []string // nil = alle Kategorien
	DateFrom         *time.Time
	DateTo           *time.Time
	ConflictStrategy ConflictStrategy
}

// HasCategory prüft ob eine Kategorie in den Options enthalten ist.
// nil Options oder leere Categories = alle Kategorien erlaubt.
func (o *ImportOptions) HasCategory(name string) bool {
	if o == nil || len(o.Categories) == 0 {
		return true
	}
	return slices.Contains(o.Categories, name)
}
