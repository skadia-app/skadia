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
	"slices"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/importer"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/tcximport"
)

// importBytesResult holds the outcome of importFileBytes.
type importBytesResult struct {
	Activity *model.Activity
	Warnings []string
	// Duplicate is true when the file was a duplicate and was handled per the
	// user's conflict strategy. In this case Activity may be nil.
	Duplicate bool
	// Multisport is true when a multi-session FIT file was imported. In this
	// case Activity holds the parent activity.
	Multisport bool
}

// ImportFromURL importiert eine Aktivitätsdatei von einer externen URL.
// POST /api/v1/import/from-url
// Body: { "url": "https://example.com/activity.fit" }
func (h *Handler) ImportFromURL(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}

	if req.URL == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "url is required", r.URL.Path)
		return
	}

	// URL validieren
	parsedURL, err := url.Parse(req.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid URL scheme (only http/https allowed)", r.URL.Path)
		return
	}

	// SSRF-Schutz: Interne Netzwerke blocken
	host := parsedURL.Hostname()
	if isInternalHost(host) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "internal URLs not allowed", r.URL.Path)
		return
	}

	// Datei herunterladen (mit Timeout + Size-Limit)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Sicherer HTTP-Client: verhindert SSRF via Redirects und DNS-Rebinding.
	secureTransport := &http.Transport{
		DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(addr)
			if splitErr != nil {
				return nil, fmt.Errorf("invalid address: %w", splitErr)
			}
			ips, lookupErr := net.DefaultResolver.LookupIPAddr(dialCtx, host)
			if lookupErr != nil {
				return nil, lookupErr
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no IPs resolved for host: %s", host)
			}
			for _, ip := range ips {
				if isPrivateIP(ip.IP) {
					return nil, fmt.Errorf("connection to private IP blocked: %s", ip.IP)
				}
			}
			// Bereits aufgelöste IP direkt verwenden — verhindert zweite DNS-Auflösung
			// (TOCTOU-DNS-Rebinding: TTL=0 könnte beim zweiten Lookup eine private IP liefern).
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(dialCtx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	secureClient := &http.Client{
		Transport: secureTransport,
		CheckRedirect: func(redirectReq *http.Request, via []*http.Request) error {
			if isInternalHost(redirectReq.URL.Hostname()) {
				return fmt.Errorf("redirect to internal host blocked")
			}
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
		Timeout: 30 * time.Second,
	}

	httpReq, _ := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	dlResp, err := secureClient.Do(httpReq)
	if err != nil {
		slog.Error("import-url: download failed", "url", req.URL, "error", err)
		httputil.WriteError(w, http.StatusBadGateway, "Download Failed", "failed to download file from URL", r.URL.Path)
		return
	}
	defer dlResp.Body.Close()

	// Max 100 MB
	limitedReader := io.LimitReader(dlResp.Body, 100*1024*1024)
	fileBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		httputil.WriteError(w, http.StatusBadGateway, "Download Failed", err.Error(), r.URL.Path)
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
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", importErr.Error(), r.URL.Path)
		case importErrParse:
			httputil.WriteError(w, http.StatusUnprocessableEntity, "Parse Error", importErr.Error(), r.URL.Path)
		case importErrStorage:
			httputil.WriteError(w, http.StatusInternalServerError, "Storage Error", importErr.Error(), r.URL.Path)
		default:
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", importErr.Error(), r.URL.Path)
		}
		return
	}

	if result.Duplicate {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "duplicate_handled",
		})
		return
	}

	if result.Multisport {
		if result.Activity != nil {
			httputil.WriteJSON(w, http.StatusCreated, result.Activity)
		} else {
			httputil.WriteJSON(w, http.StatusCreated, map[string]string{"status": "multisport_imported"})
		}
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, model.UploadResponse{
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

	// Compute SHA256 hash on file bytes before any parsing
	hash := sha256.Sum256(fileBytes)
	hashStr := hex.EncodeToString(hash[:])

	// Resolve user's conflict strategy for duplicate handling
	var conflictStrategy importer.ConflictStrategy
	if importSettings, settErr := h.DB.GetImportSettings(ctx, userID); settErr == nil && importSettings != nil {
		conflictStrategy = importer.ConflictStrategy(importSettings.ConflictStrategy)
	}

	// For FIT files: try multi-session parsing first to detect multisport activities
	if format == "fit" {
		sessions, multiErr := fitimport.ParseMultiSessionFIT(fileBytes, filename)
		if multiErr == nil && len(sessions) >= 2 {
			msSz := int64(len(fileBytes))
			msTracker := h.DB.NewImportTracker(ctx, userID, "url", filename, &msSz, nil)
			msTracker.Processing(ctx)
			msTracker.SetFileHash(ctx, hashStr)

			// Upload file to ShadowFS.
			var msFileID string
			rec, putErr := h.ShadowFS.Put(ctx, shadowfs.FileEntry{
				UserID:      userID,
				FileType:    "activity",
				ContentType: "application/octet-stream",
				FileName:    filename,
				SizeBytes:   int64(len(fileBytes)),
				FileHash:    hashStr,
				Tags: map[string]string{
					"format": format,
					"source": "url_import",
				},
			}, bytes.NewReader(fileBytes))
			if putErr != nil {
				slog.Error("import_url: ShadowFS.Put multisport failed", "error", putErr)
				msTracker.Error(ctx, fmt.Errorf("ShadowFS upload: %w", putErr))
				return nil, &importError{importErrStorage, "failed to store file"}
			}
			msFileID = rec.ID

			msResult, msErr := h.ImportService.ProcessMultisport(
				ctx, userID, sessions, fileBytes, hashStr,
				msFileID, format, filename,
				importer.ImportMetadata{
					SourcePlatform:   "url",
					ConflictStrategy: conflictStrategy,
				},
			)
			if msErr != nil {
				msTracker.Error(ctx, fmt.Errorf("process multisport: %w", msErr))
				return nil, &importError{importErrGeneral, fmt.Sprintf("process multisport: %s", msErr)}
			}
			msTracker.Completed(ctx, msResult.ParentID)

			parentAct, _ := h.DB.GetActivity(ctx, msResult.ParentID)
			return &importBytesResult{Activity: parentAct, Multisport: true}, nil
		}
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

	actData.FileHash = hashStr

	if actData.SportType == "unknown" {
		slog.Warn("import_url: sport type unknown", "filename", filename, "format", format)
	}

	// Import tracking
	sz := int64(len(fileBytes))
	tracker := h.DB.NewImportTracker(ctx, userID, "url", filename, &sz, nil)
	tracker.Processing(ctx)
	tracker.SetFileHash(ctx, hashStr)

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
	result, processErr := h.ImportService.ProcessActivity(ctx, userID, actData, fileBytes, importer.ImportMetadata{SourcePlatform: "url", ConflictStrategy: conflictStrategy})
	if processErr != nil {
		slog.Error("import_url: process activity failed", "error", processErr)
		tracker.Error(ctx, processErr)
		return nil, &importError{importErrGeneral, "failed to process activity"}
	}

	// enriched/replaced are successful operations — mark as completed
	if result.Status == "enriched" || result.Status == "replaced" {
		slog.Info("import_url: activity enriched/replaced", "status", result.Status, "id", result.ActivityID)
		tracker.Completed(ctx, result.ActivityID)
		return &importBytesResult{Duplicate: true}, nil
	}

	// skipped = true duplicate, no new activity
	if result.Status == "skipped" {
		slog.Info("import_url: duplicate skipped", "id", result.ActivityID)
		tracker.Skipped(ctx, "duplicate", &result.ActivityID)
		return &importBytesResult{Duplicate: true}, nil
	}

	// New activity created: store file via ShadowFS (preferred) or legacy S3, then PostProcess
	contentType := "application/octet-stream"
	if format == "gpx" || format == "tcx" {
		contentType = "application/xml"
	}
	var fileID string
	{
		rec, putErr := h.ShadowFS.Put(ctx, shadowfs.FileEntry{
			UserID:      userID,
			FileType:    "activity",
			ContentType: contentType,
			FileName:    filename,
			SizeBytes:   int64(len(fileBytes)),
			FileHash:    hashStr,
			Tags: map[string]string{
				"format": format,
				"source": "url_import",
			},
		}, bytes.NewReader(fileBytes))
		if putErr != nil {
			slog.Error("import_url: ShadowFS.Put failed", "error", putErr)
			tracker.Error(ctx, putErr)
			return nil, &importError{importErrStorage, "failed to store file"}
		}
		fileID = rec.ID
	}

	if format == "fit" && actData.UserProfileImport != nil {
		if profErr := h.DB.SetUserProfileFieldsIfNull(ctx, userID, actData.UserProfileImport); profErr != nil {
			slog.Warn("import_url: set user profile fields failed", "error", profErr)
		}
	}

	activity, getErr := h.DB.GetActivity(ctx, result.ActivityID)
	if getErr != nil || activity == nil {
		slog.Error("import_url: failed to fetch inserted activity", "id", result.ActivityID, "error", getErr)
		tracker.Error(ctx, fmt.Errorf("failed to fetch inserted activity"))
		return nil, &importError{importErrGeneral, "failed to fetch inserted activity"}
	}

	if postErr := h.ImportService.PostProcess(ctx, userID, result.ActivityID, actData, fileID, format, filename); postErr != nil {
		slog.Error("import_url: post-process failed", "id", result.ActivityID, "error", postErr)
	}

	tracker.Completed(ctx, result.ActivityID)

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

// isPrivateIP returns true if the given net.IP is in a private/loopback/link-local range.
func isPrivateIP(ip net.IP) bool {
	return isBlockedIP(ip.String())
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

	return slices.ContainsFunc(ips, isBlockedIP)
}

// isBlockedIP checks whether a single IP string belongs to a private/loopback/link-local range.
func isBlockedIP(ipStr string) bool {
	// Block IPv4-mapped IPv6 addresses (::ffff:x.x.x.x) — extract the IPv4 part
	if after, ok := strings.CutPrefix(ipStr, "::ffff:"); ok {
		ipStr = after
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
