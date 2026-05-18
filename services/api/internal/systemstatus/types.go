package systemstatus

import "time"

type StatusState string

const (
	StatusOK               StatusState = "ok"
	StatusDegraded         StatusState = "degraded"
	StatusUnavailable      StatusState = "unavailable"
	StatusDisabled         StatusState = "disabled"
	StatusNotConfigured    StatusState = "not_configured"
	StatusMissingArtifacts StatusState = "missing_artifacts"
	StatusBlocked          StatusState = "blocked"
	StatusUnknown          StatusState = "unknown"
)

type ComponentStatus struct {
	Key        string            `json:"key"`
	Label      string            `json:"label"`
	State      StatusState       `json:"state"`
	Required   bool              `json:"required,omitempty"`
	ReasonCode string            `json:"reason_code,omitempty"`
	Summary    string            `json:"summary,omitempty"`
	CheckedAt  time.Time         `json:"checked_at"`
	LatencyMS  int64             `json:"latency_ms,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
}

type BuildInfo struct {
	Service       string `json:"service"`
	GitCommit     string `json:"git_commit"`
	CommitTime    string `json:"commit_time"`
	BuildTime     string `json:"build_time"`
	ImageDigest   string `json:"image_digest,omitempty"`
	ConfigVersion string `json:"config_version,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	UptimeSeconds int64  `json:"uptime_seconds,omitempty"`
}

type SystemStatus struct {
	SchemaVersion int                        `json:"schema_version"`
	GeneratedAt   time.Time                  `json:"generated_at"`
	Overall       StatusState                `json:"overall"`
	Build         map[string]BuildInfo       `json:"build"`
	Components    map[string]ComponentStatus `json:"components"`
	FeatureMatrix map[string]ComponentStatus `json:"feature_matrix,omitempty"`
}

func checkedAtOrNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}
