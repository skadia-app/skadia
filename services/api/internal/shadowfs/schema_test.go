package shadowfs

import "testing"

func TestRenderPath_Activity(t *testing.T) {
	r := NewSchemaRenderer()
	vars := map[string]string{
		"year": "2024", "month": "03", "day": "15",
		"time": "14-30", "sport": "cycling",
	}
	got := r.RenderPath("activity", "{year}/{month}/{day}_{time}_{sport}", vars, "fit")
	want := "2024/03/15_14-30_cycling.fit"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderPath_Media(t *testing.T) {
	r := NewSchemaRenderer()
	vars := map[string]string{
		"year": "2024", "month": "03", "day": "15", "title": "IMG_0001",
	}
	got := r.RenderPath("media", "{year}/{month}/{day}_{title}", vars, "jpg")
	want := "2024/03/15_IMG_0001.jpg"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderPath_Truncation(t *testing.T) {
	r := NewSchemaRenderer()
	longTitle := ""
	for range 60 {
		longTitle += "x"
	}
	vars := map[string]string{"title": longTitle}
	got := r.RenderPath("route", "{title}", vars, "gpx")
	if len(got) > 55 {
		t.Errorf("path too long: %d chars", len(got))
	}
}

func TestRenderPath_Sanitize(t *testing.T) {
	r := NewSchemaRenderer()
	vars := map[string]string{"title": "my:file*name"}
	got := r.RenderPath("route", "{title}", vars, "gpx")
	want := "my_file_name.gpx"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDefaultSchemas(t *testing.T) {
	r := NewSchemaRenderer()
	for _, ft := range []string{"activity", "media", "document", "health", "route"} {
		if r.DefaultSchema(ft) == "" {
			t.Errorf("no default schema for %q", ft)
		}
	}
}

func TestValidateSchema_OK(t *testing.T) {
	r := NewSchemaRenderer()
	warns, err := r.ValidateSchema("activity", "{year}/{month}/{day}_{sport}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) > 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestValidateSchema_TooLong(t *testing.T) {
	r := NewSchemaRenderer()
	long := ""
	for range 200 {
		long += "x"
	}
	warns, _ := r.ValidateSchema("activity", long+"/{year}")
	if len(warns) == 0 {
		t.Error("expected path-length warning")
	}
}
