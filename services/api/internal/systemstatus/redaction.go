package systemstatus

import (
	"regexp"
	"strings"
)

var (
	dsnRE        = regexp.MustCompile(`(?i)\b(?:postgres|postgresql|mysql|redis|nats|s3)://[^\s"']+`)
	signedURLRE  = regexp.MustCompile(`(?i)\bhttps?://[^\s"']*(?:signature|credential|token|secret|access[_-]?key|api[_-]?key)[^\s"']*`)
	pathRE       = regexp.MustCompile(`(?i)(/home|/var/home|/Users|/srv|/mnt|/media|[A-Z]:\\)[^\s"']+`)
	secretRE     = regexp.MustCompile(`(?i)([a-z0-9_-]*(password|passwd|secret|token|signature|credential|access[_-]?key|api[_-]?key)[a-z0-9_-]*)=([^\s&]+)`)
	uuidRE       = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	coordinateRE = regexp.MustCompile(`(?i)\b(lat|latitude|lon|lng|longitude)=(-?\d{1,3}(?:\.\d+)?)`)
	storageIDRE  = regexp.MustCompile(`(?i)\b(bucket|prefix)=([^\s&]+)`)
)

var SafeDetailKeys = map[string]struct{}{
	"backend":                    {},
	"mode":                       {},
	"service":                    {},
	"version":                    {},
	"migration_version":          {},
	"postgis":                    {},
	"server_version":             {},
	"missing_extension":          {},
	"replica_count":              {},
	"configured":                 {},
	"available":                  {},
	"state":                      {},
	"oldest_pending_age_seconds": {},
	"running_count":              {},
	"stuck_jobs_count":           {},
	"failed_last_24h_count":      {},
	"compressed_chunks_count":    {},
	"uncompressed_chunks_count":  {},
}

func RedactDiagnosticString(input string) string {
	out := signedURLRE.ReplaceAllString(input, "[redacted-url]")
	out = dsnRE.ReplaceAllString(out, "[redacted-dsn]")
	out = pathRE.ReplaceAllString(out, "[redacted-path]")
	out = secretRE.ReplaceAllString(out, "$1=[redacted-secret]")
	out = uuidRE.ReplaceAllString(out, "[redacted-id]")
	out = coordinateRE.ReplaceAllString(out, "$1=[redacted-coordinate]")
	out = storageIDRE.ReplaceAllString(out, "$1=[redacted-storage-id]")
	return out
}

func SanitizeComponentStatus(in ComponentStatus) ComponentStatus {
	out := in
	out.Key = RedactDiagnosticString(in.Key)
	out.Label = RedactDiagnosticString(in.Label)
	out.ReasonCode = RedactDiagnosticString(in.ReasonCode)
	out.Summary = RedactDiagnosticString(in.Summary)
	out.CheckedAt = checkedAtOrNow(in.CheckedAt)
	if len(in.Details) == 0 {
		return out
	}
	out.Details = map[string]string{}
	for key, value := range in.Details {
		if isSafeDetailKey(key) {
			out.Details[key] = RedactDiagnosticString(value)
		}
	}
	if len(out.Details) == 0 {
		out.Details = nil
	}
	return out
}

func isSafeDetailKey(key string) bool {
	if _, ok := SafeDetailKeys[key]; ok {
		return true
	}
	return strings.HasPrefix(key, "extension_") ||
		strings.HasPrefix(key, "optional_extension_") ||
		strings.HasSuffix(key, "_count") ||
		strings.HasSuffix(key, "_seconds") ||
		strings.HasSuffix(key, "_bytes")
}

func SanitizeSystemStatusForUI(in SystemStatus) SystemStatus {
	out := in
	out.Build = map[string]BuildInfo{}
	for key, build := range in.Build {
		safeKey := RedactDiagnosticString(key)
		out.Build[safeKey] = BuildInfo{
			Service:       RedactDiagnosticString(build.Service),
			GitCommit:     RedactDiagnosticString(build.GitCommit),
			CommitTime:    RedactDiagnosticString(build.CommitTime),
			BuildTime:     RedactDiagnosticString(build.BuildTime),
			ImageDigest:   RedactDiagnosticString(build.ImageDigest),
			ConfigVersion: RedactDiagnosticString(build.ConfigVersion),
			StartedAt:     RedactDiagnosticString(build.StartedAt),
			UptimeSeconds: build.UptimeSeconds,
		}
	}
	out.Components = map[string]ComponentStatus{}
	for key, component := range in.Components {
		out.Components[RedactDiagnosticString(key)] = SanitizeComponentStatus(component)
	}
	out.FeatureMatrix = map[string]ComponentStatus{}
	for key, component := range in.FeatureMatrix {
		out.FeatureMatrix[RedactDiagnosticString(key)] = SanitizeComponentStatus(component)
	}
	if len(out.FeatureMatrix) == 0 {
		out.FeatureMatrix = nil
	}
	return out
}
