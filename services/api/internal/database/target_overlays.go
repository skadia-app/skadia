package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

var ErrTargetOverlayValidation = errors.New("target overlay validation failed")

func validateOverlaySubjectKind(kind string) error {
	switch strings.TrimSpace(kind) {
	case "course", "route_publication", "place", "region", "collection":
		return nil
	default:
		return fmt.Errorf("%w: invalid overlay subject kind %q", ErrTargetOverlayValidation, kind)
	}
}

func validateOverlayVisibility(visibility string, accessScopeID *string) error {
	switch strings.TrimSpace(visibility) {
	case "", "public", "private":
		if accessScopeID != nil && strings.TrimSpace(*accessScopeID) != "" {
			return fmt.Errorf("%w: access_scope_id is only valid for shared overlays", ErrTargetOverlayValidation)
		}
	case "shared":
		if accessScopeID == nil || strings.TrimSpace(*accessScopeID) == "" {
			return fmt.Errorf("%w: shared overlays require access_scope_id", ErrTargetOverlayValidation)
		}
	default:
		return fmt.Errorf("%w: invalid overlay visibility %q", ErrTargetOverlayValidation, visibility)
	}
	return nil
}

func validateObservationType(observationType string) error {
	switch strings.TrimSpace(observationType) {
	case "matched_traversal", "visit", "dwell", "check_in", "completion":
		return nil
	default:
		return fmt.Errorf("%w: invalid observation type %q", ErrTargetOverlayValidation, observationType)
	}
}

func validateReportType(reportType string) error {
	switch strings.TrimSpace(reportType) {
	case "condition", "issue", "warning", "trail_work", "closure", "metadata":
		return nil
	default:
		return fmt.Errorf("%w: invalid report type %q", ErrTargetOverlayValidation, reportType)
	}
}

func validateReportStatus(status string) error {
	switch strings.TrimSpace(status) {
	case "open", "resolved", "rejected", "superseded", "informational", "redacted":
		return nil
	default:
		return fmt.Errorf("%w: invalid report status %q", ErrTargetOverlayValidation, status)
	}
}

func validateUserContentKind(kind string) error {
	switch strings.TrimSpace(kind) {
	case "comment", "rating", "save", "completion", "check_in":
		return nil
	default:
		return fmt.Errorf("%w: invalid target content kind %q", ErrTargetOverlayValidation, kind)
	}
}

func validateRatingPayload(payload json.RawMessage) error {
	var body struct {
		Rating *float64 `json:"rating"`
	}
	if err := json.Unmarshal(rawOrDefault(payload, `{}`), &body); err != nil {
		return fmt.Errorf("%w: rating payload must be valid JSON", ErrTargetOverlayValidation)
	}
	if body.Rating == nil {
		return fmt.Errorf("%w: rating payload requires rating", ErrTargetOverlayValidation)
	}
	if *body.Rating < 1 || *body.Rating > 5 {
		return fmt.Errorf("%w: rating must be between 1 and 5", ErrTargetOverlayValidation)
	}
	return nil
}

func validateAttributionMode(mode string) error {
	switch strings.TrimSpace(mode) {
	case "", "named", "anonymous_public", "moderator_only", "hidden":
		return nil
	default:
		return fmt.Errorf("%w: invalid attribution mode %q", ErrTargetOverlayValidation, mode)
	}
}

func validateTargetOverlayVersion(ctx context.Context, q Querier, subjectKind, subjectID string, targetVersionID *string) error {
	if targetVersionID == nil || strings.TrimSpace(*targetVersionID) == "" {
		return nil
	}
	switch strings.TrimSpace(subjectKind) {
	case "course":
		var exists bool
		if err := q.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM segment_versions
				WHERE id = $1 AND segment_id = $2
			)
		`, *targetVersionID, subjectID).Scan(&exists); err != nil {
			return fmt.Errorf("validate target overlay course version: %w", err)
		}
		if !exists {
			return ErrTargetNotFound
		}
	case "route_publication":
		var exists bool
		if err := q.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM route_course_publications
				WHERE id = $1 AND course_segment_version_id = $2 AND retired_at IS NULL
			)
		`, subjectID, *targetVersionID).Scan(&exists); err != nil {
			return fmt.Errorf("validate target overlay route publication version: %w", err)
		}
		if !exists {
			return ErrTargetNotFound
		}
	default:
		return fmt.Errorf("%w: target_version_id is only valid for course and route publication overlays", ErrTargetOverlayValidation)
	}
	return nil
}

func (db *DB) CanViewTargetOverlaySubject(ctx context.Context, userID, subjectKind, subjectID string, visibleScopes []string) (bool, error) {
	subjectKind = strings.TrimSpace(subjectKind)
	subjectID = strings.TrimSpace(subjectID)
	if err := validateOverlaySubjectKind(subjectKind); err != nil {
		return false, err
	}
	if subjectID == "" {
		return false, fmt.Errorf("%w: subject_id is required", ErrTargetOverlayValidation)
	}
	switch subjectKind {
	case "course":
		return targetCourseVisible(ctx, db, subjectID, userID, visibleScopes)
	case "route_publication":
		return targetRoutePublicationVisible(ctx, db, subjectID, userID, visibleScopes)
	case "place", "region":
		return targetObjectVisibleByProjection(ctx, db, subjectKind, subjectID, userID, visibleScopes)
	case "collection":
		return targetCollectionVisibleBySummary(ctx, db, subjectID, userID, visibleScopes)
	default:
		return false, nil
	}
}

func (db *DB) targetOverlayVisibleScopesForUser(ctx context.Context, userID string) ([]string, error) {
	if strings.TrimSpace(userID) == "" {
		return []string{}, nil
	}
	scopes, err := db.ResolveVisibleAccessScopes(ctx, userID, nil, nil)
	if err != nil {
		return nil, err
	}
	if scopes == nil {
		return []string{}, nil
	}
	return scopes, nil
}

func (db *DB) CanManageTargetReports(ctx context.Context, userID, subjectKind, subjectID, permission string) (bool, error) {
	userID = strings.TrimSpace(userID)
	subjectKind = strings.TrimSpace(subjectKind)
	subjectID = strings.TrimSpace(subjectID)
	if userID == "" || subjectKind == "" || subjectID == "" {
		return false, nil
	}
	if permission == "" {
		permission = "manage_reports"
	}
	switch subjectKind {
	case "course":
		var exists bool
		if err := db.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM segments WHERE id = $1 AND created_by = $2)`, subjectID, userID).Scan(&exists); err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
		return db.userHasTargetGovernancePermission(ctx, userID, subjectKind, subjectID, permission)
	case "route_publication":
		var exists bool
		if err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM route_course_publications
				WHERE id = $1 AND actor_user_id = $2 AND retired_at IS NULL
			)
		`, subjectID, userID).Scan(&exists); err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
		return db.userHasTargetGovernancePermission(ctx, userID, subjectKind, subjectID, permission)
	case "place", "region", "collection":
		return db.CanEditTarget(ctx, userID, subjectKind, subjectID, permission)
	default:
		return false, nil
	}
}

func (db *DB) userHasTargetGovernancePermission(ctx context.Context, userID, targetKind, targetID, permission string) (bool, error) {
	targets, err := db.ListGovernanceEffectiveTargets(ctx, model.GovernanceActor{
		ActorType:   "user",
		ActorUserID: &userID,
	}, model.GovernanceEffectiveTargetFilter{
		TargetKind: targetKind,
		TargetID:   targetID,
		Permission: permission,
		Limit:      1,
	})
	if err != nil {
		return false, err
	}
	return len(targets) > 0, nil
}

func (db *DB) validateTargetOverlayAccessScopeForSubject(ctx context.Context, actorUserID, subjectKind, subjectID, visibility string, accessScopeID *string) error {
	if strings.TrimSpace(visibility) != "shared" {
		return nil
	}
	if accessScopeID == nil || strings.TrimSpace(*accessScopeID) == "" {
		return fmt.Errorf("%w: shared overlays require access_scope_id", ErrTargetOverlayValidation)
	}
	ownerUserID, err := db.targetOverlaySubjectOwnerUserID(ctx, subjectKind, subjectID)
	if err != nil {
		return err
	}
	var exists bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM object_access_scopes s
			WHERE s.id = $1
			  AND s.owner_kind = 'user'
			  AND (
			      s.owner_user_id = $2::uuid
			      OR (
			          s.owner_user_id = NULLIF($3, '')::uuid
			          AND EXISTS (
			              SELECT 1
			              FROM object_access_grants actor_grant
			              WHERE actor_grant.access_scope_id = s.id
			                AND actor_grant.permission = 'view'
			                AND actor_grant.revoked_at IS NULL
			                AND actor_grant.principal_kind = 'user'
			                AND actor_grant.principal_user_id = $2::uuid
			          )
			      )
			  )
			  AND s.revoked_at IS NULL
			  AND EXISTS (
			      SELECT 1
			      FROM object_access_grants g
			      WHERE g.access_scope_id = s.id
			        AND g.permission = 'view'
			        AND g.revoked_at IS NULL
			  )
		)
	`, *accessScopeID, strings.TrimSpace(actorUserID), strings.TrimSpace(ownerUserID)).Scan(&exists); err != nil {
		return fmt.Errorf("validate target overlay access scope: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: access_scope_id is not valid for this overlay subject", ErrTargetOverlayValidation)
	}
	return nil
}

func (db *DB) targetOverlaySubjectOwnerUserID(ctx context.Context, subjectKind, subjectID string) (string, error) {
	var owner *string
	switch strings.TrimSpace(subjectKind) {
	case "course":
		if err := db.Pool.QueryRow(ctx, `SELECT created_by::text FROM segments WHERE id = $1`, subjectID).Scan(&owner); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", ErrTargetNotFound
			}
			return "", err
		}
	case "route_publication":
		if err := db.Pool.QueryRow(ctx, `SELECT actor_user_id::text FROM route_course_publications WHERE id = $1 AND retired_at IS NULL`, subjectID).Scan(&owner); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", ErrTargetNotFound
			}
			return "", err
		}
	case "place":
		if err := db.Pool.QueryRow(ctx, `SELECT owner_user_id::text FROM target_places WHERE id = $1 AND archived_at IS NULL`, subjectID).Scan(&owner); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", ErrTargetNotFound
			}
			return "", err
		}
	case "region":
		if err := db.Pool.QueryRow(ctx, `SELECT owner_user_id::text FROM target_regions WHERE id = $1 AND archived_at IS NULL`, subjectID).Scan(&owner); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", ErrTargetNotFound
			}
			return "", err
		}
	case "collection":
		if err := db.Pool.QueryRow(ctx, `SELECT owner_user_id::text FROM target_collections WHERE id = $1 AND archived_at IS NULL`, subjectID).Scan(&owner); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", ErrTargetNotFound
			}
			return "", err
		}
	default:
		return "", fmt.Errorf("%w: invalid overlay subject kind %q", ErrTargetOverlayValidation, subjectKind)
	}
	if owner == nil {
		return "", nil
	}
	return *owner, nil
}

func (db *DB) CreateTargetReport(ctx context.Context, userID string, input model.CreateTargetReportInput) (*model.TargetReport, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user is required", ErrTargetOverlayValidation)
	}
	if err := validateCreateTargetReportInput(input); err != nil {
		return nil, err
	}
	scopes, err := db.targetOverlayVisibleScopesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	visible, err := db.CanViewTargetOverlaySubject(ctx, userID, input.SubjectKind, input.SubjectID, scopes)
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, ErrTargetNotFound
	}
	if err := db.validateTargetOverlayAccessScopeForSubject(ctx, userID, input.SubjectKind, input.SubjectID, targetOverlayVisibilityOrDefault(input.Visibility), input.AccessScopeID); err != nil {
		return nil, err
	}
	if input.SourceActivityID != nil {
		if owns, err := db.userOwnsActivity(ctx, userID, *input.SourceActivityID); err != nil {
			return nil, err
		} else if !owns {
			return nil, ErrTargetNotFound
		}
	}
	if err := validateTargetOverlayVersion(ctx, db.Pool, input.SubjectKind, input.SubjectID, input.TargetVersionID); err != nil {
		return nil, err
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create target report: %w", err)
	}
	defer tx.Rollback(ctx)

	report, err := insertTargetReportTx(ctx, tx, userID, input)
	if err != nil {
		return nil, err
	}
	if _, err := insertTargetReportStatusTx(ctx, tx, userID, report.ID, "open", "created"); err != nil {
		return nil, err
	}
	if err := rebuildTargetOverlaySummaryTx(ctx, tx, report.SubjectKind, report.SubjectID, report.TargetVersionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create target report: %w", err)
	}
	status := "open"
	report.LatestStatus = &status
	return report, nil
}

func validateCreateTargetReportInput(input model.CreateTargetReportInput) error {
	if err := validateOverlaySubjectKind(input.SubjectKind); err != nil {
		return err
	}
	if strings.TrimSpace(input.SubjectID) == "" {
		return fmt.Errorf("%w: subject_id is required", ErrTargetOverlayValidation)
	}
	if err := validateReportType(input.ReportType); err != nil {
		return err
	}
	if err := validateOverlayVisibility(targetOverlayVisibilityOrDefault(input.Visibility), input.AccessScopeID); err != nil {
		return err
	}
	if err := validateAttributionMode(input.AttributionMode); err != nil {
		return err
	}
	if strings.TrimSpace(input.Description) == "" {
		return fmt.Errorf("%w: description is required", ErrTargetOverlayValidation)
	}
	return nil
}

func insertTargetReportTx(ctx context.Context, tx pgx.Tx, userID string, input model.CreateTargetReportInput) (*model.TargetReport, error) {
	visibility := targetOverlayVisibilityOrDefault(input.Visibility)
	attribution := strings.TrimSpace(input.AttributionMode)
	if attribution == "" {
		attribution = "named"
	}
	sourcePolicy := strings.TrimSpace(input.SourcePolicyKey)
	if sourcePolicy == "" {
		sourcePolicy = "none"
	}
	observedAt := input.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	appliesTo := rawOrDefault(input.AppliesTo, `{}`)
	mediaRefs := rawOrDefault(input.MediaRefs, `[]`)

	report := &model.TargetReport{}
	if err := tx.QueryRow(ctx, `
		INSERT INTO target_reports (
			overlay_subject_kind, subject_id, target_version_id, author_user_id,
			report_type, visibility, access_scope_id, attribution_mode, applies_to,
			condition_state, condition_severity, description, source_activity_id,
			source_policy_key, media_refs, observed_at, valid_from, valid_until,
			moderation_state, resolver_effect, redaction_policy_key
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, 'visible', 'none', $19)
		RETURNING id, overlay_subject_kind, subject_id, target_version_id, author_user_id,
		          report_type, visibility, access_scope_id, attribution_mode, applies_to,
		          condition_state, condition_severity, description, source_activity_id,
		          source_policy_key, media_refs, observed_at, valid_from, valid_until,
		          moderation_state, resolver_effect, redaction_policy_key, created_at, updated_at
	`, strings.TrimSpace(input.SubjectKind), strings.TrimSpace(input.SubjectID), input.TargetVersionID, userID,
		strings.TrimSpace(input.ReportType), visibility, input.AccessScopeID, attribution, appliesTo,
		input.ConditionState, input.ConditionSeverity, strings.TrimSpace(input.Description), input.SourceActivityID,
		sourcePolicy, mediaRefs, observedAt, input.ValidFrom, input.ValidUntil, input.RedactionPolicyKey).Scan(
		&report.ID, &report.SubjectKind, &report.SubjectID, &report.TargetVersionID, &report.AuthorUserID,
		&report.ReportType, &report.Visibility, &report.AccessScopeID, &report.AttributionMode, &report.AppliesTo,
		&report.ConditionState, &report.ConditionSeverity, &report.Description, &report.SourceActivityID,
		&report.SourcePolicyKey, &report.MediaRefs, &report.ObservedAt, &report.ValidFrom, &report.ValidUntil,
		&report.ModerationState, &report.ResolverEffect, &report.RedactionPolicyKey, &report.CreatedAt, &report.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("insert target report: %w", err)
	}
	return report, nil
}

func (db *DB) AddTargetReportStatus(ctx context.Context, userID string, reportID string, input model.AddTargetReportStatusInput) (*model.TargetReportStatusEvent, error) {
	userID = strings.TrimSpace(userID)
	reportID = strings.TrimSpace(reportID)
	status := strings.TrimSpace(input.Status)
	if userID == "" || reportID == "" {
		return nil, fmt.Errorf("%w: user and report are required", ErrTargetOverlayValidation)
	}
	if err := validateReportStatus(status); err != nil {
		return nil, err
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin add target report status: %w", err)
	}
	defer tx.Rollback(ctx)
	var subjectKind, subjectID string
	var targetVersionID *string
	if err := tx.QueryRow(ctx, `
		SELECT overlay_subject_kind, subject_id, target_version_id
		FROM target_reports
		WHERE id = $1
	`, reportID).Scan(&subjectKind, &subjectID, &targetVersionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTargetNotFound
		}
		return nil, fmt.Errorf("load target report for status: %w", err)
	}
	if status == "redacted" {
		if _, err := tx.Exec(ctx, `
			UPDATE target_reports
			SET moderation_state = 'redacted', updated_at = NOW()
			WHERE id = $1
		`, reportID); err != nil {
			return nil, fmt.Errorf("redact target report: %w", err)
		}
	}
	event, err := insertTargetReportStatusTx(ctx, tx, userID, reportID, status, input.Reason)
	if err != nil {
		return nil, err
	}
	if err := rebuildTargetOverlaySummaryTx(ctx, tx, subjectKind, subjectID, targetVersionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit add target report status: %w", err)
	}
	return event, nil
}

func insertTargetReportStatusTx(ctx context.Context, tx pgx.Tx, userID, reportID, status, reason string) (*model.TargetReportStatusEvent, error) {
	event := &model.TargetReportStatusEvent{}
	if err := tx.QueryRow(ctx, `
		INSERT INTO target_report_status_events (report_id, actor_user_id, status, reason)
		VALUES ($1, $2, $3, $4)
		RETURNING id, report_id, actor_user_id, status, reason, created_at
	`, reportID, userID, strings.TrimSpace(status), strings.TrimSpace(reason)).Scan(
		&event.ID, &event.ReportID, &event.ActorUserID, &event.Status, &event.Reason, &event.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("insert target report status: %w", err)
	}
	return event, nil
}

func (db *DB) ListTargetReportStatusEvents(ctx context.Context, userID, reportID string) ([]model.TargetReportStatusEvent, error) {
	report, err := db.GetTargetReport(ctx, userID, reportID, nil)
	if err != nil {
		return nil, err
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT id, report_id, actor_user_id, status, reason, created_at
		FROM target_report_status_events
		WHERE report_id = $1
		ORDER BY created_at ASC, id ASC
	`, report.ID)
	if err != nil {
		return nil, fmt.Errorf("list target report statuses: %w", err)
	}
	defer rows.Close()
	var events []model.TargetReportStatusEvent
	for rows.Next() {
		var event model.TargetReportStatusEvent
		if err := rows.Scan(&event.ID, &event.ReportID, &event.ActorUserID, &event.Status, &event.Reason, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (db *DB) GetTargetReport(ctx context.Context, userID, reportID string, visibleScopes []string) (*model.TargetReport, error) {
	reportID = strings.TrimSpace(reportID)
	if reportID == "" {
		return nil, fmt.Errorf("%w: report id is required", ErrTargetOverlayValidation)
	}
	report, err := scanTargetReportRow(db.Pool.QueryRow(ctx, targetReportSelectSQL()+` WHERE r.id = $1`, reportID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	if ok, err := db.canReadOverlayRow(ctx, userID, report.SubjectKind, report.SubjectID, report.Visibility, report.AccessScopeID, report.AuthorUserID, visibleScopes); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrTargetNotFound
	}
	if !canReadTargetReportAfterModeration(report, userID, false) {
		return nil, ErrTargetNotFound
	}
	if err := db.redactTargetReportForViewer(ctx, report, userID, false); err != nil {
		return nil, err
	}
	return report, nil
}

func (db *DB) GetTargetReportForManagement(ctx context.Context, reportID string) (*model.TargetReport, error) {
	reportID = strings.TrimSpace(reportID)
	if reportID == "" {
		return nil, fmt.Errorf("%w: report id is required", ErrTargetOverlayValidation)
	}
	report, err := scanTargetReportRow(db.Pool.QueryRow(ctx, targetReportSelectSQL()+` WHERE r.id = $1`, reportID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTargetNotFound
	}
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (db *DB) ListTargetReports(ctx context.Context, params model.ListTargetReportsParams) ([]model.TargetReport, error) {
	if strings.TrimSpace(params.ReportType) != "" {
		if err := validateReportType(params.ReportType); err != nil {
			return nil, err
		}
	}
	if ok, err := db.CanViewTargetOverlaySubject(ctx, params.UserID, params.SubjectKind, params.SubjectID, params.VisibleAccessScopeIDs); err != nil {
		return nil, err
	} else if !ok {
		return nil, nil
	}
	args := []any{params.SubjectKind, params.SubjectID, params.TargetVersionID, targetLimit(params.Limit), targetOffset(params.Offset)}
	where := `r.overlay_subject_kind = $1 AND r.subject_id = $2 AND r.target_version_id IS NOT DISTINCT FROM $3`
	if params.ReportType != "" {
		args = append(args, params.ReportType)
		where += fmt.Sprintf(" AND r.report_type = $%d", len(args))
	}
	if !params.Admin {
		where += " AND r.moderation_state = 'visible'"
	}
	where += overlayVisibilitySQL("r", "author_user_id", params.UserID, params.VisibleAccessScopeIDs, params.Admin, &args)
	rows, err := db.Pool.Query(ctx, targetReportSelectSQL()+`
		WHERE `+where+`
		ORDER BY r.observed_at DESC, r.id DESC
		LIMIT $4 OFFSET $5
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list target reports: %w", err)
	}
	defer rows.Close()
	var reports []model.TargetReport
	for rows.Next() {
		report, err := scanTargetReportRows(rows)
		if err != nil {
			return nil, err
		}
		if err := db.redactTargetReportForViewer(ctx, report, params.UserID, params.Admin); err != nil {
			return nil, err
		}
		reports = append(reports, *report)
	}
	return reports, rows.Err()
}

func (db *DB) CreateTargetActivityObservation(ctx context.Context, actor model.TargetOverlayActor, input model.CreateTargetActivityObservationInput) (*model.TargetActivityObservation, error) {
	if err := validateCreateTargetObservationInput(input); err != nil {
		return nil, err
	}
	if !actor.System {
		if strings.TrimSpace(actor.UserID) == "" {
			return nil, fmt.Errorf("%w: actor is required", ErrTargetOverlayValidation)
		}
		scopes, err := db.targetOverlayVisibleScopesForUser(ctx, actor.UserID)
		if err != nil {
			return nil, err
		}
		visible, err := db.CanViewTargetOverlaySubject(ctx, actor.UserID, input.SubjectKind, input.SubjectID, scopes)
		if err != nil {
			return nil, err
		}
		if !visible {
			return nil, ErrTargetNotFound
		}
		if input.ActivityID != nil {
			if owns, err := db.userOwnsActivity(ctx, actor.UserID, *input.ActivityID); err != nil {
				return nil, err
			} else if !owns {
				return nil, ErrTargetNotFound
			}
		}
		if input.UserID != nil && *input.UserID != actor.UserID {
			return nil, fmt.Errorf("%w: user_id must match actor", ErrTargetOverlayValidation)
		}
		if err := db.validateTargetOverlayAccessScopeForSubject(ctx, actor.UserID, input.SubjectKind, input.SubjectID, targetOverlayVisibilityOrDefault(input.Visibility), input.AccessScopeID); err != nil {
			return nil, err
		}
	}
	if err := validateTargetOverlayVersion(ctx, db.Pool, input.SubjectKind, input.SubjectID, input.TargetVersionID); err != nil {
		return nil, err
	}
	userID := input.UserID
	if userID == nil && strings.TrimSpace(actor.UserID) != "" {
		userID = &actor.UserID
	}
	visibility := targetOverlayVisibilityOrDefault(input.Visibility)
	visibilitySnapshot := strings.TrimSpace(input.VisibilitySnapshot)
	if visibilitySnapshot == "" {
		visibilitySnapshot = visibility
	}
	privacyPolicy := strings.TrimSpace(input.PrivacyPolicyKey)
	if privacyPolicy == "" {
		privacyPolicy = "live_source_activity_v1"
	}
	authorizationMode := strings.TrimSpace(input.AuthorizationMode)
	if authorizationMode == "" {
		authorizationMode = "live_source_activity"
	}
	occurredAt := input.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create target activity observation: %w", err)
	}
	defer tx.Rollback(ctx)
	observation := &model.TargetActivityObservation{}
	if err := tx.QueryRow(ctx, `
		INSERT INTO target_activity_observations (
			overlay_subject_kind, subject_id, target_version_id, activity_id, user_id,
			observation_type, source_type, source_id, source_occurrence_key,
			source_match_id, source_effort_id, source_effort_started_at, source_effort_segment_version_id,
			activity_tuple, stats, visibility, access_scope_id, visibility_snapshot,
			privacy_policy_key, authorization_mode, redaction_policy_key, occurred_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
		ON CONFLICT (
			overlay_subject_kind, subject_id, target_version_id, source_type, source_id,
			source_occurrence_key, observation_type
		) WHERE superseded_at IS NULL
		DO UPDATE SET created_at = target_activity_observations.created_at
		RETURNING id, overlay_subject_kind, subject_id, target_version_id, activity_id, user_id,
		          observation_type, source_type, source_id, source_occurrence_key,
		          source_match_id, source_effort_id, source_effort_started_at, source_effort_segment_version_id,
		          activity_tuple, stats, visibility, access_scope_id, visibility_snapshot,
		          privacy_policy_key, authorization_mode, redaction_policy_key, occurred_at, created_at, superseded_at
	`, input.SubjectKind, input.SubjectID, input.TargetVersionID, input.ActivityID, userID,
		input.ObservationType, input.SourceType, input.SourceID, input.SourceOccurrenceKey,
		input.SourceMatchID, input.SourceEffortID, input.SourceEffortStartedAt, input.SourceEffortSegmentVersionID,
		rawOrDefault(input.ActivityTuple, `{}`), rawOrDefault(input.Stats, `{}`), visibility, input.AccessScopeID, visibilitySnapshot,
		privacyPolicy, authorizationMode, input.RedactionPolicyKey, occurredAt).Scan(
		&observation.ID, &observation.SubjectKind, &observation.SubjectID, &observation.TargetVersionID, &observation.ActivityID, &observation.UserID,
		&observation.ObservationType, &observation.SourceType, &observation.SourceID, &observation.SourceOccurrenceKey,
		&observation.SourceMatchID, &observation.SourceEffortID, &observation.SourceEffortStartedAt, &observation.SourceEffortSegmentVersionID,
		&observation.ActivityTuple, &observation.Stats, &observation.Visibility, &observation.AccessScopeID, &observation.VisibilitySnapshot,
		&observation.PrivacyPolicyKey, &observation.AuthorizationMode, &observation.RedactionPolicyKey, &observation.OccurredAt, &observation.CreatedAt, &observation.SupersededAt,
	); err != nil {
		return nil, fmt.Errorf("insert target activity observation: %w", err)
	}
	if err := rebuildTargetOverlaySummaryTx(ctx, tx, observation.SubjectKind, observation.SubjectID, observation.TargetVersionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create target activity observation: %w", err)
	}
	return observation, nil
}

func validateCreateTargetObservationInput(input model.CreateTargetActivityObservationInput) error {
	if err := validateOverlaySubjectKind(input.SubjectKind); err != nil {
		return err
	}
	if strings.TrimSpace(input.SubjectID) == "" || strings.TrimSpace(input.SourceType) == "" || strings.TrimSpace(input.SourceID) == "" || strings.TrimSpace(input.SourceOccurrenceKey) == "" {
		return fmt.Errorf("%w: subject and source identity are required", ErrTargetOverlayValidation)
	}
	if err := validateObservationType(input.ObservationType); err != nil {
		return err
	}
	return validateOverlayVisibility(targetOverlayVisibilityOrDefault(input.Visibility), input.AccessScopeID)
}

func (db *DB) ListTargetActivityObservations(ctx context.Context, params model.ListTargetActivityObservationsParams) ([]model.TargetActivityObservation, error) {
	if ok, err := db.CanViewTargetOverlaySubject(ctx, params.UserID, params.SubjectKind, params.SubjectID, params.VisibleAccessScopeIDs); err != nil {
		return nil, err
	} else if !ok {
		return nil, nil
	}
	args := []any{params.SubjectKind, params.SubjectID, params.TargetVersionID, targetLimit(params.Limit), targetOffset(params.Offset)}
	where := `o.overlay_subject_kind = $1 AND o.subject_id = $2 AND o.target_version_id IS NOT DISTINCT FROM $3 AND o.superseded_at IS NULL`
	where += overlayVisibilitySQL("o", "user_id", params.UserID, params.VisibleAccessScopeIDs, params.Admin, &args)
	where += liveActivityAuthorizationSQL("o", params.UserID, params.Admin, &args)
	rows, err := db.Pool.Query(ctx, `
		SELECT id, overlay_subject_kind, subject_id, target_version_id, activity_id, user_id,
		       observation_type, source_type, source_id, source_occurrence_key,
		       source_match_id, source_effort_id, source_effort_started_at, source_effort_segment_version_id,
		       activity_tuple, stats, visibility, access_scope_id, visibility_snapshot,
		       privacy_policy_key, authorization_mode, redaction_policy_key, occurred_at, created_at, superseded_at
		FROM target_activity_observations o
		WHERE `+where+`
		ORDER BY o.occurred_at DESC, o.id DESC
		LIMIT $4 OFFSET $5
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list target activity observations: %w", err)
	}
	defer rows.Close()
	var observations []model.TargetActivityObservation
	for rows.Next() {
		var observation model.TargetActivityObservation
		if err := rows.Scan(
			&observation.ID, &observation.SubjectKind, &observation.SubjectID, &observation.TargetVersionID, &observation.ActivityID, &observation.UserID,
			&observation.ObservationType, &observation.SourceType, &observation.SourceID, &observation.SourceOccurrenceKey,
			&observation.SourceMatchID, &observation.SourceEffortID, &observation.SourceEffortStartedAt, &observation.SourceEffortSegmentVersionID,
			&observation.ActivityTuple, &observation.Stats, &observation.Visibility, &observation.AccessScopeID, &observation.VisibilitySnapshot,
			&observation.PrivacyPolicyKey, &observation.AuthorizationMode, &observation.RedactionPolicyKey, &observation.OccurredAt, &observation.CreatedAt, &observation.SupersededAt,
		); err != nil {
			return nil, err
		}
		observations = append(observations, observation)
	}
	return observations, rows.Err()
}

func (db *DB) RebuildTargetObservationsFromSegmentEfforts(ctx context.Context, segmentID string, limit int) (int, error) {
	if strings.TrimSpace(segmentID) == "" {
		return 0, fmt.Errorf("%w: segment id is required", ErrTargetOverlayValidation)
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if _, err := db.Pool.Exec(ctx, `
		UPDATE target_activity_observations o
		SET superseded_at = NOW()
		WHERE o.overlay_subject_kind = 'course'
		  AND o.subject_id = $1
		  AND o.source_type = 'segment_effort'
		  AND o.observation_type = 'matched_traversal'
		  AND o.superseded_at IS NULL
		  AND NOT EXISTS (
			SELECT 1
			FROM segment_efforts se
			WHERE se.id::text = o.source_id
			  AND se.segment_id = $1
			  AND se.superseded_at IS NULL
		  )
	`, segmentID); err != nil {
		return 0, fmt.Errorf("retire stale segment effort observations: %w", err)
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT id, segment_id, segment_version_id, activity_id, user_id, started_at
		FROM segment_efforts
		WHERE segment_id = $1 AND superseded_at IS NULL
		ORDER BY started_at DESC
		LIMIT $2
	`, segmentID, limit)
	if err != nil {
		return 0, fmt.Errorf("query segment efforts for observations: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var effortID, subjectID, versionID, activityID, userID string
		var startedAt time.Time
		if err := rows.Scan(&effortID, &subjectID, &versionID, &activityID, &userID, &startedAt); err != nil {
			return count, err
		}
		if _, err := db.CreateTargetActivityObservation(ctx, model.TargetOverlayActor{System: true}, model.CreateTargetActivityObservationInput{
			SubjectKind:                  "course",
			SubjectID:                    subjectID,
			TargetVersionID:              &versionID,
			ActivityID:                   &activityID,
			UserID:                       &userID,
			ObservationType:              "matched_traversal",
			SourceType:                   "segment_effort",
			SourceID:                     effortID,
			SourceOccurrenceKey:          startedAt.UTC().Format(time.RFC3339Nano),
			SourceEffortID:               &effortID,
			SourceEffortStartedAt:        &startedAt,
			SourceEffortSegmentVersionID: &versionID,
			Visibility:                   "public",
			VisibilitySnapshot:           "private",
			OccurredAt:                   startedAt,
		}); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func (db *DB) CreateTargetUserContent(ctx context.Context, userID string, input model.CreateTargetUserContentInput) (*model.TargetUserContent, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user is required", ErrTargetOverlayValidation)
	}
	if err := validateCreateTargetUserContentInput(input); err != nil {
		return nil, err
	}
	scopes, err := db.targetOverlayVisibleScopesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if ok, err := db.CanViewTargetOverlaySubject(ctx, userID, input.SubjectKind, input.SubjectID, scopes); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrTargetNotFound
	}
	if err := db.validateTargetOverlayAccessScopeForSubject(ctx, userID, input.SubjectKind, input.SubjectID, targetOverlayVisibilityOrDefault(input.Visibility), input.AccessScopeID); err != nil {
		return nil, err
	}
	if err := validateTargetOverlayVersion(ctx, db.Pool, input.SubjectKind, input.SubjectID, input.TargetVersionID); err != nil {
		return nil, err
	}
	if input.ActivityID != nil {
		if owns, err := db.userOwnsActivity(ctx, userID, *input.ActivityID); err != nil {
			return nil, err
		} else if !owns {
			return nil, ErrTargetNotFound
		}
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create target user content: %w", err)
	}
	defer tx.Rollback(ctx)
	content, err := upsertTargetUserContentTx(ctx, tx, userID, input)
	if err != nil {
		return nil, err
	}
	if err := rebuildTargetOverlaySummaryTx(ctx, tx, content.SubjectKind, content.SubjectID, content.TargetVersionID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create target user content: %w", err)
	}
	return content, nil
}

func validateCreateTargetUserContentInput(input model.CreateTargetUserContentInput) error {
	if err := validateOverlaySubjectKind(input.SubjectKind); err != nil {
		return err
	}
	if strings.TrimSpace(input.SubjectID) == "" {
		return fmt.Errorf("%w: subject_id is required", ErrTargetOverlayValidation)
	}
	if err := validateUserContentKind(input.ContentKind); err != nil {
		return err
	}
	if strings.TrimSpace(input.ContentKind) == "rating" {
		if err := validateRatingPayload(input.Payload); err != nil {
			return err
		}
	}
	return validateOverlayVisibility(targetOverlayVisibilityOrDefault(input.Visibility), input.AccessScopeID)
}

func (db *DB) upsertTargetUserContent(ctx context.Context, userID string, input model.CreateTargetUserContentInput) (*model.TargetUserContent, error) {
	return upsertTargetUserContentTx(ctx, db.Pool, userID, input)
}

func upsertTargetUserContentTx(ctx context.Context, q Querier, userID string, input model.CreateTargetUserContentInput) (*model.TargetUserContent, error) {
	visibility := targetOverlayVisibilityOrDefault(input.Visibility)
	payload := rawOrDefault(input.Payload, `{}`)
	activityTuple := rawOrDefault(input.ActivityTuple, `{}`)
	kind := strings.TrimSpace(input.ContentKind)
	conflictWhere := ""
	switch kind {
	case "save":
		conflictWhere = "content_kind = 'save' AND deleted_at IS NULL"
	case "rating":
		conflictWhere = "content_kind = 'rating' AND deleted_at IS NULL"
	}
	query := `
		INSERT INTO target_user_content (
			overlay_subject_kind, subject_id, target_version_id, user_id, content_kind,
			activity_id, activity_tuple, payload, visibility, access_scope_id, moderation_state
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'visible')`
	if conflictWhere != "" {
		query += `
		ON CONFLICT (overlay_subject_kind, subject_id, target_version_id, user_id)
		WHERE ` + conflictWhere + `
		DO UPDATE SET payload = EXCLUDED.payload,
		              visibility = EXCLUDED.visibility,
		              access_scope_id = EXCLUDED.access_scope_id,
		              updated_at = NOW()`
	}
	query += `
		RETURNING id, overlay_subject_kind, subject_id, target_version_id, user_id,
		          content_kind, activity_id, activity_tuple, payload, visibility,
		          access_scope_id, moderation_state, created_at, updated_at, deleted_at`
	content := &model.TargetUserContent{}
	if err := q.QueryRow(ctx, query,
		input.SubjectKind, input.SubjectID, input.TargetVersionID, userID, kind,
		input.ActivityID, activityTuple, payload, visibility, input.AccessScopeID,
	).Scan(
		&content.ID, &content.SubjectKind, &content.SubjectID, &content.TargetVersionID, &content.UserID,
		&content.ContentKind, &content.ActivityID, &content.ActivityTuple, &content.Payload, &content.Visibility,
		&content.AccessScopeID, &content.ModerationState, &content.CreatedAt, &content.UpdatedAt, &content.DeletedAt,
	); err != nil {
		return nil, fmt.Errorf("insert target user content: %w", err)
	}
	return content, nil
}

func (db *DB) ListTargetUserContent(ctx context.Context, params model.ListTargetUserContentParams) ([]model.TargetUserContent, error) {
	if strings.TrimSpace(params.ContentKind) != "" {
		if err := validateUserContentKind(params.ContentKind); err != nil {
			return nil, err
		}
	}
	if ok, err := db.CanViewTargetOverlaySubject(ctx, params.UserID, params.SubjectKind, params.SubjectID, params.VisibleAccessScopeIDs); err != nil {
		return nil, err
	} else if !ok {
		return nil, nil
	}
	args := []any{params.SubjectKind, params.SubjectID, params.TargetVersionID, targetLimit(params.Limit), targetOffset(params.Offset)}
	where := `c.overlay_subject_kind = $1 AND c.subject_id = $2 AND c.target_version_id IS NOT DISTINCT FROM $3 AND c.deleted_at IS NULL`
	if params.ContentKind != "" {
		args = append(args, params.ContentKind)
		where += fmt.Sprintf(" AND c.content_kind = $%d", len(args))
	}
	if !params.Admin {
		where += " AND c.moderation_state = 'visible'"
	}
	where += overlayVisibilitySQL("c", "user_id", params.UserID, params.VisibleAccessScopeIDs, params.Admin, &args)
	rows, err := db.Pool.Query(ctx, `
		SELECT id, overlay_subject_kind, subject_id, target_version_id, user_id,
		       content_kind, activity_id, activity_tuple, payload, visibility,
		       access_scope_id, moderation_state, created_at, updated_at, deleted_at
		FROM target_user_content c
		WHERE `+where+`
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT $4 OFFSET $5
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list target user content: %w", err)
	}
	defer rows.Close()
	var content []model.TargetUserContent
	for rows.Next() {
		var row model.TargetUserContent
		if err := rows.Scan(
			&row.ID, &row.SubjectKind, &row.SubjectID, &row.TargetVersionID, &row.UserID,
			&row.ContentKind, &row.ActivityID, &row.ActivityTuple, &row.Payload, &row.Visibility,
			&row.AccessScopeID, &row.ModerationState, &row.CreatedAt, &row.UpdatedAt, &row.DeletedAt,
		); err != nil {
			return nil, err
		}
		content = append(content, row)
	}
	return content, rows.Err()
}

func (db *DB) DeleteTargetUserContent(ctx context.Context, userID, contentID string) error {
	userID = strings.TrimSpace(userID)
	contentID = strings.TrimSpace(contentID)
	if userID == "" || contentID == "" {
		return fmt.Errorf("%w: user and content are required", ErrTargetOverlayValidation)
	}
	var subjectKind, subjectID string
	var targetVersionID *string
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete target user content: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `
		UPDATE target_user_content
		SET deleted_at = COALESCE(deleted_at, NOW()), updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		RETURNING overlay_subject_kind, subject_id, target_version_id
	`, contentID, userID).Scan(&subjectKind, &subjectID, &targetVersionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTargetNotFound
		}
		return fmt.Errorf("delete target user content: %w", err)
	}
	if err := rebuildTargetOverlaySummaryTx(ctx, tx, subjectKind, subjectID, targetVersionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (db *DB) RebuildTargetOverlaySummary(ctx context.Context, subjectKind, subjectID string, targetVersionID *string) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rebuild target overlay summary: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := rebuildTargetOverlaySummaryTx(ctx, tx, subjectKind, subjectID, targetVersionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func rebuildTargetOverlaySummaryTx(ctx context.Context, tx pgx.Tx, subjectKind, subjectID string, targetVersionID *string) error {
	if err := validateOverlaySubjectKind(subjectKind); err != nil {
		return err
	}
	var summary model.TargetOverlaySummary
	if err := tx.QueryRow(ctx, `
		WITH latest_status AS (
			SELECT DISTINCT ON (report_id) report_id, status, created_at
			FROM target_report_status_events
			ORDER BY report_id, created_at DESC, id DESC
		),
		public_reports AS (
			SELECT r.*
			FROM target_reports r
			JOIN latest_status ls ON ls.report_id = r.id
			WHERE r.overlay_subject_kind = $1
			  AND r.subject_id = $2
			  AND r.target_version_id IS NOT DISTINCT FROM $3
			  AND r.visibility = 'public'
			  AND r.moderation_state = 'visible'
			  AND ls.status IN ('open', 'informational')
			  AND (r.valid_from IS NULL OR r.valid_from <= NOW())
			  AND (r.valid_until IS NULL OR r.valid_until >= NOW())
		),
		condition AS (
			SELECT condition_state, condition_severity, observed_at
			FROM public_reports
			WHERE report_type IN ('condition', 'warning', 'closure', 'trail_work')
			ORDER BY COALESCE(condition_severity, 0) DESC, observed_at DESC, id DESC
			LIMIT 1
		),
		public_observations AS (
			SELECT occurred_at
			FROM target_activity_observations
			WHERE overlay_subject_kind = $1
			  AND subject_id = $2
			  AND target_version_id IS NOT DISTINCT FROM $3
			  AND visibility = 'public'
			  AND superseded_at IS NULL
			  AND activity_id IS NULL
		),
		public_content AS (
			SELECT content_kind, payload
			FROM target_user_content
			WHERE overlay_subject_kind = $1
			  AND subject_id = $2
			  AND target_version_id IS NOT DISTINCT FROM $3
			  AND visibility = 'public'
			  AND moderation_state = 'visible'
			  AND deleted_at IS NULL
		)
		SELECT
			(SELECT COUNT(*)::int FROM public_reports),
			(SELECT MAX(observed_at) FROM public_reports),
			(SELECT condition_state FROM condition),
			(SELECT condition_severity FROM condition),
			(SELECT observed_at FROM condition),
			(SELECT COUNT(*)::int FROM public_observations WHERE occurred_at >= NOW() - INTERVAL '30 days'),
			(SELECT MAX(occurred_at) FROM public_observations),
			(SELECT COUNT(*)::int FROM public_content WHERE content_kind = 'comment'),
			(SELECT COUNT(*)::int FROM public_content WHERE content_kind = 'rating'),
			(
				SELECT AVG((payload->>'rating')::double precision)
				FROM public_content
				WHERE content_kind = 'rating'
				  AND jsonb_typeof(payload->'rating') = 'number'
			),
			(SELECT COUNT(*)::int FROM public_content WHERE content_kind = 'completion'),
			(SELECT COUNT(*)::int FROM public_content WHERE content_kind = 'check_in')
	`, subjectKind, subjectID, targetVersionID).Scan(
		&summary.PublicOpenReportCount,
		&summary.LatestReportAt,
		&summary.CurrentConditionState,
		&summary.CurrentConditionSeverity,
		&summary.CurrentConditionObservedAt,
		&summary.PublicRecentObservationCount30d,
		&summary.LastObservationAt,
		&summary.PublicCommentCount,
		&summary.PublicRatingCount,
		&summary.PublicRatingMean,
		&summary.PublicCompletionCount,
		&summary.PublicCheckInCount,
	); err != nil {
		return fmt.Errorf("build target overlay summary: %w", err)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO target_overlay_summary_projection (
			overlay_subject_kind, subject_id, target_version_id, visibility_scope,
			public_open_report_count, latest_report_at, current_condition_state,
			current_condition_severity, current_condition_observed_at,
			public_recent_observation_count_30d, last_observation_at,
			public_comment_count, public_rating_count, public_rating_mean,
			public_completion_count, public_check_in_count, updated_at, source_hash
		)
		VALUES ($1, $2, $3, 'public', $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), '')
		ON CONFLICT (overlay_subject_kind, subject_id, target_version_id, visibility_scope)
		DO UPDATE SET
			public_open_report_count = EXCLUDED.public_open_report_count,
			latest_report_at = EXCLUDED.latest_report_at,
			current_condition_state = EXCLUDED.current_condition_state,
			current_condition_severity = EXCLUDED.current_condition_severity,
			current_condition_observed_at = EXCLUDED.current_condition_observed_at,
			public_recent_observation_count_30d = EXCLUDED.public_recent_observation_count_30d,
			last_observation_at = EXCLUDED.last_observation_at,
			public_comment_count = EXCLUDED.public_comment_count,
			public_rating_count = EXCLUDED.public_rating_count,
			public_rating_mean = EXCLUDED.public_rating_mean,
			public_completion_count = EXCLUDED.public_completion_count,
			public_check_in_count = EXCLUDED.public_check_in_count,
			updated_at = NOW()
	`, subjectKind, subjectID, targetVersionID,
		summary.PublicOpenReportCount, summary.LatestReportAt, summary.CurrentConditionState,
		summary.CurrentConditionSeverity, summary.CurrentConditionObservedAt,
		summary.PublicRecentObservationCount30d, summary.LastObservationAt,
		summary.PublicCommentCount, summary.PublicRatingCount, summary.PublicRatingMean,
		summary.PublicCompletionCount, summary.PublicCheckInCount)
	if err != nil {
		return fmt.Errorf("upsert target overlay summary: %w", err)
	}
	return nil
}

func (db *DB) GetTargetOverlaySummary(ctx context.Context, subjectKind, subjectID string, targetVersionID *string) (*model.TargetOverlaySummary, error) {
	summary := &model.TargetOverlaySummary{}
	if err := db.Pool.QueryRow(ctx, `
		SELECT overlay_subject_kind, subject_id, target_version_id, visibility_scope,
		       public_open_report_count, latest_report_at, current_condition_state,
		       current_condition_severity, current_condition_observed_at,
		       public_recent_observation_count_30d, last_observation_at,
		       public_comment_count, public_rating_count, public_rating_mean,
		       public_completion_count, public_check_in_count, updated_at
		FROM target_overlay_summary_projection
		WHERE overlay_subject_kind = $1
		  AND subject_id = $2
		  AND target_version_id IS NOT DISTINCT FROM $3
		  AND visibility_scope = 'public'
	`, subjectKind, subjectID, targetVersionID).Scan(
		&summary.SubjectKind, &summary.SubjectID, &summary.TargetVersionID, &summary.VisibilityScope,
		&summary.PublicOpenReportCount, &summary.LatestReportAt, &summary.CurrentConditionState,
		&summary.CurrentConditionSeverity, &summary.CurrentConditionObservedAt,
		&summary.PublicRecentObservationCount30d, &summary.LastObservationAt,
		&summary.PublicCommentCount, &summary.PublicRatingCount, &summary.PublicRatingMean,
		&summary.PublicCompletionCount, &summary.PublicCheckInCount, &summary.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get target overlay summary: %w", err)
	}
	return summary, nil
}

func (db *DB) GetTargetOverlaySummaryForViewer(ctx context.Context, userID, subjectKind, subjectID string, targetVersionID *string, visibleScopes []string) (*model.TargetOverlaySummary, error) {
	if ok, err := db.CanViewTargetOverlaySubject(ctx, userID, subjectKind, subjectID, visibleScopes); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrTargetNotFound
	}
	return db.GetTargetOverlaySummary(ctx, subjectKind, subjectID, targetVersionID)
}

func (db *DB) canReadOverlayRow(ctx context.Context, userID, subjectKind, subjectID, visibility string, accessScopeID, ownerUserID *string, visibleScopes []string) (bool, error) {
	targetVisible, err := db.CanViewTargetOverlaySubject(ctx, userID, subjectKind, subjectID, visibleScopes)
	if err != nil || !targetVisible {
		return targetVisible, err
	}
	switch visibility {
	case "public":
		return true, nil
	case "shared":
		if accessScopeID == nil {
			return false, nil
		}
		for _, scopeID := range visibleScopes {
			if scopeID == *accessScopeID {
				return true, nil
			}
		}
		if strings.TrimSpace(userID) == "" {
			return false, nil
		}
		var exists bool
		if err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM object_access_scopes s
				JOIN object_access_grants g ON g.access_scope_id = s.id
				WHERE s.id = $1
				  AND s.revoked_at IS NULL
				  AND g.revoked_at IS NULL
				  AND g.permission = 'view'
				  AND g.principal_kind = 'user'
				  AND g.principal_user_id = $2
			)
		`, *accessScopeID, userID).Scan(&exists); err != nil {
			return false, err
		}
		return exists, nil
	case "private":
		return ownerUserID != nil && strings.TrimSpace(userID) != "" && strings.TrimSpace(*ownerUserID) == strings.TrimSpace(userID), nil
	default:
		return false, nil
	}
}

func overlayVisibilitySQL(alias, userColumn, userID string, visibleScopes []string, admin bool, args *[]any) string {
	if admin {
		return ""
	}
	var userArg any
	if strings.TrimSpace(userID) != "" {
		userArg = strings.TrimSpace(userID)
	}
	*args = append(*args, userArg)
	userRef := fmt.Sprintf("$%d::uuid", len(*args))
	*args = append(*args, visibleScopes)
	scopeRef := fmt.Sprintf("$%d::uuid[]", len(*args))
	return fmt.Sprintf(` AND (
		%[1]s.visibility = 'public'
		OR %[1]s.%[4]s = %[2]s
		OR (%[1]s.visibility = 'shared' AND %[1]s.access_scope_id = ANY(%[3]s))
	)`, alias, userRef, scopeRef, userColumn)
}

func liveActivityAuthorizationSQL(alias, userID string, admin bool, args *[]any) string {
	if admin {
		return ""
	}
	var userArg any
	if strings.TrimSpace(userID) != "" {
		userArg = strings.TrimSpace(userID)
	}
	*args = append(*args, userArg)
	userRef := fmt.Sprintf("$%d::uuid", len(*args))
	return fmt.Sprintf(` AND (
		%[1]s.activity_id IS NULL
		OR EXISTS (
			SELECT 1
			FROM activities a
			WHERE a.id = %[1]s.activity_id
			  AND (a.visibility = 'public' OR a.user_id = %[2]s)
		)
	)`, alias, userRef)
}

func (db *DB) userOwnsActivity(ctx context.Context, userID, activityID string) (bool, error) {
	var owns bool
	if err := db.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activities WHERE id = $1 AND user_id = $2)`, activityID, userID).Scan(&owns); err != nil {
		return false, err
	}
	return owns, nil
}

func targetOverlayVisibilityOrDefault(visibility string) string {
	visibility = strings.TrimSpace(visibility)
	if visibility == "" {
		return "private"
	}
	return visibility
}

func rawOrDefault(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(fallback)
	}
	return raw
}

func targetReportSelectSQL() string {
	return `
		SELECT r.id, r.overlay_subject_kind, r.subject_id, r.target_version_id, r.author_user_id,
		       r.report_type, r.visibility, r.access_scope_id, r.attribution_mode, r.applies_to,
		       r.condition_state, r.condition_severity, r.description, r.source_activity_id,
		       r.source_policy_key, r.media_refs, r.observed_at, r.valid_from, r.valid_until,
		       r.moderation_state, r.resolver_effect, r.redaction_policy_key, r.created_at, r.updated_at,
		       ls.status
		FROM target_reports r
		LEFT JOIN LATERAL (
			SELECT status
			FROM target_report_status_events e
			WHERE e.report_id = r.id
			ORDER BY e.created_at DESC, e.id DESC
			LIMIT 1
		) ls ON true`
}

func scanTargetReportRow(row pgx.Row) (*model.TargetReport, error) {
	report := &model.TargetReport{}
	err := row.Scan(
		&report.ID, &report.SubjectKind, &report.SubjectID, &report.TargetVersionID, &report.AuthorUserID,
		&report.ReportType, &report.Visibility, &report.AccessScopeID, &report.AttributionMode, &report.AppliesTo,
		&report.ConditionState, &report.ConditionSeverity, &report.Description, &report.SourceActivityID,
		&report.SourcePolicyKey, &report.MediaRefs, &report.ObservedAt, &report.ValidFrom, &report.ValidUntil,
		&report.ModerationState, &report.ResolverEffect, &report.RedactionPolicyKey, &report.CreatedAt, &report.UpdatedAt,
		&report.LatestStatus,
	)
	return report, err
}

func scanTargetReportRows(rows pgx.Rows) (*model.TargetReport, error) {
	return scanTargetReportRow(rows)
}

func canReadTargetReportAfterModeration(report *model.TargetReport, userID string, admin bool) bool {
	if report == nil {
		return false
	}
	if admin {
		return true
	}
	isAuthor := report.AuthorUserID != nil && strings.TrimSpace(*report.AuthorUserID) == strings.TrimSpace(userID)
	return isAuthor || report.ModerationState == "visible"
}

func (db *DB) redactTargetReportForViewer(ctx context.Context, report *model.TargetReport, userID string, admin bool) error {
	if report == nil || admin {
		return nil
	}
	isAuthor := report.AuthorUserID != nil && strings.TrimSpace(*report.AuthorUserID) == strings.TrimSpace(userID)
	if !isAuthor && report.AttributionMode != "named" {
		report.AuthorUserID = nil
	}
	if report.SourceActivityID != nil && !isAuthor {
		visible, err := db.canViewOverlaySourceActivity(ctx, userID, *report.SourceActivityID)
		if err != nil {
			return err
		}
		if !visible {
			report.SourceActivityID = nil
		}
	}
	latestStatus := ""
	if report.LatestStatus != nil {
		latestStatus = *report.LatestStatus
	}
	if (report.ModerationState == "redacted" || latestStatus == "redacted") && !isAuthor {
		report.Description = ""
		report.MediaRefs = json.RawMessage(`[]`)
	}
	return nil
}

func (db *DB) canViewOverlaySourceActivity(ctx context.Context, userID, activityID string) (bool, error) {
	var userArg any
	if strings.TrimSpace(userID) != "" {
		userArg = strings.TrimSpace(userID)
	}
	var exists bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM activities
			WHERE id = $1
			  AND (visibility = 'public' OR user_id = $2::uuid)
		)
	`, strings.TrimSpace(activityID), userArg).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
