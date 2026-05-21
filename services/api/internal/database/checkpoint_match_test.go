package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestMatchCheckpointSequenceV1(t *testing.T) {
	radius := 30.0
	start := checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius)
	mid := checkpointPoint(1, model.CheckpointControlCheckpoint, 8.010, 49.010, radius)
	finish := checkpointPoint(2, model.CheckpointControlFinish, 8.020, 49.020, radius)
	circle := checkpointCircle(1, model.CheckpointControlCheckpoint, 8.010, 49.010, 80)
	polygon := checkpointPolygon(1, model.CheckpointControlCheckpoint, [][][]float64{{
		{8.009, 49.009}, {8.011, 49.009}, {8.011, 49.011}, {8.009, 49.011}, {8.009, 49.009},
	}})

	t.Run("start finish only", func(t *testing.T) {
		attempts, err := MatchCheckpointSequenceV1(samplesForCoords([][2]float64{{8, 49}, {8.02, 49.02}}), []model.CheckpointControl{start, finishForIndex(1, finish)})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 1 {
			t.Fatalf("attempts = %d, want 1", len(attempts))
		}
		if attempts[0].OccurrenceIndex != 0 || len(attempts[0].ControlHits) != 2 {
			t.Fatalf("attempt = %#v", attempts[0])
		}
	})

	t.Run("ordered checkpoint required", func(t *testing.T) {
		attempts, err := MatchCheckpointSequenceV1(samplesForCoords([][2]float64{{8, 49}, {8.01, 49.01}, {8.02, 49.02}}), []model.CheckpointControl{start, mid, finish})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 1 {
			t.Fatalf("attempts = %d, want 1", len(attempts))
		}
	})

	t.Run("skipped checkpoint rejects attempt", func(t *testing.T) {
		attempts, err := MatchCheckpointSequenceV1(samplesForCoords([][2]float64{{8, 49}, {8.02, 49.02}}), []model.CheckpointControl{start, mid, finish})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 0 {
			t.Fatalf("attempts = %d, want 0", len(attempts))
		}
	})

	t.Run("finish before checkpoint is ignored", func(t *testing.T) {
		attempts, err := MatchCheckpointSequenceV1(samplesForCoords([][2]float64{{8, 49}, {8.02, 49.02}, {8.01, 49.01}}), []model.CheckpointControl{start, mid, finish})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 0 {
			t.Fatalf("attempts = %d, want 0", len(attempts))
		}
	})

	t.Run("repeated attempts are deterministic", func(t *testing.T) {
		samples := samplesForCoords([][2]float64{{8, 49}, {8.02, 49.02}, {8, 49}, {8.02, 49.02}, {8, 49}, {8.02, 49.02}})
		attempts, err := MatchCheckpointSequenceV1(samples, []model.CheckpointControl{start, finishForIndex(1, finish)})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 3 {
			t.Fatalf("attempts = %d, want 3", len(attempts))
		}
		for i, attempt := range attempts {
			if attempt.OccurrenceIndex != i || attempt.OccurrenceHash == "" {
				t.Fatalf("attempt[%d] = %#v", i, attempt)
			}
		}
	})

	t.Run("dwell cluster does not reuse finish", func(t *testing.T) {
		samples := samplesForCoords([][2]float64{{8, 49}, {8.00001, 49.00001}, {8.02, 49.02}, {8.02001, 49.02001}})
		attempts, err := MatchCheckpointSequenceV1(samples, []model.CheckpointControl{start, finishForIndex(1, finish)})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 1 {
			t.Fatalf("attempts = %d, want 1", len(attempts))
		}
	})

	t.Run("same sample cannot satisfy adjacent controls", func(t *testing.T) {
		overlapStart := checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, 5)
		overlapFinish := checkpointPoint(1, model.CheckpointControlFinish, 8.0001, 49.0001, 5)
		attempts, err := MatchCheckpointSequenceV1(samplesForCoords([][2]float64{{8, 49}}), []model.CheckpointControl{overlapStart, overlapFinish})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 0 {
			t.Fatalf("attempts = %d, want 0", len(attempts))
		}
	})

	t.Run("point radius rejects outside", func(t *testing.T) {
		attempts, err := MatchCheckpointSequenceV1(samplesForCoords([][2]float64{{8, 49}, {8.03, 49.03}}), []model.CheckpointControl{start, finishForIndex(1, finish)})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 0 {
			t.Fatalf("attempts = %d, want 0", len(attempts))
		}
	})

	t.Run("circle and polygon controls", func(t *testing.T) {
		attempts, err := MatchCheckpointSequenceV1(samplesForCoords([][2]float64{{8, 49}, {8.01, 49.01}, {8.02, 49.02}}), []model.CheckpointControl{start, circle, finish})
		if err != nil {
			t.Fatalf("circle MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 1 {
			t.Fatalf("circle attempts = %d, want 1", len(attempts))
		}
		attempts, err = MatchCheckpointSequenceV1(samplesForCoords([][2]float64{{8, 49}, {8.01, 49.01}, {8.02, 49.02}}), []model.CheckpointControl{start, polygon, finish})
		if err != nil {
			t.Fatalf("polygon MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 1 {
			t.Fatalf("polygon attempts = %d, want 1", len(attempts))
		}
	})
}

func samplesForCoords(coords [][2]float64) []CheckpointActivitySample {
	base := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	samples := make([]CheckpointActivitySample, 0, len(coords))
	for i, coord := range coords {
		distance := float64(i * 100)
		ascent := float64(i * 10)
		samples = append(samples, CheckpointActivitySample{
			Index:     i,
			Time:      base.Add(time.Duration(i) * time.Minute),
			Lng:       coord[0],
			Lat:       coord[1],
			DistanceM: &distance,
			AscentM:   &ascent,
		})
	}
	return samples
}

func finishForIndex(index int, finish model.CheckpointControl) model.CheckpointControl {
	finish.SequenceIndex = index
	return finish
}

func TestInsertCheckpointMatchedEffortsTxIsIdempotentWithEvidence(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	radius := 30.0
	startedAt := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	activityID := insertVersioningTestActivity(t, db, userID, startedAt)

	course, err := db.CreateCheckpointCourse(ctx, userID, &model.CheckpointCourseCreate{
		Name:       "checkpoint-evidence",
		SportGroup: "bike",
		Visibility: "private",
		Controls: []model.CheckpointControl{
			checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius),
			checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius),
		},
	})
	if err != nil {
		t.Fatalf("CreateCheckpointCourse: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = $1`, course.ID)
	})
	versionID := latestSegmentVersionID(t, db, course.ID)
	controls, err := db.GetCheckpointControls(ctx, versionID)
	if err != nil {
		t.Fatalf("GetCheckpointControls: %v", err)
	}
	attempts, err := MatchCheckpointSequenceV1(samplesForCoords([][2]float64{{8, 49}, {8.02, 49.02}}), controls)
	if err != nil {
		t.Fatalf("MatchCheckpointSequenceV1: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(attempts))
	}
	attempt := attempts[0]
	effort := &model.SegmentEffort{
		SegmentID:        course.ID,
		SegmentVersionID: versionID,
		ActivityID:       activityID,
		UserID:           userID,
		StartedAt:        attempt.StartedAt,
		ElapsedTimeS:     attempt.ElapsedTimeS,
		IsReverse:        false,
		SportType:        "cycling",
		SubType:          "mountain",
		DistanceM:        attempt.DistanceM,
		ElevationGainM:   attempt.ElevationGainM,
		OccurrenceIndex:  attempt.OccurrenceIndex,
		OccurrenceHash:   &attempt.OccurrenceHash,
		InLeaderboard:    true,
	}
	PrepareCheckpointSequenceV1EffortMetadata(effort, SegmentMatchActivityContext{})
	match := model.CheckpointMatchedEffort{Effort: effort, ControlHits: attempt.ControlHits}

	for i := 0; i < 2; i++ {
		tx, err := db.Pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		if err := InsertCheckpointMatchedEffortsTx(ctx, tx, []model.CheckpointMatchedEffort{match}); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("InsertCheckpointMatchedEffortsTx run %d: %v", i, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit tx: %v", err)
		}
	}

	var effortCount, evidenceCount int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM segment_efforts
		WHERE segment_id = $1 AND matching_contract_key = 'checkpoint_sequence_v1'
	`, course.ID).Scan(&effortCount); err != nil {
		t.Fatalf("count checkpoint efforts: %v", err)
	}
	if effortCount != 1 {
		t.Fatalf("checkpoint effort count = %d, want 1", effortCount)
	}
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM checkpoint_course_effort_controls c
		JOIN segment_efforts se ON se.id = c.segment_effort_id
		WHERE se.segment_id = $1
	`, course.ID).Scan(&evidenceCount); err != nil {
		t.Fatalf("count checkpoint evidence: %v", err)
	}
	if evidenceCount != 2 {
		t.Fatalf("checkpoint evidence count = %d, want 2", evidenceCount)
	}
}
