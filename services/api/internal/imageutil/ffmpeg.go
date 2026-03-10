package imageutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// GenerateThumbnailFFmpeg extrahiert einen Frame aus einem Video oder dekodiert ein
// Bildformat das image.Decode() nicht unterstützt (HEIC, JXL).
// Gibt JPEG-Bytes zurück (maxSize × maxSize, Seitenverhältnis beibehalten).
//
// ext ist die Dateiendung inkl. Punkt (z.B. ".mp4", ".heic"), sie wird für die
// temp-Datei verwendet damit ffmpeg das Format korrekt erkennt.
//
// offsetS gibt den Zeitpunkt in Sekunden an, ab dem der Frame entnommen wird.
// 0 = erster Frame (Standard für Thumbnails), >0 = z.B. Mitte des Videos für Preview.
func GenerateThumbnailFFmpeg(inputBytes []byte, ext string, maxSize int, offsetS float64) ([]byte, error) {
	// Temporäre Eingabedatei anlegen
	tmp, err := os.CreateTemp("", "ffmpeg-in-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("ffmpeg temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(inputBytes); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("ffmpeg temp write: %w", err)
	}
	tmp.Close()

	scaleFilter := fmt.Sprintf(
		"scale=%s:%s:force_original_aspect_ratio=decrease",
		strconv.Itoa(maxSize),
		strconv.Itoa(maxSize),
	)

	// -ss vor -i (Input Seeking) ist schneller als Output Seeking für Videos.
	args := []string{"-y"}
	if offsetS > 0 {
		args = append(args, "-ss", strconv.FormatFloat(offsetS, 'f', 3, 64))
	}
	args = append(args,
		"-i", tmp.Name(),
		"-vframes", "1",
		"-vf", scaleFilter,
		"-f", "image2",
		"-vcodec", "mjpeg",
		"pipe:1",
	)
	cmd := exec.Command("ffmpeg", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg exec: %w (stderr: %s)", err, stderr.String())
	}

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg produced no output (stderr: %s)", stderr.String())
	}

	return stdout.Bytes(), nil
}
