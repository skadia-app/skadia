package osm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BRouterPaths holds configurable filesystem paths for BRouter operations.
// The JAR path is intentionally excluded — it is baked into the container image.
type BRouterPaths struct {
	ProfilesDir string // e.g. /data/brouter/profiles (shared with BRouter container)
	DEMDir      string // e.g. /data/dem (parent of sonny-05/ and glo30/ sub-dirs)
}

// BuildBRouterSegments builds rd5 segment files from local PBF data using
// BRouter's mapcreator classes (OsmFastCutter, PosUnifier, WayLinker).
// This replaces the previous approach of downloading pre-built segments
// from brouter.de which broke due to URL structure changes.
//
// The build mirrors the upstream create-routing-data.sh script from
// ghcr.io/mjaschen/brouter-routingdata-builder but runs Java directly
// inside the API container.
func (p *Pipeline) BuildBRouterSegments(ctx context.Context, paths BRouterPaths, regions []Region,
	osmDataDir, segmentsDir string, reporter ProgressReporter) error {

	// 1. Find PBF files for the imported regions (unfiltered *-latest.osm.pbf)
	pbfFiles, err := findRegionPBFs(osmDataDir, regions)
	if err != nil {
		return fmt.Errorf("find PBF files: %w", err)
	}
	if len(pbfFiles) == 0 {
		return fmt.Errorf("no PBF files found in %s for %d regions", osmDataDir, len(regions))
	}
	slog.Info("brouter: found PBF files for segment build", "count", len(pbfFiles), "files", pbfFiles)

	// 2. Merge PBFs with osmium if multiple, otherwise use directly
	planetPBF, needsCleanup, err := mergePBFs(ctx, osmDataDir, pbfFiles)
	if err != nil {
		return fmt.Errorf("merge PBFs: %w", err)
	}
	if needsCleanup {
		defer os.Remove(planetPBF)
	}

	// 3. Create working directories
	tmpBase := filepath.Join(segmentsDir, "brouter-tmp")
	if err := os.MkdirAll(segmentsDir, 0o755); err != nil {
		return fmt.Errorf("create segments dir: %w", err)
	}
	if err := os.MkdirAll(tmpBase, 0o755); err != nil {
		return fmt.Errorf("create tmp dir: %w", err)
	}
	defer os.RemoveAll(tmpBase)

	tmpOutput := filepath.Join(tmpBase, "output")
	for _, sub := range []string{"nodetiles", "waytiles", "waytiles55", "nodes55", "unodes55", "output"} {
		if err := os.MkdirAll(filepath.Join(tmpBase, sub), 0o755); err != nil {
			return fmt.Errorf("create %s dir: %w", sub, err)
		}
	}

	// 4. BRouter paths — JAR is baked into the container image (not user-configurable).
	// Profiles and DEM dirs come from BRouterPaths so callers can inject config values.
	const jar = "/brouter/brouter.jar"

	lookups     := filepath.Join(paths.ProfilesDir, "lookups.dat")
	allBrf      := filepath.Join(paths.ProfilesDir, "all.brf")
	trekkingBrf := filepath.Join(paths.ProfilesDir, "trekking.brf")
	softBrf     := filepath.Join(paths.ProfilesDir, "softaccess.brf")

	// DEM directories — BRouter accepts HGT files; we pass all available
	// resolution tiers. PosUnifier takes two args: srtm1 and srtm3 dirs.
	// We use our highest-res DEM as "srtm1" and GLO-30 as "srtm3" fallback.
	srtm1Dir := filepath.Join(paths.DEMDir, "sonny-05")
	srtm3Dir := filepath.Join(paths.DEMDir, "glo30")

	// Ensure DEM dirs exist (may be empty if user hasn't downloaded DEM yet)
	os.MkdirAll(srtm1Dir, 0o755)
	os.MkdirAll(srtm3Dir, 0o755)

	// Java memory settings — 2 GB is reasonable for regional extracts
	javaOpts := strings.Split(os.Getenv("BROUTER_JAVA_OPTS"), " ")
	if len(javaOpts) == 1 && javaOpts[0] == "" {
		javaOpts = []string{"-Xmx2048m", "-Xms2048m", "-Xmn256m"}
	}

	// 5. Run the three BRouter mapcreator steps
	type step struct {
		name string
		args []string
	}

	steps := []step{
		{
			name: "OsmFastCutter",
			args: append(javaOpts,
				"-cp", jar,
				"-DavoidMapPolling=true",
				"-Ddeletetmpfiles=false",
				"-DuseDenseMaps=true",
				"btools.mapcreator.OsmFastCutter",
				lookups,
				filepath.Join(tmpBase, "nodetiles"),
				filepath.Join(tmpBase, "waytiles"),
				filepath.Join(tmpBase, "nodes55"),
				filepath.Join(tmpBase, "waytiles55"),
				filepath.Join(tmpBase, "bordernids.dat"),
				filepath.Join(tmpBase, "relations.dat"),
				filepath.Join(tmpBase, "restrictions.dat"),
				allBrf,
				trekkingBrf,
				softBrf,
				planetPBF,
			),
		},
		{
			name: "PosUnifier",
			args: append(javaOpts,
				"-cp", jar,
				"-Ddeletetmpfiles=true",
				"-DuseDenseMaps=true",
				"btools.mapcreator.PosUnifier",
				filepath.Join(tmpBase, "nodes55"),
				filepath.Join(tmpBase, "unodes55"),
				filepath.Join(tmpBase, "bordernids.dat"),
				filepath.Join(tmpBase, "bordernodes.dat"),
				srtm1Dir,
				srtm3Dir,
			),
		},
		{
			name: "WayLinker",
			args: append(javaOpts,
				"-cp", jar,
				"-DuseDenseMaps=true",
				"-DskipEncodingCheck=true",
				"btools.mapcreator.WayLinker",
				filepath.Join(tmpBase, "unodes55"),
				filepath.Join(tmpBase, "waytiles55"),
				filepath.Join(tmpBase, "bordernodes.dat"),
				filepath.Join(tmpBase, "restrictions.dat"),
				lookups,
				allBrf,
				tmpOutput, // Write to temp dir, then merge with size-guard
				"rd5",
			),
		},
	}

	for i, s := range steps {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		reporter.SetPhase(fmt.Sprintf("brouter_%s", s.name))
		reporter.SetProgress(int64(i * 100 / len(steps)))
		slog.Info("brouter: running step", "step", s.name, "index", i+1, "total", len(steps))

		cmd := exec.CommandContext(ctx, "java", s.args...)
		cmd.Dir = tmpBase
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("brouter %s failed: %w", s.name, err)
		}

		slog.Info("brouter: step complete", "step", s.name)
	}

	// 6. Move built segments from tmpOutput into segmentsDir.
	// CRITICAL: Only overwrite existing rd5 files if the new file is LARGER.
	// Local PBF builds only cover a small region (e.g. Slowenien) but BRouter's
	// WayLinker writes rd5 tiles for the entire 5°×5° grid that the PBF overlaps.
	// These partial tiles are much smaller than pre-downloaded full tiles from
	// brouter.de. Overwriting a 120MB downloaded E10_N45.rd5 with a 9MB
	// Slowenien-only build destroys routing for Bayern/Österreich.
	matches, _ := filepath.Glob(filepath.Join(tmpOutput, "*.rd5"))
	var kept, replaced, added int
	for _, src := range matches {
		name := filepath.Base(src)
		dst := filepath.Join(segmentsDir, name)

		srcInfo, err := os.Stat(src)
		if err != nil {
			continue
		}

		dstInfo, dstErr := os.Stat(dst)
		if dstErr == nil && dstInfo.Size() >= srcInfo.Size() {
			// Existing file is same size or larger — keep it.
			kept++
			slog.Info("brouter: keeping existing segment (larger/equal)",
				"file", name,
				"existing_bytes", dstInfo.Size(),
				"new_bytes", srcInfo.Size())
			continue
		}

		// Copy new file (either doesn't exist or new is larger).
		data, err := os.ReadFile(src)
		if err != nil {
			slog.Warn("brouter: failed to read built segment", "file", name, "error", err)
			continue
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			slog.Warn("brouter: failed to write segment", "file", name, "error", err)
			continue
		}

		if dstErr == nil {
			replaced++
			slog.Info("brouter: replaced segment (new is larger)",
				"file", name,
				"old_bytes", dstInfo.Size(),
				"new_bytes", srcInfo.Size())
		} else {
			added++
			slog.Info("brouter: added new segment", "file", name, "bytes", srcInfo.Size())
		}
	}

	reporter.SetProgress(100)
	slog.Info("brouter: segment build complete",
		"segments_dir", segmentsDir,
		"added", added,
		"replaced", replaced,
		"kept_existing", kept)
	return nil
}

// findRegionPBFs locates the unfiltered *-latest.osm.pbf files for each region.
// It matches by region ID embedded in the filename (e.g., "switzerland-latest.osm.pbf").
func findRegionPBFs(osmDataDir string, regions []Region) ([]string, error) {
	var pbfFiles []string
	seen := make(map[string]bool)

	for _, reg := range regions {
		// Region IDs look like "europe/switzerland" — we want "switzerland"
		parts := strings.Split(reg.ID, "/")
		baseName := parts[len(parts)-1]

		// Look for unfiltered PBF: <region>-latest.osm.pbf
		pattern := filepath.Join(osmDataDir, baseName+"-latest.osm.pbf")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", pattern, err)
		}

		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				pbfFiles = append(pbfFiles, m)
			}
		}
	}

	return pbfFiles, nil
}

// mergePBFs merges multiple PBF files into one using osmium merge.
// Returns the path to the merged file, whether it needs cleanup, and any error.
// If only one file is provided, it is returned directly without merging.
func mergePBFs(ctx context.Context, osmDataDir string, pbfFiles []string) (string, bool, error) {
	if len(pbfFiles) == 1 {
		return pbfFiles[0], false, nil
	}

	merged := filepath.Join(osmDataDir, "brouter-merged.osm.pbf")
	args := append([]string{"merge", "-o", merged, "--overwrite"}, pbfFiles...)

	slog.Info("brouter: merging PBF files", "count", len(pbfFiles), "output", merged)
	cmd := exec.CommandContext(ctx, "osmium", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", false, fmt.Errorf("osmium merge: %w", err)
	}

	return merged, true, nil
}
