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
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
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
	"forests":        {"osm_forests"},
	"glaciers":       {"osm_glaciers"},
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
	"forests":    {"osm_forests"},
	"glaciers":   {"osm_glaciers"},
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
	"forests":    {"SKADIA_IMPORT_FORESTS"},
	"glaciers":   {"SKADIA_IMPORT_GLACIERS"},
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

// osmJobCancels speichert CancelFuncs für laufende OSM-Import-Jobs.
var osmJobCancels sync.Map

// CancelOSMImport bricht einen laufenden OSM-Import ab.
// POST /api/v1/admin/osm-import/cancel/{jobID}
func (h *Handler) CancelOSMImport(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "missing job ID", r.URL.Path)
		return
	}
	_, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE background_jobs SET status = 'failed', error = 'Vom Benutzer abgebrochen',
		 finished_at = now() WHERE id = $1 AND status IN ('running', 'pending')`, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}
	if fn, ok := osmJobCancels.LoadAndDelete(jobID); ok {
		fn.(context.CancelFunc)()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
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

// osm2pgsql 2.x Fortschritts-Pattern: "Processing: Node(12345k 1500.0/s) Way(0k 0.00/s) Relation(0 0.00/s)"
var osmProgressNodeRe = regexp.MustCompile(`Node\((\d+)k`)
var osmProgressWayRe = regexp.MustCompile(`Way\((\d+)k`)
var osmProgressRelRe = regexp.MustCompile(`Relation\((\d+)\s`)

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

// failJob setzt error-Text und markiert den Job als fehlgeschlagen.
// Nutzt eigenen Context (nicht den Job-Context), damit das Update auch nach Cancel ankommt.
func (h *Handler) failJob(_ context.Context, jobID, errMsg string) {
	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.DB.Pool.Exec(dbCtx, `UPDATE background_jobs SET error = $1 WHERE id = $2`, errMsg, jobID)
	h.DB.CompleteBackgroundJob(dbCtx, jobID, true)
}

func (h *Handler) runOSMImport(jobID, userID string, regions []osmregions.Region, req osmImportRequest) {
	// Cancellable context: CancelOSMImport fires cancel() to abort osm2pgsql.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	osmJobCancels.Store(jobID, cancel)
	defer osmJobCancels.Delete(jobID)
	categories := req.Categories
	dataDir := os.Getenv("OSM_DATA_DIR")
	if dataDir == "" {
		dataDir = "/data/osm"
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		slog.Error("osm import: failed to create data dir", "error", err)
		h.failJob(ctx, jobID, "Datenverzeichnis konnte nicht erstellt werden: "+err.Error())
		return
	}

	// Lua-Script in Temp-Datei schreiben
	luaTmp, err := os.CreateTemp("", "skadia-osm-*.lua")
	if err != nil {
		slog.Error("osm import: failed to write lua tmp", "error", err)
		h.failJob(ctx, jobID, "Lua-Script konnte nicht geschrieben werden: "+err.Error())
		return
	}
	defer os.Remove(luaTmp.Name())
	if _, err := luaTmp.Write(osmregions.LuaScript); err != nil {
		slog.Error("osm import: failed to write lua content", "error", err)
		h.failJob(ctx, jobID, "Lua-Script konnte nicht geschrieben werden: "+err.Error())
		return
	}
	luaTmp.Close()

	dbURL := os.Getenv("DATABASE_URL")

	// === Phase 1: Alle PBFs herunterladen ===
	h.DB.UpdateBackgroundJobPhase(ctx, jobID, "download")
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 0)

	var pbfPaths []string
	var downloadedOffset int64
	const maxPBFAge = 7 * 24 * time.Hour

	for _, reg := range regions {
		pbfPath := filepath.Join(dataDir, reg.ID+"-latest.osm.pbf")

		// Download-Skip: PBF wiederverwenden wenn jünger als maxPBFAge
		skipDownload := false
		if info, statErr := os.Stat(pbfPath); statErr == nil {
			age := time.Since(info.ModTime())
			if age < maxPBFAge {
				slog.Info("osm import: reusing existing PBF",
					"region", reg.ID, "age", age.Round(time.Hour), "size", info.Size())
				skipDownload = true
				downloadedOffset += info.Size()
			}
		}

		if !skipDownload {
			// Disk-Space-Check: mindestens 8x PBF-Größe frei
			var stat syscall.Statfs_t
			if sysErr := syscall.Statfs(dataDir, &stat); sysErr == nil {
				freeBytes := stat.Bavail * uint64(stat.Bsize)
				requiredBytes := uint64(reg.SizeMB) * 1024 * 1024 * 8
				if freeBytes < requiredBytes {
					slog.Error("osm import: insufficient disk space",
						"free_mb", freeBytes/1024/1024,
						"required_mb", requiredBytes/1024/1024,
						"region", reg.ID)
					h.failJob(ctx, jobID, fmt.Sprintf(
						"Zu wenig Speicherplatz: %d MB frei, ~%d MB benötigt",
						freeBytes/1024/1024, requiredBytes/1024/1024))
					return
				}
			}

			slog.Info("osm import: downloading", "region", reg.ID, "url", reg.URL)
			regionOffset := downloadedOffset
			if err := downloadWithProgress(ctx, reg.URL, pbfPath, func(downloaded, total int64) {
				if total > 0 {
					h.DB.UpdateBackgroundJobProgress(ctx, jobID, regionOffset+downloaded)
				}
			}); err != nil {
				slog.Error("osm import: download failed", "region", reg.ID, "error", err)
				h.failJob(ctx, jobID, fmt.Sprintf("Download fehlgeschlagen (%s): %s", reg.Label, err.Error()))
				return
			}
			if info, err := os.Stat(pbfPath); err == nil {
				downloadedOffset += info.Size()
			}
		}

		pbfPaths = append(pbfPaths, pbfPath)
	}

	// === Phase 2: Daten löschen + EIN osm2pgsql-Aufruf mit allen PBFs ===
	h.DB.UpdateBackgroundJobPhase(ctx, jobID, "import")
	h.DB.UpdateBackgroundJobProgress(ctx, jobID, 0)

	// Gewählte Kategorien EINMAL leeren (vor dem einzigen osm2pgsql-Lauf)
	if err := h.clearOSMData(ctx, categories); err != nil {
		slog.Error("osm import: clearOSMData failed", "error", err)
		h.failJob(ctx, jobID, "Datenlöschung fehlgeschlagen: "+err.Error())
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

	// --flat-nodes: Speichert Node-Koordinaten in Datei statt in PostgreSQL (~100 GB).
	// Dateigröße richtet sich nach dem maximalen Node-ID-Wert (globaler OSM-Adressraum),
	// nicht nach der Datenmenge — für Europa-Extrakte typisch 50–100 GB.
	// Wird nach erfolgreichem Import automatisch gelöscht (s. os.Remove unten).
	flatNodesPath := filepath.Join(dataDir, "flat-nodes.bin")

	// osm2pgsql 2.x kann mehrere PBFs als Input — ein Lauf, ein flat-nodes.bin,
	// automatische Deduplizierung an Regionsgrenzen über OSM-Node-IDs.
	args := []string{
		"--database", dbURL,
		"--output=flex",
		"--style", luaTmp.Name(),
		"--flat-nodes", flatNodesPath,
		"--cache", "0", // Mit --flat-nodes kein RAM-Cache nötig; verhindert OOM-Kill
		"--slim", "--drop",
	}
	args = append(args, pbfPaths...)

	// Stale flat-nodes.bin entfernen — bei Retry nach Crash enthält die Datei
	// veraltete Node-Koordinaten, die zu Geometry-Korruption führen würden.
	os.Remove(flatNodesPath)

	cmd := exec.CommandContext(ctx, "osm2pgsql", args...)
	cmd.Env = envVars

	// stderr für Logging + Fortschritt
	stderr, _ := cmd.StderrPipe()
	importStart := time.Now()
	if err := cmd.Start(); err != nil {
		slog.Error("osm import: osm2pgsql start failed", "error", err)
		h.failJob(ctx, jobID, "osm2pgsql konnte nicht gestartet werden: "+err.Error())
		return
	}

	// Letzte stderr-Zeilen für Fehlerbericht sammeln
	var lastLines []string
	var lastLinesMu sync.Mutex

	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 256*1024), 256*1024) // 256 KB Buffer
		for scanner.Scan() {
			line := scanner.Text()
			slog.Info("osm2pgsql", "line", line)

			lastLinesMu.Lock()
			lastLines = append(lastLines, line)
			if len(lastLines) > 5 {
				lastLines = lastLines[len(lastLines)-5:]
			}
			lastLinesMu.Unlock()

			// Node-Fortschritt (erste Phase)
			if m := osmProgressNodeRe.FindStringSubmatch(line); len(m) > 1 {
				if n, err := strconv.Atoi(m[1]); err == nil {
					h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(n)*1000)
				}
			}
			// Way-Fortschritt (zweite Phase)
			if m := osmProgressWayRe.FindStringSubmatch(line); len(m) > 1 {
				if n, err := strconv.Atoi(m[1]); err == nil {
					h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(n)*1000)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			slog.Warn("osm2pgsql: scanner error", "error", err)
		}
	}()

	if err := cmd.Wait(); err != nil {
		lastLinesMu.Lock()
		errDetail := strings.Join(lastLines, "\n")
		lastLinesMu.Unlock()
		slog.Error("osm import: osm2pgsql failed", "error", err, "last_output", errDetail)
		h.failJob(ctx, jobID, fmt.Sprintf("osm2pgsql fehlgeschlagen: %s\n%s", err.Error(), errDetail))
		return
	}
	slog.Info("osm import: osm2pgsql completed", "regions", len(regions))

	durationS := int(time.Since(importStart).Seconds())

	// Alle Regionen in osm_import_regions eintragen
	for _, reg := range regions {
		pbfPath := filepath.Join(dataDir, reg.ID+"-latest.osm.pbf")
		pbfInfo, _ := os.Stat(pbfPath)
		var pbfSize int64
		if pbfInfo != nil {
			pbfSize = pbfInfo.Size()
		}
		_, regErr := h.DB.Pool.Exec(ctx, `
			INSERT INTO osm_import_regions (region_id, label, continent, pbf_size_bytes, categories, import_duration_s, status)
			VALUES ($1, $2, $3, $4, $5, $6, 'imported')
			ON CONFLICT (region_id) DO UPDATE SET
				imported_at = now(), pbf_size_bytes = $4,
				categories = (SELECT ARRAY(SELECT DISTINCT unnest(osm_import_regions.categories || $5::text[]))),
				import_duration_s = $6, status = 'imported'`,
			reg.ID, reg.Label, reg.Continent, pbfSize, categories, durationS)
		if regErr != nil {
			slog.Warn("osm import: failed to track region", "region", reg.ID, "error", regErr)
		}

		// PBF für BRouter-Build aufbewahren wenn angefordert
		if !req.BuildBRouter {
			os.Remove(pbfPath)
		}
	}

	// flat-nodes Datei aufräumen (wird nur während osm2pgsql benötigt)
	os.Remove(flatNodesPath)

	// === Phase 3: Indizieren (parallel mit Semaphore) ===
	// Max 4 gleichzeitige GIST-Builds — begrenzt Peak-RAM,
	// trotzdem ~2x schneller als sequentiell.
	h.DB.UpdateBackgroundJobPhase(ctx, jobID, "index")

	const maxParallelDBOps = 4
	indexGroups := buildParallelIndexGroups(categories)
	var completed int64
	for _, group := range indexGroups {
		sem := make(chan struct{}, maxParallelDBOps)
		var wg sync.WaitGroup
		for _, stmt := range group {
			// Context-Check vor Semaphore-Send verhindert Blockade nach Cancel
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				break
			}
			if ctx.Err() != nil {
				break
			}
			wg.Add(1)
			go func(s string) {
				defer wg.Done()
				defer func() { <-sem }()
				if _, err := h.DB.Pool.Exec(ctx, s); err != nil {
					slog.Warn("osm import: index step failed", "stmt", s, "error", err)
				}
			}(stmt)
		}
		wg.Wait()
		completed += int64(len(group))
		h.DB.UpdateBackgroundJobProgress(ctx, jobID, completed)
	}

	// === Phase 4: Optimieren (nur betroffene Tabellen, parallel) ===
	h.DB.UpdateBackgroundJobPhase(ctx, jobID, "optimize")

	catSet := make(map[string]bool, len(categories))
	for _, c := range categories {
		catSet[c] = true
	}

	// Quantize + Berechnungen — nur für tatsächlich importierte Kategorien
	type optQuery struct {
		cats  []string // mindestens eine muss aktiv sein
		query string
	}
	allOpts := []optQuery{
		{[]string{"peaks", "passes", "settlements", "gastronomy", "accommodation",
			"infrastructure", "transport", "viewpoints", "shops", "sport",
			"camping", "information", "landmarks", "religion"},
			"UPDATE osm_pois SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"ways", "ferries", "climbing"}, "UPDATE osm_ways SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"lifts"}, "UPDATE osm_lifts SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"waterways"}, "UPDATE osm_waterways SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"waterways"}, "UPDATE osm_waters SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"pistes"}, "UPDATE osm_pistes SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"forests"}, "UPDATE osm_forests SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"glaciers"}, "UPDATE osm_glaciers SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"wetlands"}, "UPDATE osm_wetlands SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"routes"}, "UPDATE osm_routes SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"ways", "ferries", "climbing"}, "UPDATE osm_ways SET length_m = ST_Length(geom::geography) WHERE length_m IS NULL OR length_m = 0"},
		{[]string{"routes"}, "UPDATE osm_routes SET distance = ST_Length(geom::geography) / 1000.0 WHERE distance IS NULL"},
	}

	// Parallele UPDATEs (verschiedene Tabellen = kein Lock-Konflikt)
	var optimizeStmts []string
	for _, opt := range allOpts {
		for _, c := range opt.cats {
			if catSet[c] {
				optimizeStmts = append(optimizeStmts, opt.query)
				break
			}
		}
	}

	{
		sem := make(chan struct{}, maxParallelDBOps)
		var wg sync.WaitGroup
		for _, q := range optimizeStmts {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				break
			}
			if ctx.Err() != nil {
				break
			}
			wg.Add(1)
			go func(query string) {
				defer wg.Done()
				defer func() { <-sem }()
				if _, err := h.DB.Pool.Exec(ctx, query); err != nil {
					preview := query
					if len(preview) > 60 {
						preview = preview[:60]
					}
					slog.Warn("osm import: optimize step failed", "query", preview, "error", err)
				}
			}(q)
		}
		wg.Wait()
	}

	// VACUUM ANALYZE sequentiell (kann nicht parallel auf gleicher Tabelle laufen),
	// nur für betroffene Tabellen
	vacuumTables := buildVacuumTables(categories)
	for _, tbl := range vacuumTables {
		if _, err := h.DB.Pool.Exec(ctx, "VACUUM ANALYZE "+tbl); err != nil {
			slog.Warn("osm import: vacuum failed", "table", tbl, "error", err)
		}
	}

	// === Phase 5: Trail-Topologie (optional) ===
	if req.BuildTopology {
		waysImported := false
		for _, cat := range categories {
			if cat == "ways" {
				waysImported = true
				break
			}
		}
		if waysImported {
			h.DB.UpdateBackgroundJobPhase(ctx, jobID, "topology")
			h.DB.UpdateBackgroundJobProgress(ctx, jobID, 0)
			slog.Info("osm import: starting trail topology build")

			// Phase 5a: pgr_createTopology — WICHTIG: 'id' ist die PK-Spalte, NICHT 'way_id'
			h.DB.UpdateBackgroundJobProgress(ctx, jobID, 1)
			_, topoErr := h.DB.Pool.Exec(ctx, `
				SELECT pgr_createTopology('osm_ways', 0.00001, 'geom', 'id', 'source', 'target')
			`)
			if topoErr != nil {
				slog.Error("osm import: topology build failed", "error", topoErr)
				// Nicht fatal — Import war erfolgreich, nur Topologie fehlgeschlagen
			} else {
				// Phase 5b: cost/reverse_cost berechnen (exakt wie trail_topology.go)
				h.DB.UpdateBackgroundJobProgress(ctx, jobID, 2)
				_, costErr := h.DB.Pool.Exec(ctx, `
					UPDATE osm_ways SET
						cost = CASE
							WHEN is_lift THEN 0.001
							WHEN oneway  THEN -1
							ELSE length_m / NULLIF(tobler_speed(COALESCE(avg_slope, 0)) * 1000.0 / 60.0, 0)
							     * sac_cost_factor(sac_scale)
							     * surface_cost_factor(surface)
						END,
						reverse_cost = CASE
							WHEN is_lift THEN 60.0
							WHEN oneway  THEN -1
							ELSE length_m / NULLIF(tobler_speed(-COALESCE(avg_slope, 0)) * 1000.0 / 60.0, 0)
							     * sac_cost_factor(sac_scale)
							     * surface_cost_factor(surface)
						END
					WHERE cost IS NULL OR reverse_cost IS NULL
				`)
				if costErr != nil {
					slog.Warn("osm import: cost computation failed", "error", costErr)
				}

				// Phase 5c: Lifte aus osm_lifts in osm_ways importieren
				h.DB.UpdateBackgroundJobProgress(ctx, jobID, 3)
				_, liftErr := h.DB.Pool.Exec(ctx, `
					INSERT INTO osm_ways (osm_id, highway, name, length_m, avg_slope, is_lift, geom)
					SELECT l.osm_id, 'lift', l.name, ST_Length(l.geom::geography), 0.0, TRUE, l.geom
					FROM osm_lifts l
					WHERE NOT EXISTS (
						SELECT 1 FROM osm_ways t
						WHERE t.osm_id = l.osm_id AND t.is_lift = TRUE
					)
				`)
				if liftErr != nil {
					slog.Warn("osm import: lift merge failed", "error", liftErr)
				}

				// Phase 5d: Re-Topologie für Lifte (source/target für neue Rows)
				h.DB.UpdateBackgroundJobProgress(ctx, jobID, 4)
				_, retopoErr := h.DB.Pool.Exec(ctx, `
					SELECT pgr_createTopology('osm_ways', 0.00001, 'geom', 'id', 'source', 'target')
				`)
				if retopoErr != nil {
					slog.Warn("osm import: lift topology rebuild failed (non-fatal)", "error", retopoErr)
				}

				slog.Info("osm import: trail topology build completed")
			}
		} else {
			slog.Info("osm import: skipping topology build — 'ways' category not imported")
		}
	}

	// === Phase 6: Spatial Enrichment (on osm_ways, BEFORE splitting) ===
	// Runs independently of Phase 5 — enriches the original osm_ways table.
	{
		waysImported := false
		for _, cat := range categories {
			if cat == "ways" {
				waysImported = true
				break
			}
		}
		if waysImported {
			h.DB.UpdateBackgroundJobPhase(ctx, jobID, "enrichment")
			h.DB.UpdateBackgroundJobProgress(ctx, jobID, 0)
			slog.Info("osm import: starting spatial enrichment")

			if err := osmregions.RunSpatialEnrichment(ctx, h.DB.Pool, func(pct int) {
				h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(pct))
			}); err != nil {
				slog.Error("osm import: spatial enrichment failed", "error", err)
				// Non-fatal: enrichment failure should not block the import
			}

			// === Phase 7: Intersection Splitting (creates osm_ways_noded) ===
			h.DB.UpdateBackgroundJobPhase(ctx, jobID, "splitting")
			h.DB.UpdateBackgroundJobProgress(ctx, jobID, 0)
			slog.Info("osm import: starting intersection splitting")

			if err := osmregions.SplitAtIntersections(ctx, h.DB.Pool, func(pct int) {
				h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(pct))
			}); err != nil {
				slog.Error("osm import: intersection splitting failed", "error", err)
				// Non-fatal: splitting failure should not block the import
			}
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
	var lastProgressAt time.Time
	buf := make([]byte, 256*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			downloaded += int64(n)
			if time.Since(lastProgressAt) >= 2*time.Second {
				progress(downloaded, total)
				lastProgressAt = time.Now()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	progress(downloaded, total) // Final update
	return nil
}

// buildParallelIndexGroups gibt Gruppen von Statements zurück, die parallel ausgeführt werden können.
// Gruppe 1: CREATE INDEX (auf verschiedenen Tabellen → parallelisierbar)
// Gruppe 2: ANALYZE (nach allen Indexes)
func buildParallelIndexGroups(categories []string) [][]string {
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
		{"glaciers", "osm_glaciers_geog_idx", "osm_glaciers", "USING gist ((geom::geography))"},
		{"boundaries", "osm_boundaries_geog_idx", "osm_boundaries", "USING gist ((geom::geography))"},
		{"pistes", "osm_pistes_geog_idx", "osm_pistes", "USING gist ((geom::geography))"},
		{"routes", "osm_routes_geog_idx", "osm_routes", "USING gist ((geom::geography))"},
	}

	var indexStmts []string
	analyzeTables := make(map[string]bool)

	// osm_pois: ANALYZE wenn mindestens eine POI-Kategorie importiert wurde
	poiCats := []string{
		"peaks", "passes", "settlements", "gastronomy", "accommodation",
		"infrastructure", "transport", "viewpoints", "shops", "sport",
		"camping", "information", "landmarks", "religion",
	}
	for _, pc := range poiCats {
		if catSet[pc] {
			analyzeTables["osm_pois"] = true
			break
		}
	}

	for _, idx := range allIndexes {
		if !catSet[idx.cat] {
			continue
		}
		indexStmts = append(indexStmts, fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s %s",
			idx.name, idx.table, idx.extra,
		))
		analyzeTables[idx.table] = true
	}

	var analyzeStmts []string
	for table := range analyzeTables {
		analyzeStmts = append(analyzeStmts, "ANALYZE "+table)
	}

	var groups [][]string
	if len(indexStmts) > 0 {
		groups = append(groups, indexStmts)
	}
	if len(analyzeStmts) > 0 {
		groups = append(groups, analyzeStmts)
	}
	return groups
}

// buildVacuumTables gibt die Tabellennamen zurück, die nach dem Import ein VACUUM ANALYZE brauchen.
func buildVacuumTables(categories []string) []string {
	catSet := make(map[string]bool, len(categories))
	for _, c := range categories {
		catSet[c] = true
	}

	// Kategorie → betroffene Tabelle(n)
	catToTables := map[string][]string{
		"peaks": {"osm_pois"}, "passes": {"osm_pois"}, "settlements": {"osm_pois"},
		"gastronomy": {"osm_pois"}, "accommodation": {"osm_pois"}, "infrastructure": {"osm_pois"},
		"transport": {"osm_pois"}, "viewpoints": {"osm_pois"}, "shops": {"osm_pois"},
		"sport": {"osm_pois"}, "camping": {"osm_pois"}, "information": {"osm_pois"},
		"landmarks": {"osm_pois"}, "religion": {"osm_pois"},
		"ways":       {"osm_ways"},
		"ferries":    {"osm_ways"},
		"climbing":   {"osm_ways"},
		"lifts":      {"osm_lifts"},
		"waterways":  {"osm_waterways", "osm_waters"},
		"forests":    {"osm_forests"},
		"glaciers":   {"osm_glaciers"},
		"wetlands":   {"osm_wetlands"},
		"boundaries": {"osm_boundaries"},
		"pistes":     {"osm_pistes"},
		"routes":     {"osm_routes"},
	}

	seen := make(map[string]bool)
	var tables []string
	for _, cat := range categories {
		if !catSet[cat] {
			continue
		}
		for _, tbl := range catToTables[cat] {
			if !seen[tbl] {
				seen[tbl] = true
				tables = append(tables, tbl)
			}
		}
	}
	return tables
}
