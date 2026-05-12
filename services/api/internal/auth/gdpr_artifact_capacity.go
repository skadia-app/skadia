package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/jackc/pgx/v5"
)

const (
	defaultExportReservationBytes = int64(4) << 30
	exportReservationPaddingBytes = int64(256) << 20
)

type gdprArtifactCapacityRequest struct {
	JobID         string
	UserID        string
	Backend       string
	Kind          string
	ReservedBytes int64
	ExpiresAt     time.Time
}

func (h *Handler) reserveGDPRArtifactCapacity(ctx context.Context, req gdprArtifactCapacityRequest) error {
	if h == nil || h.DB == nil || h.DB.Pool == nil {
		return errors.New("database unavailable")
	}
	tx, err := h.DB.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := h.reserveGDPRArtifactCapacityTx(ctx, tx, req); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *Handler) reserveGDPRArtifactCapacityTx(ctx context.Context, tx pgx.Tx, req gdprArtifactCapacityRequest) error {
	if req.ReservedBytes < 0 {
		return errors.New("reserved bytes must not be negative")
	}
	if req.Backend == "" {
		req.Backend = h.gdprArtifactBackendName()
	}
	if req.ExpiresAt.IsZero() {
		req.ExpiresAt = time.Now().UTC().Add(dataExportDownloadTTL)
	}
	if err := lockGDPRArtifactCapacity(ctx, tx); err != nil {
		return err
	}
	if err := h.checkGDPRArtifactCapacityTx(ctx, tx, req.UserID, req.Backend, req.Kind, req.ReservedBytes, true); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO gdpr_artifact_reservations
			(job_id, user_id, backend, kind, reserved_bytes, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (job_id) DO UPDATE
		SET reserved_bytes = EXCLUDED.reserved_bytes,
		    expires_at = EXCLUDED.expires_at,
		    released_at = NULL,
		    updated_at = NOW()`,
		req.JobID, req.UserID, req.Backend, req.Kind, req.ReservedBytes, req.ExpiresAt)
	return err
}

func (h *Handler) growGDPRArtifactCapacity(ctx context.Context, jobID, userID string, additionalBytes int64) error {
	if additionalBytes <= 0 {
		return nil
	}
	tx, err := h.DB.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockGDPRArtifactCapacity(ctx, tx); err != nil {
		return err
	}
	var backend, kind string
	if err := tx.QueryRow(ctx, `
		SELECT backend, kind
		FROM gdpr_artifact_reservations
		WHERE job_id = $1 AND user_id = $2 AND released_at IS NULL`,
		jobID, userID).Scan(&backend, &kind); err != nil {
		return err
	}
	if err := h.checkGDPRArtifactCapacityTx(ctx, tx, userID, backend, kind, additionalBytes, true); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE gdpr_artifact_reservations
		SET reserved_bytes = reserved_bytes + $3,
		    updated_at = NOW()
		WHERE job_id = $1 AND user_id = $2 AND released_at IS NULL`,
		jobID, userID, additionalBytes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *Handler) commitGDPRArtifactCapacity(ctx context.Context, jobID, userID string, committedBytes int64) error {
	if h == nil || h.DB == nil || h.DB.Pool == nil {
		return nil
	}
	if committedBytes < 0 {
		committedBytes = 0
	}
	tx, err := h.DB.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockGDPRArtifactCapacity(ctx, tx); err != nil {
		return err
	}
	var reserved int64
	var backend, kind string
	if err := tx.QueryRow(ctx, `
		SELECT reserved_bytes, backend, kind
		FROM gdpr_artifact_reservations
		WHERE job_id = $1 AND user_id = $2 AND released_at IS NULL`,
		jobID, userID).Scan(&reserved, &backend, &kind); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if committedBytes > reserved {
		if err := h.checkGDPRArtifactCapacityTx(ctx, tx, userID, backend, kind, committedBytes-reserved, false); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE gdpr_artifact_reservations
		SET committed_bytes = $3,
		    reserved_bytes = GREATEST(reserved_bytes, $3),
		    updated_at = NOW()
		WHERE job_id = $1 AND user_id = $2 AND released_at IS NULL`,
		jobID, userID, committedBytes)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *Handler) releaseGDPRArtifactCapacity(ctx context.Context, jobID, userID string, committedBytes int64) error {
	if h == nil || h.DB == nil || h.DB.Pool == nil {
		return nil
	}
	if committedBytes < 0 {
		committedBytes = 0
	}
	_, err := h.DB.Pool.Exec(ctx, `
		UPDATE gdpr_artifact_reservations
		SET committed_bytes = $3,
		    released_at = COALESCE(released_at, NOW()),
		    updated_at = NOW()
		WHERE job_id = $1 AND user_id = $2`,
		jobID, userID, committedBytes)
	return err
}

func (h *Handler) checkGDPRArtifactCapacityTx(ctx context.Context, tx pgx.Tx, userID, backend, kind string, additionalBytes int64, checkStore bool) error {
	if h == nil || h.Config == nil {
		return nil
	}
	if limit := h.Config.GDPRArtifactUserActiveBytes; limit > 0 {
		active, err := activeArtifactBytes(ctx, tx, `user_id = $1::uuid`, userID)
		if err != nil {
			return err
		}
		if active+additionalBytes > limit {
			return gdprartifacts.CapacityError{Reason: "user_active_quota_exceeded"}
		}
	}
	switch backend {
	case "local":
		if limit := h.Config.GDPRArtifactLocalQuotaBytes; limit > 0 {
			active, err := activeArtifactBytes(ctx, tx, `backend = 'local'`)
			if err != nil {
				return err
			}
			if active+additionalBytes > limit {
				return gdprartifacts.CapacityError{Reason: "local_quota_exceeded"}
			}
		}
	case "s3":
		if limit := h.Config.GDPRArtifactS3QuotaBytes; limit > 0 {
			active, err := activeArtifactBytes(ctx, tx, `backend = 's3'`)
			if err != nil {
				return err
			}
			if active+additionalBytes > limit {
				return gdprartifacts.CapacityError{Reason: "s3_quota_exceeded"}
			}
		}
	}
	if checkStore {
		checker, ok := h.ArtifactStore.(gdprartifacts.CapacityChecker)
		if !ok {
			return nil
		}
		result, err := checker.CheckAvailable(ctx, gdprartifacts.CapacityRequest{
			Kind:          kind,
			UserID:        userID,
			ExpectedBytes: additionalBytes,
		})
		if err != nil {
			return err
		}
		if !result.Allowed {
			return gdprartifacts.CapacityError{Reason: result.Reason}
		}
	}
	return nil
}

func activeArtifactBytes(ctx context.Context, tx pgx.Tx, predicate string, args ...any) (int64, error) {
	query := fmt.Sprintf(`
		SELECT COALESCE(SUM(reserved_bytes), 0)
		FROM gdpr_artifact_reservations
		WHERE released_at IS NULL
		  AND %s`, predicate)
	var active int64
	if err := tx.QueryRow(ctx, query, args...).Scan(&active); err != nil {
		return 0, err
	}
	return active, nil
}

func lockGDPRArtifactCapacity(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(47002137, 1)`)
	return err
}

func (h *Handler) gdprArtifactBackendName() string {
	if h == nil || h.Config == nil {
		return "local"
	}
	backend := h.Config.GDPRArtifactBackend
	if backend == "" || backend == "auto" {
		backend = h.Config.StorageBackend
	}
	if backend == "s3" {
		return "s3"
	}
	return "local"
}

func (h *Handler) estimateGDPRExportReservationBytes(ctx context.Context, userID string) int64 {
	if h != nil && h.ShadowFS != nil {
		if usage, err := h.ShadowFS.DiskUsage(ctx, userID); err == nil && usage > 0 {
			return usage + exportReservationPaddingBytes
		}
	}
	return defaultExportReservationBytes
}

func isArtifactCapacityError(err error) bool {
	return errors.Is(err, gdprartifacts.ErrCapacityExceeded)
}

func totalArtifactBytes(artifact gdprartifacts.Artifact) int64 {
	var total int64
	for _, ref := range artifact.Objects {
		total += ref.SizeBytes
	}
	return total
}
