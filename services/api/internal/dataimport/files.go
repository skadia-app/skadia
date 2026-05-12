package dataimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// userFileRecord is the JSON shape of each entry in data/user_files.json.
type userFileRecord struct {
	ID          string  `json:"id"`
	FileType    string  `json:"file_type"`
	ContentType string  `json:"content_type"`
	FileName    string  `json:"file_name"`
	SizeBytes   int64   `json:"size_bytes"`
	FileHash    *string `json:"file_hash"`
	ParentType  *string `json:"parent_type"`
	ParentID    *string `json:"parent_id"`
	HasSidecar  bool    `json:"has_sidecar"`
	Status      string  `json:"status"`
}

type shadowFSPutStreamer interface {
	PutStream(ctx context.Context, entry shadowfs.FileEntry, data io.Reader) (shadowfs.FileRecord, error)
}

// loadUserFilesIndex reads data/user_files.json from source and returns an
// ID-keyed map of all records.
func loadUserFilesIndex(source Source) (map[string]userFileRecord, error) {
	rc, err := source.OpenDataStream("data/user_files.json")
	if err != nil {
		// Missing file index is not fatal — export may simply have no files.
		return make(map[string]userFileRecord), nil
	}
	defer rc.Close()

	var records []userFileRecord
	if err := json.NewDecoder(rc).Decode(&records); err != nil {
		return nil, fmt.Errorf("dataimport: decode user_files.json: %w", err)
	}

	index := make(map[string]userFileRecord, len(records))
	for _, rec := range records {
		index[rec.ID] = rec
	}
	return index, nil
}

func quotaRelevantUserFileBytes(source Source) (int64, error) {
	fileIndex, err := loadUserFilesIndex(source)
	if err != nil {
		return 0, err
	}

	var total int64
	for _, rec := range fileIndex {
		if rec.Status != "active" {
			continue
		}
		if rec.FileType == "thumb" || rec.FileType == "preview" {
			continue
		}
		if rec.SizeBytes > 0 {
			total += rec.SizeBytes
		}
	}
	return total, nil
}

type exportedActivityFileTags struct {
	FileID             *string   `json:"file_id"`
	OriginalFileFormat string    `json:"original_file_format"`
	Source             string    `json:"source"`
	SportType          string    `json:"sport_type"`
	SubType            string    `json:"sub_type"`
	StartedAt          time.Time `json:"started_at"`
	Title              string    `json:"title"`
	TitleAuto          string    `json:"title_auto"`
	StartCity          string    `json:"start_city"`
	StartCountry       string    `json:"start_country"`
	DistanceM          float64   `json:"distance_m"`
	ElapsedTimeS       int       `json:"elapsed_time_s"`
	ElevationGainM     float64   `json:"elevation_gain_m"`
	IsElectric         bool      `json:"is_electric"`
	IsIndoor           bool      `json:"is_indoor"`
	IsVirtual          bool      `json:"is_virtual"`
	IsCommute          bool      `json:"is_commute"`
}

func loadActivityFileTagIndex(source Source) (map[string]shadowfs.ActivitySchemaTagsInput, []string, error) {
	rc, err := source.OpenDataStream("data/activities.json")
	if err != nil {
		return map[string]shadowfs.ActivitySchemaTagsInput{}, nil, nil
	}
	defer rc.Close()

	dec := json.NewDecoder(rc)
	token, err := dec.Token()
	if err != nil {
		return map[string]shadowfs.ActivitySchemaTagsInput{}, []string{
			fmt.Sprintf("activity file tag index skipped: decode activities.json: %v", err),
		}, nil
	}
	if delim, ok := token.(json.Delim); !ok || delim != '[' {
		return map[string]shadowfs.ActivitySchemaTagsInput{}, []string{
			"activity file tag index skipped: data/activities.json is not a JSON array",
		}, nil
	}

	index := make(map[string]shadowfs.ActivitySchemaTagsInput)
	var warnings []string
	rowNumber := 0
	for dec.More() {
		rowNumber++
		var row exportedActivityFileTags
		if err := dec.Decode(&row); err != nil {
			warnings = append(warnings, fmt.Sprintf("activity file tag index skipped remaining rows after row %d: %v", rowNumber, err))
			return index, warnings, nil
		}
		if row.FileID == nil {
			continue
		}
		fileID := strings.TrimSpace(*row.FileID)
		if fileID == "" {
			warnings = append(warnings, fmt.Sprintf("activity file tag index row %d has blank file_id and was skipped", rowNumber))
			continue
		}
		if _, exists := index[fileID]; exists {
			warnings = append(warnings, fmt.Sprintf("activity file tag index row %d has duplicate file_id %q and was skipped", rowNumber, fileID))
			continue
		}
		index[fileID] = shadowfs.ActivitySchemaTagsInput{
			Format:         row.OriginalFileFormat,
			Source:         row.Source,
			StartedAt:      row.StartedAt,
			SportType:      row.SportType,
			SubType:        row.SubType,
			Title:          firstNonBlankActivityFileText(row.Title, row.TitleAuto),
			TitleAuto:      row.TitleAuto,
			City:           row.StartCity,
			Country:        row.StartCountry,
			DistanceM:      row.DistanceM,
			ElapsedTimeS:   row.ElapsedTimeS,
			ElevationGainM: row.ElevationGainM,
			IsElectric:     row.IsElectric,
			IsIndoor:       row.IsIndoor,
			IsVirtual:      row.IsVirtual,
			IsCommute:      row.IsCommute,
		}
	}
	if _, err := dec.Token(); err != nil {
		warnings = append(warnings, fmt.Sprintf("activity file tag index ended with malformed JSON: %v", err))
	}
	return index, warnings, nil
}

func firstNonBlankActivityFileText(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// importFileByID imports a single file from the export into ShadowFS and
// updates the IDMap with the new ShadowFS-assigned file ID.
func (e *Engine) importFileByID(
	ctx context.Context,
	source Source,
	idMap *IDMap,
	userID, oldFileID string,
	fileIndex map[string]userFileRecord,
	activityTags map[string]shadowfs.ActivitySchemaTagsInput,
	result *ImportResult,
) error {
	rec, ok := fileIndex[oldFileID]
	if !ok {
		slog.WarnContext(ctx, "dataimport: file not found in index, skipping",
			"old_file_id", oldFileID)
		return nil
	}

	filePath := findFilePath(source, rec)
	if filePath == "" {
		slog.WarnContext(ctx, "dataimport: file blob not found in source, skipping",
			"old_file_id", oldFileID,
			"file_type", rec.FileType,
			"file_name", rec.FileName)
		if result != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Phase 0 file %s (%s): file blob not found in source", oldFileID, rec.FileName))
		}
		return nil
	}

	fileRC, err := source.OpenFile(filePath)
	if err != nil {
		return fmt.Errorf("dataimport: open file %q: %w", filePath, err)
	}
	defer fileRC.Close()

	fileHash := ""
	if rec.FileHash != nil {
		fileHash = *rec.FileHash
	}

	// Finding 11: remap ParentID via IDMap only if parent is already imported.
	var parentType *string
	var parentID *string
	if rec.ParentType != nil && rec.ParentID != nil {
		parentType = rec.ParentType
		if newID, ok := idMap.GetIfExists(*rec.ParentID); ok {
			parentID = &newID
		}
		// else: parent not yet imported — leave nil, will be set when parent is created.
	}

	entry := shadowfs.FileEntry{
		UserID:      userID,
		FileType:    rec.FileType,
		ContentType: rec.ContentType,
		FileName:    rec.FileName,
		SizeBytes:   rec.SizeBytes,
		FileHash:    fileHash,
		ParentType:  parentType,
		ParentID:    parentID,
		HasSidecar:  rec.HasSidecar,
	}
	if isActivityFileRecord(rec) {
		if input, ok := activityTags[oldFileID]; ok {
			if input.Format == "" {
				input.Format = inferFileFormat(rec.FileName)
			}
			entry.Tags = shadowfs.ActivitySchemaTags(input)
		} else if format := inferFileFormat(rec.FileName); format != "" {
			entry.Tags = map[string]string{"format": format}
		}
	}

	var fileRecord shadowfs.FileRecord
	if streamingFS, ok := e.ShadowFS.(shadowFSPutStreamer); ok {
		fileRecord, err = streamingFS.PutStream(ctx, entry, fileRC)
	} else {
		fileRecord, err = e.ShadowFS.Put(ctx, entry, fileRC)
	}
	if err != nil {
		return fmt.Errorf("dataimport: ShadowFS restore for old_file_id=%q: %w", oldFileID, err)
	}

	// CRITICAL: map old export ID → new ShadowFS-assigned ID.
	idMap.Set(oldFileID, fileRecord.ID)
	result.importedFileIDs = append(result.importedFileIDs, fileRecord.ID)

	result.FilesImported++
	result.BytesImported += rec.SizeBytes

	// Finding 13: set status='archived' after Put (which creates with status='active').
	if rec.Status == "archived" {
		if _, err := e.DB.Exec(ctx, "UPDATE user_files SET status = 'archived' WHERE id = $1", fileRecord.ID); err != nil {
			return fmt.Errorf("dataimport: set archived status for file %q: %w", fileRecord.ID, err)
		}
	}

	if rec.HasSidecar {
		// Finding 14: use the actual resolved filePath for the companion, not a reconstructed path.
		metaPath := filePath + ".meta"
		if err := e.importCompanion(ctx, source, userID, fileRecord.ID, metaPath); err != nil {
			return fmt.Errorf("dataimport: importCompanion for file %q: %w", fileRecord.ID, err)
		}
	}

	return nil
}

// importCompanion imports the .meta sidecar companion for a file.
// A missing companion file is treated as non-fatal (returns nil).
func (e *Engine) importCompanion(
	ctx context.Context,
	source Source,
	userID, newFileID, metaPath string,
) error {
	rc, err := source.OpenFile(metaPath)
	if err != nil {
		// Companion not present in export — non-fatal.
		slog.DebugContext(ctx, "dataimport: companion file not found, skipping",
			"meta_path", metaPath,
			"new_file_id", newFileID)
		return nil
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("dataimport: read companion %q: %w", metaPath, err)
	}

	if err := e.ShadowFS.PutCompanion(ctx, userID, newFileID, "meta", data); err != nil {
		return fmt.Errorf("dataimport: PutCompanion for file %q: %w", newFileID, err)
	}

	return nil
}

// findFilePath resolves the path for a file blob within the export source.
// For archived records: tries files/_versions/<file_type>/<filename> first.
// Primary pattern: files/<file_type>/<filename>
// Fallback: linear scan of ListFiles() for any path ending in /<filename>.
func findFilePath(source Source, rec userFileRecord) string {
	// Finding 13: archived files live under files/_versions/.
	if rec.Status == "archived" {
		versioned := fmt.Sprintf("files/_versions/%s/%s", rec.FileType, rec.FileName)
		if rc, err := source.OpenFile(versioned); err == nil {
			rc.Close()
			return versioned
		}
	}

	primary := fmt.Sprintf("files/%s/%s", rec.FileType, rec.FileName)
	if rc, err := source.OpenFile(primary); err == nil {
		rc.Close()
		return primary
	}

	// Fallback: scan all paths for a matching filename suffix.
	paths, err := source.ListFiles()
	if err != nil {
		return ""
	}

	suffix := "/" + rec.FileName
	for _, p := range paths {
		if strings.HasPrefix(p, "files/") && strings.HasSuffix(p, suffix) {
			return p
		}
	}

	return ""
}

func isActivityFileRecord(rec userFileRecord) bool {
	if rec.FileType == "activity" {
		return true
	}
	if rec.FileType == "original" || rec.FileType == "" {
		return inferFileFormat(rec.FileName) != ""
	}
	return false
}

func inferFileFormat(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	idx := strings.LastIndex(fileName, ".")
	if idx < 0 || idx == len(fileName)-1 {
		return ""
	}
	switch format := strings.ToLower(fileName[idx+1:]); format {
	case "fit", "gpx", "tcx":
		return format
	default:
		return ""
	}
}

// importAllFiles imports ALL user files from the export into ShadowFS (Phase 0).
// Edit-sidecars (file_type="edit") are skipped — they require ParentID=activity_id
// which is not yet available. Use importEditSidecars after activities are imported.
// Returns the fileIndex for use by importEditSidecars in Phase 6.
func (e *Engine) importAllFiles(
	ctx context.Context,
	source Source,
	idMap *IDMap,
	userID string,
	result *ImportResult,
	onProgress func(ProgressEvent),
	activityTags map[string]shadowfs.ActivitySchemaTagsInput,
) (map[string]userFileRecord, error) {
	fileIndex, err := loadUserFilesIndex(source)
	if err != nil {
		return nil, err
	}

	total := 0
	for _, rec := range fileIndex {
		if rec.FileType != "edit" {
			total++
		}
	}
	imported := 0

	for oldFileID, rec := range fileIndex {
		// Skip edit-sidecars — they need ParentID=activity_id (Phase 6).
		if rec.FileType == "edit" {
			continue
		}

		if err := e.importFileByID(ctx, source, idMap, userID, oldFileID, fileIndex, activityTags, result); err != nil {
			if errors.Is(err, shadowfs.ErrQuotaExceeded) {
				return fileIndex, fmt.Errorf("Phase 0 file %s (%s): %w", oldFileID, rec.FileName, err)
			}
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Phase 0 file %s (%s): %v", oldFileID, rec.FileName, err))
			continue // Non-fatal: skip this file, continue with others.
		}

		imported++
		if onProgress != nil && imported%100 == 0 {
			onProgress(ProgressEvent{
				Phase:         0,
				PhaseName:     "files",
				Message:       fmt.Sprintf("Importing files... %d/%d", imported, total),
				FilesImported: result.FilesImported,
				BytesImported: result.BytesImported,
			})
		}
	}

	return fileIndex, nil
}

// importEditSidecars imports edit-sidecar files (file_type="edit") that were
// skipped in Phase 0. Called in Phase 6 after activities exist in the IDMap.
// fileIndex is the same map returned by importAllFiles.
func (e *Engine) importEditSidecars(
	ctx context.Context,
	source Source,
	idMap *IDMap,
	userID string,
	fileIndex map[string]userFileRecord,
	result *ImportResult,
) error {
	for oldFileID, rec := range fileIndex {
		if rec.FileType != "edit" {
			continue
		}
		// Skip if already imported (shouldn't happen but be safe).
		if _, exists := idMap.GetIfExists(oldFileID); exists {
			continue
		}
		// importFileByID handles ParentID remapping via idMap.GetIfExists — no
		// pre-remap needed here. The activity is in IDMap after Phase 5, so
		// importFileByID will find actOldID → actNewID correctly.
		if err := e.importFileByID(ctx, source, idMap, userID, oldFileID, fileIndex, nil, result); err != nil {
			if errors.Is(err, shadowfs.ErrQuotaExceeded) {
				return fmt.Errorf("Edit sidecar %s: %w", oldFileID, err)
			}
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Edit sidecar %s: %v", oldFileID, err))
		}
	}
	return nil
}
