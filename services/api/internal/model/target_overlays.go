package model

import (
	"encoding/json"
	"time"
)

type OverlaySubjectKind string

const (
	OverlaySubjectCourse           OverlaySubjectKind = "course"
	OverlaySubjectRoutePublication OverlaySubjectKind = "route_publication"
	OverlaySubjectPlace            OverlaySubjectKind = "place"
	OverlaySubjectRegion           OverlaySubjectKind = "region"
	OverlaySubjectCollection       OverlaySubjectKind = "collection"
)

type TargetOverlayActor struct {
	UserID string
	System bool
	Admin  bool
}

type TargetOverlaySummary struct {
	SubjectKind                     string     `json:"subject_kind"`
	SubjectID                       string     `json:"subject_id"`
	TargetVersionID                 *string    `json:"target_version_id,omitempty"`
	VisibilityScope                 string     `json:"visibility_scope"`
	PublicOpenReportCount           int        `json:"public_open_report_count"`
	LatestReportAt                  *time.Time `json:"latest_report_at,omitempty"`
	CurrentConditionState           *string    `json:"current_condition_state,omitempty"`
	CurrentConditionSeverity        *int       `json:"current_condition_severity,omitempty"`
	CurrentConditionObservedAt      *time.Time `json:"current_condition_observed_at,omitempty"`
	PublicRecentObservationCount30d int        `json:"public_recent_observation_count_30d"`
	LastObservationAt               *time.Time `json:"last_observation_at,omitempty"`
	PublicCommentCount              int        `json:"public_comment_count"`
	PublicRatingCount               int        `json:"public_rating_count"`
	PublicRatingMean                *float64   `json:"public_rating_mean,omitempty"`
	PublicCompletionCount           int        `json:"public_completion_count"`
	PublicCheckInCount              int        `json:"public_check_in_count"`
	UpdatedAt                       time.Time  `json:"updated_at"`
}

type TargetActivityObservation struct {
	ID                           string          `json:"id"`
	SubjectKind                  string          `json:"subject_kind"`
	SubjectID                    string          `json:"subject_id"`
	TargetVersionID              *string         `json:"target_version_id,omitempty"`
	ActivityID                   *string         `json:"activity_id,omitempty"`
	UserID                       *string         `json:"user_id,omitempty"`
	ObservationType              string          `json:"observation_type"`
	SourceType                   string          `json:"source_type"`
	SourceID                     string          `json:"source_id"`
	SourceOccurrenceKey          string          `json:"source_occurrence_key"`
	SourceMatchID                *string         `json:"source_match_id,omitempty"`
	SourceEffortID               *string         `json:"source_effort_id,omitempty"`
	SourceEffortStartedAt        *time.Time      `json:"source_effort_started_at,omitempty"`
	SourceEffortSegmentVersionID *string         `json:"source_effort_segment_version_id,omitempty"`
	ActivityTuple                json.RawMessage `json:"activity_tuple,omitempty"`
	Stats                        json.RawMessage `json:"stats,omitempty"`
	Visibility                   string          `json:"visibility"`
	AccessScopeID                *string         `json:"access_scope_id,omitempty"`
	VisibilitySnapshot           string          `json:"visibility_snapshot"`
	PrivacyPolicyKey             string          `json:"privacy_policy_key"`
	AuthorizationMode            string          `json:"authorization_mode"`
	RedactionPolicyKey           *string         `json:"redaction_policy_key,omitempty"`
	OccurredAt                   time.Time       `json:"occurred_at"`
	CreatedAt                    time.Time       `json:"created_at"`
	SupersededAt                 *time.Time      `json:"superseded_at,omitempty"`
}

type CreateTargetActivityObservationInput struct {
	SubjectKind                  string          `json:"subject_kind"`
	SubjectID                    string          `json:"subject_id"`
	TargetVersionID              *string         `json:"target_version_id,omitempty"`
	ActivityID                   *string         `json:"activity_id,omitempty"`
	UserID                       *string         `json:"user_id,omitempty"`
	ObservationType              string          `json:"observation_type"`
	SourceType                   string          `json:"source_type"`
	SourceID                     string          `json:"source_id"`
	SourceOccurrenceKey          string          `json:"source_occurrence_key"`
	SourceMatchID                *string         `json:"source_match_id,omitempty"`
	SourceEffortID               *string         `json:"source_effort_id,omitempty"`
	SourceEffortStartedAt        *time.Time      `json:"source_effort_started_at,omitempty"`
	SourceEffortSegmentVersionID *string         `json:"source_effort_segment_version_id,omitempty"`
	ActivityTuple                json.RawMessage `json:"activity_tuple,omitempty"`
	Stats                        json.RawMessage `json:"stats,omitempty"`
	Visibility                   string          `json:"visibility"`
	AccessScopeID                *string         `json:"access_scope_id,omitempty"`
	VisibilitySnapshot           string          `json:"visibility_snapshot"`
	PrivacyPolicyKey             string          `json:"privacy_policy_key,omitempty"`
	AuthorizationMode            string          `json:"authorization_mode,omitempty"`
	RedactionPolicyKey           *string         `json:"redaction_policy_key,omitempty"`
	OccurredAt                   time.Time       `json:"occurred_at"`
}

type ListTargetActivityObservationsParams struct {
	SubjectKind           string
	SubjectID             string
	TargetVersionID       *string
	UserID                string
	VisibleAccessScopeIDs []string
	Limit                 int
	Offset                int
	Admin                 bool
}

type TargetReport struct {
	ID                 string          `json:"id"`
	SubjectKind        string          `json:"subject_kind"`
	SubjectID          string          `json:"subject_id"`
	TargetVersionID    *string         `json:"target_version_id,omitempty"`
	AuthorUserID       *string         `json:"author_user_id,omitempty"`
	ReportType         string          `json:"report_type"`
	Visibility         string          `json:"visibility"`
	AccessScopeID      *string         `json:"access_scope_id,omitempty"`
	AttributionMode    string          `json:"attribution_mode"`
	AppliesTo          json.RawMessage `json:"applies_to,omitempty"`
	ConditionState     *string         `json:"condition_state,omitempty"`
	ConditionSeverity  *int            `json:"condition_severity,omitempty"`
	Description        string          `json:"description"`
	SourceActivityID   *string         `json:"source_activity_id,omitempty"`
	SourcePolicyKey    string          `json:"source_policy_key"`
	MediaRefs          json.RawMessage `json:"media_refs,omitempty"`
	ObservedAt         time.Time       `json:"observed_at"`
	ValidFrom          *time.Time      `json:"valid_from,omitempty"`
	ValidUntil         *time.Time      `json:"valid_until,omitempty"`
	ModerationState    string          `json:"moderation_state"`
	ResolverEffect     string          `json:"resolver_effect"`
	RedactionPolicyKey *string         `json:"redaction_policy_key,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	LatestStatus       *string         `json:"latest_status,omitempty"`
}

type TargetReportStatusEvent struct {
	ID          string    `json:"id"`
	ReportID    string    `json:"report_id"`
	ActorUserID *string   `json:"actor_user_id,omitempty"`
	Status      string    `json:"status"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateTargetReportInput struct {
	SubjectKind        string          `json:"subject_kind"`
	SubjectID          string          `json:"subject_id"`
	TargetVersionID    *string         `json:"target_version_id,omitempty"`
	ReportType         string          `json:"report_type"`
	Visibility         string          `json:"visibility"`
	AccessScopeID      *string         `json:"access_scope_id,omitempty"`
	AttributionMode    string          `json:"attribution_mode"`
	AppliesTo          json.RawMessage `json:"applies_to,omitempty"`
	ConditionState     *string         `json:"condition_state,omitempty"`
	ConditionSeverity  *int            `json:"condition_severity,omitempty"`
	Description        string          `json:"description"`
	SourceActivityID   *string         `json:"source_activity_id,omitempty"`
	SourcePolicyKey    string          `json:"source_policy_key,omitempty"`
	MediaRefs          json.RawMessage `json:"media_refs,omitempty"`
	ObservedAt         time.Time       `json:"observed_at"`
	ValidFrom          *time.Time      `json:"valid_from,omitempty"`
	ValidUntil         *time.Time      `json:"valid_until,omitempty"`
	RedactionPolicyKey *string         `json:"redaction_policy_key,omitempty"`
}

type AddTargetReportStatusInput struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type ListTargetReportsParams struct {
	SubjectKind           string
	SubjectID             string
	TargetVersionID       *string
	UserID                string
	VisibleAccessScopeIDs []string
	ReportType            string
	Limit                 int
	Offset                int
	Admin                 bool
}

type TargetUserContent struct {
	ID              string          `json:"id"`
	SubjectKind     string          `json:"subject_kind"`
	SubjectID       string          `json:"subject_id"`
	TargetVersionID *string         `json:"target_version_id,omitempty"`
	UserID          string          `json:"user_id"`
	ContentKind     string          `json:"content_kind"`
	ActivityID      *string         `json:"activity_id,omitempty"`
	ActivityTuple   json.RawMessage `json:"activity_tuple,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Visibility      string          `json:"visibility"`
	AccessScopeID   *string         `json:"access_scope_id,omitempty"`
	ModerationState string          `json:"moderation_state"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	DeletedAt       *time.Time      `json:"deleted_at,omitempty"`
}

type CreateTargetUserContentInput struct {
	SubjectKind     string          `json:"subject_kind"`
	SubjectID       string          `json:"subject_id"`
	TargetVersionID *string         `json:"target_version_id,omitempty"`
	ContentKind     string          `json:"content_kind"`
	ActivityID      *string         `json:"activity_id,omitempty"`
	ActivityTuple   json.RawMessage `json:"activity_tuple,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Visibility      string          `json:"visibility"`
	AccessScopeID   *string         `json:"access_scope_id,omitempty"`
}

type ListTargetUserContentParams struct {
	SubjectKind           string
	SubjectID             string
	TargetVersionID       *string
	UserID                string
	VisibleAccessScopeIDs []string
	ContentKind           string
	Limit                 int
	Offset                int
	Admin                 bool
}
