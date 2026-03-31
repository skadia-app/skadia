package model

// PairData enthält die Overlap-Daten eines Paares aus fusion_candidates.
// Definiert in model (nicht fusion) um Import-Cycle database↔fusion zu vermeiden.
type PairData struct {
	ActivityIDA    string
	ActivityIDB    string
	OverlapPercent float64
	FieldRichnessA int
	FieldRichnessB int
	TimeDiffSec    float64
}
