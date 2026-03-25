package importutil

import (
	"context"
	"slices"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/filestore"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/storage"
)

// DataImporter is the interface for a single import module.
type DataImporter[S any] interface {
	Name() string
	Description() string
	Priority() int // lower = runs first
	Detect(src S) (available bool, itemCount int, err error)
	Import(ctx context.Context, p ImportParams[S]) (*ImportResult, error)
}

// ImportParams bundles everything an importer needs.
type ImportParams[S any] struct {
	Source           S
	UserID           string
	DB               *database.DB
	Storage          *storage.S3Client
	FileStore        filestore.FileStore // nil when not configured
	ImportService    *importer.Service
	JobID            string
	OnProgress       func(current, total int, msg string)
	OnModuleComplete func(completed, total int, moduleName string)
	Options          importer.ImportOptions
}

// ImportResult summarises the outcome of a single module run.
type ImportResult struct {
	Imported int
	Skipped  int
	Errors   []string
}

// DetectedModule is returned by ImportRegistry.Detect.
type DetectedModule struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
	ItemCount   int    `json:"item_count"`
}

// ImportRegistry holds all registered importers and dispatches them.
type ImportRegistry[S any] struct {
	importers []DataImporter[S]
}

// NewRegistry creates an empty registry.
func NewRegistry[S any]() *ImportRegistry[S] { return &ImportRegistry[S]{} }

// Register adds an importer and keeps the list sorted by Priority.
func (r *ImportRegistry[S]) Register(imp DataImporter[S]) {
	r.importers = append(r.importers, imp)
	slices.SortFunc(r.importers, func(a, b DataImporter[S]) int {
		return a.Priority() - b.Priority()
	})
}

// Detect probes all importers against the given source.
func (r *ImportRegistry[S]) Detect(src S) []DetectedModule {
	var modules []DetectedModule
	for _, imp := range r.importers {
		// Detect errors are intentionally ignored — a failing detector means
		// the module is not available for this export, not a fatal error.
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
// Returns all non-fatal error strings collected across modules.
func (r *ImportRegistry[S]) RunModules(ctx context.Context, modules []string, params ImportParams[S]) ([]string, error) {
	var toRun []DataImporter[S]
	for _, imp := range r.importers {
		if len(modules) > 0 && !slices.Contains(modules, imp.Name()) {
			continue
		}
		available, _, _ := imp.Detect(params.Source)
		if available {
			toRun = append(toRun, imp)
		}
	}

	total := len(toRun)
	var allErrors []string

	for i, imp := range toRun {
		if ctx.Err() != nil {
			return allErrors, ctx.Err()
		}
		result, err := imp.Import(ctx, params)
		if err != nil {
			allErrors = append(allErrors, imp.Name()+": "+err.Error())
		}
		if result != nil {
			for _, e := range result.Errors {
				allErrors = append(allErrors, imp.Name()+"/"+e)
			}
		}
		if params.OnModuleComplete != nil {
			params.OnModuleComplete(i+1, total, imp.Name())
		}
	}
	// The error return is always nil: individual module failures are collected
	// in allErrors so that remaining modules can still run.
	return allErrors, nil
}
