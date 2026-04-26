package dem

import "path/filepath"

// subDirs defines the priority-ordered DEM sub-directories within a DEM data
// directory. Highest resolution first: sonny-05 (~0.5") > sonny-1 (~1") > glo30 (~1").
//
// This is the single source of truth. All call-sites that build HGT directory
// lists MUST use HGTDirsForConfig instead of hard-coding these names.
var subDirs = []string{"sonny-05", "sonny-1", "glo30"}

// HGTDirsForConfig returns the full paths to the DEM sub-directories in
// priority order (highest resolution first), rooted at demDataDir.
//
// The returned slice always has len == len(subDirs). Callers that need to
// filter out non-existent directories should do so themselves (e.g. when
// building routing tile hgtDirs that skip absent directories).
func HGTDirsForConfig(demDataDir string) []string {
	dirs := make([]string, len(subDirs))
	for i, sub := range subDirs {
		dirs[i] = filepath.Join(demDataDir, sub)
	}
	return dirs
}
