package model_test

import (
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestValidateURLTemplate_Valid(t *testing.T) {
	cases := []string{
		"https://tile.openstreetmap.org/{z}/{x}/{y}.png",
		"https://{switch:a,b,c}.tile.example.org/{z}/{x}/{y}.png",
		"http://localhost.example.org/{z}/{x}/{y}", // hostname, not IP literal
		"https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}",
	}
	for _, tpl := range cases {
		if err := model.ValidateURLTemplate(tpl, false); err != nil {
			t.Errorf("expected valid, got error %q for %s", err, tpl)
		}
	}
}

func TestValidateURLTemplate_Invalid(t *testing.T) {
	cases := map[string]string{
		"missing-z":     "https://tile.example.org/{x}/{y}.png",
		"missing-x":     "https://tile.example.org/{z}/{y}.png",
		"missing-y":     "https://tile.example.org/{z}/{x}.png",
		"bad-scheme":    "ftp://tile.example.org/{z}/{x}/{y}.png",
		"data-scheme":   "data:image/png;base64,{z}{x}{y}",
		"unknown-ph":    "https://tile.example.org/{z}/{x}/{y}/{quadkey}",
		"switch-format": "https://{switch:!}.tile.example.org/{z}/{x}/{y}",
		"too-long":      "https://" + strings.Repeat("x", 2050) + "/{z}/{x}/{y}",
	}
	for name, tpl := range cases {
		if err := model.ValidateURLTemplate(tpl, false); err == nil {
			t.Errorf("expected error for %s, got nil", name)
		}
	}
}

func TestValidateURLTemplate_PrivateIPBlocked(t *testing.T) {
	cases := []string{
		"https://10.0.0.1/{z}/{x}/{y}",
		"https://172.16.0.1/{z}/{x}/{y}",
		"https://192.168.1.1/{z}/{x}/{y}",
		"https://127.0.0.1/{z}/{x}/{y}",
		"https://169.254.0.1/{z}/{x}/{y}",
		"https://[::1]/{z}/{x}/{y}",
		"https://[fc00::1]/{z}/{x}/{y}",
	}
	for _, tpl := range cases {
		if err := model.ValidateURLTemplate(tpl, true); err == nil {
			t.Errorf("expected private-IP block for %s, got nil", tpl)
		}
		// Same URL must pass when blockPrivateIPs=false (admin tier).
		if err := model.ValidateURLTemplate(tpl, false); err != nil {
			t.Errorf("admin-tier should not block %s: %v", tpl, err)
		}
	}
}

func TestValidateURLTemplate_SwitchPlaceholder(t *testing.T) {
	// Valid switch placeholders.
	valid := []string{
		"https://{switch:a,b,c}.example.org/{z}/{x}/{y}",
		"https://{switch:tile1,tile2}.example.org/{z}/{x}/{y}",
	}
	for _, tpl := range valid {
		if err := model.ValidateURLTemplate(tpl, false); err != nil {
			t.Errorf("expected valid switch placeholder in %s, got %v", tpl, err)
		}
	}

	// Invalid switch: empty or special chars.
	invalid := []string{
		"https://{switch:}.example.org/{z}/{x}/{y}",
		"https://{switch:a,b!}.example.org/{z}/{x}/{y}",
	}
	for _, tpl := range invalid {
		if err := model.ValidateURLTemplate(tpl, false); err == nil {
			t.Errorf("expected error for invalid switch placeholder in %s, got nil", tpl)
		}
	}
}

func TestSanitizeAttribution(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		badTerms []string
		goodTerm string
	}{
		{
			name:     "legitimate link preserved",
			input:    `&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>`,
			badTerms: nil,
			goodTerm: "OpenStreetMap",
		},
		{
			name:     "script stripped",
			input:    `<script>alert(1)</script>OSM`,
			badTerms: []string{"<script"},
			goodTerm: "",
		},
		{
			name:     "javascript href stripped",
			input:    `<a href="javascript:alert(1)">x</a>`,
			badTerms: []string{"javascript:"},
			goodTerm: "",
		},
		{
			name:     "img onerror stripped",
			input:    `<img src=x onerror=alert(1)>`,
			badTerms: []string{"onerror"},
			goodTerm: "",
		},
		{
			name:     "svg onload stripped",
			input:    `<svg/onload=alert(1)>`,
			badTerms: []string{"onload"},
			goodTerm: "",
		},
		{
			name:     "plain text passes through",
			input:    `OpenStreetMap contributors`,
			badTerms: []string{"<script", "javascript:", "onerror", "onload"},
			goodTerm: "OpenStreetMap",
		},
		{
			// bluemonday's AllowURLSchemes("http","https") must reject schemeless
			// href values like "example" (relative URLs). The link text may survive
			// but the <a> element (or at least the href) must be stripped.
			name:     "schemeless href stripped",
			input:    `<a href="example">text</a>`,
			badTerms: []string{`href="example"`},
			goodTerm: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := model.SanitizeAttribution(tc.input)
			for _, bad := range tc.badTerms {
				if strings.Contains(got, bad) {
					t.Errorf("XSS vector not stripped: found %q in output %q", bad, got)
				}
			}
			if tc.goodTerm != "" && !strings.Contains(got, tc.goodTerm) {
				t.Errorf("legitimate content lost: expected %q in output %q", tc.goodTerm, got)
			}
		})
	}
}
