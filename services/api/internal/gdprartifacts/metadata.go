package gdprartifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type MultipartSummary struct {
	PartCount      int   `json:"part_count"`
	TotalSizeBytes int64 `json:"total_size_bytes"`
}

type Artifact struct {
	Version   int               `json:"artifact_version"`
	Format    ArtifactFormat    `json:"artifact_format"`
	ExpiresAt time.Time         `json:"expires_at"`
	Objects   []ObjectRef       `json:"objects"`
	Multipart *MultipartSummary `json:"multipart,omitempty"`
}

type ImportProgress map[string]any

type ArtifactReservation struct {
	ReservationID  string     `json:"reservation_id"`
	Kind           string     `json:"kind"`
	ExpectedBytes  int64      `json:"expected_bytes"`
	CommittedBytes int64      `json:"committed_bytes,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	ReleasedAt     *time.Time `json:"released_at,omitempty"`
}

type DownloadSessionObject struct {
	Name      string     `json:"name"`
	Role      ObjectRole `json:"role"`
	SHA256    string     `json:"sha256"`
	SizeBytes int64      `json:"size_bytes"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

type DownloadSessionRecord struct {
	SessionID      string                  `json:"session_id"`
	SecretHash     string                  `json:"secret_hash"`
	UserID         string                  `json:"user_id"`
	JobID          string                  `json:"job_id"`
	AllowedObjects []DownloadSessionObject `json:"allowed_objects"`
	ExpiresAt      time.Time               `json:"expires_at"`
	CreatedAt      time.Time               `json:"created_at,omitempty"`
}

type JobMetadata struct {
	Artifact             *Artifact                        `json:"artifact,omitempty"`
	LegacyFilePath       string                           `json:"-"`
	SHA256               string                           `json:"sha256,omitempty"`
	SizeBytes            int64                            `json:"size_bytes,omitempty"`
	ImportProgress       ImportProgress                   `json:"import_progress,omitempty"`
	ImportResult         map[string]any                   `json:"import_result,omitempty"`
	ArtifactReservations []ArtifactReservation            `json:"artifact_reservations,omitempty"`
	DownloadSessions     map[string]DownloadSessionRecord `json:"download_sessions,omitempty"`
}

func DecodeJobMetadata(raw []byte) (JobMetadata, error) {
	if len(raw) == 0 {
		return JobMetadata{}, nil
	}

	var envelope struct {
		Artifact             *Artifact                        `json:"artifact"`
		FilePath             string                           `json:"file_path"`
		SHA256               string                           `json:"sha256"`
		SizeBytes            int64                            `json:"size_bytes"`
		ImportProgress       ImportProgress                   `json:"import_progress"`
		ImportResult         map[string]any                   `json:"import_result"`
		ArtifactReservations []ArtifactReservation            `json:"artifact_reservations"`
		DownloadSessions     map[string]DownloadSessionRecord `json:"download_sessions"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return JobMetadata{}, err
	}

	if envelope.Artifact != nil {
		if err := validateArtifact(*envelope.Artifact); err != nil {
			return JobMetadata{}, err
		}
	}

	return JobMetadata{
		Artifact:             envelope.Artifact,
		LegacyFilePath:       envelope.FilePath,
		SHA256:               envelope.SHA256,
		SizeBytes:            envelope.SizeBytes,
		ImportProgress:       envelope.ImportProgress,
		ImportResult:         envelope.ImportResult,
		ArtifactReservations: envelope.ArtifactReservations,
		DownloadSessions:     envelope.DownloadSessions,
	}, nil
}

func EncodeArtifact(artifact Artifact) ([]byte, error) {
	if artifact.Version == 0 {
		artifact.Version = 1
	}
	if err := validateArtifact(artifact); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"artifact": artifact})
}

func MergeArtifactJSON(existing []byte, artifact Artifact) ([]byte, error) {
	if artifact.Version == 0 {
		artifact.Version = 1
	}
	if err := validateArtifact(artifact); err != nil {
		return nil, err
	}
	obj, err := decodeObject(existing)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(artifact)
	if err != nil {
		return nil, err
	}
	obj["artifact"] = encoded
	return json.Marshal(obj)
}

func MergeImportProgressJSON(existing []byte, progress ImportProgress) ([]byte, error) {
	obj, err := decodeObject(existing)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(progress)
	if err != nil {
		return nil, err
	}
	obj["import_progress"] = encoded
	return json.Marshal(obj)
}

func ReserveArtifactBytesJSON(existing []byte, reservation ArtifactReservation) ([]byte, error) {
	if reservation.ReservationID == "" {
		return nil, errors.New("reservation_id is required")
	}
	if reservation.ExpectedBytes < 0 {
		return nil, errors.New("expected_bytes must not be negative")
	}
	obj, err := decodeObject(existing)
	if err != nil {
		return nil, err
	}
	var reservations []ArtifactReservation
	if raw := obj["artifact_reservations"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &reservations); err != nil {
			return nil, err
		}
	}
	reservations = append(reservations, reservation)
	encoded, err := json.Marshal(reservations)
	if err != nil {
		return nil, err
	}
	obj["artifact_reservations"] = encoded
	return json.Marshal(obj)
}

func ReleaseArtifactReservationJSON(existing []byte, reservationID string, committedBytes int64, releasedAt time.Time) ([]byte, error) {
	obj, err := decodeObject(existing)
	if err != nil {
		return nil, err
	}
	var reservations []ArtifactReservation
	if raw := obj["artifact_reservations"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &reservations); err != nil {
			return nil, err
		}
	}
	found := false
	for i := range reservations {
		if reservations[i].ReservationID == reservationID {
			reservations[i].CommittedBytes = committedBytes
			reservations[i].ReleasedAt = &releasedAt
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("artifact reservation %q not found", reservationID)
	}
	encoded, err := json.Marshal(reservations)
	if err != nil {
		return nil, err
	}
	obj["artifact_reservations"] = encoded
	return json.Marshal(obj)
}

func AddDownloadSessionJSON(existing []byte, session DownloadSessionRecord) ([]byte, error) {
	if session.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if session.SecretHash == "" {
		return nil, errors.New("secret_hash is required")
	}
	obj, err := decodeObject(existing)
	if err != nil {
		return nil, err
	}
	var sessions map[string]DownloadSessionRecord
	if raw := obj["download_sessions"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &sessions); err != nil {
			return nil, err
		}
	}
	if sessions == nil {
		sessions = make(map[string]DownloadSessionRecord)
	}
	sessions[session.SessionID] = session
	encoded, err := json.Marshal(sessions)
	if err != nil {
		return nil, err
	}
	obj["download_sessions"] = encoded
	return json.Marshal(obj)
}

func MarkDownloadSessionObjectUsedJSON(existing []byte, sessionID, objectName string, usedAt time.Time) ([]byte, error) {
	obj, err := decodeObject(existing)
	if err != nil {
		return nil, err
	}
	var sessions map[string]DownloadSessionRecord
	if raw := obj["download_sessions"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &sessions); err != nil {
			return nil, err
		}
	}
	session, ok := sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("download session %q not found", sessionID)
	}
	found := false
	for i := range session.AllowedObjects {
		if session.AllowedObjects[i].Name == objectName {
			session.AllowedObjects[i].UsedAt = &usedAt
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("download session object %q not found", objectName)
	}
	sessions[sessionID] = session
	encoded, err := json.Marshal(sessions)
	if err != nil {
		return nil, err
	}
	obj["download_sessions"] = encoded
	return json.Marshal(obj)
}

func MarkArtifactMissingJSON(existing []byte, missingAt time.Time, objectNames []string) ([]byte, error) {
	obj, err := decodeObject(existing)
	if err != nil {
		return nil, err
	}
	missingAtRaw, err := json.Marshal(missingAt)
	if err != nil {
		return nil, err
	}
	missingObjectsRaw, err := json.Marshal(objectNames)
	if err != nil {
		return nil, err
	}
	obj["artifact_missing_at"] = missingAtRaw
	obj["artifact_missing_objects"] = missingObjectsRaw
	delete(obj, "download_sessions")
	return json.Marshal(obj)
}

func HashDownloadSessionSecret(secret []byte) string {
	sum := sha256.Sum256(secret)
	return hex.EncodeToString(sum[:])
}

func decodeObject(raw []byte) (map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		return make(map[string]json.RawMessage), nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		obj = make(map[string]json.RawMessage)
	}
	return obj, nil
}

func validateArtifact(artifact Artifact) error {
	if artifact.Version != 1 {
		return fmt.Errorf("unsupported artifact version %d", artifact.Version)
	}
	switch artifact.Format {
	case ArtifactFormatSingleZIP, ArtifactFormatMultipart:
	default:
		return fmt.Errorf("unsupported artifact format %q", artifact.Format)
	}
	if artifact.ExpiresAt.IsZero() {
		return errors.New("artifact expires_at is required")
	}
	if len(artifact.Objects) == 0 {
		return errors.New("artifact must contain at least one object")
	}
	for _, ref := range artifact.Objects {
		if err := validateObjectRef(ref); err != nil {
			return err
		}
	}
	return nil
}

func validateObjectRef(ref ObjectRef) error {
	switch ref.Backend {
	case "local":
		if ref.Path == "" {
			return errors.New("local object path is required")
		}
	case "s3":
		if ref.Bucket == "" || ref.Key == "" {
			return errors.New("s3 object bucket and key are required")
		}
	default:
		return fmt.Errorf("unsupported artifact backend %q", ref.Backend)
	}
	switch ref.Role {
	case RoleSingleZIP, RoleManifest, RoleMetadataZIP, RoleFilesZIP:
	default:
		return fmt.Errorf("unsupported artifact object role %q", ref.Role)
	}
	if ref.Name == "" {
		return errors.New("object name is required")
	}
	if ref.SHA256 == "" {
		return errors.New("object sha256 is required")
	}
	if ref.SizeBytes < 0 {
		return errors.New("object size_bytes must not be negative")
	}
	if ref.ContentType == "" {
		return errors.New("object content_type is required")
	}
	return nil
}
