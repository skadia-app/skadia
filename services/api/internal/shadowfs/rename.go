package shadowfs

import (
	"log/slog"
)

// RenameItem represents a single file rename operation.
type RenameItem struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

// RenameResult holds the result of a batch rename operation.
type RenameResult struct {
	Succeeded      int          `json:"succeeded"`
	Failed         int          `json:"failed"`
	SucceededItems []RenameItem `json:"-"` // items that were actually moved on disk
}

// ExecuteRename performs the actual file moves for a list of rename items.
// Only items that succeed on disk are included in SucceededItems — callers must
// use SucceededItems (not the full input list) when updating DB paths.
func ExecuteRename(basePath string, items []RenameItem) RenameResult {
	var result RenameResult

	for _, item := range items {
		if item.OldPath == item.NewPath {
			result.Succeeded++
			result.SucceededItems = append(result.SucceededItems, item)
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
			result.SucceededItems = append(result.SucceededItems, item)
		}
	}

	return result
}
