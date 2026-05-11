package main

import (
	"fmt"
	"os"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
	"github.com/skadia-app/skadia/services/api/internal/gpximport"
)

func loadGPXPoints(path string) ([]lifts.StreamPoint, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := gpximport.ParseGPX(f, path)
	if err != nil {
		return nil, fmt.Errorf("parse gpx: %w", err)
	}
	return lifts.ConvertActivityDataToStream(data), nil
}
