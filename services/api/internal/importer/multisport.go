package importer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
)

// MultisportResult is returned by ProcessMultisport.
type MultisportResult struct {
	ParentID      string
	ParentCreated bool
	Children      []*ActivityResult
}

// ProcessMultisport handles multi-session FIT files (e.g. triathlons).
// It creates a multisport parent (if not already present via file hash dedup),
// then routes each child session through ProcessActivity + PostProcess.
// Each child gets an individual file hash: SHA256(originalHash + ":" + sessionIndex).
func (s *Service) ProcessMultisport(
	ctx context.Context,
	userID string,
	sessions []*model.ActivityData,
	fileBytes []byte,
	fileHash string,
	fileID string,
	format string,
	filename string,
	meta ImportMetadata,
) (*MultisportResult, error) {
	if len(sessions) < 2 {
		return nil, fmt.Errorf("ProcessMultisport requires >= 2 sessions, got %d", len(sessions))
	}

	result := &MultisportResult{}

	// 1. Parent dedup: check if multisport parent with this file hash already exists
	parentID, err := s.DB.FindMultisportParentByHash(ctx, userID, fileHash)
	if err != nil {
		slog.Warn("multisport parent lookup failed", "err", err)
	}

	if parentID != "" {
		// Parent already exists (re-import of same file)
		result.ParentID = parentID
		result.ParentCreated = false
		// Link file_id on parent when ShadowFS is in use
		if s.ShadowFS != nil && fileID != "" {
			if _, execErr := s.DB.Pool.Exec(ctx,
				`UPDATE activities SET file_id = $1 WHERE id = $2`,
				fileID, result.ParentID,
			); execErr != nil {
				slog.Warn("multisport: set parent file_id failed", "parent", result.ParentID, "err", execErr)
			}
		}
	} else {
		// Create new parent
		parent := &model.Activity{
			ID:        newUUID(),
			UserID:    userID,
			SportType: "multisport",
			Title:     new("Multisport"),
			StartedAt: sessions[0].StartedAt,
			FileHash:  &fileHash,
		}
		if err := s.DB.InsertMultisportParent(ctx, parent); err != nil {
			return nil, fmt.Errorf("insert multisport parent: %w", err)
		}
		result.ParentID = parent.ID
		result.ParentCreated = true
		// Link file_id on parent when ShadowFS is in use
		if s.ShadowFS != nil && fileID != "" {
			if _, execErr := s.DB.Pool.Exec(ctx,
				`UPDATE activities SET file_id = $1 WHERE id = $2`,
				fileID, result.ParentID,
			); execErr != nil {
				slog.Warn("multisport: set parent file_id failed", "parent", result.ParentID, "err", execErr)
			}
		}
	}

	// 2. Process each child session
	for i, childData := range sessions {
		// Skip multisport summary sessions
		if childData.SportType == "multisport" {
			continue
		}

		// Individual hash per child to avoid FileHash collisions
		childHash := fmt.Sprintf("%x", sha256.Sum256([]byte(fileHash+":"+fmt.Sprintf("%d", i))))
		childData.FileHash = childHash

		childMeta := meta
		childMeta.ParentID = result.ParentID

		actResult, err := s.ProcessActivity(ctx, userID, childData, fileBytes, childMeta)
		if err != nil {
			slog.Warn("multisport child import failed", "session", i, "sport", childData.SportType, "err", err)
			continue
		}
		result.Children = append(result.Children, actResult)

		// PostProcess only for newly created activities
		if actResult.Status == "created" {
			if err := s.PostProcess(ctx, userID, actResult.ActivityID, childData, fileID, format, filename); err != nil {
				slog.Warn("multisport child postprocess failed", "activity", actResult.ActivityID, "err", err)
			}
		}
	}

	// 3. Re-aggregate parent metrics (always, to pick up any new/enriched children)
	if err := s.DB.AggregateParentMetrics(ctx, result.ParentID); err != nil {
		slog.Warn("aggregate parent metrics failed", "parent", result.ParentID, "err", err)
	}

	// Write .meta companion for multisport parent.
	parentAct, _ := s.DB.GetActivity(ctx, result.ParentID)
	if parentAct != nil && fileID != "" {
		sidecar.WriteMetaCompanion(ctx, s.ShadowFS, s.DB, userID, result.ParentID, fileID, nil, parentAct)
	}

	return result, nil
}

func newUUID() string { return uuid.New().String() }
