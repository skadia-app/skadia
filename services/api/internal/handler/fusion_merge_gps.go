package handler

// fusion_merge_gps.go — Bridge zwischen Handler und dem fusion-Package.
//
// Task 2a (fusion_merge.go) definiert den HTTP-Handler MergeActivity und ruft
// h.fuseGPSTracks auf. Diese Datei liefert die echte Implementierung dieses Stubs.
//
// Merge-Hinweis: Falls Task 2a einen Stub "fuseGPSTracks" in fusion_merge.go
// definiert, muss dieser beim Zusammenführen entfernt werden — Go erlaubt
// keine doppelten Methoden-Definitionen im selben Package.

import (
	"context"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/fusion"
)

// GPSFusionConfig konfiguriert die GPS-Fusion für einen Merge-Vorgang.
// Wird von MergeActivity (fusion_merge.go) befüllt und an fuseGPSTracks übergeben.
type GPSFusionConfig struct {
	// PrimarySource: Aktivitäts-ID, deren GPS-Daten bei Zeitüberlappung bevorzugt werden.
	// Leer = Target-Aktivität ist Primary.
	PrimarySource string `json:"primary_source,omitempty"`

	// ExtendTrack: Zeitpunkte aus Quellen übernehmen, die außerhalb des
	// Target-Zeitfensters liegen (Track verlängern statt nur füllen).
	ExtendTrack bool `json:"extend_track,omitempty"`
}

// fuseGPSTracks ist die echte Implementierung des GPS-Stitching-Stubs.
// Sie wird von MergeActivity (fusion_merge.go) aufgerufen.
//
// tempName: Name der TEMP TABLE, in die der fusionierte Track geschrieben wird.
// mergedID: UUID der neu erstellten merged Activity (activity_id in TEMP).
// sourceIDs: Bis zu 6 Aktivitäten als GPS-Quellen.
// config: Optionale Konfiguration; nil bedeutet kein Primary, kein Extend.
func (h *Handler) fuseGPSTracks(ctx context.Context, q database.Querier, tempName, mergedID string, sourceIDs []string, config *GPSFusionConfig) error {
	var stitchConfig *fusion.StitchConfig
	if config != nil {
		stitchConfig = &fusion.StitchConfig{
			PrimarySource: config.PrimarySource,
			ExtendTrack:   config.ExtendTrack,
		}
	}
	return fusion.StitchGPSTracks(ctx, q, tempName, mergedID, sourceIDs, stitchConfig)
}
