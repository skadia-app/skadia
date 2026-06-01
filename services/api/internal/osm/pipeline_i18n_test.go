package osm

import "testing"

type structuredReporterForTest struct {
	phase       string
	failed      string
	messageCode string
	params      map[string]any
}

func (r *structuredReporterForTest) SetPhase(phase string)     { r.phase = phase }
func (r *structuredReporterForTest) SetProgress(current int64) {}
func (r *structuredReporterForTest) SetTotal(total int64)      {}
func (r *structuredReporterForTest) Fail(errMsg string)        { r.failed = errMsg }
func (r *structuredReporterForTest) Complete()                 {}
func (r *structuredReporterForTest) SetMessageCode(code string, params map[string]any) {
	r.messageCode = code
	r.params = params
}

func TestReportPipelineFailureKeepsLegacyMessageAndAddsStructuredCode(t *testing.T) {
	reporter := &structuredReporterForTest{}

	reportPipelineFailure(reporter, "osm.import.download_failed", map[string]any{
		"region_id": "test-region",
		"error":     "network failed",
	}, "Download fehlgeschlagen (Testregion): network failed")

	if reporter.failed != "Download fehlgeschlagen (Testregion): network failed" {
		t.Fatalf("legacy fail message = %q", reporter.failed)
	}
	if reporter.messageCode != "osm.import.download_failed" {
		t.Fatalf("message code = %q, want osm.import.download_failed", reporter.messageCode)
	}
	if reporter.params["region_id"] != "test-region" {
		t.Fatalf("region_id param = %q, want test-region", reporter.params["region_id"])
	}
	if reporter.params["error"] != "network failed" {
		t.Fatalf("error param = %q, want network failed", reporter.params["error"])
	}
}
