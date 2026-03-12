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

// Kategorie-Definitionen: ID → betroffene Tabellen.
// Kategorien die in osm_pois landen, werden mit einem DELETE-Filter gelöscht (kein TRUNCATE).
var osmCategories = map[string][]string{
	"peaks":          {"osm_pois"},
	"passes":         {"osm_pois"},
	"settlements":    {"osm_pois"},
	"gastronomy":     {"osm_pois"},
	"accommodation":  {"osm_pois"},
	"infrastructure": {"osm_pois"},
	"transport":      {"osm_pois"},
	"viewpoints":     {"osm_pois"},
	"shops":          {"osm_pois"},
	"sport":          {"osm_pois"},
	"camping":        {"osm_pois"},
	"information":    {"osm_pois"},
	"landmarks":      {"osm_pois"},
	"religion":       {"osm_pois"},
	"ways":           {"osm_ways"},
	"waterways":      {"osm_waterways", "osm_waters"},
	"forests":        {"osm_forests", "osm_glaciers"},
	"wetlands":       {"osm_wetlands"},
	"boundaries":     {"osm_boundaries"},
	"pistes":         {"osm_pistes"},
	"lifts":          {"osm_lifts"},
	"ferries":        {}, // shares osm_ways — cleared via DELETE WHERE highway='ferry'
	"climbing":       {}, // shares osm_ways — cleared via DELETE WHERE highway='via_ferrata'
	"routes":         {"osm_routes"},
}

// categoryToPOI mappt Import-Kategorie → osm_pois.category Werte
var categoryToPOI = map[string][]string{
	"peaks":          {"peak"},
	"passes":         {"pass"},
	"settlements":    {"settlement"},
	"gastronomy":     {"gastronomy"},
	"accommodation":  {"accommodation", "shelter"},
	"infrastructure": {"water_supply", "infrastructure", "parking", "barrier"},
	"transport":      {"transport"},
	"viewpoints":     {"viewpoint"},
	"shops":          {"shop"},
	"sport":          {"sport"},
	"camping":        {"camping"},
	"information":    {"information"},
	"landmarks":      {"landmark"},
	"religion":       {"religious"},
}

var categoryToTable = map[string][]string{
	// "ways" intentionally omitted — uses targeted DELETE to preserve ferry/via_ferrata rows
	"waterways":  {"osm_waterways", "osm_waters"},
	"forests":    {"osm_forests", "osm_glaciers"},
	"wetlands":   {"osm_wetlands"},
	"boundaries": {"osm_boundaries"},
	"pistes":     {"osm_pistes"},
	"lifts":      {"osm_lifts"},
	"routes":     {"osm_routes"},
	// "ferries" intentionally omitted — uses targeted DELETE, not TRUNCATE
}

// Env-Vars für Lua-Script (eine Kategorie kann mehrere Lua-Flags setzen)
var categoryEnvVars = map[string][]string{
	"peaks":          {"SKADIA_IMPORT_PEAKS"},
	"passes":         {"SKADIA_IMPORT_PASSES"},
	"lifts":          {"SKADIA_IMPORT_LIFTS"},
	"settlements":    {"SKADIA_IMPORT_SETTLEMENTS"},
	"gastronomy":     {"SKADIA_IMPORT_GASTRONOMY"},
	"accommodation":  {"SKADIA_IMPORT_ACCOMMODATION"},
	"infrastructure": {"SKADIA_IMPORT_INFRASTRUCTURE"},
	"transport":      {"SKADIA_IMPORT_TRANSPORT"},
	"viewpoints":     {"SKADIA_IMPORT_VIEWPOINTS"},
	"shops":          {"SKADIA_IMPORT_SHOPS"},
	"sport":          {"SKADIA_IMPORT_SPORT"},
	"camping":        {"SKADIA_IMPORT_CAMPING"},
	"information":    {"SKADIA_IMPORT_INFORMATION"},
	"landmarks":      {"SKADIA_IMPORT_LANDMARKS"},
	"religion":       {"SKADIA_IMPORT_RELIGION"},
	"ways":       {"SKADIA_IMPORT_WAYS"},
	"waterways":  {"SKADIA_IMPORT_WATERWAYS"},
	// forests importiert auch Gletscher (gleiche Flächen-Logik)
	"forests":    {"SKADIA_IMPORT_FORESTS", "SKADIA_IMPORT_GLACIERS"},
	"wetlands":   {"SKADIA_IMPORT_WETLANDS"},
	"boundaries": {"SKADIA_IMPORT_BOUNDARIES"},
	"pistes":     {"SKADIA_IMPORT_PISTES"},
	"ferries":   {"SKADIA_IMPORT_FERRIES"},
	"climbing":  {"SKADIA_IMPORT_CLIMBING"},
	"routes":    {"SKADIA_IMPORT_ROUTES"},
}

type osmImportRequest struct {
	Regions       []string `json:"regions"`
	Categories    []string `json:"categories"`
	BuildBRouter  bool     `json:"build_brouter"`
	BuildTopology bool     `json:"build_topology"`
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

	// Initialer progress_total = geschätzte Gesamtgröße in Bytes (int64, kann >2 GB sein)
	progressTotal := int64(totalSizeMB) * 1024 * 1024

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "osm_import", nil, &entityName, progressTotal)
	if err != nil {
		slog.Error("failed to insert osm import job", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}

	go h.runOSMImport(jobID, userID, validRegions, req)

	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

var osmProgressRe = regexp.MustCompile(`Node\((\d+)k`)

// clearOSMData löscht die Daten der gewählten Kategorien:
// osm_pois wird per DELETE mit category-Filter geleert, andere Tabellen per TRUNCATE.
func (h *Handler) clearOSMData(ctx context.Context, categories []string) error {
	var poiCategories []string
	var tablesToTruncate []string

	for _, cat := range categories {
		if pois, ok := categoryToPOI[cat]; ok {
			poiCategories = append(poiCategories, pois...)
		}
		if tables, ok := categoryToTable[cat]; ok {
			tablesToTruncate = append(tablesToTruncate, tables...)
		}
	}

	if len(poiCategories) > 0 {
		_, err := h.DB.Pool.Exec(ctx,
			"DELETE FROM osm_pois WHERE category = ANY($1)", poiCategories)
		if err != nil {
			return err
		}
	}
	for _, table := range tablesToTruncate {
		_, err := h.DB.Pool.Exec(ctx, "TRUNCATE "+table+" CASCADE")
		if err != nil {
			return err
		}
	}
	// osm_ways wird nie per TRUNCATE geleert — gezieltes DELETE schützt ferry/via_ferrata-Rows
	// die ggf. als separate Kategorie importiert wurden.
	for _, cat := range categories {
		var sql string
		switch cat {
		case "ways":
			// Löscht alles außer Ferry/Climbing (die über eigene Kategorien verwaltet werden)
			sql = "DELETE FROM osm_ways WHERE highway NOT IN ('ferry', 'via_ferrata')"
		case "ferries":
			sql = "DELETE FROM osm_ways WHERE highway = 'ferry'"
		case "climbing":
			sql = "DELETE FROM osm_ways WHERE highway = 'via_ferrata'"
		}
		if sql != "" {
			if _, err := h.DB.Pool.Exec(ctx, sql); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) runOSMImport(jobID, userID string, regions []osmregions.Region, req osmImportRequest) {
	ctx := context.Background()
	categories := req.Categories
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
				h.DB.UpdateBackgroundJobProgress(ctx, jobID, downloaded)
			}
		}); err != nil {
			slog.Error("osm import: download failed", "region", reg.ID, "error", err)
			h.DB.CompleteBackgroundJob(ctx, jobID, true)
			return
		}

		// === Phase 2: Daten löschen + osm2pgsql ===
		h.DB.UpdateBackgroundJobPhase(ctx, jobID, "import")
		h.DB.UpdateBackgroundJobProgress(ctx, jobID, 0)

		// Gewählte Kategorien leeren
		if err := h.clearOSMData(ctx, categories); err != nil {
			slog.Error("osm import: clearOSMData failed", "error", err)
			h.DB.CompleteBackgroundJob(ctx, jobID, true)
			return
		}

		// Env-Vars für Lua-Script zusammenstellen
		envVars := os.Environ()
		for _, cat := range categories {
			if envVarList, ok := categoryEnvVars[cat]; ok {
				for _, envVar := range envVarList {
					envVars = append(envVars, envVar+"=1")
				}
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
						h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(n)*1000)
					}
				}
			}
		}()

		if err := cmd.Wait(); err != nil {
			slog.Error("osm import: osm2pgsql failed", "region", reg.ID, "error", err)
			h.DB.CompleteBackgroundJob(ctx, jobID, true)
			return
		}

		// PBF für BRouter-Build aufbewahren wenn angefordert
		if req.BuildBRouter {
			// PBF bleibt für scripts/brouter-build-segments.sh erhalten
			slog.Info("osm import: PBF preserved for BRouter build", "path", pbfPath)
		} else {
			os.Remove(pbfPath)
		}
	}

	// === Phase 3: Indizieren ===
	h.DB.UpdateBackgroundJobPhase(ctx, jobID, "index")

	indexSteps := buildIndexStatements(categories)
	for i, stmt := range indexSteps {
		h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1))
		if _, err := h.DB.Pool.Exec(ctx, stmt); err != nil {
			slog.Warn("osm import: index step failed", "stmt", stmt, "error", err)
		}
	}
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(len(indexSteps)))

	// === Phase 4: Optimieren ===
	h.DB.UpdateBackgroundJobPhase(ctx, jobID, "optimize")

	optimizeQueries := []string{
		"UPDATE osm_pois SET geom = ST_QuantizeCoordinates(geom, 6)",
		"UPDATE osm_ways SET geom = ST_QuantizeCoordinates(geom, 6)",
		"UPDATE osm_lifts SET geom = ST_QuantizeCoordinates(geom, 6)",
		"UPDATE osm_waterways SET geom = ST_QuantizeCoordinates(geom, 6)",
		"UPDATE osm_pistes SET geom = ST_QuantizeCoordinates(geom, 6)",
		"UPDATE osm_forests SET geom = ST_QuantizeCoordinates(geom, 6)",
		"UPDATE osm_waters SET geom = ST_QuantizeCoordinates(geom, 6)",
		"UPDATE osm_glaciers SET geom = ST_QuantizeCoordinates(geom, 6)",
		"UPDATE osm_wetlands SET geom = ST_QuantizeCoordinates(geom, 6)",
		"UPDATE osm_routes SET geom = ST_QuantizeCoordinates(geom, 6)",
		"UPDATE osm_ways SET length_m = ST_Length(geom::geography) WHERE length_m IS NULL OR length_m = 0",
		"UPDATE osm_routes SET distance = ST_Length(geom::geography) / 1000.0 WHERE distance IS NULL",
		"VACUUM ANALYZE osm_pois",
		"VACUUM ANALYZE osm_ways",
		"VACUUM ANALYZE osm_routes",
	}
	for _, q := range optimizeQueries {
		preview := q
		if len(preview) > 50 {
			preview = preview[:50]
		}
		if _, err := h.DB.Pool.Exec(ctx, q); err != nil {
			slog.Warn("osm import: optimize step failed", "query", preview, "error", err)
		}
	}

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
		{"lifts", "osm_lifts_geog_idx", "osm_lifts", "USING gist ((geom::geography))"},
		{"ways", "osm_ways_geog_idx", "osm_ways", "USING gist ((geom::geography))"},
		{"waterways", "osm_waterways_geog_idx", "osm_waterways", "USING gist ((geom::geography))"},
		{"waterways", "osm_waters_geog_idx", "osm_waters", "USING gist ((geom::geography))"},
		{"forests", "osm_forests_geog_idx", "osm_forests", "USING gist (geom)"},
		{"wetlands", "osm_wetlands_geog_idx", "osm_wetlands", "USING gist ((geom::geography))"},
		{"forests", "osm_glaciers_geog_idx", "osm_glaciers", "USING gist ((geom::geography))"},
		{"boundaries", "osm_boundaries_geog_idx", "osm_boundaries", "USING gist ((geom::geography))"},
		{"pistes", "osm_pistes_geog_idx", "osm_pistes", "USING gist ((geom::geography))"},
		{"routes", "osm_routes_geog_idx", "osm_routes", "USING gist ((geom::geography))"},
	}

	var stmts []string
	analyzedTables := make(map[string]bool)

	// osm_pois: ANALYZE wenn mindestens eine POI-Kategorie importiert wurde
	poiCats := []string{
		"peaks", "passes", "settlements", "gastronomy", "accommodation",
		"infrastructure", "transport", "viewpoints", "shops", "sport",
		"camping", "information", "landmarks", "religion",
	}
	for _, pc := range poiCats {
		if catSet[pc] && !analyzedTables["osm_pois"] {
			stmts = append(stmts, "ANALYZE osm_pois")
			analyzedTables["osm_pois"] = true
			break
		}
	}

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
