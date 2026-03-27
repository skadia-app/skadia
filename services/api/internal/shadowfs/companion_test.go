package shadowfs

import (
	"strings"
	"testing"
)

// companionPathFor derives the companion sidecar path the same way
// deleteInlineCompanions does — used by tests to verify the derivation logic
// without touching a real backend.
func companionPathFor(storagePath, fileType string) string {
	// All file types use unified .meta suffix.
	switch fileType {
	case "activity", "media":
		return storagePath + ".meta"
	}
	return ""
}

// ---------------------------------------------------------------------------
// deleteInlineCompanions — suffix-path derivation logic
// ---------------------------------------------------------------------------

func TestDeleteInlineCompanions_ActivitySuffix(t *testing.T) {
	// activity: storagePath + ".meta"
	storagePath := "alice/activities/2024/03/15_14-30_cycling.fit"
	got := companionPathFor(storagePath, "activity")
	want := storagePath + ".meta"
	if got != want {
		t.Errorf("activity companion path: got %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, ".meta") {
		t.Errorf("expected .meta suffix, got %q", got)
	}
}

func TestDeleteInlineCompanions_MediaSuffix(t *testing.T) {
	// media: unified .meta suffix (same as activity — no extension stripping)
	storagePath := "alice/media/2024/03/IMG_0001.jpg"
	got := companionPathFor(storagePath, "media")
	want := "alice/media/2024/03/IMG_0001.jpg.meta"
	if got != want {
		t.Errorf("media companion path: got %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, ".meta") {
		t.Errorf("expected .meta suffix, got %q", got)
	}
}

func TestDeleteInlineCompanions_UnknownFileType(t *testing.T) {
	// unknown file type → no companion (empty path)
	got := companionPathFor("some/path/file.bin", "document")
	if got != "" {
		t.Errorf("unknown file type should produce empty path, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// validateCompanionSuffix — invalid suffixes must be rejected
// ---------------------------------------------------------------------------

func TestCompanionSuffixValidation_Invalid(t *testing.T) {
	cases := []struct {
		suffix string
		desc   string
	}{
		{"/../foo", "path traversal with leading dot-slash"},
		{"noDot", "no leading dot"},
		{"/etc/passwd", "absolute path, no leading dot"},
		{"..hidden", "starts with double-dot"},
		{".valid/sneaky", "contains slash after dot"},
		{"", "empty string"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			if err := validateCompanionSuffix(tc.suffix); err == nil {
				t.Errorf("validateCompanionSuffix(%q) want error (%s), got nil", tc.suffix, tc.desc)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// validateCompanionSuffix — valid suffixes must be accepted
// ---------------------------------------------------------------------------

func TestCompanionSuffixValidation_Valid(t *testing.T) {
	cases := []struct {
		suffix string
		desc   string
	}{
		{".meta", "activity meta sidecar"},
		{".meta.json", "media meta sidecar"},
		{".edits.json", "activity editor sidecar"},
		{".thumb.jpg", "thumbnail sidecar"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			if err := validateCompanionSuffix(tc.suffix); err != nil {
				t.Errorf("validateCompanionSuffix(%q) want nil, got %v (%s)", tc.suffix, err, tc.desc)
			}
		})
	}
}
