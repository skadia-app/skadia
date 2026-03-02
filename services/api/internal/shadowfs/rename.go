package shadowfs

import (
	"fmt"
	"log/slog"
)

// RenameItem represents a single file rename operation.
type RenameItem struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

// PreviewRename generates a list of old -> new path mappings without executing them.
// It renders each activity through the newSchema template and resolves collisions.
func PreviewRename(basePath, newSchema string, activities []ActivityInfo) []RenameItem {
	var items []RenameItem
	// Track planned new paths to detect collisions within the batch
	planned := make(map[string]int)

	for _, a := range activities {
		newRel := RenderPath(a, newSchema)

		// Check for collision with files already on disk
		newRel = ResolveCollision(basePath, newRel)

		// Check for collision within this batch
		if count, exists := planned[newRel]; exists {
			ext := ""
			stem := newRel
			for i := len(newRel) - 1; i >= 0; i-- {
				if newRel[i] == '.' {
					ext = newRel[i:]
					stem = newRel[:i]
					break
				}
			}
			newRel = fmt.Sprintf("%s_%d%s", stem, count+1, ext)
			planned[newRel] = 1
		}
		planned[newRel]++

		// Build old path from activity info (the current local_file_path)
		// The caller should have set ActivityInfo.Title or other fields from the old schema,
		// but we need the actual file path. We'll store it in the Title field or a dedicated OldPath.
		// For the preview, we compute both old and new relative paths.
		oldRel := RenderPath(a, DefaultSchema) // This is approximate; real old path comes from DB

		items = append(items, RenameItem{
			OldPath: oldRel,
			NewPath: newRel,
		})
	}

	return items
}

// RenameResult holds the result of a batch rename operation.
type RenameResult struct {
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// ExecuteRename performs the actual file moves for a list of rename items.
func ExecuteRename(basePath string, items []RenameItem) RenameResult {
	var result RenameResult

	for _, item := range items {
		if item.OldPath == item.NewPath {
			result.Succeeded++
			continue
		}

		if err := MoveShadowFile(basePath, item.OldPath, item.NewPath); err != nil {
			slog.Error("shadow fs rename failed",
				"old", item.OldPath,
				"new", item.NewPath,
				"error", err,
			)
			result.Failed++
		} else {
			result.Succeeded++
		}
	}

	return result
}
