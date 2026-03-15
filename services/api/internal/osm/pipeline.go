package osm

import (
	"bufio"
	"context"
	"fmt"
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
	Categories    []string
	BuildBRouter  bool
	BuildTopology bool
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
}

// Run executes the full OSM import pipeline. It reports progress via reporter and never returns an error —
// failures are reported via reporter.Fail().
func (p *Pipeline) Run(ctx context.Context, regions []Region, config PipelineConfig, reporter ProgressReporter) {
	categories := config.Categories

	if err := os.MkdirAll(p.DataDir, 0755); err != nil {
		slog.Error("osm import: failed to create data dir", "error", err)
		reporter.Fail("Datenverzeichnis konnte nicht erstellt werden: " + err.Error())
		return
	}

	// Lua-Script in Temp-Datei schreiben
	luaTmp, err := os.CreateTemp("", "skadia-osm-*.lua")
	if err != nil {
		slog.Error("osm import: failed to write lua tmp", "error", err)
		reporter.Fail("Lua-Script konnte nicht geschrieben werden: " + err.Error())
		return
	}
	defer os.Remove(luaTmp.Name())
	if _, err := luaTmp.Write(LuaScript); err != nil {
		slog.Error("osm import: failed to write lua content", "error", err)
		reporter.Fail("Lua-Script konnte nicht geschrieben werden: " + err.Error())
		return
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
					return
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
				return
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
		return
	}
	// Originale pbfPaths für BRouter/Cleanup aufbewahren
	originalPBFs := pbfPaths
	_ = originalPBFs // Used by BRouter preservation loop (reg.ID-based paths)
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

	// PostgreSQL Import-Tuning (temporär — wird nach Import zurückgesetzt)
	maintWorkMemMB := availableRAMMB() * 15 / 100
	if maintWorkMemMB > 8192 {
		maintWorkMemMB = 8192
	}
	if maintWorkMemMB < 256 {
		maintWorkMemMB = 256
	}

	pgTuning := []string{
		fmt.Sprintf("ALTER SYSTEM SET maintenance_work_mem = '%dMB'", maintWorkMemMB),
		"ALTER SYSTEM SET max_parallel_maintenance_workers = 4",
		"SELECT pg_reload_conf()",
	}
	for _, stmt := range pgTuning {
		if _, err := p.Pool.Exec(ctx, stmt); err != nil {
			slog.Warn("osm import: PG tuning failed (non-fatal)", "stmt", stmt, "error", err)
		}
	}
	slog.Info("osm import: PG import tuning applied", "maintenance_work_mem_mb", maintWorkMemMB)
	defer func() {
		for _, stmt := range []string{
			"ALTER SYSTEM RESET maintenance_work_mem",
			"ALTER SYSTEM RESET max_parallel_maintenance_workers",
			"SELECT pg_reload_conf()",
		} {
			p.Pool.Exec(context.Background(), stmt)
		}
		slog.Info("osm import: PG import tuning reset")
	}()

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
		return
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
		return
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

	// === Phase 5: Trail-Topologie (optional) ===
	if config.BuildTopology {
		waysImported := slices.Contains(categories, "ways")
		if waysImported {
			reporter.SetPhase("topology")
			reporter.SetProgress(0)
			slog.Info("osm import: starting trail topology build")

			// Phase 5a: pgr_createTopology — WICHTIG: 'id' ist die PK-Spalte, NICHT 'way_id'
			// TODO(pgRouting): pgr_createTopology wurde in pgRouting 4.0.1 entfernt und
			// schlägt hier fehl. Skadia's A*-Router (routing/graph/) nutzt KEINE
			// pgRouting-Topologie — er baut seinen CSR-Graphen direkt aus osm_ways.
			// source/target in osm_ways bleiben NULL, was für den eigenen Router irrelevant ist.
			// Fix: Manuellen Topology-Builder implementieren (ST_StartPoint/ST_EndPoint +
			// KNN-Vertex-Matching) oder pgRouting <4.0 verwenden.
			reporter.SetProgress(1)
			_, topoErr := p.Pool.Exec(ctx, `
				SELECT pgr_createTopology('osm_ways', 0.00001, 'geom', 'id', 'source', 'target')
			`)
			if topoErr != nil {
				slog.Error("osm import: topology build failed", "error", topoErr)
				// Nicht fatal — Import war erfolgreich, nur Topologie fehlgeschlagen
			} else {
				// Phase 5b: cost/reverse_cost berechnen (exakt wie trail_topology.go)
				reporter.SetProgress(2)
				_, costErr := p.Pool.Exec(ctx, `
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
				reporter.SetProgress(3)
				_, liftErr := p.Pool.Exec(ctx, `
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
				// TODO(pgRouting): Wie Phase 5a — pgr_createTopology in pgRouting 4.0.1 entfernt.
				reporter.SetProgress(4)
				_, retopoErr := p.Pool.Exec(ctx, `
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
		waysImported := slices.Contains(categories, "ways")
		if waysImported {
			reporter.SetPhase("enrichment")
			reporter.SetProgress(0)
			slog.Info("osm import: starting spatial enrichment")

			if err := RunSpatialEnrichment(ctx, p.Pool, true, func(pct int) {
				reporter.SetProgress(int64(pct))
			}); err != nil {
				slog.Error("osm import: spatial enrichment failed", "error", err)
				reporter.Fail("Spatial Enrichment fehlgeschlagen: " + err.Error())
				return
			}

			// === Phase 7: Intersection Splitting (creates osm_ways_noded) ===
			reporter.SetPhase("splitting")
			reporter.SetProgress(0)
			slog.Info("osm import: starting intersection splitting")

			if err := SplitAtIntersections(ctx, p.Pool, func(pct int) {
				reporter.SetProgress(int64(pct))
			}); err != nil {
				slog.Error("osm import: intersection splitting failed", "error", err)
				reporter.Fail("Intersection Splitting fehlgeschlagen: " + err.Error())
				return
			}
		}
	}

	reporter.Complete()
	slog.Info("osm import: completed")
}

// RunTopologyOnly executes only the topology build phase (Phase 5) on existing osm_ways.
// Used by the retry-topology endpoint.
func (p *Pipeline) RunTopologyOnly(ctx context.Context, reporter ProgressReporter) {
	slog.Info("osm topology retry: starting")

	// TODO(pgRouting): pgr_createTopology wurde in pgRouting 4.0.1 entfernt. Diese Funktion
	// (RunTopologyOnly) wird durch den Retry-Topology-Endpoint aufgerufen und schlägt aktuell
	// grundsätzlich fehl. Skadia's A*-Router (routing/graph/) nutzt pgRouting-Topologie nicht —
	// er baut seinen CSR-Graphen direkt aus osm_ways. Die Retry-Topology-Funktion ist daher
	// derzeit ohne Effekt und sollte langfristig entweder entfernt oder auf einen manuellen
	// Topology-Builder umgestellt werden (ST_StartPoint/ST_EndPoint + KNN-Vertex-Matching).
	reporter.SetProgress(1)
	_, topoErr := p.Pool.Exec(ctx, `
		SELECT pgr_createTopology('osm_ways', 0.00001, 'geom', 'id', 'source', 'target')
	`)
	if topoErr != nil {
		slog.Error("osm topology retry: topology build failed", "error", topoErr)
		reporter.Fail("Topologie-Build fehlgeschlagen: " + topoErr.Error())
		return
	}

	reporter.SetProgress(2)
	_, costErr := p.Pool.Exec(ctx, `
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
		slog.Warn("osm topology retry: cost computation failed", "error", costErr)
	}

	reporter.SetProgress(3)
	_, liftErr := p.Pool.Exec(ctx, `
		INSERT INTO osm_ways (osm_id, highway, name, length_m, avg_slope, is_lift, geom)
		SELECT l.osm_id, 'lift', l.name, ST_Length(l.geom::geography), 0.0, TRUE, l.geom
		FROM osm_lifts l
		WHERE NOT EXISTS (
			SELECT 1 FROM osm_ways t
			WHERE t.osm_id = l.osm_id AND t.is_lift = TRUE
		)
	`)
	if liftErr != nil {
		slog.Warn("osm topology retry: lift merge failed", "error", liftErr)
	}

	// TODO(pgRouting): Wie oben — pgr_createTopology in pgRouting 4.0.1 entfernt.
	reporter.SetProgress(4)
	_, retopoErr := p.Pool.Exec(ctx, `
		SELECT pgr_createTopology('osm_ways', 0.00001, 'geom', 'id', 'source', 'target')
	`)
	if retopoErr != nil {
		slog.Warn("osm topology retry: lift topology rebuild failed", "error", retopoErr)
	}

	reporter.SetProgress(5)
	slog.Info("osm topology retry: completed")
}

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
	// Enrichment-Raster-Tiles invalidieren wenn relevante Kategorien importiert werden
	enrichmentCats := map[string]bool{"forests": true, "boundaries": true, "landuse": true, "ways": true}
	for _, cat := range categories {
		if enrichmentCats[cat] {
			if _, err := p.Pool.Exec(ctx, "TRUNCATE enrichment.raster_tiles"); err != nil {
				slog.Warn("clearOSMData: truncate enrichment.raster_tiles failed (may not exist yet)", "error", err)
			}
			break
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

	// 40% of available RAM
	cacheMB := availMB * 40 / 100
	// Cap at 2x PBF size (more is wasteful — all node coords fit in ~1.5x PBF)
	if maxCache := pbfMB * 2; cacheMB > maxCache {
		cacheMB = maxCache
	}
	// Floor: at least 256 MB if we have enough RAM
	if cacheMB < 256 {
		if availMB >= 1024 {
			cacheMB = 256 // Minimum sinnvoller Cache
		} else {
			return 0 // Nicht genug RAM → flat-nodes Fallback
		}
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
