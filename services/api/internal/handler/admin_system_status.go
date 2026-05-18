package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/buildinfo"
	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/migrate"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/systemstatus"
	"github.com/google/uuid"
)

var processStartedAt = time.Now()

type systemStatusStorageProbe interface {
	HeadBucket(context.Context) error
	PutGetDelete(context.Context) error
	ListPrefix(context.Context) error
	MultipartAbortDryRun(context.Context) error
}

type shadowFSSystemStatusProbe struct {
	backend  shadowfs.StorageBackend
	bucketFn func(string) string
}

func (h *Handler) GetAdminSystemStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := h.collectAdminSystemStatus(ctx)
	w.Header().Set("Cache-Control", "no-store")
	httputil.WriteJSON(w, http.StatusOK, systemstatus.SanitizeSystemStatusForUI(status))
}

func (h *Handler) GetAdminSystemStatusExport(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := h.collectAdminSystemStatus(ctx)
	payload, err := systemstatus.BuildSupportExport(status)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to build diagnostic export", r.URL.Path)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="skadia-system-status.json"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (h *Handler) collectAdminSystemStatus(ctx context.Context) systemstatus.SystemStatus {
	build := buildinfo.Snapshot()
	buildMap := map[string]systemstatus.BuildInfo{
		"api": {
			Service:       "api",
			GitCommit:     build.GitCommit,
			CommitTime:    build.CommitTime,
			BuildTime:     build.BuildTime,
			ImageDigest:   build.ImageDigest,
			ConfigVersion: build.ConfigVersion,
			StartedAt:     processStartedAt.Format(time.RFC3339),
			UptimeSeconds: int64(time.Since(processStartedAt).Seconds()),
		},
	}
	if h.Config != nil {
		if frontend, ok := buildinfo.ReadFrontendManifest(ctx, h.Config.FrontendStaticDir); ok {
			buildMap["frontend"] = systemstatus.BuildInfo{
				Service:       "frontend",
				GitCommit:     frontend.GitCommit,
				CommitTime:    frontend.CommitTime,
				BuildTime:     frontend.BuildTime,
				ImageDigest:   frontend.ImageDigest,
				ConfigVersion: frontend.ConfigVersion,
			}
		}
	}
	mlSnap, mlBuild := h.collectMLSnapshot(ctx)
	if mlBuild != nil {
		buildMap["ml"] = *mlBuild
	} else if mlSnap.Configured {
		buildMap["ml"] = systemstatus.BuildInfo{Service: "ml"}
	}
	caps := h.capabilitiesForStatus(ctx)
	featureMatrix := systemstatus.FeatureMatrixFromCapabilities(caps)
	collector := systemstatus.Collector{
		Build: buildMap,
		ComponentCollectors: []systemstatus.ComponentCollector{
			h.collectAPIComponent,
			h.collectDBComponent,
			h.collectTimescaleCompressionComponent,
			h.collectStorageComponent,
			h.collectJobsComponent,
			h.collectGDPRArtifactComponent,
			func(context.Context) systemstatus.ComponentStatus {
				return h.collectNATSComponent(ctx, mlSnap.Configured && mlSnap.Live)
			},
			func(context.Context) systemstatus.ComponentStatus {
				return systemstatus.MLStatusFromSnapshot(mlSnap)
			},
			func(context.Context) systemstatus.ComponentStatus { return h.collectBRouterComponent(ctx, caps) },
			func(context.Context) systemstatus.ComponentStatus { return h.collectBadRouterComponent(ctx, caps) },
			func(context.Context) systemstatus.ComponentStatus { return h.collectOSMComponent(ctx, caps) },
			func(context.Context) systemstatus.ComponentStatus { return h.collectDEMComponent(ctx, caps) },
			func(context.Context) systemstatus.ComponentStatus { return h.collectCoverageComponent(ctx, caps) },
			func(context.Context) systemstatus.ComponentStatus { return h.collectTilesComponent(ctx, caps) },
		},
		FeatureMatrix: featureMatrix,
	}
	return collector.Collect(ctx)
}

func (h *Handler) capabilitiesForStatus(ctx context.Context) capabilities.Capabilities {
	if h == nil || h.Config == nil {
		return capabilities.BuildFromRuntime(capabilities.DetectInput{}, capabilities.RuntimeProbeState{})
	}
	if h.DB == nil {
		return capabilities.Detect(ctx, nil, h.detectInputForGate())
	}
	return capabilities.Detect(ctx, h.DB.Pool, h.detectInputForGate())
}

func (h *Handler) collectAPIComponent(ctx context.Context) systemstatus.ComponentStatus {
	return systemstatus.ComponentStatus{
		Key:       "api",
		Label:     "API",
		State:     systemstatus.StatusOK,
		Required:  true,
		CheckedAt: time.Now().UTC(),
	}
}

func (h *Handler) collectDBComponent(ctx context.Context) systemstatus.ComponentStatus {
	if h.DB == nil || h.DB.Pool == nil {
		return systemstatus.ComponentStatus{
			Key:        "db",
			Label:      "Database",
			State:      systemstatus.StatusNotConfigured,
			Required:   true,
			ReasonCode: "db_not_configured",
			CheckedAt:  time.Now().UTC(),
		}
	}
	snap := systemstatus.DBSnapshot{
		PingOK:             true,
		ExpectedExtensions: []string{"postgis", "timescaledb", "fuzzystrmatch", "vector", "postgis_raster", "pg_trgm", "pgcrypto"},
		OptionalExtensions: []string{"vectorscale"},
		Extensions:         map[string]systemstatus.ExtensionSnapshot{},
		CheckedAt:          time.Now().UTC(),
	}
	if err := h.DB.Ping(ctx); err != nil {
		snap.PingOK = false
		return systemstatus.DBStatusFromSnapshot(snap)
	}
	if status, err := migrate.ReadSchemaVersionStatus(ctx, h.DB.Pool); err == nil {
		snap.MigrationVersion = int(status.Version)
		snap.MigrationDirty = status.Dirty
	} else {
		snap.MigrationReadError = true
	}
	_ = h.DB.Pool.QueryRow(ctx, `SELECT postgis_version()`).Scan(&snap.PostGISVersion)
	_ = h.DB.Pool.QueryRow(ctx, `SHOW server_version`).Scan(&snap.ServerVersion)
	rows, err := h.DB.Pool.Query(ctx, `SELECT extname, extversion FROM pg_extension`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, version string
			if scanErr := rows.Scan(&name, &version); scanErr == nil {
				snap.Extensions[name] = systemstatus.ExtensionSnapshot{Installed: true, Version: version}
			}
		}
	}
	snap.Compression = h.collectCompressionSnapshot(ctx)
	return systemstatus.DBStatusFromSnapshot(snap)
}

func (h *Handler) collectTimescaleCompressionComponent(ctx context.Context) systemstatus.ComponentStatus {
	return systemstatus.CompressionStatusFromSnapshot(h.collectCompressionSnapshot(ctx))
}

func (h *Handler) collectCompressionSnapshot(ctx context.Context) systemstatus.CompressionSnapshot {
	snap := systemstatus.CompressionSnapshot{CheckedAt: time.Now().UTC()}
	if h.DB == nil || h.DB.Pool == nil {
		return snap
	}
	var setting string
	if err := h.DB.Pool.QueryRow(ctx, `SELECT value FROM server_settings WHERE key = 'timescale.compression'`).Scan(&setting); err == nil {
		snap.SettingEnabled = setting == "true"
	}
	_ = h.DB.Pool.QueryRow(ctx, `
		SELECT COALESCE(compression_enabled, false)
		FROM timescaledb_information.hypertables
		WHERE hypertable_name = 'activity_streams'
		LIMIT 1`).Scan(&snap.HypertableCompressed)
	_ = h.DB.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM timescaledb_information.jobs
			WHERE hypertable_name = 'activity_streams'
			  AND proc_name = 'policy_compression'
		)`).Scan(&snap.PolicyPresent)
	rows, err := h.DB.Pool.Query(ctx, `
		SELECT attname,
		       COALESCE(segmentby_column_index, 0),
		       COALESCE(orderby_column_index, 0),
		       COALESCE(orderby_asc, false)
		FROM timescaledb_information.compression_settings
		WHERE hypertable_name = 'activity_streams'`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var attname string
			var segmentIdx, orderIdx int
			var orderAsc bool
			if scanErr := rows.Scan(&attname, &segmentIdx, &orderIdx, &orderAsc); scanErr != nil {
				continue
			}
			if segmentIdx > 0 {
				snap.SegmentBy = attname
			}
			if orderIdx > 0 {
				direction := " DESC"
				if orderAsc {
					direction = " ASC"
				}
				snap.OrderBy = attname + direction
			}
		}
	}
	var compressedChunks, uncompressedChunks int64
	_ = h.DB.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE is_compressed),
		       COUNT(*) FILTER (WHERE NOT is_compressed)
		FROM timescaledb_information.chunks
		WHERE hypertable_name = 'activity_streams'`).Scan(&compressedChunks, &uncompressedChunks)
	snap.CompressedChunks = int(compressedChunks)
	snap.UncompressedChunks = int(uncompressedChunks)
	var toggleRunning, toggleFailed int64
	_ = h.DB.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE status IN ('queued','running')),
		       COUNT(*) FILTER (WHERE status = 'failed' AND COALESCE(updated_at, finished_at, started_at, NOW()) >= NOW() - INTERVAL '24 hours')
		  FROM background_jobs
		 WHERE job_type = 'compression_toggle'`).Scan(&toggleRunning, &toggleFailed)
	snap.ToggleJobsRunning = int(toggleRunning)
	snap.ToggleJobsFailed24h = int(toggleFailed)
	return snap
}

func (h *Handler) collectStorageComponent(ctx context.Context) systemstatus.ComponentStatus {
	cfg := h.Config
	if cfg == nil {
		return systemstatus.StorageStatusFromSnapshot(systemstatus.StorageSnapshot{Backend: "unknown", StorageMode: "unknown", SharedStorage: "unknown"})
	}
	snap := systemstatus.StorageSnapshot{
		Backend:       cfg.StorageBackend,
		StorageMode:   cfg.StorageMode,
		APIReplicas:   cfg.APIReplicaCount,
		SharedStorage: cfg.SharedStorage,
		CheckedAt:     time.Now().UTC(),
	}
	if snap.APIReplicas == 0 && snap.SharedStorage == "" {
		snap.SharedStorage = "unknown"
	}
	if snap.Backend == "s3" {
		probe := h.StorageProbe
		if probe == nil {
			probe = newShadowFSSystemStatusProbe(cfg.S3BE, cfg.BucketFn)
		}
		probed := systemstatus.StorageSnapshotFromProbe(ctx, snap.Backend, snap.StorageMode, probe)
		snap.HeadBucketOK = probed.HeadBucketOK
		snap.PutGetDeleteOK = probed.PutGetDeleteOK
		snap.ListPrefixOK = probed.ListPrefixOK
		snap.MultipartAbortOK = probed.MultipartAbortOK
	}
	return systemstatus.StorageStatusFromSnapshot(snap)
}

func (h *Handler) collectJobsComponent(ctx context.Context) systemstatus.ComponentStatus {
	snap := systemstatus.JobSnapshot{CheckedAt: time.Now().UTC()}
	if h.DB != nil && h.DB.Pool != nil {
		var running, stuck, failed int64
		_ = h.DB.Pool.QueryRow(ctx, `
			SELECT
			  COUNT(*) FILTER (WHERE status IN ('queued','running')) AS running,
			  COUNT(*) FILTER (WHERE status IN ('queued','running') AND COALESCE(updated_at, started_at, NOW()) < NOW() - INTERVAL '2 hours') AS stuck,
			  COUNT(*) FILTER (WHERE status = 'failed' AND COALESCE(updated_at, finished_at, started_at, NOW()) >= NOW() - INTERVAL '24 hours') AS failed_last_24h,
			  COALESCE(EXTRACT(EPOCH FROM (NOW() - (MIN(COALESCE(updated_at, started_at)) FILTER (WHERE status IN ('queued','running')))))::bigint, 0) AS oldest_age_sec
			FROM background_jobs`).Scan(&running, &stuck, &failed, &snap.OldestAgeSec)
		snap.Running = int(running)
		snap.Stuck = int(stuck)
		snap.FailedLast24h = int(failed)
	}
	return systemstatus.JobStatusFromSnapshot(snap)
}

func (h *Handler) collectGDPRArtifactComponent(ctx context.Context) systemstatus.ComponentStatus {
	snap := systemstatus.GDPRArtifactSnapshot{CheckedAt: time.Now().UTC()}
	if h.DB != nil && h.DB.Pool != nil {
		var staging string
		var activeExports, activeImports, expired int64
		_ = h.DB.Pool.QueryRow(ctx, `
			SELECT
			  COUNT(*) FILTER (WHERE j.job_type = 'gdpr_export' AND j.status IN ('queued','running')) AS active_exports,
			  COUNT(*) FILTER (WHERE j.job_type = 'data_import' AND j.status IN ('queued','running')) AS active_imports,
			  COUNT(*) FILTER (WHERE r.released_at IS NULL AND r.expires_at < NOW()) AS expired_pending_cleanup,
			  COALESCE(SUM(r.committed_bytes) FILTER (WHERE r.released_at IS NULL), 0)::text AS staging_bytes
			FROM gdpr_artifact_reservations r
			JOIN background_jobs j ON j.id = r.job_id`).Scan(&activeExports, &activeImports, &expired, &staging)
		snap.ActiveExports = int(activeExports)
		snap.ActiveImports = int(activeImports)
		snap.ExpiredPendingCleanup = int(expired)
		if parsed, err := strconv.ParseInt(staging, 10, 64); err == nil {
			snap.StagingBytes = parsed
		}
	}
	if h.GDPRArtifactCounter != nil {
		if count, err := h.GDPRArtifactCounter.CountIncompleteUploads(ctx, gdprartifacts.ArtifactPrefix{Kind: "export"}); err == nil {
			snap.IncompleteMultipart += count
		} else {
			snap.CounterErrors++
		}
		if count, err := h.GDPRArtifactCounter.CountIncompleteUploads(ctx, gdprartifacts.ArtifactPrefix{Kind: "import"}); err == nil {
			snap.IncompleteMultipart += count
		} else {
			snap.CounterErrors++
		}
	}
	return systemstatus.GDPRArtifactStatusFromSnapshot(snap)
}

func newShadowFSSystemStatusProbe(backend shadowfs.StorageBackend, bucketFn func(string) string) systemStatusStorageProbe {
	if backend == nil {
		return nil
	}
	return shadowFSSystemStatusProbe{backend: backend, bucketFn: bucketFn}
}

func (p shadowFSSystemStatusProbe) bucketNames() []string {
	if p.bucketFn == nil {
		return nil
	}
	seen := map[string]bool{}
	var buckets []string
	for _, fileType := range shadowfs.UserFacingFileTypes {
		bucket := p.bucketFn(fileType)
		if bucket == "" || seen[bucket] {
			continue
		}
		seen[bucket] = true
		buckets = append(buckets, bucket)
	}
	return buckets
}

func (p shadowFSSystemStatusProbe) optsForBucket(bucket string) *shadowfs.BackendOpts {
	if bucket == "" {
		return nil
	}
	return &shadowfs.BackendOpts{Bucket: bucket}
}

func (p shadowFSSystemStatusProbe) HeadBucket(ctx context.Context) error {
	type bucketPinger interface {
		PingBucket(context.Context, string) error
	}
	type pinger interface {
		Ping(context.Context) error
	}
	if bucket, ok := p.backend.(bucketPinger); ok {
		buckets := p.bucketNames()
		if len(buckets) > 0 {
			for _, name := range buckets {
				if err := bucket.PingBucket(ctx, name); err != nil {
					return err
				}
			}
			return nil
		}
	}
	if ping, ok := p.backend.(pinger); ok {
		return ping.Ping(ctx)
	}
	return p.ListPrefix(ctx)
}

func (p shadowFSSystemStatusProbe) PutGetDelete(ctx context.Context) error {
	buckets := p.bucketNames()
	if len(buckets) == 0 {
		return p.putGetDeleteBucket(ctx, nil)
	}
	for _, bucket := range buckets {
		if err := p.putGetDeleteBucket(ctx, p.optsForBucket(bucket)); err != nil {
			return err
		}
	}
	return nil
}

func (p shadowFSSystemStatusProbe) putGetDeleteBucket(ctx context.Context, opts *shadowfs.BackendOpts) error {
	key := ".skadia/status-probe/" + uuid.NewString()
	if _, _, err := p.backend.Write(ctx, key, bytes.NewReader([]byte("ok")), "text/plain", false, opts); err != nil {
		return err
	}
	defer func() { _ = p.backend.Delete(context.Background(), key, opts) }()
	exists, err := p.backend.Exists(ctx, key, opts)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("storage probe object missing after write")
	}
	return p.backend.Delete(ctx, key, opts)
}

func (p shadowFSSystemStatusProbe) ListPrefix(ctx context.Context) error {
	buckets := p.bucketNames()
	if len(buckets) == 0 {
		return p.backend.ListFunc(ctx, ".skadia/status-probe/", nil, func(string) error { return nil })
	}
	for _, bucket := range buckets {
		if err := p.backend.ListFunc(ctx, ".skadia/status-probe/", p.optsForBucket(bucket), func(string) error { return nil }); err != nil {
			return err
		}
	}
	return nil
}

func (p shadowFSSystemStatusProbe) MultipartAbortDryRun(ctx context.Context) error {
	type multipartLister interface {
		ListMultipartUploads(ctx context.Context, bucket, prefix string) error
	}
	if lister, ok := p.backend.(multipartLister); ok {
		buckets := p.bucketNames()
		if len(buckets) == 0 {
			return lister.ListMultipartUploads(ctx, "", ".skadia/status-probe/")
		}
		for _, bucket := range buckets {
			if err := lister.ListMultipartUploads(ctx, bucket, ".skadia/status-probe/"); err != nil {
				return err
			}
		}
	}
	return nil
}
