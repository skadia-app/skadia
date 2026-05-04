package main

import (
	"fmt"
	"os"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
)

func loadFITPoints(path string) ([]lifts.StreamPoint, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := fitimport.ParseFIT(f, path)
	if err != nil {
		return nil, fmt.Errorf("parse fit: %w", err)
	}
	return lifts.ConvertActivityDataToStream(data), nil
}
