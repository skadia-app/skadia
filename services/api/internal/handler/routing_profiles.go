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

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// systemProfileNames are the 5 built-in BadRouter profiles.
var systemProfileNames = []string{"hiking", "mtb", "gravel", "trekking", "road"}

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

	// 1. System defaults (BadRouter built-ins)
	for _, name := range systemProfileNames {
		p, err := routing.ProfileDefaults(name)
		if err != nil {
			slog.Warn("HandleListProfiles: ProfileDefaults failed", "name", name, "error", err)
			continue
		}
		raw, err := json.Marshal(p)
		if err != nil {
			slog.Warn("HandleListProfiles: marshal system profile failed", "name", name, "error", err)
			continue
		}
		msg := json.RawMessage(raw)
		profiles = append(profiles, profileListItem{
			ID:          nil,
			Name:        name,
			Engine:      "badrouter",
			BaseProfile: name,
			SportTypes:  systemSportTypes(name),
			Source:      "system",
			Config:      &msg,
			Editable:    false,
		})
	}

	// 2. User custom + filesystem profiles from DB
	dbProfiles, err := h.DB.ListRoutingProfiles(r.Context(), userID)
	if err != nil {
		slog.Error("HandleListProfiles: ListRoutingProfiles failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Server Error", "failed to list profiles", r.URL.Path)
		return
	}
	for _, dp := range dbProfiles {
		id := dp.ID
		editable := dp.UserID != nil && *dp.UserID == userID
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

	// 3. BRouter profiles from filesystem
	brouterDir := filepath.Join("data", "brouter", "profiles2")
	matches, _ := filepath.Glob(filepath.Join(brouterDir, "*.brf"))
	for _, path := range matches {
		base := strings.TrimSuffix(filepath.Base(path), ".brf")
		st := brouterSportTypes[base]
		if st == nil {
			st = []string{}
		}
		profiles = append(profiles, profileListItem{
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

// importProfileEntry is one entry in the import JSON format.
type importProfileEntry struct {
	Name        string               `json:"name"`
	BaseProfile string               `json:"base_profile"`
	SportTypes  []string             `json:"sport_types"`
	Config      routing.ProfileConfig `json:"config"`
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

		var entry importProfileEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			results = append(results, importResult{Name: fh.Filename, Success: false, Error: fmt.Sprintf("invalid JSON: %v", err)})
			continue
		}

		if strings.TrimSpace(entry.Name) == "" {
			entry.Name = strings.TrimSuffix(fh.Filename, ".json")
		}
		if strings.TrimSpace(entry.BaseProfile) == "" {
			results = append(results, importResult{Name: entry.Name, Success: false, Error: "base_profile is required"})
			continue
		}
		if !slices.Contains(routing.ValidProfileNames, entry.BaseProfile) {
			results = append(results, importResult{Name: entry.Name, Success: false, Error: "unknown base_profile"})
			continue
		}

		if err := routing.ValidateProfileConfig(entry.Config); err != nil {
			results = append(results, importResult{Name: entry.Name, Success: false, Error: err.Error()})
			continue
		}
		if err := routing.ValidateSportTypes(entry.SportTypes); err != nil {
			results = append(results, importResult{Name: entry.Name, Success: false, Error: err.Error()})
			continue
		}

		configJSON, err := json.Marshal(entry.Config)
		if err != nil {
			results = append(results, importResult{Name: entry.Name, Success: false, Error: "failed to serialize config"})
			continue
		}

		_, err = h.DB.CreateRoutingProfile(r.Context(), userID, entry.Name, entry.BaseProfile, entry.SportTypes, configJSON)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				results = append(results, importResult{Name: entry.Name, Success: false, Error: "profile name already exists"})
				continue
			}
			results = append(results, importResult{Name: entry.Name, Success: false, Error: "failed to create profile"})
			continue
		}

		results = append(results, importResult{Name: entry.Name, Success: true})
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

	export := importProfileEntry{
		Name:        profile.Name,
		BaseProfile: profile.BaseProfile,
		SportTypes:  profile.SportTypes,
		Config:      exportConfig,
	}

	data, err := json.MarshalIndent(export, "", "  ")
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

// systemSportTypes returns the default sport types for a built-in BadRouter profile.
func systemSportTypes(profile string) []string {
	switch profile {
	case "hiking":
		return []string{"hiking", "trail_running", "running", "walking", "mountaineering"}
	case "mtb":
		return []string{"mountain_biking"}
	case "gravel":
		return []string{"gravel_cycling"}
	case "trekking":
		return []string{"cycling"}
	case "road":
		return []string{"cycling", "gravel_cycling"}
	default:
		return []string{}
	}
}

// filesystemProfileSportTypes derives sport types from the base profile name for
// filesystem-imported BadRouter profiles. Uses the same defaults as system profiles
// with extensions for additional base profile aliases.
func filesystemProfileSportTypes(baseProfile string) []string {
	switch baseProfile {
	case "hiking":
		return []string{"hiking", "trail_running", "running", "walking", "mountaineering"}
	case "mtb":
		return []string{"mountain_biking"}
	case "gravel":
		return []string{"gravel_cycling"}
	case "trekking":
		return []string{"cycling"}
	case "road":
		return []string{"cycling", "gravel_cycling"}
	default:
		return []string{}
	}
}

// ScanFilesystemProfiles scans dir for *.json BadRouter profile files and upserts
// them into the database as global (filesystem-source) profiles. The directory
// missing entirely is treated as a no-op (returns 0, nil). Per-file errors are
// logged but do not abort the scan. Returns the count of successfully imported files.
func ScanFilesystemProfiles(ctx context.Context, dir string, db *database.DB) (int, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		// Glob only returns ErrBadPattern which would be a programming error.
		return 0, fmt.Errorf("ScanFilesystemProfiles: glob failed: %w", err)
	}
	// No directory / no files — not an error.
	if len(matches) == 0 {
		return 0, nil
	}

	count := 0
	for _, path := range matches {
		name := strings.TrimSuffix(filepath.Base(path), ".json")

		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("ScanFilesystemProfiles: read failed", "file", path, "error", err)
			continue
		}

		var cfg routing.ProfileConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			slog.Warn("ScanFilesystemProfiles: parse failed", "file", path, "error", err)
			continue
		}

		if err := routing.ValidateProfileConfig(cfg); err != nil {
			slog.Warn("ScanFilesystemProfiles: validation failed", "file", path, "error", err)
			continue
		}

		baseProfile := cfg.Profile
		if baseProfile == "" {
			slog.Warn("ScanFilesystemProfiles: profile (base_profile) missing", "file", path)
			continue
		}

		sportTypes := filesystemProfileSportTypes(baseProfile)

		configBytes, err := json.Marshal(cfg)
		if err != nil {
			slog.Warn("ScanFilesystemProfiles: marshal failed", "file", path, "error", err)
			continue
		}

		if _, err := db.UpsertFilesystemProfile(ctx, name, baseProfile, sportTypes, configBytes); err != nil {
			slog.Warn("ScanFilesystemProfiles: upsert failed", "file", path, "error", err)
			continue
		}

		count++
	}

	return count, nil
}
