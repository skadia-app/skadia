package database

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

	t.Run("backtracking matches later ordered finish", func(t *testing.T) {
		attempts, err := MatchCheckpointSequenceV1(samplesForCoords([][2]float64{{8, 49}, {8.02, 49.02}, {8.01, 49.01}, {8.02, 49.02}}), []model.CheckpointControl{start, mid, finish})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 1 {
			t.Fatalf("attempts = %d, want 1", len(attempts))
		}
		if got := attempts[0].ControlHits[2].ActivityStreamIndex; got != 3 {
			t.Fatalf("finish stream index = %d, want 3", got)
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

	t.Run("overlapping attempts keep separate start entries", func(t *testing.T) {
		samples := samplesForCoords([][2]float64{
			{8.000, 49.000},
			{8.005, 49.005},
			{8.000, 49.000},
			{8.010, 49.010},
			{8.020, 49.020},
		})
		attempts, err := MatchCheckpointSequenceV1(samples, []model.CheckpointControl{start, mid, finish})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1: %v", err)
		}
		if len(attempts) != 2 {
			t.Fatalf("attempts = %d, want 2: %#v", len(attempts), attempts)
		}
		for i, attempt := range attempts {
			if attempt.OccurrenceIndex != i {
				t.Fatalf("attempt[%d].OccurrenceIndex = %d, want %d", i, attempt.OccurrenceIndex, i)
			}
			if got := attempt.ControlHits[0].ActivityStreamIndex; got != i*2 {
				t.Fatalf("attempt[%d] start stream index = %d, want %d", i, got, i*2)
			}
			if got := attempt.ControlHits[1].ActivityStreamIndex; got != 3 {
				t.Fatalf("attempt[%d] checkpoint stream index = %d, want 3", i, got)
			}
			if got := attempt.ControlHits[2].ActivityStreamIndex; got != 4 {
				t.Fatalf("attempt[%d] finish stream index = %d, want 4", i, got)
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

	t.Run("non increasing timestamps reject attempt", func(t *testing.T) {
		samples := samplesForCoords([][2]float64{{8, 49}, {8.02, 49.02}})
		samples[1].Time = samples[0].Time
		attempts, err := MatchCheckpointSequenceV1(samples, []model.CheckpointControl{start, finishForIndex(1, finish)})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1 equal timestamp: %v", err)
		}
		if len(attempts) != 0 {
			t.Fatalf("equal timestamp attempts = %d, want 0", len(attempts))
		}

		samples = samplesForCoords([][2]float64{{8, 49}, {8.02, 49.02}})
		samples[1].Time = samples[0].Time.Add(-time.Second)
		attempts, err = MatchCheckpointSequenceV1(samples, []model.CheckpointControl{start, finishForIndex(1, finish)})
		if err != nil {
			t.Fatalf("MatchCheckpointSequenceV1 decreasing timestamp: %v", err)
		}
		if len(attempts) != 0 {
			t.Fatalf("decreasing timestamp attempts = %d, want 0", len(attempts))
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

func TestGetCheckpointCandidateSegmentsBatchLoadsControls(t *testing.T) {
	counter := &checkpointControlQueryCounter{}
	db, userID := checkpointCandidateTraceTestDB(t, counter)
	ctx := context.Background()
	radius := 30.0
	activityDate := time.Now().UTC()
	activityID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, title, started_at, file_hash, visibility, enrichment_status)
		VALUES ($1, $2, 'cycling', '', false, false, false, 'Checkpoint Candidate Batch Test', $3, $4, 'private', 'complete')
	`, activityID, userID, activityDate, "checkpoint-candidate-batch-"+activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		VALUES (
			$1,
			ST_GeomFromText('LINESTRING(8 49, 8.02 49.02)', 4326),
			ST_GeomFromText('LINESTRING(8 49, 8.02 49.02)', 4326),
			ST_Envelope(ST_GeomFromText('LINESTRING(8 49, 8.02 49.02)', 4326)),
			2
		)
	`, activityID); err != nil {
		t.Fatalf("insert activity track: %v", err)
	}

	courses := make([]*model.Segment, 0, 2)
	for i := range 2 {
		course, err := db.CreateCheckpointCourse(ctx, userID, &model.CheckpointCourseCreate{
			Name:       "checkpoint-candidate-batch",
			SportGroup: "bike",
			Visibility: "private",
			Controls: []model.CheckpointControl{
				checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000+float64(i)*0.00001, radius),
				checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020+float64(i)*0.00001, radius),
			},
		})
		if err != nil {
			t.Fatalf("CreateCheckpointCourse %d: %v", i, err)
		}
		courses = append(courses, course)
	}
	t.Cleanup(func() {
		for _, course := range courses {
			_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = $1`, course.ID)
		}
	})

	counter.controlSelects.Store(0)
	candidates, err := db.GetCheckpointCandidateSegments(ctx, activityID, userID, candidateBucketKey(t, "cycling", ""), activityDate)
	if err != nil {
		t.Fatalf("GetCheckpointCandidateSegments: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %d, want 2: %#v", len(candidates), candidates)
	}
	for _, candidate := range candidates {
		if len(candidate.Controls) != 2 {
			t.Fatalf("candidate %s controls = %d, want 2", candidate.SegmentID, len(candidate.Controls))
		}
	}
	if got := counter.controlSelects.Load(); got != 1 {
		t.Fatalf("checkpoint control SELECTs = %d, want 1", got)
	}
}

type checkpointControlQueryCounter struct {
	controlSelects atomic.Int64
}

func (c *checkpointControlQueryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "ST_AsGeoJSON(geom)") && strings.Contains(data.SQL, "FROM course_version_controls") {
		c.controlSelects.Add(1)
	}
	return ctx
}

func (c *checkpointControlQueryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {
}

func checkpointCandidateTraceTestDB(t *testing.T, counter *checkpointControlQueryCounter) (*DB, string) {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	config.ConnConfig.Tracer = counter
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, "checkpoint-batch-"+userID[:8], "checkpoint-batch-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM activities WHERE user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	return &DB{Pool: pool}, userID
}
