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
func GenerateThumbnailFFmpeg(inputBytes []byte, ext string, maxSize int) ([]byte, error) {
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

	// ffmpeg: ersten Frame als JPEG in stdout ausgeben
	// -vf scale=W:H:force_original_aspect_ratio=decrease hält das Seitenverhältnis
	scaleFilter := fmt.Sprintf(
		"scale=%s:%s:force_original_aspect_ratio=decrease",
		strconv.Itoa(maxSize),
		strconv.Itoa(maxSize),
	)
	cmd := exec.Command(
		"ffmpeg",
		"-y",             // Ausgabe ohne Rückfrage überschreiben
		"-i", tmp.Name(), // Eingabedatei
		"-vframes", "1",  // nur einen Frame extrahieren
		"-vf", scaleFilter,
		"-f", "image2",  // Ausgabeformat: einzelnes Bild
		"-vcodec", "mjpeg", // JPEG-Codec
		"pipe:1", // Ausgabe auf stdout
	)

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
