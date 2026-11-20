package handler

import (
	"encoding/json"
	"testing"
)

func TestSanitizeTileProxyScheme(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "https"},
		{"https", "https"},
		{"HTTP", "http"},
		{"http, https", "http"},
		{"javascript", "https"},
		{"http\"><script>", "https"},
	}
	for _, tt := range tests {
		if got := sanitizeTileProxyScheme(tt.in); got != tt.want {
			t.Errorf("sanitizeTileProxyScheme(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSanitizeTileProxyHost(t *testing.T) {
	valid := []string{
		"maps.example.test",
		"maps.example.test:8443",
		"127.0.0.1:8080",
		"[::1]:8080",
		"maps.example.test, proxy.internal",
	}
	for _, in := range valid {
		if got, ok := sanitizeTileProxyHost(in); !ok || got == "" {
			t.Errorf("sanitizeTileProxyHost(%q) = (%q, %v), want ok", in, got, ok)
		}
	}

	invalid := []string{
		"",
		`evil.example"><script>alert(1)</script>`,
		`evil.example/path`,
		`user@example.invalid`,
		`evil.example?x=1`,
		`evil.example#frag`,
		"evil.example\r\nX-Injected: 1",
		"evil.example \"quoted\"",
	}
	for _, in := range invalid {
		if got, ok := sanitizeTileProxyHost(in); ok {
			t.Errorf("sanitizeTileProxyHost(%q) = (%q, true), want rejected", in, got)
		}
	}
}

func TestRewriteMartinJSONURLs(t *testing.T) {
	raw := `{"tiles":["http://martin:3000/heatmap/{z}/{x}/{y}"],"base":"http://martin:3000","nested":{"v":1}}`
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rewritten := rewriteMartinJSONURLs(parsed, "https://maps.example.test/api/v1/tiles")
	out, err := json.Marshal(rewritten)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	tiles := got["tiles"].([]any)
	if tiles[0] != "https://maps.example.test/api/v1/tiles/heatmap/{z}/{x}/{y}" {
		t.Errorf("rewritten tile URL = %v", tiles[0])
	}
	if got["base"] != "https://maps.example.test/api/v1/tiles" {
		t.Errorf("rewritten base URL = %v", got["base"])
	}
}
