package lifts

type DetectorConfig struct{ /* filled in Phase 3 */ }

func DefaultDetectorConfig(sport string) DetectorConfig { return DetectorConfig{} }

func DetectHeuristic(_ []StreamPoint, _ DetectorConfig) []LiftRide { return nil }
