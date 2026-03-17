package osm

import (
	"bufio"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pipeline orchestrates OSM data import.
type Pipeline struct {
	Pool    *pgxpool.Pool
	DataDir string
	DBURL   string
}

// PipelineConfig holds per-run import settings.
type PipelineConfig struct {
	Categories   []string
	BuildBRouter bool
}

// ProgressReporter abstracts job tracking from pipeline logic.
type ProgressReporter interface {
	SetPhase(phase string)
	SetProgress(current int64)
	Fail(errMsg string)
	Complete()
}

// osm2pgsql 2.x Fortschritts-Pattern: "Processing: Node(12345k 1500.0/s) Way(0k 0.00/s) Relation(0 0.00/s)"
var osmProgressNodeRe = regexp.MustCompile(`Node\((\d+)k`)
var osmProgressWayRe = regexp.MustCompile(`Way\((\d+)k`)
var osmProgressRelRe = regexp.MustCompile(`Relation\((\d+)\s`)

const maxParallelDBOps = 4

// categoryToPOI mappt Import-Kategorie → osm_pois.category Werte
var categoryToPOI = map[string][]string{
	"peaks":          {"peak"},
	"passes":         {"pass"},
	"settlements":    {"settlement"},
	"gastronomy":     {"gastronomy"},
	"accommodation":  {"accommodation", "shelter"},
	"infrastructure": {"water_supply", "infrastructure", "parking", "barrier"},
	"transport":      {"transport", "aerodrome"},
	"viewpoints":     {"viewpoint"},
	"shops":          {"shop"},
	"sport":          {"sport", "shooting_range"},
	"camping":        {"camping"},
	"information":    {"information"},
	"landmarks":      {"landmark"},
	"religion":       {"religious"},
	"wind_turbines":  {"wind_turbine"},
}

var categoryToTable = map[string][]string{
	// "ways" intentionally omitted — uses targeted DELETE to preserve ferry/via_ferrata rows
	"waterways":  {"osm_waterways", "osm_waters"},
	"forests":    {"osm_forests"},
	"glaciers":   {"osm_glaciers"},
	"wetlands":   {"osm_wetlands"},
	"boundaries": {"osm_boundaries"},
	"pistes":     {"osm_pistes", "osm_piste_areas"},
	"lifts":      {"osm_lifts"},
	"routes":     {"osm_routes"},
	"landuse":    {"osm_landuse"},
	"railways":   {"osm_railways"},
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
	"ways":           {"SKADIA_IMPORT_WAYS"},
	"waterways":      {"SKADIA_IMPORT_WATERWAYS"},
	"forests":        {"SKADIA_IMPORT_FORESTS"},
	"glaciers":       {"SKADIA_IMPORT_GLACIERS"},
	"wetlands":       {"SKADIA_IMPORT_WETLANDS"},
	"boundaries":     {"SKADIA_IMPORT_BOUNDARIES"},
	"pistes":         {"SKADIA_IMPORT_PISTES"},
	"ferries":        {"SKADIA_IMPORT_FERRIES"},
	"climbing":       {"SKADIA_IMPORT_CLIMBING"},
	"routes":         {"SKADIA_IMPORT_ROUTES"},
	"landuse":        {"SKADIA_IMPORT_LANDUSE"},
	"railways":       {"SKADIA_IMPORT_RAILWAYS"},
	"wind_turbines":  {"SKADIA_IMPORT_WIND_TURBINES"},
}

// Deprecated: Use RunPerRegion() instead. Run imports all regions at once without per-region tracking.
// Run executes the full OSM import pipeline. It reports progress via reporter and never returns an error —
// failures are reported via reporter.Fail().
func (p *Pipeline) Run(ctx context.Context, regions []Region, config PipelineConfig, reporter ProgressReporter) bool {
	categories := config.Categories

	if err := os.MkdirAll(p.DataDir, 0755); err != nil {
		slog.Error("osm import: failed to create data dir", "error", err)
		reporter.Fail("Datenverzeichnis konnte nicht erstellt werden: " + err.Error())
		return false
	}

	// Lua-Script in Temp-Datei schreiben
	luaTmp, err := os.CreateTemp("", "skadia-osm-*.lua")
	if err != nil {
		slog.Error("osm import: failed to write lua tmp", "error", err)
		reporter.Fail("Lua-Script konnte nicht geschrieben werden: " + err.Error())
		return false
	}
	defer os.Remove(luaTmp.Name())
	if _, err := luaTmp.Write(LuaScript); err != nil {
		slog.Error("osm import: failed to write lua content", "error", err)
		reporter.Fail("Lua-Script konnte nicht geschrieben werden: " + err.Error())
		return false
	}
	luaTmp.Close()

	// === Phase 1: Alle PBFs herunterladen ===
	reporter.SetPhase("download")
	reporter.SetProgress(0)

	var pbfPaths []string
	var downloadedOffset int64
	const maxPBFAge = 7 * 24 * time.Hour

	for _, reg := range regions {
		pbfPath := filepath.Join(p.DataDir, reg.ID+"-latest.osm.pbf")

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
			if sysErr := syscall.Statfs(p.DataDir, &stat); sysErr == nil {
				freeBytes := stat.Bavail * uint64(stat.Bsize)
				requiredBytes := uint64(reg.SizeMB) * 1024 * 1024 * 8
				if freeBytes < requiredBytes {
					slog.Error("osm import: insufficient disk space",
						"free_mb", freeBytes/1024/1024,
						"required_mb", requiredBytes/1024/1024,
						"region", reg.ID)
					reporter.Fail(fmt.Sprintf(
						"Zu wenig Speicherplatz: %d MB frei, ~%d MB benötigt",
						freeBytes/1024/1024, requiredBytes/1024/1024))
					return false
				}
			}

			slog.Info("osm import: downloading", "region", reg.ID, "url", reg.URL)
			regionOffset := downloadedOffset
			if err := downloadWithProgress(ctx, reg.URL, pbfPath, func(downloaded, total int64) {
				if total > 0 {
					reporter.SetProgress(regionOffset + downloaded)
				}
			}); err != nil {
				slog.Error("osm import: download failed", "region", reg.ID, "error", err)
				reporter.Fail(fmt.Sprintf("Download fehlgeschlagen (%s): %s", reg.Label, err.Error()))
				return false
			}
			if info, err := os.Stat(pbfPath); err == nil {
				downloadedOffset += info.Size()
			}
		}

		pbfPaths = append(pbfPaths, pbfPath)
	}

	// === Phase 1.5: osmium Preprocessing (filter + merge) ===
	// Reduziert PBF-Größe um ~40-55% durch Entfernung irrelevanter Tags (Buildings, Adressen, etc.)
	mergedPBF, osmErr := OsmiumPreprocess(ctx, p.DataDir, pbfPaths, reporter)
	if osmErr != nil {
		slog.Error("osm import: osmium preprocessing failed", "error", osmErr)
		reporter.Fail("osmium Preprocessing fehlgeschlagen: " + osmErr.Error())
		return false
	}
	if mergedPBF != "" {
		pbfPaths = []string{mergedPBF}
		defer CleanupMergedPBF(p.DataDir)
	}

	// === Phase 2: Daten löschen + EIN osm2pgsql-Aufruf mit allen PBFs ===
	reporter.SetPhase("import")
	reporter.SetProgress(0)

	// Alle Regionen als 'importing' markieren (Crash-Recovery: nächster Start erkennt unvollständige Imports)
	for _, reg := range regions {
		p.Pool.Exec(ctx, `
			INSERT INTO osm_import_regions (region_id, label, continent, categories, status)
			VALUES ($1, $2, $3, $4, 'importing')
			ON CONFLICT (region_id) DO UPDATE SET status = 'importing', imported_at = now()`,
			reg.ID, reg.Label, reg.Continent, categories)
	}

	// Gewählte Kategorien EINMAL leeren (vor dem einzigen osm2pgsql-Lauf)
	if err := p.clearOSMData(ctx, categories); err != nil {
		slog.Error("osm import: clearOSMData failed", "error", err)
		reporter.Fail("Datenlöschung fehlgeschlagen: " + err.Error())
		return false
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

	// PostgreSQL Import-Tuning (connection-level, not ALTER SYSTEM which affects all connections).
	// Note: osm2pgsql manages its own connections — these settings apply to post-import queries only.
	maintWorkMemMB := availableRAMMB() * 15 / 100
	if maintWorkMemMB > 8192 {
		maintWorkMemMB = 8192
	}
	if maintWorkMemMB < 256 {
		maintWorkMemMB = 256
	}

	importConn, err := p.Pool.Acquire(ctx)
	if err != nil {
		slog.Warn("osm import: failed to acquire tuning conn (non-fatal)", "error", err)
	} else {
		for _, stmt := range []string{
			fmt.Sprintf("SET maintenance_work_mem = '%dMB'", maintWorkMemMB),
			"SET max_parallel_maintenance_workers = 4",
		} {
			if _, stmtErr := importConn.Exec(ctx, stmt); stmtErr != nil {
				slog.Warn("osm import: PG tuning failed (non-fatal)", "stmt", stmt, "error", stmtErr)
			}
		}
		slog.Info("osm import: PG import tuning applied (connection-level)", "maintenance_work_mem_mb", maintWorkMemMB)
		defer importConn.Release()
	}

	// Dynamischer Cache: Verfügbaren RAM ermitteln und osm2pgsql --cache setzen.
	// --flat-nodes (100 GB Sparse-Datei) wird nur als Fallback verwendet wenn RAM nicht reicht.
	// Vorher: --flat-nodes + --cache 0 → WORST CASE für Performance (jeder Node-Lookup = Disk I/O).
	var totalPBFBytes int64
	for _, p := range pbfPaths {
		if info, err := os.Stat(p); err == nil {
			totalPBFBytes += info.Size()
		}
	}
	cacheMB := computeCacheMB(totalPBFBytes)

	args := []string{
		"--database", p.DBURL,
		"--output=flex",
		"--style", luaTmp.Name(),
		"--slim", "--drop",
	}

	flatNodesPath := filepath.Join(p.DataDir, "flat-nodes.bin")
	if cacheMB > 0 {
		args = append(args, "--cache", strconv.Itoa(cacheMB))
		slog.Info("osm import: using RAM cache", "cache_mb", cacheMB, "pbf_total_mb", totalPBFBytes/(1024*1024))
	} else {
		// Fallback: --flat-nodes wenn zu wenig RAM (< 512 MB frei)
		os.Remove(flatNodesPath) // Stale flat-nodes entfernen
		args = append(args, "--flat-nodes", flatNodesPath, "--cache", "0")
		slog.Warn("osm import: insufficient RAM, falling back to --flat-nodes",
			"pbf_total_mb", totalPBFBytes/(1024*1024))
	}

	args = append(args, pbfPaths...)

	cmd := exec.CommandContext(ctx, "osm2pgsql", args...)
	cmd.Env = envVars

	// stderr für Logging + Fortschritt
	stderr, _ := cmd.StderrPipe()
	importStart := time.Now()
	if err := cmd.Start(); err != nil {
		slog.Error("osm import: osm2pgsql start failed", "error", err)
		reporter.Fail("osm2pgsql konnte nicht gestartet werden: " + err.Error())
		return false
	}

	// Letzte stderr-Zeilen für Fehlerbericht sammeln
	var lastLines []string
	var lastLinesMu sync.Mutex

	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1 MB Buffer
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
					reporter.SetProgress(int64(n) * 1000)
				}
			}
			// Way-Fortschritt (zweite Phase)
			if m := osmProgressWayRe.FindStringSubmatch(line); len(m) > 1 {
				if n, err := strconv.Atoi(m[1]); err == nil {
					reporter.SetProgress(int64(n) * 1000)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			slog.Warn("osm2pgsql: scanner error, draining stderr to prevent deadlock", "error", err)
		}
		// Drain remaining stderr to prevent pipe deadlock — if the scanner
		// fails (token too long), osm2pgsql blocks on write(stderr) unless
		// someone keeps reading. This was the root cause of imports silently
		// hanging for hours without error or completion.
		io.Copy(io.Discard, stderr)
	}()

	if err := cmd.Wait(); err != nil {
		lastLinesMu.Lock()
		errDetail := strings.Join(lastLines, "\n")
		lastLinesMu.Unlock()
		slog.Error("osm import: osm2pgsql failed", "error", err, "last_output", errDetail)
		reporter.Fail(fmt.Sprintf("osm2pgsql fehlgeschlagen: %s\n%s", err.Error(), errDetail))
		return false
	}
	slog.Info("osm import: osm2pgsql completed", "regions", len(regions))

	durationS := int(time.Since(importStart).Seconds())

	// Alle Regionen in osm_import_regions eintragen
	for _, reg := range regions {
		pbfPath := filepath.Join(p.DataDir, reg.ID+"-latest.osm.pbf")
		pbfInfo, _ := os.Stat(pbfPath)
		var pbfSize int64
		if pbfInfo != nil {
			pbfSize = pbfInfo.Size()
		}
		_, regErr := p.Pool.Exec(ctx, `
			INSERT INTO osm_import_regions (region_id, label, continent, pbf_size_bytes, categories, import_duration_s, status)
			VALUES ($1, $2, $3, $4, $5, $6, 'imported')
			ON CONFLICT (region_id) DO UPDATE SET
				imported_at = now(), pbf_size_bytes = $4,
				categories = $5,
				import_duration_s = $6, status = 'imported'`,
			reg.ID, reg.Label, reg.Continent, pbfSize, categories, durationS)
		if regErr != nil {
			slog.Warn("osm import: failed to track region", "region", reg.ID, "error", regErr)
		}

		// PBF für BRouter-Build aufbewahren wenn angefordert
		if !config.BuildBRouter {
			os.Remove(pbfPath)
		}
	}

	// flat-nodes Datei aufräumen (nur vorhanden wenn Fallback-Modus aktiv war)
	os.Remove(flatNodesPath)

	// === Schema-Restore: osm2pgsql --create löscht alle Migrations-Spalten ===
	// osm_ways entstand als Umbenennung von osm_trails (000151). Nach jedem --create
	// rekonstruiert osm2pgsql nur das Lua-Schema — Routing- und Enrichment-Spalten fehlen.
	if slices.Contains(categories, "ways") || slices.Contains(categories, "ferries") || slices.Contains(categories, "climbing") {
		for _, stmt := range []string{
			// Routing-Spalten (ursprünglich aus 000129 osm_trails)
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS length_m DOUBLE PRECISION",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS avg_slope DOUBLE PRECISION",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS source BIGINT",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS target BIGINT",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS cost DOUBLE PRECISION DEFAULT -1",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS reverse_cost DOUBLE PRECISION DEFAULT -1",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS is_lift BOOLEAN DEFAULT FALSE",
			// Enrichment-Spalten (000157)
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS forest_pct SMALLINT",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS noise_score SMALLINT",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS traffic_class SMALLINT",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS protected_area SMALLINT",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS landuse_class SMALLINT",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS is_cycleroute BOOLEAN DEFAULT false",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS is_hikeroute BOOLEAN DEFAULT false",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS cycleroute_network TEXT",
			"ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS hikeroute_network TEXT",
		} {
			if _, err := p.Pool.Exec(ctx, stmt); err != nil {
				slog.Warn("osm import: schema restore failed", "error", err)
			}
		}
	}

	// === LZ4-Kompression auf geom + tags Spalten (nach --create gehen Migration-Settings verloren) ===
	lz4Tables := []struct{ table, col string }{
		{"osm_pois", "geom"},
		{"osm_pois", "tags"},
		{"osm_ways", "geom"},
		{"osm_ways", "tags"},
		{"osm_lifts", "geom"},
		// osm_lifts hat keine tags-Spalte
		{"osm_waterways", "geom"},
		// osm_waterways hat keine tags-Spalte
		{"osm_waters", "geom"},
		{"osm_pistes", "geom"},
		{"osm_pistes", "tags"},
		{"osm_forests", "geom"},
		{"osm_glaciers", "geom"},
		{"osm_wetlands", "geom"},
		{"osm_routes", "geom"},
		{"osm_routes", "tags"},
		{"osm_boundaries", "geom"},
		// osm_boundaries hat keine tags-Spalte
		{"osm_landuse", "geom"},
		{"osm_railways", "geom"},
		{"osm_piste_areas", "geom"},
		{"osm_piste_areas", "tags"},
	}
	for _, t := range lz4Tables {
		if _, err := p.Pool.Exec(ctx, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET COMPRESSION lz4", t.table, t.col)); err != nil {
			slog.Warn("osm import: lz4 compression failed", "table", t.table, "column", t.col, "error", err)
		}
	}

	// === Phase 3: Indizieren (parallel mit Semaphore) ===
	// Max 4 gleichzeitige GIST-Builds — begrenzt Peak-RAM,
	// trotzdem ~2x schneller als sequentiell.
	reporter.SetPhase("index")

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
			wg.Go(func() {
				defer func() { <-sem }()
				if _, err := p.Pool.Exec(ctx, stmt); err != nil {
					slog.Warn("osm import: index step failed", "stmt", stmt, "error", err)
				}
			})
		}
		wg.Wait()
		completed += int64(len(group))
		reporter.SetProgress(completed)
	}

	// === Phase 4: Optimieren (nur betroffene Tabellen, parallel) ===
	reporter.SetPhase("optimize")

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
			"UPDATE osm_pois SET geom = ST_MakeValid(ST_QuantizeCoordinates(ST_MakeValid(geom), 6))"},
		{[]string{"ways", "ferries", "climbing"}, "UPDATE osm_ways SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"lifts"}, "UPDATE osm_lifts SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"waterways"}, "UPDATE osm_waterways SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"waterways"}, "UPDATE osm_waters SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"pistes"}, "UPDATE osm_pistes SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"pistes"}, "UPDATE osm_piste_areas SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"landuse"}, "UPDATE osm_landuse SET geom = ST_QuantizeCoordinates(geom, 6)"},
		{[]string{"railways"}, "UPDATE osm_railways SET geom = ST_QuantizeCoordinates(geom, 6)"},
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
			wg.Go(func() {
				defer func() { <-sem }()
				if _, err := p.Pool.Exec(ctx, q); err != nil {
					preview := q
					if len(preview) > 60 {
						preview = preview[:60]
					}
					slog.Warn("osm import: optimize step failed", "query", preview, "error", err)
				}
			})
		}
		wg.Wait()
	}

	// VACUUM ANALYZE sequentiell (kann nicht parallel auf gleicher Tabelle laufen),
	// nur für betroffene Tabellen
	vacuumTables := buildVacuumTables(categories)
	for _, tbl := range vacuumTables {
		if _, err := p.Pool.Exec(ctx, "VACUUM ANALYZE "+tbl); err != nil {
			slog.Warn("osm import: vacuum failed", "table", tbl, "error", err)
		}
	}

	// Phase 5 (Trail-Topologie) removed: pgr_createTopology removed in pgRouting 4.0.1.
	// FITroute's CSR graph builder computes topology from osm_ways directly.

	// === Import abgeschlossen — Enrichment wird vom Handler als separater Job gestartet ===
	reporter.Complete()
	slog.Info("osm import: completed (enrichment starts separately)")
	return true
}

// RunEnrichmentAndSplitting führt Spatial Enrichment + Intersection Splitting aus.
// Kann als separater Background-Job nach dem Import laufen.
func (p *Pipeline) RunEnrichmentAndSplitting(ctx context.Context,
	regionFilter string, reporter ProgressReporter) {
	reporter.SetPhase("enrichment")
	reporter.SetProgress(0)
	slog.Info("osm enrichment: starting spatial enrichment")

	// Build subdivided polygon tables for faster spatial joins.
	// Must run AFTER all regions are imported (not per-region, otherwise DROP destroys prior regions).
	// Uses regular tables (not UNLOGGED) to survive PostgreSQL crash.
	slog.Info("osm enrichment: building subdivided polygon tables")
	// Each table may need extra columns carried forward (e.g., osm_landuse needs "landuse"
	// for WHERE filtering in EnrichLanduseFromPolygons point-sampling).
	subdivideSpecs := []struct{ table, extra string }{
		{"osm_forests", ""},
		{"osm_boundaries", ""},
		{"osm_landuse", ", landuse"},
	}
	for _, spec := range subdivideSpecs {
		subTable := spec.table + "_subdivided"
		stmts := []string{
			fmt.Sprintf("DROP TABLE IF EXISTS %s", subTable),
			fmt.Sprintf(`CREATE TABLE %s AS
				SELECT osm_id, source_region%s, (ST_Dump(ST_Subdivide(geom, 128))).geom AS geom
				FROM %s`, subTable, spec.extra, spec.table),
			fmt.Sprintf("CREATE INDEX ON %s USING GIST (geom)", subTable),
			fmt.Sprintf("ANALYZE %s", subTable),
		}
		for _, stmt := range stmts {
			if _, err := p.Pool.Exec(ctx, stmt); err != nil {
				slog.Warn("osm enrichment: subdivide failed (non-fatal)", "table", spec.table, "error", err)
				break
			}
		}
	}
	slog.Info("osm enrichment: subdivided polygon tables built")

	if err := RunSpatialEnrichment(ctx, p.Pool, regionFilter, func(pct int) {
		reporter.SetProgress(int64(pct))
	}); err != nil {
		slog.Error("osm enrichment: spatial enrichment failed", "error", err)
		reporter.Fail("Spatial Enrichment fehlgeschlagen: " + err.Error())
		return
	}

	// SplitAtIntersections removed: osm_ways_noded is no longer consumed.
	// The Go-side topology builder (routing/graph/topology.go: BuildTopology)
	// reads osm_ways directly and creates per-vertex-pair edges with
	// coordinate-hashing for intersection detection. pgr_createTopology
	// was already non-fatal (removed in pgRouting 4.0+).

	reporter.Complete()
	slog.Info("osm enrichment: completed")
}

// RunTopologyOnly was removed: pgr_createTopology was removed in pgRouting 4.0.1,
// and FITroute's CSR graph builder (routing/graph/topology.go) computes topology
// directly from osm_ways coordinates. No SQL-based topology needed.

// clearOSMData löscht die Daten der gewählten Kategorien:
// osm_pois wird per DELETE mit category-Filter geleert, andere Tabellen per TRUNCATE.
func (p *Pipeline) clearOSMData(ctx context.Context, categories []string) error {
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
		_, err := p.Pool.Exec(ctx,
			"DELETE FROM osm_pois WHERE category = ANY($1)", poiCategories)
		if err != nil {
			return err
		}
	}
	for _, table := range tablesToTruncate {
		_, err := p.Pool.Exec(ctx, "TRUNCATE "+table+" CASCADE")
		if err != nil {
			return err
		}
	}
	// osm_ways: Wenn alle Sub-Kategorien gewählt → TRUNCATE (schneller als 3 DELETEs)
	hasWays, hasFerries, hasClimbing := false, false, false
	for _, cat := range categories {
		switch cat {
		case "ways":
			hasWays = true
		case "ferries":
			hasFerries = true
		case "climbing":
			hasClimbing = true
		}
	}
	if hasWays && hasFerries && hasClimbing {
		if _, err := p.Pool.Exec(ctx, "TRUNCATE osm_ways CASCADE"); err != nil {
			return err
		}
	} else {
		// Gezieltes DELETE schützt ferry/via_ferrata-Rows die als separate Kategorie verwaltet werden
		for _, cat := range categories {
			var sql string
			switch cat {
			case "ways":
				sql = "DELETE FROM osm_ways WHERE highway NOT IN ('ferry', 'via_ferrata')"
			case "ferries":
				sql = "DELETE FROM osm_ways WHERE highway = 'ferry'"
			case "climbing":
				sql = "DELETE FROM osm_ways WHERE highway = 'via_ferrata'"
			}
			if sql != "" {
				if _, err := p.Pool.Exec(ctx, sql); err != nil {
					return err
				}
			}
		}
	}
	return nil
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
		{"landuse", "idx_osm_landuse_geom", "osm_landuse", "USING GIST (geom)"},
		{"landuse", "idx_osm_landuse_type", "osm_landuse", "(landuse)"},
		{"railways", "idx_osm_railways_geom", "osm_railways", "USING GIST (geom)"},
		{"pistes", "idx_osm_piste_areas_geom", "osm_piste_areas", "USING GIST (geom)"},
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
			// osm_pois: Partial GIST indexes + category B-Tree + trigram (lost in --create mode)
			poisIndexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_geom ON osm_pois USING GIST(geom)",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_peak ON osm_pois USING GIST(geom) WHERE category = 'peak'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_pass ON osm_pois USING GIST(geom) WHERE category = 'pass'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_settlement ON osm_pois USING GIST(geom) WHERE category = 'settlement'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_gastronomy ON osm_pois USING GIST(geom) WHERE category = 'gastronomy'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_accommodation ON osm_pois USING GIST(geom) WHERE category = 'accommodation'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_shelter ON osm_pois USING GIST(geom) WHERE category = 'shelter'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_water_supply ON osm_pois USING GIST(geom) WHERE category = 'water_supply'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_transport ON osm_pois USING GIST(geom) WHERE category = 'transport'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_parking ON osm_pois USING GIST(geom) WHERE category = 'parking'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_viewpoint ON osm_pois USING GIST(geom) WHERE category = 'viewpoint'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_shop ON osm_pois USING GIST(geom) WHERE category = 'shop'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_barrier ON osm_pois USING GIST(geom) WHERE category = 'barrier'",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_category ON osm_pois(category)",
				"CREATE INDEX IF NOT EXISTS idx_osm_pois_name_trgm ON osm_pois USING GIN(name gin_trgm_ops) WHERE name IS NOT NULL",
			}
			indexStmts = append(indexStmts, poisIndexes...)
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
		"pistes":     {"osm_pistes", "osm_piste_areas"},
		"routes":     {"osm_routes"},
		"landuse":    {"osm_landuse"},
		"railways":   {"osm_railways"},
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

// computeCacheMB calculates the optimal osm2pgsql --cache value in MB.
// Reads available RAM from /proc/meminfo (Linux). Returns 0 if RAM is insufficient
// (caller should fall back to --flat-nodes).
// Scales dynamically: uses 40% of available RAM, capped at 2x PBF size.
func computeCacheMB(totalPBFBytes int64) int {
	availMB := availableRAMMB()
	pbfMB := totalPBFBytes / (1024 * 1024)
	if pbfMB < 1 {
		pbfMB = 1
	}

	// Mindestens 1 GB RAM frei → Cache verwenden
	if availMB < 1024 {
		slog.Warn("osm import: insufficient RAM for cache, using --flat-nodes",
			"available_mb", availMB)
		return 0
	}

	// 40% of available RAM
	cacheMB := availMB * 40 / 100
	// Cap at 2x PBF size (more is wasteful) — but only if maxCache > 256
	if maxCache := pbfMB * 2; cacheMB > maxCache && maxCache > 256 {
		cacheMB = maxCache
	}
	// Floor: mindestens 256 MB
	if cacheMB < 256 {
		cacheMB = 256
	}

	slog.Info("osm import: cache calculation",
		"available_ram_mb", availMB, "pbf_total_mb", pbfMB, "cache_mb", cacheMB)
	return int(cacheMB)
}

// availableRAMMB reads MemAvailable from /proc/meminfo.
// Returns a conservative fallback (2048 MB) if detection fails.
func availableRAMMB() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 2048
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 2048
}

// sanitizeStagingSchema validates a staging schema name contains only safe characters.
// [I5] SQL injection defense — only alphanumeric + underscore allowed.
var validSchemaRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// querier abstracts pgxpool.Pool and pgx.Tx so that ClearRegionData / ImportRegionFromStaging
// can run either standalone or inside a transaction.
type querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// ClearRegionData removes all OSM data associated with a specific region.
// Junction tables (osm_way_regions, osm_route_regions) track ownership — orphaned
// ways/routes (no remaining region) are cleaned up afterward.
func (p *Pipeline) ClearRegionData(ctx context.Context, regionID string) error {
	return clearRegionData(ctx, p.Pool, regionID)
}

func clearRegionData(ctx context.Context, q querier, regionID string) error {
	slog.Info("osm import: clearing region data", "region", regionID)

	// Step 1: Collect osm_ids that will be orphaned (region-exclusive ways/routes).
	// Do this BEFORE deleting junction entries, so we know which IDs to target.
	// A way is region-exclusive if it belongs ONLY to this region (no other region owns it).
	// Using TEMP tables to avoid scanning the entire osm_ways/osm_routes tables later.
	_, _ = q.Exec(ctx, `CREATE TEMP TABLE IF NOT EXISTS _clear_way_ids ON COMMIT DROP AS
		SELECT wr.osm_id FROM osm_way_regions wr
		WHERE wr.region_id = $1
		  AND NOT EXISTS (
			SELECT 1 FROM osm_way_regions wr2
			WHERE wr2.osm_id = wr.osm_id AND wr2.region_id != $1
		  )`, regionID)
	_, _ = q.Exec(ctx, `CREATE TEMP TABLE IF NOT EXISTS _clear_route_ids ON COMMIT DROP AS
		SELECT rr.osm_id FROM osm_route_regions rr
		WHERE rr.region_id = $1
		  AND NOT EXISTS (
			SELECT 1 FROM osm_route_regions rr2
			WHERE rr2.osm_id = rr.osm_id AND rr2.region_id != $1
		  )`, regionID)

	// Step 2: Delete route_way_members for routes that will be orphaned.
	// Must happen BEFORE deleting the routes themselves, using the collected IDs.
	// PK index (route_osm_id, way_osm_id) makes this fast.
	_, _ = q.Exec(ctx, `DELETE FROM osm_route_way_members m
		WHERE m.route_osm_id IN (SELECT osm_id FROM _clear_route_ids)`)

	// Step 3: Junction-Tables: Region-Ownership entfernen
	for _, jt := range []string{"osm_way_regions", "osm_route_regions"} {
		if _, err := q.Exec(ctx,
			fmt.Sprintf("DELETE FROM %s WHERE region_id = $1", jt), regionID); err != nil {
			return fmt.Errorf("clear %s: %w", jt, err)
		}
	}

	// Step 4: Delete orphaned ways/routes using the pre-collected IDs (not full table scan).
	_, _ = q.Exec(ctx, `DELETE FROM osm_ways w WHERE w.osm_id IN (SELECT osm_id FROM _clear_way_ids)`)
	_, _ = q.Exec(ctx, `DELETE FROM osm_routes r WHERE r.osm_id IN (SELECT osm_id FROM _clear_route_ids)`)

	// Step 5: Alle Tabellen mit source_region
	for _, table := range []string{
		"osm_pois", "osm_forests", "osm_boundaries", "osm_landuse",
		"osm_railways", "osm_waterways", "osm_waters", "osm_glaciers",
		"osm_wetlands", "osm_pistes", "osm_piste_areas", "osm_lifts",
	} {
		if _, err := q.Exec(ctx,
			fmt.Sprintf("DELETE FROM %s WHERE source_region = $1", table), regionID); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}

	// Temp tables are dropped on commit (ON COMMIT DROP).
	slog.Info("osm import: region data cleared", "region", regionID)
	return nil
}

// ImportRegionFromStaging copies data from the staging schema into public tables.
// [C1] Column names are read dynamically from information_schema.columns (intersection
// of staging and public schema columns). Hardcoded column lists are forbidden.
// osm_ways/osm_routes: INSERT + junction table registration.
// All others: INSERT with source_region.
func (p *Pipeline) ImportRegionFromStaging(ctx context.Context, stagingSchema, regionID string) error {
	return importRegionFromStaging(ctx, p.Pool, stagingSchema, regionID)
}

func importRegionFromStaging(ctx context.Context, q querier, stagingSchema, regionID string) error {
	slog.Info("osm import: importing from staging", "region", regionID, "schema", stagingSchema)

	// [I5] Validate staging schema name
	if !validSchemaRe.MatchString(stagingSchema) {
		return fmt.Errorf("invalid staging schema name: %q", stagingSchema)
	}

	// getStagingColumns returns the intersection of columns in both staging and public schemas.
	// excludeCols allows excluding auto-generated ID columns that public generates via sequence.
	// Only osm_ways excludes way_id (router needs sequential IDs for version ordering).
	// All other tables copy their IDs from staging (deterministic OSM IDs, no routing involvement).
	getStagingColumns := func(table string, excludeCols ...string) ([]string, error) {
		rows, err := q.Query(ctx, `
			SELECT s.column_name
			FROM information_schema.columns s
			JOIN information_schema.columns p
			  ON p.table_schema = 'public' AND p.table_name = s.table_name AND p.column_name = s.column_name
			WHERE s.table_schema = $1 AND s.table_name = $2
			  AND ($3::text[] IS NULL OR s.column_name != ALL($3))
			ORDER BY s.ordinal_position`, stagingSchema, table, excludeCols)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var cols []string
		for rows.Next() {
			var col string
			if err := rows.Scan(&col); err != nil {
				return nil, err
			}
			cols = append(cols, col)
		}
		return cols, rows.Err()
	}

	// === osm_ways: dynamic columns, NO ON CONFLICT (duplicates allowed) ===
	// Lua can insert same osm_id multiple times (highway+ferry), so no UNIQUE.
	// Border duplicates (~0.1%) are acceptable. Graph builder uses DISTINCT ON (osm_id).
	wayCols, err := getStagingColumns("osm_ways", "way_id")
	if err != nil {
		return fmt.Errorf("get way columns: %w", err)
	}
	if len(wayCols) > 0 {
		colList := strings.Join(wayCols, ", ")

		_, err = q.Exec(ctx, fmt.Sprintf(`
			INSERT INTO public.osm_ways (%s, length_m)
			SELECT %s, ST_Length(geom::geography) FROM %s.osm_ways
		`, colList, colList, stagingSchema))
		if err != nil {
			return fmt.Errorf("import ways from staging: %w", err)
		}

		// Junction-Table: Region-Ownership registrieren
		_, err = q.Exec(ctx, fmt.Sprintf(`
			INSERT INTO public.osm_way_regions (osm_id, region_id)
			SELECT osm_id, $1 FROM %s.osm_ways
			ON CONFLICT DO NOTHING
		`, stagingSchema), regionID)
		if err != nil {
			return fmt.Errorf("import way_regions: %w", err)
		}
	}

	// === osm_routes: dynamic columns, NO ON CONFLICT ===
	routeCols, err := getStagingColumns("osm_routes")
	if err != nil {
		return fmt.Errorf("get route columns: %w", err)
	}
	if len(routeCols) > 0 {
		routeColList := strings.Join(routeCols, ", ")

		_, err = q.Exec(ctx, fmt.Sprintf(`
			INSERT INTO public.osm_routes (%s) SELECT %s FROM %s.osm_routes
		`, routeColList, routeColList, stagingSchema))
		if err != nil {
			return fmt.Errorf("import routes from staging: %w", err)
		}

		_, err = q.Exec(ctx, fmt.Sprintf(`
			INSERT INTO public.osm_route_regions (osm_id, region_id)
			SELECT osm_id, $1 FROM %s.osm_routes
			ON CONFLICT DO NOTHING
		`, stagingSchema), regionID)
		if err != nil {
			return fmt.Errorf("import route_regions: %w", err)
		}
	}

	// [C2] osm_route_way_members: clean stale memberships for this region's routes,
	// then re-insert current memberships from staging.
	// Routes were just cleared and re-inserted above, so staging contains the authoritative set.
	_, err = q.Exec(ctx, fmt.Sprintf(`
		DELETE FROM osm_route_way_members m
		WHERE m.route_osm_id IN (
			SELECT osm_id FROM %s.osm_routes
		)`, stagingSchema))
	if err != nil {
		slog.Warn("osm import: stale route_way_members cleanup failed", "error", err)
	}

	// Plain INSERT with DISTINCT — the C2-DELETE above already removed all
	// memberships for routes in this staging set, so no cross-region dedup needed.
	// DISTINCT guards against staging-internal duplicates (osm2pgsql can produce
	// duplicate relation members for forward+backward traversal of the same way).
	// DO NOT use ON CONFLICT — there is no unique constraint (by design: border
	// regions with different data dates produce legitimate duplicate tuples that
	// are resolved by "newest import wins" via the C2-DELETE-then-INSERT pattern).
	// DO NOT use WHERE NOT EXISTS — causes sequential scans on 50M+ row table (9h hang).
	_, err = q.Exec(ctx, fmt.Sprintf(`
		INSERT INTO public.osm_route_way_members (route_osm_id, way_osm_id, source)
		SELECT DISTINCT s.route_osm_id, s.way_osm_id, s.source FROM %s.osm_route_way_members s
	`, stagingSchema))
	if err != nil {
		slog.Warn("osm import: route_way_members staging insert failed", "error", err)
	}

	// === All other tables: dynamic columns + source_region ===
	otherTables := []string{
		"osm_pois", "osm_forests", "osm_boundaries", "osm_landuse",
		"osm_railways", "osm_waterways", "osm_waters", "osm_glaciers",
		"osm_wetlands", "osm_pistes", "osm_piste_areas", "osm_lifts",
	}
	for _, table := range otherTables {
		cols, err := getStagingColumns(table)
		if err != nil {
			slog.Warn("osm import: get columns failed", "table", table, "error", err)
			continue
		}
		if len(cols) == 0 {
			continue
		}
		cl := strings.Join(cols, ", ")
		_, err = q.Exec(ctx, fmt.Sprintf(
			"INSERT INTO public.%s (%s, source_region) SELECT %s, $1 FROM %s.%s",
			table, cl, cl, stagingSchema, table), regionID)
		if err != nil {
			slog.Warn("osm import: staging insert failed", "table", table, "error", err)
		}
	}

	slog.Info("osm import: staging import complete", "region", regionID)
	return nil
}

// RunPerRegion imports a single region via staging schema. This is the preferred
// import method — it supports per-region add/remove/update without affecting other regions.
//
// Concurrency: An advisory lock prevents concurrent imports of the SAME region.
// Parallel imports of DIFFERENT regions are allowed but may experience row-level
// lock waits on border ways (shared osm_ids). For sequential behavior, callers
// should serialize region imports (which runOSMImport already does).
//
// Flow:
//  1. Download PBF (with cache)
//  2. osmium tags-filter
//  3. Mark region as 'importing'
//  4. Clear old region data
//  5. Create staging schema + osm2pgsql --create --schema=staging
//  6. Import staging -> public
//  7. Drop staging schema
//  8. ANALYZE affected tables
//  9. Update osm_import_regions with replication info
func (p *Pipeline) RunPerRegion(ctx context.Context, reg Region, config PipelineConfig,
	reporter ProgressReporter) bool {

	regionID := reg.ID

	// Acquire advisory lock to prevent concurrent imports of the same region.
	// Uses FNV-1a hash of regionID for a stable int64 lock key.
	// IMPORTANT: The lock connection must be pinned (Acquire) because advisory locks
	// are session-level — if we used Pool.QueryRow, pgx would return the connection
	// to the pool immediately and the lock would be held on an arbitrary connection.
	h := fnv.New32a()
	h.Write([]byte(regionID))
	lockKey := int64(h.Sum32())

	lockConn, err := p.Pool.Acquire(ctx)
	if err != nil {
		reporter.Fail("Lock-Connection nicht verfügbar: " + err.Error())
		return false
	}
	defer lockConn.Release()

	var acquired bool
	if err := lockConn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired); err != nil {
		slog.Error("osm import: advisory lock query failed", "region", regionID, "error", err)
		reporter.Fail("Advisory Lock fehlgeschlagen: " + err.Error())
		return false
	}
	if !acquired {
		slog.Warn("osm import: region already being imported", "region", regionID)
		reporter.Fail("Region wird bereits importiert")
		return false
	}
	defer lockConn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)

	if err := os.MkdirAll(p.DataDir, 0755); err != nil {
		slog.Error("osm import: failed to create data dir", "error", err)
		reporter.Fail("Datenverzeichnis konnte nicht erstellt werden: " + err.Error())
		return false
	}

	// Lua-Script in Temp-Datei schreiben
	luaTmp, err := os.CreateTemp("", "skadia-osm-*.lua")
	if err != nil {
		slog.Error("osm import: failed to write lua tmp", "error", err)
		reporter.Fail("Lua-Script konnte nicht geschrieben werden: " + err.Error())
		return false
	}
	defer os.Remove(luaTmp.Name())
	if _, err := luaTmp.Write(LuaScript); err != nil {
		slog.Error("osm import: failed to write lua content", "error", err)
		reporter.Fail("Lua-Script konnte nicht geschrieben werden: " + err.Error())
		return false
	}
	luaTmp.Close()

	// === Phase 1: Download PBF ===
	reporter.SetPhase("download")
	reporter.SetProgress(0)

	pbfPath := filepath.Join(p.DataDir, reg.ID+"-latest.osm.pbf")
	const maxPBFAge = 7 * 24 * time.Hour

	skipDownload := false
	if info, statErr := os.Stat(pbfPath); statErr == nil {
		age := time.Since(info.ModTime())
		if age < maxPBFAge {
			slog.Info("osm import: reusing existing PBF",
				"region", reg.ID, "age", age.Round(time.Hour), "size", info.Size())
			skipDownload = true
		}
	}

	if !skipDownload {
		// Disk-Space-Check: mindestens 8x PBF-Größe frei
		var stat syscall.Statfs_t
		if sysErr := syscall.Statfs(p.DataDir, &stat); sysErr == nil {
			freeBytes := stat.Bavail * uint64(stat.Bsize)
			requiredBytes := uint64(reg.SizeMB) * 1024 * 1024 * 8
			if freeBytes < requiredBytes {
				slog.Error("osm import: insufficient disk space",
					"free_mb", freeBytes/1024/1024,
					"required_mb", requiredBytes/1024/1024,
					"region", reg.ID)
				reporter.Fail(fmt.Sprintf(
					"Zu wenig Speicherplatz: %d MB frei, ~%d MB benötigt",
					freeBytes/1024/1024, requiredBytes/1024/1024))
				return false
			}
		}

		slog.Info("osm import: downloading", "region", reg.ID, "url", reg.URL)
		if err := downloadWithProgress(ctx, reg.URL, pbfPath, func(downloaded, total int64) {
			if total > 0 {
				reporter.SetProgress(downloaded)
			}
		}); err != nil {
			slog.Error("osm import: download failed", "region", reg.ID, "error", err)
			reporter.Fail(fmt.Sprintf("Download fehlgeschlagen (%s): %s", reg.Label, err.Error()))
			return false
		}
	}

	// === Phase 1.5: osmium tags-filter ===
	filteredPBF, osmErr := OsmiumPreprocess(ctx, p.DataDir, []string{pbfPath}, reporter)
	if osmErr != nil {
		slog.Error("osm import: osmium preprocessing failed", "error", osmErr)
		reporter.Fail("osmium Preprocessing fehlgeschlagen: " + osmErr.Error())
		return false
	}
	importPBF := pbfPath
	if filteredPBF != "" {
		importPBF = filteredPBF
		defer CleanupMergedPBF(p.DataDir)
	}

	// Mark region as 'importing' (crash recovery: next start detects incomplete imports)
	_, _ = p.Pool.Exec(ctx, `
		INSERT INTO osm_import_regions (region_id, label, continent, categories, status)
		VALUES ($1, $2, $3, $4, 'importing')
		ON CONFLICT (region_id) DO UPDATE SET status = 'importing', imported_at = now()`,
		reg.ID, reg.Label, reg.Continent, config.Categories)

	// === Phase 2: Create staging schema + osm2pgsql import ===
	// NOTE: ClearRegionData is deferred to Phase 4 (atomic clear+merge in a single transaction)
	// to prevent data loss if the process crashes between clear and merge.
	// [C3] Unique schema name per region prevents race conditions
	stagingSchema := "staging_" + strings.ReplaceAll(regionID, "-", "_")
	// [I5] Validate schema name
	if !validSchemaRe.MatchString(stagingSchema) {
		reporter.Fail(fmt.Sprintf("Ungültiger Staging-Schema-Name: %q", stagingSchema))
		return false
	}

	reporter.SetPhase("import")
	reporter.SetProgress(0)

	if _, err := p.Pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+stagingSchema); err != nil {
		reporter.Fail("Staging-Schema konnte nicht erstellt werden: " + err.Error())
		return false
	}
	defer func() {
		_, _ = p.Pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+stagingSchema+" CASCADE")
	}()

	// Env-Vars für Lua-Script
	envVars := os.Environ()
	for _, cat := range config.Categories {
		if envVarList, ok := categoryEnvVars[cat]; ok {
			for _, envVar := range envVarList {
				envVars = append(envVars, envVar+"=1")
			}
		}
	}

	// PostgreSQL Import-Tuning (connection-level, not ALTER SYSTEM which affects all connections).
	maintWorkMemMB := availableRAMMB() * 15 / 100
	if maintWorkMemMB > 8192 {
		maintWorkMemMB = 8192
	}
	if maintWorkMemMB < 256 {
		maintWorkMemMB = 256
	}
	regionImportConn, err := p.Pool.Acquire(ctx)
	if err != nil {
		slog.Warn("osm import: failed to acquire tuning conn (non-fatal)", "error", err)
	} else {
		for _, stmt := range []string{
			fmt.Sprintf("SET maintenance_work_mem = '%dMB'", maintWorkMemMB),
			"SET max_parallel_maintenance_workers = 4",
		} {
			if _, stmtErr := regionImportConn.Exec(ctx, stmt); stmtErr != nil {
				slog.Warn("osm import: PG tuning failed (non-fatal)", "stmt", stmt, "error", stmtErr)
			}
		}
		defer regionImportConn.Release()
	}

	// Dynamic cache
	var pbfBytes int64
	if info, err := os.Stat(importPBF); err == nil {
		pbfBytes = info.Size()
	}
	cacheMB := computeCacheMB(pbfBytes)

	args := []string{
		"--database", p.DBURL,
		"--output=flex",
		"--style", luaTmp.Name(),
		"--schema=" + stagingSchema,
		"--middle-schema=" + stagingSchema,
		"--slim", "--drop",
	}

	flatNodesPath := filepath.Join(p.DataDir, "flat-nodes.bin")
	if cacheMB > 0 {
		args = append(args, "--cache", strconv.Itoa(cacheMB))
	} else {
		os.Remove(flatNodesPath)
		args = append(args, "--flat-nodes", flatNodesPath, "--cache", "0")
	}

	args = append(args, importPBF)

	cmd := exec.CommandContext(ctx, "osm2pgsql", args...)
	cmd.Env = envVars

	stderr, _ := cmd.StderrPipe()
	importStart := time.Now()
	if err := cmd.Start(); err != nil {
		slog.Error("osm import: osm2pgsql start failed", "error", err)
		reporter.Fail("osm2pgsql konnte nicht gestartet werden: " + err.Error())
		return false
	}

	var lastLines []string
	var lastLinesMu sync.Mutex

	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			slog.Info("osm2pgsql", "line", line)

			lastLinesMu.Lock()
			lastLines = append(lastLines, line)
			if len(lastLines) > 5 {
				lastLines = lastLines[len(lastLines)-5:]
			}
			lastLinesMu.Unlock()

			if m := osmProgressNodeRe.FindStringSubmatch(line); len(m) > 1 {
				if n, err := strconv.Atoi(m[1]); err == nil {
					reporter.SetProgress(int64(n) * 1000)
				}
			}
			if m := osmProgressWayRe.FindStringSubmatch(line); len(m) > 1 {
				if n, err := strconv.Atoi(m[1]); err == nil {
					reporter.SetProgress(int64(n) * 1000)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			slog.Warn("osm2pgsql: scanner error, draining stderr", "error", err)
		}
		io.Copy(io.Discard, stderr)
	}()

	if err := cmd.Wait(); err != nil {
		lastLinesMu.Lock()
		errDetail := strings.Join(lastLines, "\n")
		lastLinesMu.Unlock()
		slog.Error("osm import: osm2pgsql failed", "error", err, "last_output", errDetail)
		reporter.Fail(fmt.Sprintf("osm2pgsql fehlgeschlagen: %s\n%s", err.Error(), errDetail))
		return false
	}
	slog.Info("osm import: osm2pgsql completed", "region", regionID,
		"duration", time.Since(importStart).Round(time.Second))

	// flat-nodes aufräumen
	os.Remove(flatNodesPath)

	// [I1] Schema-Restore auf Staging ENTFÄLLT:
	// Enrichment-Spalten existieren bereits in public.osm_ways (Migration).
	// ImportRegionFromStaging kopiert nur Lua-definierte Spalten.

	// === Phase 3: Atomic Clear + Merge (single transaction) ===
	// Old data is cleared and new data is inserted in one transaction.
	// If anything fails, the transaction rolls back and old data remains intact.
	reporter.SetPhase("merge")
	reporter.SetProgress(0)

	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		slog.Error("osm import: begin transaction failed", "error", err)
		reporter.Fail("Transaktion konnte nicht gestartet werden: " + err.Error())
		return false
	}
	defer tx.Rollback(context.Background())

	if err := clearRegionData(ctx, tx, regionID); err != nil {
		slog.Error("osm import: clear region data in tx failed", "error", err)
		reporter.Fail("Region-Daten löschen fehlgeschlagen: " + err.Error())
		return false
	}

	if err := importRegionFromStaging(ctx, tx, stagingSchema, regionID); err != nil {
		slog.Error("osm import: staging import in tx failed", "error", err)
		reporter.Fail("Staging-Import fehlgeschlagen: " + err.Error())
		return false
	}

	// Update osm_import_regions INSIDE the transaction so that if the process
	// crashes after COMMIT, the status is already 'imported'. Without this,
	// a crash between COMMIT and the status UPDATE would leave the region stuck
	// in 'importing' even though data was successfully merged.
	durationS := int(time.Since(importStart).Seconds())
	pbfInfo, _ := os.Stat(pbfPath)
	var pbfSize int64
	if pbfInfo != nil {
		pbfSize = pbfInfo.Size()
	}

	// [I3] PBF NOT deleted — kept for future updates/patching.
	// Extract replication metadata from the PBF header so the DB reflects the
	// actual data date and replication coordinates rather than import-wall-clock.
	// These calls read the local PBF via osmium — safe inside a transaction (<1s).
	replURL, replSeq, _ := extractReplicationInfo(pbfPath)
	dataDate, _ := extractReplicationTimestamp(pbfPath)

	var replURLArg *string
	if replURL != "" {
		replURLArg = &replURL
	}
	var replSeqArg *int
	if replSeq != 0 {
		replSeqArg = &replSeq
	}

	_, regErr := tx.Exec(ctx, `
		UPDATE osm_import_regions SET
			status = 'imported', imported_at = now(),
			pbf_size_bytes = $2, pbf_path = $3,
			categories = $4, import_duration_s = $5,
			replication_url = COALESCE($6, replication_url),
			last_sequence_number = COALESCE($7, last_sequence_number),
			data_date = COALESCE($8, now())
		WHERE region_id = $1`,
		regionID, pbfSize, pbfPath, config.Categories, durationS,
		replURLArg, replSeqArg, dataDate)
	if regErr != nil {
		slog.Error("osm import: status update in tx failed", "region", regionID, "error", regErr)
		reporter.Fail("Region-Status konnte nicht aktualisiert werden: " + regErr.Error())
		return false
	}

	if err := tx.Commit(ctx); err != nil {
		slog.Error("osm import: commit failed", "error", err)
		reporter.Fail("Transaktion konnte nicht committed werden: " + err.Error())
		return false
	}

	// === Phase 5: ANALYZE affected tables ===
	// [I4] Without ANALYZE the query planner uses stale statistics -> bad query plans
	reporter.SetPhase("analyze")
	reporter.SetProgress(0)
	analyzeTables := []string{
		"osm_ways", "osm_pois", "osm_forests", "osm_boundaries", "osm_landuse",
		"osm_railways", "osm_waterways", "osm_waters", "osm_glaciers",
		"osm_wetlands", "osm_pistes", "osm_piste_areas", "osm_lifts",
		"osm_routes", "osm_way_regions", "osm_route_regions", "osm_route_way_members",
	}
	for i, table := range analyzeTables {
		if _, err := p.Pool.Exec(ctx, "ANALYZE "+table); err != nil {
			slog.Warn("osm import: analyze failed", "table", table, "error", err)
		}
		reporter.SetProgress(int64(i + 1))
	}

	// === Phase 5.5: Validate polygon geometries (einmalig, statt bei jedem Enrichment-Join) ===
	slog.Info("osm import: validating polygon geometries")
	for _, table := range []string{"osm_forests", "osm_boundaries", "osm_landuse", "osm_waters", "osm_wetlands", "osm_glaciers", "osm_piste_areas"} {
		_, err := p.Pool.Exec(ctx, fmt.Sprintf(
			"UPDATE %s SET geom = ST_MakeValid(geom) WHERE NOT ST_IsValid(geom) AND source_region = $1", table), regionID)
		if err != nil {
			slog.Warn("osm import: ST_MakeValid failed (non-fatal)", "table", table, "error", err)
		}
	}

	// NOTE: reporter.Complete() is NOT called here — the caller manages job completion
	// because multiple regions share one reporter in sequential mode.
	slog.Info("osm import: per-region import completed", "region", regionID,
		"duration_s", durationS)
	return true
}
