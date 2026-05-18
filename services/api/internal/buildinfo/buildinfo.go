package buildinfo

var (
	GitCommit     = "unknown"
	CommitTime    = "unknown"
	BuildTime     = "unknown"
	ImageDigest   = ""
	ConfigVersion = "1"
)

type Info struct {
	Service       string `json:"service,omitempty"`
	GitCommit     string `json:"git_commit"`
	CommitTime    string `json:"commit_time"`
	BuildTime     string `json:"build_time"`
	ImageDigest   string `json:"image_digest,omitempty"`
	ConfigVersion string `json:"config_version"`
}

func Snapshot() Info {
	return Info{
		Service:       "api",
		GitCommit:     valueOrUnknown(GitCommit),
		CommitTime:    valueOrUnknown(CommitTime),
		BuildTime:     valueOrUnknown(BuildTime),
		ImageDigest:   ImageDigest,
		ConfigVersion: valueOrUnknown(ConfigVersion),
	}
}

func valueOrUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}
