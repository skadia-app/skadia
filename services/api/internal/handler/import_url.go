package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

// importBytesResult holds the outcome of importFileBytes.
type importBytesResult struct {
	Activity *model.Activity
	Warnings []string
	// Duplicate is true when the file was a duplicate and was handled per the
	// user's conflict strategy. In this case Activity may be nil.
	Duplicate bool
}

// ImportFromURL importiert eine Aktivitätsdatei von einer externen URL.
// POST /api/v1/import/from-url
// Body: { "url": "https://example.com/activity.fit" }
func (h *Handler) ImportFromURL(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}

	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "url is required", r.URL.Path)
		return
	}

	// URL validieren
	parsedURL, err := url.Parse(req.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid URL scheme (only http/https allowed)", r.URL.Path)
		return
	}

	// SSRF-Schutz: Interne Netzwerke blocken
	host := parsedURL.Hostname()
	if isInternalHost(host) {
		writeError(w, http.StatusBadRequest, "Bad Request", "internal URLs not allowed", r.URL.Path)
		return
	}

	// Datei herunterladen (mit Timeout + Size-Limit)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	httpReq, _ := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	dlResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		slog.Error("import-url: download failed", "url", req.URL, "error", err)
		writeError(w, http.StatusBadGateway, "Download Failed", "failed to download file from URL", r.URL.Path)
		return
	}
	defer dlResp.Body.Close()

	// Max 100 MB
	limitedReader := io.LimitReader(dlResp.Body, 100*1024*1024)
	fileBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Download Failed", err.Error(), r.URL.Path)
		return
	}

	// Filename aus URL oder Content-Disposition
	filename := path.Base(parsedURL.Path)
	if cd := dlResp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, parseErr := mime.ParseMediaType(cd); parseErr == nil {
			if fn, ok := params["filename"]; ok {
				filename = fn
			}
		}
	}
	if filename == "" || filename == "." {
		filename = "imported.fit"
	}

	result, importErr := h.importFileBytes(r.Context(), userID, fileBytes, filename)
	if importErr != nil {
		switch importErr.kind {
		case importErrBadFormat:
			writeError(w, http.StatusBadRequest, "Bad Request", importErr.Error(), r.URL.Path)
		case importErrParse:
			writeError(w, http.StatusUnprocessableEntity, "Parse Error", importErr.Error(), r.URL.Path)
		case importErrStorage:
			writeError(w, http.StatusInternalServerError, "Storage Error", importErr.Error(), r.URL.Path)
		default:
			writeError(w, http.StatusInternalServerError, "Internal Error", importErr.Error(), r.URL.Path)
		}
		return
	}

	if result.Duplicate {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "duplicate_handled",
		})
		return
	}

	writeJSON(w, http.StatusCreated, model.UploadResponse{
		Activity: result.Activity,
		Warnings: result.Warnings,
	})
}

type importErrKind int

const (
	importErrGeneral   importErrKind = iota
	importErrBadFormat importErrKind = iota
	importErrParse     importErrKind = iota
	importErrStorage   importErrKind = iota
)

type importError struct {
	kind    importErrKind
	message string
}

func (e *importError) Error() string { return e.message }

// importFileBytes parses and persists the given file bytes on behalf of userID.
// Returns (result, nil) on success, or (nil, *importError) on failure.
// When result.Duplicate is true the duplicate was handled and no new activity was created.
func (h *Handler) importFileBytes(ctx context.Context, userID string, fileBytes []byte, filename string) (*importBytesResult, *importError) {
	format := detectFormat(fileBytes, filename)
	if format == "" {
		return nil, &importError{importErrBadFormat, "unsupported file format (expected FIT, GPX, or TCX)"}
	}

	var (
		actData *model.ActivityData
		err     error
	)
	switch format {
	case "fit":
		actData, err = fitimport.ParseFIT(bytes.NewReader(fileBytes), filename)
	case "gpx":
		actData, err = gpximport.ParseGPX(bytes.NewReader(fileBytes), filename)
	case "tcx":
		actData, err = tcximport.ParseTCX(bytes.NewReader(fileBytes), filename)
	}
	if err != nil {
		slog.Error("import_url: parse failed", "format", format, "error", err)
		return nil, &importError{importErrParse, fmt.Sprintf("failed to parse %s file: %s", format, err)}
	}

	// Set file hash on data before processing
	hash := sha256.Sum256(fileBytes)
	hashStr := hex.EncodeToString(hash[:])
	actData.FileHash = hashStr

	if actData.SportType == "unknown" {
		slog.Warn("import_url: sport type unknown", "filename", filename, "format", format)
	}

	// Time overlap check (informational warnings only)
	var warnings []string
	if !actData.StartedAt.IsZero() && !actData.EndedAt.IsZero() {
		overlap, overlapErr := h.DB.CheckTimeOverlap(ctx, userID, actData.StartedAt, actData.EndedAt)
		if overlapErr != nil {
			slog.Error("import_url: time overlap check failed", "error", overlapErr)
		} else if overlap != nil {
			warnings = append(warnings, "time_overlap:"+overlap.ID)
		}
	}

	// Delegate duplicate detection + insert to ImportService
	result, processErr := h.ImportService.ProcessActivity(ctx, userID, actData, fileBytes, importer.ImportMetadata{SourcePlatform: "url"})
	if processErr != nil {
		slog.Error("import_url: process activity failed", "error", processErr)
		return nil, &importError{importErrGeneral, "failed to process activity"}
	}

	// Duplicate handled (enriched / replaced / skipped) — no new activity
	if result.Status != "created" {
		slog.Info("import_url: duplicate handled", "status", result.Status, "id", result.ActivityID)
		return &importBytesResult{Duplicate: true}, nil
	}

	// New activity created: upload original to Garage + run PostProcess
	s3Key := fmt.Sprintf("%s/%s.%s", userID, hashStr, format)
	contentType := "application/octet-stream"
	if format == "gpx" || format == "tcx" {
		contentType = "application/xml"
	}
	if uploadErr := h.Storage.Upload(ctx, s3Key, fileBytes, contentType); uploadErr != nil {
		slog.Error("import_url: S3 upload failed", "error", uploadErr)
		return nil, &importError{importErrStorage, "failed to store file"}
	}

	if format == "fit" && actData.UserProfileImport != nil {
		if profErr := h.DB.SetUserProfileFieldsIfNull(ctx, userID, actData.UserProfileImport); profErr != nil {
			slog.Warn("import_url: set user profile fields failed", "error", profErr)
		}
	}

	activity, getErr := h.DB.GetActivity(ctx, result.ActivityID)
	if getErr != nil || activity == nil {
		slog.Error("import_url: failed to fetch inserted activity", "id", result.ActivityID, "error", getErr)
		return nil, &importError{importErrGeneral, "failed to fetch inserted activity"}
	}

	if postErr := h.ImportService.PostProcess(ctx, userID, result.ActivityID, actData, fileBytes, s3Key, format, filename); postErr != nil {
		slog.Error("import_url: post-process failed", "id", result.ActivityID, "error", postErr)
	}

	// Re-fetch full activity for response
	full, err := h.DB.GetActivity(ctx, result.ActivityID)
	if err != nil || full == nil {
		full = activity
	}

	slog.Info("import_url: activity imported",
		"id", result.ActivityID,
		"sport", actData.SportType,
		"format", format,
		"filename", filename,
	)
	return &importBytesResult{Activity: full, Warnings: warnings}, nil
}

// isInternalHost returns true for RFC-1918 / loopback / link-local hosts to
// prevent SSRF attacks. It resolves hostnames to IPs before checking.
func isInternalHost(host string) bool {
	if host == "localhost" {
		return true
	}

	// Resolve hostname to IPs and check each resolved address
	ips, err := net.LookupHost(host)
	if err != nil {
		// If we can't resolve, also check the raw host string as an IP
		ips = []string{host}
	}

	for _, ipStr := range ips {
		if isBlockedIP(ipStr) {
			return true
		}
	}
	return false
}

// isBlockedIP checks whether a single IP string belongs to a private/loopback/link-local range.
func isBlockedIP(ipStr string) bool {
	// Block IPv4-mapped IPv6 addresses (::ffff:x.x.x.x) — extract the IPv4 part
	if strings.HasPrefix(ipStr, "::ffff:") {
		ipStr = strings.TrimPrefix(ipStr, "::ffff:")
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// Explicit zero address
	if ip.Equal(net.IPv4zero) || ipStr == "0.0.0.0" {
		return true
	}

	// Loopback (127.0.0.0/8, ::1)
	if ip.IsLoopback() {
		return true
	}

	// Link-local (169.254.0.0/16, fe80::/10)
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Private (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7)
	if ip.IsPrivate() {
		return true
	}

	// Unspecified (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return true
	}

	return false
}
