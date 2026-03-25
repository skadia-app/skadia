package shadowfs

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var illegalChars = regexp.MustCompile(`[/\\:*?"<>|{}]`)

// SchemaRenderer renders file paths from templates and variables.
type SchemaRenderer struct {
	defaults map[string]string
}

// NewSchemaRenderer returns a renderer with default schemas for all file types.
func NewSchemaRenderer() *SchemaRenderer {
	return &SchemaRenderer{
		defaults: map[string]string{
			"activity": "{year}/{month}/{day}_{time}_{sport}",
			"media":    "{year}/{month}/{day}_{title}",
			"document": "{year}/{category}_{title}",
			"health":   "{year}/{month}/{day}_{type}",
			"route":    "{sport}/{title}",
			"report":   "{year}/{title}",
			"chat":     "{year}/{month}/{title}",
		},
	}
}

// DefaultSchema returns the default schema template for a file type.
func (r *SchemaRenderer) DefaultSchema(fileType string) string {
	return r.defaults[fileType]
}

// RenderPath renders a schema template with variables and appends the file extension.
func (r *SchemaRenderer) RenderPath(fileType, schema string, vars map[string]string, ext string) string {
	if schema == "" {
		schema = r.defaults[fileType]
	}
	result := schema
	for key, val := range vars {
		rendered := sanitize(val)
		if len(rendered) > 50 {
			rendered = rendered[:50]
		}
		result = strings.ReplaceAll(result, "{"+key+"}", rendered)
	}
	for i := 0; i < 20 && strings.Contains(result, "{"); i++ {
		start := strings.Index(result, "{")
		end := strings.Index(result, "}")
		if end < 0 {
			break
		}
		result = result[:start] + "unknown" + result[end+1:]
	}
	if ext != "" {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		result += ext
	}
	return result
}

// ValidateSchema checks a schema template for issues.
func (r *SchemaRenderer) ValidateSchema(fileType, schema string) (warnings []string, err error) {
	if schema == "" {
		return nil, fmt.Errorf("schema is empty")
	}
	if strings.ContainsRune(schema, 0) {
		return nil, fmt.Errorf("schema must not contain null bytes")
	}
	if strings.Contains(schema, "..") {
		return nil, fmt.Errorf("schema must not contain '..'")
	}
	if strings.HasPrefix(schema, "/") {
		return nil, fmt.Errorf("schema must not start with '/'")
	}
	if strings.Count(schema, "/") > 6 {
		return nil, fmt.Errorf("schema exceeds maximum directory depth of 6")
	}
	maxVars := map[string]string{
		"year": "2024", "month": "03", "day": "15", "time": "14-30",
		"sport": strings.Repeat("x", 30), "subCategory": strings.Repeat("x", 30),
		"city": strings.Repeat("x", 50), "title": strings.Repeat("x", 50),
		"distance_km": "9999.9", "duration_h": "99-59", "elevation_m": "99999",
		"country": strings.Repeat("x", 30),
		"category": strings.Repeat("x", 30), "type": strings.Repeat("x", 20),
		"parent_type": strings.Repeat("x", 20), "parent_name": strings.Repeat("x", 50),
		"camera": strings.Repeat("x", 30), "width": "9999", "height": "9999",
		"difficulty": strings.Repeat("x", 20),
	}
	rendered := r.RenderPath(fileType, schema, maxVars, "fit")
	if len(rendered) > 190 {
		warnings = append(warnings, fmt.Sprintf("rendered path may exceed 260 chars on Windows (%d chars with max-length variables)", len(rendered)+62))
	}
	return warnings, nil
}

// sanitize normalises a single variable value for use inside a file path component.
// Backtick (`) is intentionally not stripped — it is a valid filename character
// on Linux (the only supported deployment platform) and carries no security risk here.
func sanitize(s string) string {
	s = norm.NFC.String(s)
	s = strings.ReplaceAll(s, "\x00", "")
	s = illegalChars.ReplaceAllString(s, "_")
	s = strings.ReplaceAll(s, "..", "_")
	// Strip leading dots from individual path components (e.g. ".hidden" -> "hidden").
	// Intermediate dots within a component (e.g. "file.fit") are intentionally kept.
	// Path-level dot-dot ("..") is blocked above; full path validation is done by validateLocalPath.
	s = strings.TrimLeft(s, ".")
	return strings.TrimSpace(s)
}
