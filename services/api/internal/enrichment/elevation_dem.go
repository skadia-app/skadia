package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	openMeteoElevationURL = "https://api.open-meteo.com/v1/elevation"
	demBatchSize          = 100
	demRequestTimeout     = 10 * time.Second
)

type demResponse struct {
	Elevation []float64 `json:"elevation"`
}

// FetchDEMElevations holt Ground-Truth-Höhenwerte von Open-Meteo.
// Die Koordinaten werden in Batches à demBatchSize Punkte abgefragt.
func FetchDEMElevations(ctx context.Context, lats, lngs []float64) ([]float64, error) {
	if len(lats) != len(lngs) || len(lats) == 0 {
		return nil, fmt.Errorf("invalid coordinates: len mismatch or empty")
	}

	result := make([]float64, len(lats))

	for i := 0; i < len(lats); i += demBatchSize {
		end := i + demBatchSize
		if end > len(lats) {
			end = len(lats)
		}

		batch, err := fetchDEMBatch(ctx, lats[i:end], lngs[i:end])
		if err != nil {
			return nil, fmt.Errorf("DEM batch %d: %w", i/demBatchSize, err)
		}
		copy(result[i:end], batch)
	}

	return result, nil
}

func fetchDEMBatch(ctx context.Context, lats, lngs []float64) ([]float64, error) {
	latStrs := make([]string, len(lats))
	lngStrs := make([]string, len(lngs))
	for i := range lats {
		latStrs[i] = fmt.Sprintf("%.6f", lats[i])
		lngStrs[i] = fmt.Sprintf("%.6f", lngs[i])
	}

	url := fmt.Sprintf("%s?latitude=%s&longitude=%s",
		openMeteoElevationURL,
		strings.Join(latStrs, ","),
		strings.Join(lngStrs, ","),
	)

	reqCtx, cancel := context.WithTimeout(ctx, demRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Skadia/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo returned %d", resp.StatusCode)
	}

	var data demResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if len(data.Elevation) != len(lats) {
		return nil, fmt.Errorf("DEM response length mismatch: got %d, want %d",
			len(data.Elevation), len(lats))
	}

	return data.Elevation, nil
}

// extractStreamCoordinates extrahiert Lat/Lng-Arrays aus den Activity-Streams.
// Nur Punkte mit vorhandenen GPS-Koordinaten werden berücksichtigt.
// Gibt zusätzlich die originalen Indizes zurück, um die Zuordnung zu erhalten.
func extractStreamCoordinates(streams []StreamPointForDEM) (lats, lngs []float64, indices []int) {
	for i, s := range streams {
		if s.Lat != nil && s.Lng != nil {
			lats = append(lats, *s.Lat)
			lngs = append(lngs, *s.Lng)
			indices = append(indices, i)
		}
	}
	return
}

// StreamPointForDEM ist ein Interface-kompatibler Typ, der von applyDEMCorrection
// und extractStreamCoordinates genutzt wird. Wird direkt auf model.StreamPoint gemappt.
type StreamPointForDEM struct {
	Lat      *float64
	Lng      *float64
	Altitude *float64 // entspricht model.StreamPoint.AltitudeM
}

// applyDEMCorrectionToStreams ersetzt Höhenwerte in den Streams durch DEM-Werte.
// Bei starker Abweichung (>50m) wird der barometrische/GPS-Wert des Geräts beibehalten,
// da lokale Druckänderungen genauer sein können als das DEM-Raster.
// Gibt die Anzahl korrigierter Punkte zurück.
func applyDEMCorrectionToStreams(altitudes []*float64, demElevs []float64, indices []int) int {
	corrected := 0
	for j, idx := range indices {
		if j >= len(demElevs) {
			break
		}
		demVal := demElevs[j]

		if altitudes[idx] != nil {
			diff := demVal - *altitudes[idx]
			if diff < 0 {
				diff = -diff
			}
			if diff > 50 {
				// Barometrische Höhe beibehalten (>50m Abweichung deutet auf Druckmessung hin)
				continue
			}
		}
		v := demVal
		altitudes[idx] = &v
		corrected++
	}
	return corrected
}
