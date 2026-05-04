package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type GroundTruthCase struct {
	File             string `yaml:"file"`
	ExpectedRidesMin int    `yaml:"expected_rides_min"`
	ExpectedRidesMax int    `yaml:"expected_rides_max"`
	DistinctLifts    int    `yaml:"distinct_lifts"`
	Sport            string `yaml:"sport"`
	Notes            string `yaml:"notes,omitempty"`
}

type GroundTruth struct {
	Cases []GroundTruthCase `yaml:"cases"`
}

func loadManifest(path string) (*GroundTruth, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var m GroundTruth
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}
