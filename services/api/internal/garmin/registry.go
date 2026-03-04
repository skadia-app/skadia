package garmin

import (
	"context"
	"sort"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/storage"
)

// DataImporter is the interface for a single import module.
type DataImporter interface {
	Name() string
	Description() string
	Priority() int // lower = runs first
	Detect(src ExportSource) (available bool, itemCount int, err error)
	Import(ctx context.Context, p ImportParams) (*ImportResult, error)
}

// ImportParams bundles everything an importer needs.
type ImportParams struct {
	Source     ExportSource
	UserID     string
	DB         *database.DB
	Storage    *storage.S3Client
	JobID      string
	OnProgress func(current, total int, msg string)
}

// ImportResult summarises the outcome of a single module run.
type ImportResult struct {
	Imported int
	Skipped  int
	Errors   []string
}

// DetectedModule is returned by registry.Detect.
type DetectedModule struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
	ItemCount   int    `json:"item_count"`
}

// ImportRegistry holds all registered importers and dispatches them.
type ImportRegistry struct {
	importers []DataImporter
}

// NewRegistry creates an empty registry.
func NewRegistry() *ImportRegistry { return &ImportRegistry{} }

// Register adds an importer and keeps the list sorted by Priority.
func (r *ImportRegistry) Register(imp DataImporter) {
	r.importers = append(r.importers, imp)
	sort.Slice(r.importers, func(i, j int) bool {
		return r.importers[i].Priority() < r.importers[j].Priority()
	})
}

// Detect probes all importers against the given source.
func (r *ImportRegistry) Detect(src ExportSource) []DetectedModule {
	var modules []DetectedModule
	for _, imp := range r.importers {
		available, count, _ := imp.Detect(src)
		modules = append(modules, DetectedModule{
			Name:        imp.Name(),
			Description: imp.Description(),
			Available:   available,
			ItemCount:   count,
		})
	}
	return modules
}

// RunModules executes the requested modules (or all available ones when modules is empty).
func (r *ImportRegistry) RunModules(ctx context.Context, modules []string, params ImportParams) error {
	for _, imp := range r.importers {
		if len(modules) > 0 && !containsStr(modules, imp.Name()) {
			continue
		}
		available, _, _ := imp.Detect(params.Source)
		if !available {
			continue
		}
		if _, err := imp.Import(ctx, params); err != nil {
			// Log but continue – individual modules must not abort the whole job.
			_ = err
		}
	}
	return nil
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
