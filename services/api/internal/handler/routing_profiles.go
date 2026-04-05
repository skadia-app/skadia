package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// brouterCache caches the result of the BRouter profiles2 directory scan.
var (
	brouterCacheMu    sync.Mutex
	brouterCacheTime  time.Time
	brouterCacheItems []profileListItem
)

const brouterCacheTTL = 30 * time.Second

// brouterSportTypes maps BRouter profile file names (without .brf) to sport types.
var brouterSportTypes = map[string][]string{
	"trekking":         {"cycling"},
	"hiking-mountain":  {"hiking", "mountaineering"},
	"hiking-sac2":      {"hiking", "mountaineering"},
	"cycling-road":     {"cycling"},
	"cycling-mountain": {"mountain_biking"},
	"cycling-gravel":   {"gravel_cycling"},
	"running":          {"running", "trail_running"},
	"walking":          {"walking", "hiking"},
}

// profileListItem is one entry in the HandleListProfiles response.
type profileListItem struct {
	ID          *string          `json:"id"`
	Name        string           `json:"name"`
	Engine      string           `json:"engine"`
	BaseProfile string           `json:"base_profile"`
	SportTypes  []string         `json:"sport_types"`
	Source      string           `json:"source"`
	Config      *json.RawMessage `json:"config"`
	Editable    bool             `json:"editable"`
}

// HandleListProfiles returns the combined list of system, user/filesystem, and BRouter profiles.
// GET /routing/profiles
func (h *Handler) HandleListProfiles(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var profiles []profileListItem

	// 1. All BadRouter profiles from DB (system, filesystem, user)
	dbProfiles, err := h.DB.ListRoutingProfiles(r.Context(), userID)
	if err != nil {
		slog.Error("HandleListProfiles: ListRoutingProfiles failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to list profiles", r.URL.Path)
		return
	}
	for _, dp := range dbProfiles {
		id := dp.ID
		editable := dp.Source == "user" && dp.UserID != nil && *dp.UserID == userID
		var configMsg *json.RawMessage
		if len(dp.Config) > 0 {
			msg := json.RawMessage(dp.Config)
			configMsg = &msg
		}
		profiles = append(profiles, profileListItem{
			ID:          &id,
			Name:        dp.Name,
			Engine:      "badrouter",
			BaseProfile: dp.BaseProfile,
			SportTypes:  dp.SportTypes,
			Source:      dp.Source,
			Config:      configMsg,
			Editable:    editable,
		})
	}

	// 2. BRouter profiles from filesystem (cached with TTL to avoid per-request Glob).
	brouterCacheMu.Lock()
	if time.Since(brouterCacheTime) < brouterCacheTTL && brouterCacheItems != nil {
		cached := brouterCacheItems
		brouterCacheMu.Unlock()
		profiles = append(profiles, cached...)
	} else {
		brouterCacheMu.Unlock()
		brouterDir := filepath.Join("data", "brouter", "profiles2")
		matches, _ := filepath.Glob(filepath.Join(brouterDir, "*.brf"))
		var items []profileListItem
		for _, path := range matches {
			base := strings.TrimSuffix(filepath.Base(path), ".brf")
			st := brouterSportTypes[base]
			if st == nil {
				st = []string{}
			}
			items = append(items, profileListItem{
				ID:          nil,
				Name:        base,
				Engine:      "brouter",
				BaseProfile: base,
				SportTypes:  st,
				Source:      "filesystem",
				Config:      nil,
				Editable:    false,
			})
		}
		brouterCacheMu.Lock()
		brouterCacheItems = items
		brouterCacheTime = time.Now()
		brouterCacheMu.Unlock()
		profiles = append(profiles, items...)
	}

	if profiles == nil {
		profiles = []profileListItem{}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
}

// createProfileRequest is the request body for POST /routing/profiles.
type createProfileRequest struct {
	Name        string               `json:"name"`
	BaseProfile string               `json:"base_profile"`
	SportTypes  []string             `json:"sport_types"`
	Config      routing.ProfileConfig `json:"config"`
}

// HandleCreateProfile creates a new user-owned routing profile.
// POST /routing/profiles
func (h *Handler) HandleCreateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[createProfileRequest](w, r)
	if !ok {
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if strings.TrimSpace(req.BaseProfile) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "base_profile is required", r.URL.Path)
		return
	}
	if !slices.Contains(routing.ValidProfileNames, req.BaseProfile) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "unknown base_profile", r.URL.Path)
		return
	}

	if err := routing.ValidateProfileConfig(req.Config); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	if err := routing.ValidateBuiltinCompleteness(req.Config); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			fmt.Sprintf("profile config incomplete: %v", err), r.URL.Path)
		return
	}
	if err := routing.ValidateSportTypes(req.SportTypes); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to serialize config", r.URL.Path)
		return
	}

	profile, err := h.DB.CreateRoutingProfile(r.Context(), userID, req.Name, req.BaseProfile, req.SportTypes, configJSON)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httputil.WriteError(w, http.StatusConflict, "Conflict", "profile name already exists", r.URL.Path)
			return
		}
		slog.Error("HandleCreateProfile: CreateRoutingProfile failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to create profile", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, profile)
}

// updateProfileRequest is the request body for PUT /routing/profiles/{id}.
type updateProfileRequest struct {
	Name       string               `json:"name"`
	SportTypes []string             `json:"sport_types"`
	Config     routing.ProfileConfig `json:"config"`
}

// HandleUpdateProfile updates an existing user-owned routing profile.
// PUT /routing/profiles/{id}
func (h *Handler) HandleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	// Ownership check
	existing, err := h.DB.GetRoutingProfile(r.Context(), id)
	if err != nil {
		slog.Error("HandleUpdateProfile: GetRoutingProfile failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to fetch profile", r.URL.Path)
		return
	}
	if existing == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "profile not found", r.URL.Path)
		return
	}
	if existing.UserID == nil || *existing.UserID != userID {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "you do not own this profile", r.URL.Path)
		return
	}

	req, ok := httputil.DecodeJSON[updateProfileRequest](w, r)
	if !ok {
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}

	if err := routing.ValidateProfileConfig(req.Config); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	if err := routing.ValidateBuiltinCompleteness(req.Config); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			fmt.Sprintf("profile config incomplete: %v", err), r.URL.Path)
		return
	}
	if err := routing.ValidateSportTypes(req.SportTypes); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to serialize config", r.URL.Path)
		return
	}

	updated, err := h.DB.UpdateRoutingProfile(r.Context(), id, req.Name, req.SportTypes, configJSON)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httputil.WriteError(w, http.StatusConflict, "Conflict", "profile name already exists", r.URL.Path)
			return
		}
		slog.Error("HandleUpdateProfile: UpdateRoutingProfile failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to update profile", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, updated)
}

// HandleDeleteProfile deletes a user-owned routing profile.
// DELETE /routing/profiles/{id}
func (h *Handler) HandleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	if err := h.DB.DeleteRoutingProfile(r.Context(), id, userID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "profile not found or not owned by user", r.URL.Path)
			return
		}
		slog.Error("HandleDeleteProfile: DeleteRoutingProfile failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to delete profile", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleImportProfiles imports one or more profiles from a multipart form with .json files.
// POST /routing/profiles/import
func (h *Handler) HandleImportProfiles(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "failed to parse multipart form", r.URL.Path)
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "no files provided", r.URL.Path)
		return
	}
	if len(files) > 50 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			"too many files (max 50)", r.URL.Path)
		return
	}

	type importResult struct {
		Name    string `json:"name"`
		Success bool   `json:"success"`
		Error   string `json:"error,omitempty"`
	}
	var results []importResult

	for _, fh := range files {
		if !strings.HasSuffix(strings.ToLower(fh.Filename), ".json") {
			results = append(results, importResult{Name: fh.Filename, Success: false, Error: "only .json files are supported"})
			continue
		}

		f, err := fh.Open()
		if err != nil {
			results = append(results, importResult{Name: fh.Filename, Success: false, Error: "failed to open file"})
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, 1<<20))
		f.Close()
		if err != nil {
			results = append(results, importResult{Name: fh.Filename, Success: false, Error: "failed to read file"})
			continue
		}

		var cfg routing.ProfileConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			results = append(results, importResult{Name: fh.Filename, Success: false, Error: fmt.Sprintf("invalid JSON: %v", err)})
			continue
		}

		// Backward compat: old wrapper format had nested "config" object
		if cfg.Profile == "" {
			var wrapper struct {
				Name        string                `json:"name"`
				BaseProfile string                `json:"base_profile"`
				SportTypes  []string              `json:"sport_types"`
				Config      routing.ProfileConfig `json:"config"`
			}
			if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.BaseProfile != "" {
				cfg = wrapper.Config
				cfg.Profile = wrapper.BaseProfile
				if cfg.Name == "" {
					cfg.Name = wrapper.Name
				}
				if len(cfg.SportTypes) == 0 {
					cfg.SportTypes = wrapper.SportTypes
				}
			}
		}

		if cfg.Profile == "" {
			results = append(results, importResult{Name: fh.Filename, Success: false, Error: "profile field is required"})
			continue
		}

		// Name fallback
		if cfg.Name == "" {
			cfg.Name = strings.TrimSuffix(fh.Filename, ".json")
		}

		if err := routing.ValidateProfileConfig(cfg); err != nil {
			results = append(results, importResult{Name: cfg.Name, Success: false, Error: err.Error()})
			continue
		}
		if err := routing.ValidateBuiltinCompleteness(cfg); err != nil {
			results = append(results, importResult{Name: cfg.Name, Success: false, Error: fmt.Sprintf("incomplete profile: %v", err)})
			continue
		}

		sportTypes := cfg.SportTypes
		if len(sportTypes) == 0 {
			sportTypes = routing.DefaultSportTypes(cfg.Profile)
		}
		if err := routing.ValidateSportTypes(sportTypes); err != nil {
			results = append(results, importResult{Name: cfg.Name, Success: false, Error: err.Error()})
			continue
		}

		configJSON, err := json.Marshal(cfg)
		if err != nil {
			results = append(results, importResult{Name: cfg.Name, Success: false, Error: "failed to serialize config"})
			continue
		}

		_, err = h.DB.CreateRoutingProfile(r.Context(), userID, cfg.Name, cfg.Profile, sportTypes, configJSON)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				results = append(results, importResult{Name: cfg.Name, Success: false, Error: "profile name already exists"})
				continue
			}
			results = append(results, importResult{Name: cfg.Name, Success: false, Error: "failed to create profile"})
			continue
		}

		results = append(results, importResult{Name: cfg.Name, Success: true})
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}

// HandleExportProfile exports a single routing profile as a JSON download.
// GET /routing/profiles/{id}/export
func (h *Handler) HandleExportProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	profile, err := h.DB.GetRoutingProfile(r.Context(), id)
	if err != nil {
		slog.Error("HandleExportProfile: GetRoutingProfile failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to fetch profile", r.URL.Path)
		return
	}
	if profile == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "profile not found", r.URL.Path)
		return
	}
	// Only allow export of own profiles or filesystem/system profiles (not other users' profiles)
	if profile.UserID != nil && *profile.UserID != userID {
		httputil.WriteError(w, http.StatusForbidden, "Forbidden", "you do not own this profile", r.URL.Path)
		return
	}

	var exportConfig routing.ProfileConfig
	if len(profile.Config) > 0 {
		if err := json.Unmarshal(profile.Config, &exportConfig); err != nil {
			slog.Warn("HandleExportProfile: unmarshal config failed", "id", id, "error", err)
		}
	}

	// Merge with base defaults so export is always complete
	if exportConfig.Profile != "" {
		if base, err := routing.BuiltinProfile(exportConfig.Profile); err == nil {
			exportConfig = routing.MergeProfile(base, exportConfig)
		}
	}

	// Set metadata from DB record
	exportConfig.Name = profile.Name
	if len(exportConfig.SportTypes) == 0 {
		exportConfig.SportTypes = profile.SportTypes
	}

	data, err := json.MarshalIndent(exportConfig, "", "  ")
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to serialize profile", r.URL.Path)
		return
	}

	filename := profile.Name + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// HandleUploadBrouterProfile uploads a .brf profile file to the BRouter profiles directory.
// POST /admin/routing/brouter-profiles
func (h *Handler) HandleUploadBrouterProfile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "failed to parse multipart form", r.URL.Path)
		return
	}

	file, fh, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "file field required", r.URL.Path)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(fh.Filename), ".brf") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "only .brf files are supported", r.URL.Path)
		return
	}

	// Sanitise filename — reject path traversal
	base := filepath.Base(fh.Filename)
	if base == "." || base == "/" || strings.ContainsAny(base, "/\\") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid filename", r.URL.Path)
		return
	}

	brouterDir := filepath.Join("data", "brouter", "profiles2")
	if err := os.MkdirAll(brouterDir, 0o755); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to create profiles directory", r.URL.Path)
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, 4<<20))
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to read file", r.URL.Path)
		return
	}

	dest := filepath.Join(brouterDir, base)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		slog.Error("HandleUploadBrouterProfile: write failed", "dest", dest, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to write profile file", r.URL.Path)
		return
	}

	slog.Info("BRouter profile uploaded", "filename", base, "bytes", len(data))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"name": strings.TrimSuffix(base, ".brf"), "filename": base})
}

// HandleDeleteBrouterProfile deletes a .brf profile file from the BRouter profiles directory.
// DELETE /admin/routing/brouter-profiles/{name}
func (h *Handler) HandleDeleteBrouterProfile(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	// Reject path traversal
	if strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid profile name", r.URL.Path)
		return
	}

	brouterDir := filepath.Join("data", "brouter", "profiles2")
	dest := filepath.Join(brouterDir, name+".brf")

	if err := os.Remove(dest); err != nil {
		if os.IsNotExist(err) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "BRouter profile not found", r.URL.Path)
			return
		}
		slog.Error("HandleDeleteBrouterProfile: remove failed", "dest", dest, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to delete profile file", r.URL.Path)
		return
	}

	slog.Info("BRouter profile deleted", "name", name)
	w.WriteHeader(http.StatusNoContent)
}

// SeedProfiles scans dir for *.json profile files and upserts them into the
// database as global profiles with the given source ("system" or "filesystem").
// Missing dir or no files is a no-op (returns 0, nil).
func SeedProfiles(ctx context.Context, dir string, db *database.DB, source string) (int, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		// Glob only returns ErrBadPattern which would be a programming error.
		return 0, fmt.Errorf("SeedProfiles: glob failed: %w", err)
	}
	// No directory / no files — not an error.
	if len(matches) == 0 {
		return 0, nil
	}

	count := 0
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("SeedProfiles: read failed", "file", path, "error", err)
			continue
		}

		var cfg routing.ProfileConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			slog.Warn("SeedProfiles: parse failed", "file", path, "error", err)
			continue
		}

		if err := routing.ValidateProfileConfig(cfg); err != nil {
			slog.Warn("SeedProfiles: validation failed", "file", path, "error", err)
			continue
		}

		if cfg.Profile == "" {
			slog.Warn("SeedProfiles: profile field missing", "file", path)
			continue
		}

		sportTypes := cfg.SportTypes
		if len(sportTypes) == 0 {
			sportTypes = routing.DefaultSportTypes(cfg.Profile)
		}

		displayName := cfg.Name
		if displayName == "" {
			displayName = strings.TrimSuffix(filepath.Base(path), ".json")
		}

		configBytes, err := json.Marshal(cfg)
		if err != nil {
			slog.Warn("SeedProfiles: marshal failed", "file", path, "error", err)
			continue
		}

		if _, err := db.UpsertGlobalProfile(ctx, displayName, cfg.Profile, sportTypes, configBytes, source); err != nil {
			slog.Warn("SeedProfiles: upsert failed", "file", path, "error", err)
			continue
		}

		count++
	}

	return count, nil
}
