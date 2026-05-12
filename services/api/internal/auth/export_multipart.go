package auth

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/dataimport"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/jackc/pgx/v5"
)

type exportFileStreamer interface {
	StreamExportFiles(context.Context, string, shadowfs.ExportFileSink) ([]string, error)
}

func (h *Handler) shouldUseMultipartExport(ctx context.Context, userID string) bool {
	if h == nil || h.Config == nil || h.ShadowFS == nil {
		return false
	}
	threshold := h.Config.GDPRExportMultipartThresholdBytes
	if threshold <= 0 {
		return false
	}
	usage, err := h.ShadowFS.DiskUsage(ctx, userID)
	if err != nil {
		return false
	}
	return usage >= threshold
}

func (h *Handler) executeMultipartExportJobOnce(
	ctx context.Context,
	jobID, userID string,
	tx pgx.Tx,
	schemaMigration int,
	colCache map[string][]columnInfo,
	specs []gdprExportTableSpec,
	healthTables []string,
	introspectErrs []exportTableError,
) error {
	if h.ArtifactStore == nil {
		return fmt.Errorf("artifact_store_unconfigured")
	}
	streamer, ok := h.ShadowFS.(exportFileStreamer)
	if !ok {
		return fmt.Errorf("shadowfs_stream_export_unconfigured")
	}

	expiresAt := time.Now().UTC().Add(dataExportDownloadTTL)
	metadataWriter, err := h.ArtifactStore.CreateWriter(ctx, gdprartifacts.ObjectSpec{
		Kind:        "export",
		UserID:      userID,
		JobID:       jobID,
		Role:        gdprartifacts.RoleMetadataZIP,
		Name:        "metadata.zip",
		ContentType: "application/zip",
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return fmt.Errorf("create metadata artifact writer: %w", err)
	}
	metadataCommitted := false
	defer func() {
		if !metadataCommitted {
			_ = metadataWriter.Abort(ctx)
		}
	}()

	zw := zip.NewWriter(metadataWriter)
	entityCounts, exportErrs := h.exportAllTables(ctx, zw, tx, userID, colCache, specs, healthTables)
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	writeZipJSON(zw, "columns.json", columnsMapToExported(colCache))
	if err := zw.Close(); err != nil {
		return fmt.Errorf("close metadata zip: %w", err)
	}
	metadataRef, err := metadataWriter.Commit(ctx)
	if err != nil {
		return fmt.Errorf("commit metadata artifact: %w", err)
	}
	metadataCommitted = true

	fileSink := newMultipartArtifactFileSink(h.ArtifactStore, userID, jobID, expiresAt, h.multipartPartTargetBytes(), h.multipartMaxFileParts())
	failedFiles, streamErr := streamer.StreamExportFiles(ctx, userID, fileSink)
	if closeErr := fileSink.Close(ctx); closeErr != nil && streamErr == nil {
		streamErr = closeErr
	}
	if streamErr != nil {
		fileSink.Abort(ctx)
		return fmt.Errorf("stream export files: %w", streamErr)
	}
	fileRefs := fileSink.Refs()

	allErrs := append([]exportTableError{}, introspectErrs...)
	allErrs = append(allErrs, exportErrs...)
	manifest := dataimport.Manifest{
		FormatVersion:    1,
		SkadiaVersion:   skadiaExportVersion,
		SchemaMigration:  schemaMigration,
		ExportedAt:       time.Now().UTC(),
		UserID:           userID,
		Multipart:        true,
		EntityCounts:     entityCounts,
		Categories:       gdprCategoriesForManifest(),
		FailedFiles:      failedFiles,
		IncompleteTables: incompleteTablesFromExportErrors(allErrs),
		ExportErrors:     manifestExportErrors(allErrs),
	}
	manifest.Parts = append(manifest.Parts, dataimport.ManifestPart{
		Name:      metadataRef.Name,
		Role:      "metadata",
		SHA256:    metadataRef.SHA256,
		SizeBytes: metadataRef.SizeBytes,
	})
	for _, ref := range fileRefs {
		supported := ref.SizeBytes <= 4<<30
		manifest.Parts = append(manifest.Parts, dataimport.ManifestPart{
			Name:               ref.Name,
			Role:               "files",
			SHA256:             ref.SHA256,
			SizeBytes:          ref.SizeBytes,
			WebImportSupported: &supported,
		})
	}

	manifestRef, err := h.writeManifestArtifact(ctx, userID, jobID, expiresAt, manifest)
	if err != nil {
		return err
	}

	objects := make([]gdprartifacts.ObjectRef, 0, 2+len(fileRefs))
	objects = append(objects, manifestRef, metadataRef)
	objects = append(objects, fileRefs...)
	artifact := gdprartifacts.Artifact{
		Version:   1,
		Format:    gdprartifacts.ArtifactFormatMultipart,
		ExpiresAt: expiresAt,
		Objects:   objects,
		Multipart: &gdprartifacts.MultipartSummary{
			PartCount:      len(manifest.Parts),
			TotalSizeBytes: totalObjectRefBytes(objects),
		},
	}
	metadata, err := buildMultipartExportArtifactMetadata(artifact, manifestRef)
	if err != nil {
		return fmt.Errorf("build multipart metadata: %w", err)
	}
	status := "completed"
	var topMsg string
	if len(allErrs) > 0 {
		status = "failed"
		topMsg = fmt.Sprintf("export incomplete: %d table(s) errored, first failure on %q", len(allErrs), allErrs[0].Table)
	}
	if _, err := h.DB.Pool.Exec(ctx,
		`UPDATE background_jobs SET
			status = $2,
			error = NULLIF($3, ''),
			metadata = $4::jsonb,
			updated_at = NOW()
		 WHERE id = $1::uuid`,
		jobID, status, topMsg, string(metadata)); err != nil {
		return fmt.Errorf("store multipart metadata: %w", err)
	}
	if status == "failed" {
		return fmt.Errorf("%s", topMsg)
	}
	h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
	return nil
}

func (h *Handler) multipartPartTargetBytes() int64 {
	if h != nil && h.Config != nil && h.Config.GDPRExportPartTargetBytes > 0 {
		return h.Config.GDPRExportPartTargetBytes
	}
	return 2 << 30
}

func (h *Handler) multipartMaxFileParts() int {
	maxParts := 256
	if h != nil && h.Config != nil && h.Config.GDPRMultipartMaxParts > 0 {
		maxParts = h.Config.GDPRMultipartMaxParts
	}
	if maxParts <= 1 {
		return 1
	}
	return maxParts - 1
}

func (h *Handler) writeManifestArtifact(ctx context.Context, userID, jobID string, expiresAt time.Time, manifest dataimport.Manifest) (gdprartifacts.ObjectRef, error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return gdprartifacts.ObjectRef{}, fmt.Errorf("marshal multipart manifest: %w", err)
	}
	writer, err := h.ArtifactStore.CreateWriter(ctx, gdprartifacts.ObjectSpec{
		Kind:        "export",
		UserID:      userID,
		JobID:       jobID,
		Role:        gdprartifacts.RoleManifest,
		Name:        "manifest.json",
		ContentType: "application/json",
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return gdprartifacts.ObjectRef{}, fmt.Errorf("create manifest artifact writer: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = writer.Abort(ctx)
		}
	}()
	if _, err := writer.Write(raw); err != nil {
		return gdprartifacts.ObjectRef{}, fmt.Errorf("write manifest artifact: %w", err)
	}
	ref, err := writer.Commit(ctx)
	if err != nil {
		return gdprartifacts.ObjectRef{}, fmt.Errorf("commit manifest artifact: %w", err)
	}
	committed = true
	return ref, nil
}

func buildMultipartExportArtifactMetadata(artifact gdprartifacts.Artifact, primary gdprartifacts.ObjectRef) ([]byte, error) {
	raw, err := gdprartifacts.MergeArtifactJSON([]byte(`{}`), artifact)
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	sha, _ := json.Marshal(primary.SHA256)
	size, _ := json.Marshal(primary.SizeBytes)
	obj["sha256"] = sha
	obj["size_bytes"] = size
	return json.Marshal(obj)
}

func incompleteTablesFromExportErrors(errs []exportTableError) []string {
	if len(errs) == 0 {
		return nil
	}
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		out = append(out, err.Table)
	}
	return out
}

func manifestExportErrors(errs []exportTableError) []dataimport.ManifestExportError {
	if len(errs) == 0 {
		return nil
	}
	out := make([]dataimport.ManifestExportError, 0, len(errs))
	for _, err := range errs {
		out = append(out, dataimport.ManifestExportError{Table: err.Table, Error: err.Error})
	}
	return out
}

func totalObjectRefBytes(refs []gdprartifacts.ObjectRef) int64 {
	var total int64
	for _, ref := range refs {
		total += ref.SizeBytes
	}
	return total
}

type multipartArtifactFileSink struct {
	store     gdprartifacts.Store
	userID    string
	jobID     string
	expiresAt time.Time
	target    int64
	maxParts  int

	currentWriter gdprartifacts.ObjectWriter
	currentZip    *zip.Writer
	currentName   string
	currentBytes  int64
	currentFiles  int
	nextIndex     int
	refs          []gdprartifacts.ObjectRef
}

func newMultipartArtifactFileSink(store gdprartifacts.Store, userID, jobID string, expiresAt time.Time, target int64, maxParts int) *multipartArtifactFileSink {
	if target <= 0 {
		target = 2 << 30
	}
	if maxParts <= 0 {
		maxParts = 1
	}
	return &multipartArtifactFileSink{store: store, userID: userID, jobID: jobID, expiresAt: expiresAt, target: target, maxParts: maxParts, nextIndex: 1}
}

func (s *multipartArtifactFileSink) AddExportFile(ctx context.Context, file shadowfs.ExportFile, data io.Reader) error {
	if s.currentZip != nil && s.currentFiles > 0 && s.currentBytes >= s.target && len(s.refs) < s.maxParts-1 {
		if err := s.closeCurrent(ctx); err != nil {
			return err
		}
	}
	if s.currentZip == nil {
		if err := s.openNext(ctx); err != nil {
			return err
		}
	}
	w, err := s.currentZip.CreateHeader(&zip.FileHeader{
		Name:     file.ZIPPath,
		Method:   zip.Deflate,
		Modified: file.Modified,
	})
	if err != nil {
		return err
	}
	n, err := io.Copy(w, data)
	s.currentBytes += max(n, file.SizeBytes)
	s.currentFiles++
	return err
}

func (s *multipartArtifactFileSink) Close(ctx context.Context) error {
	if s.currentZip == nil {
		return nil
	}
	return s.closeCurrent(ctx)
}

func (s *multipartArtifactFileSink) Abort(ctx context.Context) {
	if s.currentWriter != nil {
		_ = s.currentWriter.Abort(ctx)
	}
	for _, ref := range s.refs {
		_ = s.store.Delete(ctx, ref)
	}
}

func (s *multipartArtifactFileSink) Refs() []gdprartifacts.ObjectRef {
	out := make([]gdprartifacts.ObjectRef, len(s.refs))
	copy(out, s.refs)
	return out
}

func (s *multipartArtifactFileSink) openNext(ctx context.Context) error {
	name := fmt.Sprintf("files-%04d.zip", s.nextIndex)
	writer, err := s.store.CreateWriter(ctx, gdprartifacts.ObjectSpec{
		Kind:        "export",
		UserID:      s.userID,
		JobID:       s.jobID,
		Role:        gdprartifacts.RoleFilesZIP,
		Name:        name,
		ContentType: "application/zip",
		ExpiresAt:   s.expiresAt,
	})
	if err != nil {
		return err
	}
	s.currentWriter = writer
	s.currentZip = zip.NewWriter(writer)
	s.currentName = name
	s.currentBytes = 0
	s.currentFiles = 0
	s.nextIndex++
	return nil
}

func (s *multipartArtifactFileSink) closeCurrent(ctx context.Context) error {
	if s.currentZip == nil {
		return nil
	}
	if s.currentFiles == 0 {
		_ = s.currentZip.Close()
		_ = s.currentWriter.Abort(ctx)
		s.currentZip = nil
		s.currentWriter = nil
		return nil
	}
	if err := s.currentZip.Close(); err != nil {
		_ = s.currentWriter.Abort(ctx)
		return fmt.Errorf("close %s: %w", s.currentName, err)
	}
	ref, err := s.currentWriter.Commit(ctx)
	if err != nil {
		return fmt.Errorf("commit %s: %w", s.currentName, err)
	}
	s.refs = append(s.refs, ref)
	s.currentZip = nil
	s.currentWriter = nil
	s.currentName = ""
	s.currentBytes = 0
	s.currentFiles = 0
	return nil
}

var _ shadowfs.ExportFileSink = (*multipartArtifactFileSink)(nil)
