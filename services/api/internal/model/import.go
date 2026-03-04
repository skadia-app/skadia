package model

// ImportSettings steuert das Duplikat-Handling beim Import.
type ImportSettings struct {
	ConflictStrategy      string `json:"conflict_strategy"`       // "enrich_missing"|"replace_all"|"skip"
	GPSDuplicateThreshold int    `json:"gps_duplicate_threshold"` // 60–95, Default 75
}
