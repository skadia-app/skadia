package buildinfo

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

func ReadFrontendManifest(ctx context.Context, dir string) (Info, bool) {
	if dir == "" {
		return Info{}, false
	}
	select {
	case <-ctx.Done():
		return Info{}, false
	default:
	}
	raw, err := os.ReadFile(filepath.Join(dir, "skadia-build.json"))
	if err != nil {
		return Info{}, false
	}
	var info Info
	if err := json.Unmarshal(raw, &info); err != nil {
		return Info{}, false
	}
	info.Service = valueOr(info.Service, "frontend")
	info.GitCommit = valueOrUnknown(info.GitCommit)
	info.CommitTime = valueOrUnknown(info.CommitTime)
	info.BuildTime = valueOrUnknown(info.BuildTime)
	info.ConfigVersion = valueOrUnknown(info.ConfigVersion)
	return info, true
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
