// Package phash provides perceptual image hashing for photo duplicate detection.
// Uses DCT-based 64-bit PerceptionHash from goimagehash.
// Threshold: ≤3 bits Hamming distance = duplicate (configurable per user).
package phash

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/bits"
	"os/exec"

	"github.com/corona10/goimagehash"
	"github.com/disintegration/imaging"
)

// HammingDistance returns the number of differing bits between two 64-bit hashes.
func HammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// IsDuplicate returns true if hamming distance is within threshold.
func IsDuplicate(a, b uint64, threshold int) bool {
	return HammingDistance(a, b) <= threshold
}

// Compute calculates the 64-bit perceptual hash from an image reader.
// EXIF orientation is auto-applied before hashing via imaging.AutoOrientation.
// Uses io.Reader (imaging.Decode only needs io.Reader, not io.ReadSeeker).
func Compute(r io.Reader) (uint64, error) {
	img, err := imaging.Decode(r, imaging.AutoOrientation(true))
	if err != nil {
		return 0, fmt.Errorf("decode image: %w", err)
	}
	return computeFromImage(img)
}

// ComputeFromBytes calculates pHash from raw image bytes.
func ComputeFromBytes(data []byte) (uint64, error) {
	return Compute(bytes.NewReader(data))
}

// ComputeViaFFmpeg decodes any format (HEIC, JXL, etc.) via ffmpeg pipe, then computes pHash.
func ComputeViaFFmpeg(data []byte) (uint64, error) {
	img, err := decodeViaFFmpeg(data)
	if err != nil {
		return 0, err
	}
	return computeFromImage(img)
}

// ComputeVideoFrame extracts a frame at the given timestamp and computes pHash.
func ComputeVideoFrame(videoData []byte, atSeconds float64) (uint64, error) {
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-ss", fmt.Sprintf("%.2f", atSeconds),
		"-i", "pipe:0",
		"-frames:v", "1",
		"-f", "image2pipe",
		"-vcodec", "png",
		"pipe:1",
	)
	cmd.Stdin = bytes.NewReader(videoData)
	var out bytes.Buffer
	cmd.Stdout = &out
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("ffmpeg video frame: %w: %s", err, stderr.String())
	}
	img, _, err := image.Decode(&out)
	if err != nil {
		return 0, fmt.Errorf("decode video frame: %w", err)
	}
	return computeFromImage(img)
}

// ComputeVideoHash computes pHash for a video using a frame at 10% duration.
func ComputeVideoHash(videoData []byte, durationSeconds float64) (uint64, error) {
	t := durationSeconds * 0.10
	if t < 0.5 {
		t = 0.5
	}
	return ComputeVideoFrame(videoData, t)
}

// FindBestMatch finds the closest pHash match from a list of existing hashes.
// Returns the index of the best match and the hamming distance.
// If no match is within threshold, returns -1.
func FindBestMatch(newHash uint64, existingHashes []uint64, threshold int) (bestIdx int, bestDist int) {
	bestIdx = -1
	bestDist = 65 // impossible high

	for i, existing := range existingHashes {
		if existing == 0 {
			continue // skip photos without pHash
		}
		dist := HammingDistance(newHash, existing)
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}

	if bestDist > threshold {
		return -1, bestDist
	}
	return bestIdx, bestDist
}

func computeFromImage(img image.Image) (uint64, error) {
	hash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return 0, fmt.Errorf("compute phash: %w", err)
	}
	return hash.GetHash(), nil
}

func decodeViaFFmpeg(data []byte) (image.Image, error) {
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
		"-frames:v", "1",
		"-f", "image2pipe",
		"-vcodec", "png",
		"pipe:1",
	)
	cmd.Stdin = bytes.NewReader(data)
	var out bytes.Buffer
	cmd.Stdout = &out
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg convert: %w: %s", err, stderr.String())
	}
	img, _, err := image.Decode(&out)
	if err != nil {
		return nil, fmt.Errorf("decode ffmpeg output: %w", err)
	}
	return img, nil
}
