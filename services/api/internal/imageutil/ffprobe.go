package imageutil

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// VideoMeta enthält die Metadaten eines Videos aus ffprobe.
type VideoMeta struct {
	DurationS float64
	Width     int
	Height    int
}

// ffprobeStreams ist die JSON-Struktur die ffprobe zurückgibt.
type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeStream struct {
	CodecType string `json:"codec_type"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Duration  string `json:"duration"`
}

// ProbeVideo reads video metadata from videoBytes via ffprobe.
// ext is the file extension including dot (for example ".mp4").
func ProbeVideo(videoBytes []byte, ext string) (VideoMeta, error) {
	return ProbeMedia(videoBytes, ext)
}

// ProbeMedia reads dimensions and duration from visual media via ffprobe.
// It also works for still-image formats decoded by ffmpeg but not Go's image package.
func ProbeMedia(mediaBytes []byte, ext string) (VideoMeta, error) {
	// Create a temporary input file so ffprobe can use the file extension.
	tmp, err := os.CreateTemp("", "ffprobe-*"+ext)
	if err != nil {
		return VideoMeta{}, fmt.Errorf("ffprobe temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(mediaBytes); err != nil {
		tmp.Close()
		return VideoMeta{}, fmt.Errorf("ffprobe temp write: %w", err)
	}
	tmp.Close()

	out, err := exec.Command(
		"ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		tmp.Name(),
	).Output()
	if err != nil {
		return VideoMeta{}, fmt.Errorf("ffprobe exec: %w", err)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return VideoMeta{}, fmt.Errorf("ffprobe parse: %w", err)
	}

	var meta VideoMeta
	for _, s := range probe.Streams {
		if s.CodecType == "video" {
			meta.Width = s.Width
			meta.Height = s.Height
			if s.Duration != "" {
				fmt.Sscanf(s.Duration, "%f", &meta.DurationS)
			}
			break
		}
	}
	return meta, nil
}
