package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	osmregions "github.com/skadia-app/skadia/services/api/internal/osm"
)

// Kategorie-Definitionen: ID → betroffene Tabellen
var osmCategories = map[string][]string{
	"peaks":       {"osm_peaks"},
	"passes":      {"osm_passes"},
	"lifts":       {"osm_lifts"},
	"settlements": {"osm_settlements"},
	"amenities":   {"osm_amenities"},
	"waterways":   {"osm_waterways", "osm_waters"},
	"forests":     {"osm_forests"},
	"boundaries":  {"osm_boundaries"},
}

// Env-Vars für Lua-Script
var categoryEnvVars = map[string]string{
	"peaks":       "SKADIA_IMPORT_PEAKS",
	"passes":      "SKADIA_IMPORT_PASSES",
	"lifts":       "SKADIA_IMPORT_LIFTS",
	"settlements": "SKADIA_IMPORT_SETTLEMENTS",
	"amenities":   "SKADIA_IMPORT_AMENITIES",
	"waterways":   "SKADIA_IMPORT_WATERWAYS",
	"forests":     "SKADIA_IMPORT_FORESTS",
	"boundaries":  "SKADIA_IMPORT_BOUNDARIES",
}

type osmImportRequest struct {
	Regions    []string `json:"regions"`
	Categories []string `json:"categories"`
}

// ListOSMRegions gibt den Regions-Katalog zurück.
// GET /api/v1/admin/osm-regions
func (h *Handler) ListOSMRegions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"regions": osmregions.Regions})
}

// StartOSMImport startet einen asynchronen OSM-Import-Job.
// POST /api/v1/admin/osm-import/start
func (h *Handler) StartOSMImport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req osmImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Regions) == 0 || len(req.Categories) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "regions and categories required", r.URL.Path)
		return
	}

	// Regions validieren
	var validRegions []osmregions.Region
	for _, id := range req.Regions {
		reg, ok := osmregions.RegionByID(id)
		if !ok {
			writeError(w, http.StatusBadRequest, "Bad Request", "unknown region: "+id, r.URL.Path)
			return
		}
		validRegions = append(validRegions, reg)
	}

	// Kategorien validieren
	for _, cat := range req.Categories {
		if _, ok := osmCategories[cat]; !ok {
			writeError(w, http.StatusBadRequest, "Bad Request", "unknown category: "+cat, r.URL.Path)
			return
		}
	}

	// Region-Namen für entity_name zusammenstellen
	var labels []string
	var totalSizeMB int
	for _, reg := range validRegions {
		labels = append(labels, reg.Label)
		totalSizeMB += reg.SizeMB
	}
	entityName := strings.Join(labels, ", ")

	// Initialer progress_total = geschätzte Gesamtgröße in Bytes
	progressTotal := totalSizeMB * 1024 * 1024

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "osm_import", nil, &entityName, progressTotal)
	if err != nil {
		slog.Error("failed to insert osm import job", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	go h.runOSMImport(jobID, userID, validRegions, req.Categories)

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

var osmProgressRe = regexp.MustCompile(`Node\((\d+)k`)

func (h *Handler) runOSMImport(jobID, userID string, regions []osmregions.Region, categories []string) {
	ctx := context.Background()
	dataDir := os.Getenv("OSM_DATA_DIR")
	if dataDir == "" {
		dataDir = "/data/osm"
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		slog.Error("osm import: failed to create data dir", "error", err)
		h.DB.CompleteBackgroundJob(ctx, jobID, true)
		return
	}

	// Lua-Script in Temp-Datei schreiben
	luaTmp, err := os.CreateTemp("", "skadia-osm-*.lua")
	if err != nil {
		slog.Error("osm import: failed to write lua tmp", "error", err)
		h.DB.CompleteBackgroundJob(ctx, jobID, true)
		return
	}
	defer os.Remove(luaTmp.Name())
	if _, err := luaTmp.Write(osmregions.LuaScript); err != nil {
		slog.Error("osm import: failed to write lua content", "error", err)
		h.DB.CompleteBackgroundJob(ctx, jobID, true)
		return
	}
	luaTmp.Close()

	dbURL := os.Getenv("DATABASE_URL")

	for _, reg := range regions {
		pbfPath := filepath.Join(dataDir, reg.ID+"-latest.osm.pbf")

		// === Phase 1: Download ===
		h.DB.UpdateBackgroundJobPhase(ctx, jobID, "download")
		h.DB.UpdateBackgroundJobProgress(ctx, jobID, 0)

		slog.Info("osm import: downloading", "region", reg.ID, "url", reg.URL)
		if err := downloadWithProgress(ctx, reg.URL, pbfPath, func(downloaded, total int64) {
			if total > 0 {
				h.DB.UpdateBackgroundJobProgress(ctx, jobID, int(downloaded))
			}
		}); err != nil {
			slog.Error("osm import: download failed", "region", reg.ID, "error", err)
			h.DB.CompleteBackgroundJob(ctx, jobID, true)
			return
		}

		// === Phase 2: TRUNCATE + osm2pgsql ===
		h.DB.UpdateBackgroundJobPhase(ctx, jobID, "import")
		h.DB.UpdateBackgroundJobProgress(ctx, jobID, 0)

		// Gewählte Kategorien truncaten
		for _, cat := range categories {
			tables := osmCategories[cat]
			for _, tbl := range tables {
				if _, err := h.DB.Pool.Exec(ctx, "TRUNCATE TABLE "+tbl); err != nil {
					slog.Warn("osm import: truncate failed", "table", tbl, "error", err)
				}
			}
		}

		// Env-Vars für Lua-Script zusammenstellen
		envVars := os.Environ()
		for _, cat := range categories {
			if envVar, ok := categoryEnvVars[cat]; ok {
				envVars = append(envVars, envVar+"=1")
			}
		}

		cmd := exec.CommandContext(ctx, "osm2pgsql",
			"--database", dbURL,
			"--output=flex",
			"--style", luaTmp.Name(),
			"--slim", "--drop",
			pbfPath,
		)
		cmd.Env = envVars

		// stderr parsen für Node-Zähler (Fortschrittsanzeige)
		stderr, _ := cmd.StderrPipe()
		if err := cmd.Start(); err != nil {
			slog.Error("osm import: osm2pgsql start failed", "error", err)
			h.DB.CompleteBackgroundJob(ctx, jobID, true)
			return
		}

		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				slog.Info("osm2pgsql", "line", line)
				if m := osmProgressRe.FindStringSubmatch(line); len(m) > 1 {
					if n, err := strconv.Atoi(m[1]); err == nil {
						h.DB.UpdateBackgroundJobProgress(ctx, jobID, n*1000)
					}
				}
			}
		}()

		if err := cmd.Wait(); err != nil {
			slog.Error("osm import: osm2pgsql failed", "region", reg.ID, "error", err)
			h.DB.CompleteBackgroundJob(ctx, jobID, true)
			return
		}

		// PBF-Datei löschen um Speicher zu sparen
		os.Remove(pbfPath)
	}

	// === Phase 3: Indizieren ===
	h.DB.UpdateBackgroundJobPhase(ctx, jobID, "index")

	indexSteps := buildIndexStatements(categories)
	for i, stmt := range indexSteps {
		h.DB.UpdateBackgroundJobProgress(ctx, jobID, i+1)
		if _, err := h.DB.Pool.Exec(ctx, stmt); err != nil {
			slog.Warn("osm import: index step failed", "stmt", stmt, "error", err)
		}
	}

	h.DB.UpdateBackgroundJobProgress(ctx, jobID, len(indexSteps))
	h.DB.CompleteBackgroundJob(ctx, jobID, false)
	slog.Info("osm import: completed", "job_id", jobID)
}

// downloadWithProgress lädt eine Datei herunter und ruft progress(downloaded, total) auf.
func downloadWithProgress(ctx context.Context, url, destPath string, progress func(downloaded, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			downloaded += int64(n)
			progress(downloaded, total)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// buildIndexStatements gibt CREATE INDEX IF NOT EXISTS + ANALYZE Statements zurück.
func buildIndexStatements(categories []string) []string {
	catSet := make(map[string]bool)
	for _, c := range categories {
		catSet[c] = true
	}

	type indexDef struct {
		cat   string
		name  string
		table string
		extra string
	}

	allIndexes := []indexDef{
		{"peaks", "osm_peaks_geog_idx", "osm_peaks", "USING gist ((geom::geography))"},
		{"passes", "osm_passes_geog_idx", "osm_passes", "USING gist ((geom::geography))"},
		{"lifts", "osm_lifts_geog_idx", "osm_lifts", "USING gist ((geom::geography))"},
		{"settlements", "osm_settlements_geog_idx", "osm_settlements", "USING gist ((geom::geography))"},
		{"amenities", "osm_amenities_geog_idx", "osm_amenities", "USING gist ((geom::geography))"},
		{"waterways", "osm_waterways_geog_idx", "osm_waterways", "USING gist ((geom::geography))"},
		{"waterways", "osm_waters_geog_idx", "osm_waters", "USING gist ((geom::geography))"},
		{"forests", "osm_forests_geog_idx", "osm_forests", "USING gist (geom)"},
		{"boundaries", "osm_boundaries_geog_idx", "osm_boundaries", "USING gist ((geom::geography))"},
	}

	var stmts []string
	analyzedTables := make(map[string]bool)

	for _, idx := range allIndexes {
		if !catSet[idx.cat] {
			continue
		}
		stmts = append(stmts, fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s %s",
			idx.name, idx.table, idx.extra,
		))
		if !analyzedTables[idx.table] {
			stmts = append(stmts, "ANALYZE "+idx.table)
			analyzedTables[idx.table] = true
		}
	}
	return stmts
}
