package fitimport

import (
	"testing"

	"github.com/muktihari/fit/profile/mesgdef"
)

func TestExtractDeveloperApps_KnownStrydModern(t *testing.T) {
	appID := mustDecodeHex("18fb2cf01a4b430dad66988c847421f4")
	devDataIds := []*mesgdef.DeveloperDataId{
		{ApplicationId: appID},
	}

	result := ExtractDeveloperApps(devDataIds)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].AppName != "Stryd" {
		t.Errorf("AppName: got %q, want %q", result[0].AppName, "Stryd")
	}
	if result[0].AppCategory != "app_running_power" {
		t.Errorf("AppCategory: got %q, want %q", result[0].AppCategory, "app_running_power")
	}
}

func TestExtractDeveloperApps_UnknownSkipped(t *testing.T) {
	devDataIds := []*mesgdef.DeveloperDataId{
		{ApplicationId: []byte{0xde, 0xad, 0xbe, 0xef}},
	}

	result := ExtractDeveloperApps(devDataIds)
	if len(result) != 0 {
		t.Errorf("expected 0 results for unknown app, got %d", len(result))
	}
}

func TestExtractDeveloperApps_DuplicateOnce(t *testing.T) {
	appID := mustDecodeHex("18fb2cf01a4b430dad66988c847421f4")
	devDataIds := []*mesgdef.DeveloperDataId{
		{ApplicationId: appID},
		{ApplicationId: appID},
	}

	result := ExtractDeveloperApps(devDataIds)
	if len(result) != 1 {
		t.Errorf("expected 1 result for duplicate app ID, got %d", len(result))
	}
}

func TestExtractDeveloperApps_NilApplicationId(t *testing.T) {
	devDataIds := []*mesgdef.DeveloperDataId{
		{ApplicationId: nil},
	}

	result := ExtractDeveloperApps(devDataIds)
	if len(result) != 0 {
		t.Errorf("expected 0 results for nil ApplicationId, got %d", len(result))
	}
}

func TestExtractDeveloperApps_Empty(t *testing.T) {
	result := ExtractDeveloperApps(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 results for nil input, got %d", len(result))
	}

	result = ExtractDeveloperApps([]*mesgdef.DeveloperDataId{})
	if len(result) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(result))
	}
}

func TestExtractDeveloperApps_Zepp4ByteAppID(t *testing.T) {
	devDataIds := []*mesgdef.DeveloperDataId{
		{ApplicationId: []byte{0x7a, 0x65, 0x70, 0x70}},
	}

	result := ExtractDeveloperApps(devDataIds)
	if len(result) != 1 {
		t.Fatalf("expected 1 result for Zepp 4-byte app ID, got %d", len(result))
	}
	if result[0].AppName != "Zepp" {
		t.Errorf("AppName: got %q, want %q", result[0].AppName, "Zepp")
	}
	if result[0].AppCategory != "app_wearable" {
		t.Errorf("AppCategory: got %q, want %q", result[0].AppCategory, "app_wearable")
	}
}

func TestExtractDeveloperApps_COROS(t *testing.T) {
	// COROS: 16 bytes, last byte 0x47. Exports Stryd-compatible fields
	// (Form Power, Air Power, LSS) plus COROS-own (Effort Pace, Sat, cog, hdop).
	appID := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x47}
	devDataIds := []*mesgdef.DeveloperDataId{
		{ApplicationId: appID},
	}

	result := ExtractDeveloperApps(devDataIds)
	if len(result) != 1 {
		t.Fatalf("expected 1 result for COROS, got %d", len(result))
	}
	if result[0].AppName != "COROS" {
		t.Errorf("AppName: got %q, want %q", result[0].AppName, "COROS")
	}
	if result[0].AppCategory != "app_running_power" {
		t.Errorf("AppCategory: got %q, want %q", result[0].AppCategory, "app_running_power")
	}
}

func TestExtractDeveloperApps_CORE(t *testing.T) {
	appID := mustDecodeHex("6957fe6883fe4ed68613413f70624bb5")
	devDataIds := []*mesgdef.DeveloperDataId{
		{ApplicationId: appID},
	}

	result := ExtractDeveloperApps(devDataIds)
	if len(result) != 1 {
		t.Fatalf("expected 1 result for CORE, got %d", len(result))
	}
	if result[0].AppName != "CORE" {
		t.Errorf("AppName: got %q, want %q", result[0].AppName, "CORE")
	}
	if result[0].AppCategory != "app_body_temp" {
		t.Errorf("AppCategory: got %q, want %q", result[0].AppCategory, "app_body_temp")
	}
}

func TestExtractDeveloperApps_SuuntoFitExport1(t *testing.T) {
	appID := mustDecodeHex("5375756e746f4669744578706f727431")
	devDataIds := []*mesgdef.DeveloperDataId{
		{ApplicationId: appID},
	}

	result := ExtractDeveloperApps(devDataIds)
	if len(result) != 1 {
		t.Fatalf("expected 1 result for SuuntoFitExport1, got %d", len(result))
	}
	if result[0].AppName != "SuuntoFitExport1" {
		t.Errorf("AppName: got %q, want %q", result[0].AppName, "SuuntoFitExport1")
	}
	if result[0].AppCategory != "app_sport_watch" {
		t.Errorf("AppCategory: got %q, want %q", result[0].AppCategory, "app_sport_watch")
	}
}

func TestExtractDeveloperApps_SuuntoplusFitExt(t *testing.T) {
	appID := mustDecodeHex("5375756e746f706c7573466974457874")
	devDataIds := []*mesgdef.DeveloperDataId{
		{ApplicationId: appID},
	}

	result := ExtractDeveloperApps(devDataIds)
	if len(result) != 1 {
		t.Fatalf("expected 1 result for SuuntoplusFitExt, got %d", len(result))
	}
	if result[0].AppName != "SuuntoplusFitExt" {
		t.Errorf("AppName: got %q, want %q", result[0].AppName, "SuuntoplusFitExt")
	}
	if result[0].AppCategory != "app_sport_watch" {
		t.Errorf("AppCategory: got %q, want %q", result[0].AppCategory, "app_sport_watch")
	}
}

func TestExtractDeveloperApps_VariaRadar(t *testing.T) {
	appID := mustDecodeHex("c5d949c39acb4e00bb2dc3b871e9e733")
	devDataIds := []*mesgdef.DeveloperDataId{
		{ApplicationId: appID},
	}

	result := ExtractDeveloperApps(devDataIds)
	if len(result) != 1 {
		t.Fatalf("expected 1 result for Varia Radar, got %d", len(result))
	}
	if result[0].AppName != "Varia Radar" {
		t.Errorf("AppName: got %q, want %q", result[0].AppName, "Varia Radar")
	}
	if result[0].AppCategory != "app_radar" {
		t.Errorf("AppCategory: got %q, want %q", result[0].AppCategory, "app_radar")
	}
}

func TestExtractDeveloperApps_IntervalsCIQ(t *testing.T) {
	appID := mustDecodeHex("2752dcf8d7c611eda31f1b4b4540e742")
	devDataIds := []*mesgdef.DeveloperDataId{
		{ApplicationId: appID},
	}

	result := ExtractDeveloperApps(devDataIds)
	if len(result) != 1 {
		t.Fatalf("expected 1 result for Intervals CIQ, got %d", len(result))
	}
	if result[0].AppName != "Intervals CIQ" {
		t.Errorf("AppName: got %q, want %q", result[0].AppName, "Intervals CIQ")
	}
	if result[0].AppCategory != "app_training" {
		t.Errorf("AppCategory: got %q, want %q", result[0].AppCategory, "app_training")
	}
}

// mustDecodeHex decodes a lowercase hex string into bytes, panicking on error.
func mustDecodeHex(s string) []byte {
	if len(s)%2 != 0 {
		panic("mustDecodeHex: odd length hex string: " + s)
	}
	b := make([]byte, len(s)/2)
	for i := range len(s) / 2 {
		hi := hexNibble(s[i*2])
		lo := hexNibble(s[i*2+1])
		b[i] = hi<<4 | lo
	}
	return b
}

func hexNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		panic("mustDecodeHex: invalid hex char")
	}
}
