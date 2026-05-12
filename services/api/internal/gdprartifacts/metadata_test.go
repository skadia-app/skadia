package gdprartifacts

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestArtifactMetadataRoundTripSingleAndMultipart(t *testing.T) {
	expiresAt := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)

	single := Artifact{
		Version:   1,
		Format:    ArtifactFormatSingleZIP,
		ExpiresAt: expiresAt,
		Objects: []ObjectRef{{
			Backend:     "local",
			Kind:        "export",
			UserID:      "11111111-1111-1111-1111-111111111111",
			JobID:       "22222222-2222-2222-2222-222222222222",
			Role:        RoleSingleZIP,
			Name:        "skadia-export.zip",
			Path:        "11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/export.zip",
			SHA256:      strings.Repeat("a", 64),
			SizeBytes:   123,
			ContentType: "application/zip",
			ExpiresAt:   expiresAt,
		}},
	}

	raw, err := EncodeArtifact(single)
	if err != nil {
		t.Fatalf("EncodeArtifact(single): %v", err)
	}
	meta, err := DecodeJobMetadata(raw)
	if err != nil {
		t.Fatalf("DecodeJobMetadata(single): %v", err)
	}
	if meta.Artifact == nil || meta.Artifact.Format != ArtifactFormatSingleZIP {
		t.Fatalf("single artifact not decoded: %#v", meta.Artifact)
	}
	if got := meta.Artifact.Objects[0].Role; got != RoleSingleZIP {
		t.Fatalf("single role = %q", got)
	}
	if !meta.Artifact.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expires_at = %s, want %s", meta.Artifact.ExpiresAt, expiresAt)
	}

	multipart := Artifact{
		Version:   1,
		Format:    ArtifactFormatMultipart,
		ExpiresAt: expiresAt,
		Objects: []ObjectRef{
			{Backend: "s3", Kind: "export", UserID: single.Objects[0].UserID, JobID: single.Objects[0].JobID, Role: RoleManifest, Name: "manifest.json", Bucket: "skadia-exports", Key: "gdpr/exports/user/job/multipart/manifest.json", SHA256: strings.Repeat("b", 64), SizeBytes: 100, ContentType: "application/json", ExpiresAt: expiresAt},
			{Backend: "s3", Kind: "export", UserID: single.Objects[0].UserID, JobID: single.Objects[0].JobID, Role: RoleMetadataZIP, Name: "metadata.zip", Bucket: "skadia-exports", Key: "gdpr/exports/user/job/multipart/metadata.zip", SHA256: strings.Repeat("c", 64), SizeBytes: 200, ContentType: "application/zip", ExpiresAt: expiresAt},
			{Backend: "s3", Kind: "export", UserID: single.Objects[0].UserID, JobID: single.Objects[0].JobID, Role: RoleFilesZIP, Name: "files-0001.zip", Bucket: "skadia-exports", Key: "gdpr/exports/user/job/multipart/files-0001.zip", SHA256: strings.Repeat("d", 64), SizeBytes: 300, ContentType: "application/zip", ExpiresAt: expiresAt},
		},
		Multipart: &MultipartSummary{PartCount: 3, TotalSizeBytes: 600},
	}

	raw, err = EncodeArtifact(multipart)
	if err != nil {
		t.Fatalf("EncodeArtifact(multipart): %v", err)
	}
	meta, err = DecodeJobMetadata(raw)
	if err != nil {
		t.Fatalf("DecodeJobMetadata(multipart): %v", err)
	}
	if meta.Artifact == nil || meta.Artifact.Format != ArtifactFormatMultipart {
		t.Fatalf("multipart artifact not decoded: %#v", meta.Artifact)
	}
	if meta.Artifact.Multipart == nil || meta.Artifact.Multipart.PartCount != 3 {
		t.Fatalf("multipart summary not decoded: %#v", meta.Artifact.Multipart)
	}
}

func TestArtifactMetadataRejectsUnknownBackendForNewArtifact(t *testing.T) {
	raw := []byte(`{"artifact":{"artifact_version":1,"artifact_format":"single_zip","expires_at":"2026-05-13T12:00:00Z","objects":[{"backend":"ftp","kind":"export","user_id":"11111111-1111-1111-1111-111111111111","job_id":"22222222-2222-2222-2222-222222222222","role":"single_zip","name":"export.zip","path":"x","sha256":"` + strings.Repeat("a", 64) + `","size_bytes":1,"content_type":"application/zip"}]}}`)

	_, err := DecodeJobMetadata(raw)
	if err == nil {
		t.Fatal("DecodeJobMetadata accepted unknown backend")
	}
}

func TestArtifactMetadataDecodesLegacyFilePath(t *testing.T) {
	raw := []byte(`{"file_path":"/tmp/skadia-export-old.zip","sha256":"` + strings.Repeat("a", 64) + `","size_bytes":42}`)

	meta, err := DecodeJobMetadata(raw)
	if err != nil {
		t.Fatalf("DecodeJobMetadata: %v", err)
	}
	if meta.Artifact != nil {
		t.Fatalf("legacy metadata should not synthesize artifact: %#v", meta.Artifact)
	}
	if meta.LegacyFilePath != "/tmp/skadia-export-old.zip" {
		t.Fatalf("LegacyFilePath = %q", meta.LegacyFilePath)
	}
}

func TestArtifactMetadataMergePreservesImportProgressAndResult(t *testing.T) {
	existing := []byte(`{"import_progress":{"rows":10},"import_result":{"status":"partial"},"other":"kept"}`)
	artifact := Artifact{
		Version:   1,
		Format:    ArtifactFormatSingleZIP,
		ExpiresAt: time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC),
		Objects: []ObjectRef{{
			Backend: "local", Kind: "import", UserID: "11111111-1111-1111-1111-111111111111", JobID: "22222222-2222-2222-2222-222222222222",
			Role: RoleSingleZIP, Name: "upload.zip", Path: "11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222/upload.zip",
			SHA256: strings.Repeat("a", 64), SizeBytes: 42, ContentType: "application/zip",
		}},
	}

	merged, err := MergeArtifactJSON(existing, artifact)
	if err != nil {
		t.Fatalf("MergeArtifactJSON: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("merged JSON invalid: %v", err)
	}
	for _, key := range []string{"artifact", "import_progress", "import_result", "other"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("merged metadata missing %q: %s", key, merged)
		}
	}
}

func TestDownloadSessionMetadataStoresOnlyHash(t *testing.T) {
	session := DownloadSessionRecord{
		SessionID:  "session-1",
		SecretHash: strings.Repeat("f", 64),
		UserID:     "11111111-1111-1111-1111-111111111111",
		JobID:      "22222222-2222-2222-2222-222222222222",
		AllowedObjects: []DownloadSessionObject{{
			Name: "export.zip", Role: RoleSingleZIP, SHA256: strings.Repeat("a", 64), SizeBytes: 42,
		}},
		ExpiresAt: time.Date(2026, 5, 12, 12, 1, 0, 0, time.UTC),
	}

	merged, err := AddDownloadSessionJSON([]byte(`{}`), session)
	if err != nil {
		t.Fatalf("AddDownloadSessionJSON: %v", err)
	}
	if bytes.Contains(merged, []byte("raw-secret-value")) {
		t.Fatalf("merged metadata contains raw secret: %s", merged)
	}
	if !bytes.Contains(merged, []byte("secret_hash")) {
		t.Fatalf("merged metadata missing secret_hash: %s", merged)
	}
}

func TestArtifactReservationMetadataRoundTrip(t *testing.T) {
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	reservation := ArtifactReservation{
		ReservationID: "reservation-1",
		Kind:          "export",
		ExpectedBytes: 1024,
		CreatedAt:     now,
	}

	raw, err := ReserveArtifactBytesJSON([]byte(`{"import_progress":{"rows":1}}`), reservation)
	if err != nil {
		t.Fatalf("ReserveArtifactBytesJSON: %v", err)
	}
	raw, err = ReleaseArtifactReservationJSON(raw, "reservation-1", 512, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ReleaseArtifactReservationJSON: %v", err)
	}
	meta, err := DecodeJobMetadata(raw)
	if err != nil {
		t.Fatalf("DecodeJobMetadata: %v", err)
	}
	if len(meta.ArtifactReservations) != 1 {
		t.Fatalf("reservations = %d, want 1", len(meta.ArtifactReservations))
	}
	if got := meta.ArtifactReservations[0].CommittedBytes; got != 512 {
		t.Fatalf("CommittedBytes = %d, want 512", got)
	}
	if meta.ArtifactReservations[0].ReleasedAt == nil {
		t.Fatal("ReleasedAt not decoded")
	}
	if meta.ImportProgress == nil {
		t.Fatal("import_progress was not preserved")
	}
}
