package database

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestValidateCheckpointControls(t *testing.T) {
	radius30 := 30.0
	radius5001 := 5001.0
	areaRadius := 80.0

	tests := []struct {
		name     string
		controls []model.CheckpointControl
		wantErr  string
	}{
		{
			name: "valid start checkpoint finish point sequence",
			controls: []model.CheckpointControl{
				checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius30),
				checkpointPoint(1, model.CheckpointControlCheckpoint, 8.010, 49.010, radius30),
				checkpointPoint(2, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
		},
		{
			name: "valid start finish area sequence",
			controls: []model.CheckpointControl{
				checkpointCircle(0, model.CheckpointControlStart, 8.000, 49.000, areaRadius),
				checkpointPolygon(1, model.CheckpointControlFinish, [][][]float64{{
					{8.020, 49.020}, {8.021, 49.020}, {8.021, 49.021}, {8.020, 49.021}, {8.020, 49.020},
				}}),
			},
		},
		{
			name: "reject missing start",
			controls: []model.CheckpointControl{
				checkpointPoint(0, model.CheckpointControlCheckpoint, 8.000, 49.000, radius30),
				checkpointPoint(1, model.CheckpointControlFinish, 8.010, 49.010, radius30),
			},
			wantErr: "first control must be start",
		},
		{
			name: "reject missing finish",
			controls: []model.CheckpointControl{
				checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius30),
				checkpointPoint(1, model.CheckpointControlCheckpoint, 8.010, 49.010, radius30),
			},
			wantErr: "last control must be finish",
		},
		{
			name: "reject two starts",
			controls: []model.CheckpointControl{
				checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius30),
				checkpointPoint(1, model.CheckpointControlStart, 8.010, 49.010, radius30),
				checkpointPoint(2, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
			wantErr: "intermediate controls must be checkpoint",
		},
		{
			name: "reject skipped sequence index",
			controls: []model.CheckpointControl{
				checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius30),
				checkpointPoint(2, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
			wantErr: "sequence_index",
		},
		{
			name: "reject optional control",
			controls: []model.CheckpointControl{
				checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius30),
				func() model.CheckpointControl {
					c := checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius30)
					c.Required = false
					return c
				}(),
			},
			wantErr: "required",
		},
		{
			name: "reject point radius zero",
			controls: []model.CheckpointControl{
				checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, 0),
				checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
			wantErr: "hit_radius_m",
		},
		{
			name: "reject point radius too large",
			controls: []model.CheckpointControl{
				checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius5001),
				checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
			wantErr: "hit_radius_m",
		},
		{
			name: "reject point control with polygon geometry",
			controls: []model.CheckpointControl{
				func() model.CheckpointControl {
					c := checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius30)
					c.Geometry = mustCheckpointJSON(map[string]any{"type": "Polygon", "coordinates": [][][]float64{{{8, 49}, {8.1, 49}, {8.1, 49.1}, {8, 49}}}})
					return c
				}(),
				checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
			wantErr: "Point",
		},
		{
			name: "reject area control with hit radius",
			controls: []model.CheckpointControl{
				func() model.CheckpointControl {
					c := checkpointCircle(0, model.CheckpointControlStart, 8.000, 49.000, areaRadius)
					c.HitRadiusM = &radius30
					return c
				}(),
				checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
			wantErr: "hit_radius_m",
		},
		{
			name: "reject circle without area radius",
			controls: []model.CheckpointControl{
				func() model.CheckpointControl {
					c := checkpointCircle(0, model.CheckpointControlStart, 8.000, 49.000, areaRadius)
					c.AreaRadiusM = nil
					return c
				}(),
				checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
			wantErr: "area_radius_m",
		},
		{
			name: "reject circle with polygon geometry",
			controls: []model.CheckpointControl{
				func() model.CheckpointControl {
					c := checkpointCircle(0, model.CheckpointControlStart, 8.000, 49.000, areaRadius)
					c.Geometry = mustCheckpointJSON(map[string]any{"type": "Polygon", "coordinates": [][][]float64{{{8, 49}, {8.1, 49}, {8.1, 49.1}, {8, 49}}}})
					return c
				}(),
				checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
			wantErr: "Point",
		},
		{
			name: "reject polygon area with point geometry",
			controls: []model.CheckpointControl{
				func() model.CheckpointControl {
					c := checkpointPolygon(0, model.CheckpointControlStart, [][][]float64{{{8, 49}, {8.1, 49}, {8.1, 49.1}, {8, 49}}})
					c.Geometry = mustCheckpointJSON(map[string]any{"type": "Point", "coordinates": []float64{8, 49}})
					return c
				}(),
				checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
			wantErr: "Polygon",
		},
		{
			name: "reject polygon area with radius",
			controls: []model.CheckpointControl{
				func() model.CheckpointControl {
					c := checkpointPolygon(0, model.CheckpointControlStart, [][][]float64{{
						{8, 49}, {8.1, 49}, {8.1, 49.1}, {8, 49.1}, {8, 49},
					}})
					c.AreaRadiusM = &areaRadius
					return c
				}(),
				checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
			wantErr: "area_radius_m",
		},
		{
			name: "reject invalid polygon rings",
			controls: []model.CheckpointControl{
				checkpointPolygon(0, model.CheckpointControlStart, [][][]float64{{{8, 49}, {8.1, 49}, {8.1, 49.1}, {8, 49.1}}}),
				checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius30),
			},
			wantErr: "closed",
		},
		{
			name: "reject identical start finish",
			controls: []model.CheckpointControl{
				checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius30),
				checkpointPoint(1, model.CheckpointControlFinish, 8.000, 49.000, radius30),
			},
			wantErr: "overlap",
		},
		{
			name: "reject overlapping required controls",
			controls: []model.CheckpointControl{
				checkpointPoint(0, model.CheckpointControlStart, 8.000000, 49.000000, radius30),
				checkpointPoint(1, model.CheckpointControlCheckpoint, 8.000010, 49.000010, radius30),
				checkpointPoint(2, model.CheckpointControlFinish, 8.020000, 49.020000, radius30),
			},
			wantErr: "overlap",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCheckpointControls(tc.controls, true)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateCheckpointControls returned error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateCheckpointControls returned nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestCreateCheckpointCourse(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	radius := 30.0

	course, err := db.CreateCheckpointCourse(ctx, userID, &model.CheckpointCourseCreate{
		Name:       "Checkpoint persistence",
		SportGroup: "bike",
		Visibility: "private",
		Controls: []model.CheckpointControl{
			checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius),
			checkpointPoint(1, model.CheckpointControlCheckpoint, 8.010, 49.010, radius),
			checkpointPoint(2, model.CheckpointControlFinish, 8.020, 49.020, radius),
		},
	})
	if err != nil {
		t.Fatalf("CreateCheckpointCourse: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = $1`, course.ID)
	})

	if course.CourseKind != string(model.CourseKindCheckpointSequence) {
		t.Fatalf("course_kind = %q", course.CourseKind)
	}
	if course.SegmentType != "standard" {
		t.Fatalf("segment_type = %q, want standard legacy compatibility", course.SegmentType)
	}
	if course.ControlSummary == nil || course.ControlSummary.ControlCount != 3 || course.ControlSummary.RequiredControlCount != 3 {
		t.Fatalf("control summary = %#v", course.ControlSummary)
	}
	if course.TrackGeoJSON == nil || !strings.Contains(*course.TrackGeoJSON, "LineString") {
		t.Fatalf("display LineString missing: %#v", course.TrackGeoJSON)
	}

	versionID := latestSegmentVersionID(t, db, course.ID)
	var contractKey string
	if err := db.Pool.QueryRow(ctx, `
		SELECT matching_contract_key
		FROM segment_versions
		WHERE id = $1
	`, versionID).Scan(&contractKey); err != nil {
		t.Fatalf("query segment version contract: %v", err)
	}
	if contractKey != CheckpointSequenceV1Key {
		t.Fatalf("matching_contract_key = %q", contractKey)
	}

	controls, err := db.GetCheckpointControls(ctx, versionID)
	if err != nil {
		t.Fatalf("GetCheckpointControls: %v", err)
	}
	if len(controls) != 3 {
		t.Fatalf("controls count = %d, want 3", len(controls))
	}
	if controls[0].Role != model.CheckpointControlStart || controls[2].Role != model.CheckpointControlFinish {
		t.Fatalf("roles = %s/%s", controls[0].Role, controls[2].Role)
	}

	var projectionCount int
	if err := db.Pool.QueryRow(ctx, `
		SELECT control_count
		FROM checkpoint_course_projection
		WHERE segment_id = $1
	`, course.ID).Scan(&projectionCount); err != nil {
		t.Fatalf("query checkpoint projection: %v", err)
	}
	if projectionCount != 3 {
		t.Fatalf("projection control_count = %d, want 3", projectionCount)
	}

	var eligibilityCount int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM target_course_eligibility_projection
		WHERE target_id = $1
		  AND matching_contract_key = 'checkpoint_sequence_v1'
	`, course.ID).Scan(&eligibilityCount); err != nil {
		t.Fatalf("query eligibility projection: %v", err)
	}
	if eligibilityCount == 0 {
		t.Fatal("checkpoint course eligibility projection missing")
	}
}

func TestListCheckpointCoursesKeepsDefaultLineOnly(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	radius := 30.0

	line := createCompletedFilterSegment(t, db, userID, "checkpoint-list-line")
	checkpoint, err := db.CreateCheckpointCourse(ctx, userID, &model.CheckpointCourseCreate{
		Name:       "checkpoint-list-course",
		SportGroup: "bike",
		Visibility: "public",
		Controls: []model.CheckpointControl{
			checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius),
			checkpointPoint(1, model.CheckpointControlFinish, 8.020, 49.020, radius),
		},
	})
	if err != nil {
		t.Fatalf("CreateCheckpointCourse: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = ANY($1::uuid[])`, []string{line.ID, checkpoint.ID})
	})

	defaultList, err := db.ListSegments(ctx, model.ListSegmentsParams{
		UserID: userID,
		Name:   "checkpoint-list-",
		Limit:  -1,
	})
	if err != nil {
		t.Fatalf("ListSegments default: %v", err)
	}
	if len(defaultList.Segments) != 1 || defaultList.Segments[0].ID != line.ID {
		t.Fatalf("default list = %#v, want only line segment %s", defaultList.Segments, line.ID)
	}

	checkpointList, err := db.ListSegments(ctx, model.ListSegmentsParams{
		UserID:                    userID,
		CourseKind:                string(model.CourseKindCheckpointSequence),
		IncludeGeoJSON:            true,
		IncludeControlMapFeatures: true,
		Limit:                     -1,
	})
	if err != nil {
		t.Fatalf("ListSegments checkpoint: %v", err)
	}
	if len(checkpointList.Segments) != 1 || checkpointList.Segments[0].ID != checkpoint.ID {
		t.Fatalf("checkpoint list = %#v, want %s", checkpointList.Segments, checkpoint.ID)
	}
	got := checkpointList.Segments[0]
	if got.CourseKind != string(model.CourseKindCheckpointSequence) {
		t.Fatalf("course_kind = %q", got.CourseKind)
	}
	if got.ControlSummary == nil || got.ControlSummary.ControlCount != 2 {
		t.Fatalf("control_summary = %#v", got.ControlSummary)
	}
	if got.TrackGeoJSON == nil || !strings.Contains(*got.TrackGeoJSON, "LineString") {
		t.Fatalf("display line missing from checkpoint list: %#v", got.TrackGeoJSON)
	}
	if len(got.ControlMapFeatures) != 2 {
		t.Fatalf("control_map_features count = %d, want 2", len(got.ControlMapFeatures))
	}
}

func TestCheckpointCourseRejectsLineVersionWorkflows(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	radius := 30.0

	course, err := db.CreateCheckpointCourse(ctx, userID, &model.CheckpointCourseCreate{
		Name:       "checkpoint-line-guard",
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

	if _, err := db.UpdateSegment(ctx, course.ID, userID, false, &model.SegmentUpdate{NewVersion: true}); err == nil {
		t.Fatal("UpdateSegment new_version returned nil error for checkpoint course")
	} else if !strings.Contains(err.Error(), "checkpoint") {
		t.Fatalf("UpdateSegment error = %v, want checkpoint guard", err)
	}

	versionID := latestSegmentVersionID(t, db, course.ID)
	update := model.SegmentGeometryUpdate{
		OperationID:       "checkpoint-line-guard-geometry",
		TrackGeoJSON:      json.RawMessage(`{"type":"LineString","coordinates":[[8,49],[8.03,49.03]]}`),
		StartPointGeoJSON: json.RawMessage(`{"type":"Point","coordinates":[8,49]}`),
		EndPointGeoJSON:   json.RawMessage(`{"type":"Point","coordinates":[8.03,49.03]}`),
		Recalculate:       true,
	}
	if _, err := db.UpdateSegmentVersionGeometry(ctx, course.ID, versionID, userID, false, update); err == nil {
		t.Fatal("UpdateSegmentVersionGeometry returned nil error for checkpoint course")
	} else if !strings.Contains(err.Error(), "checkpoint") {
		t.Fatalf("UpdateSegmentVersionGeometry error = %v, want checkpoint guard", err)
	}
}

func TestCheckpointCourseRollbackRestoresControls(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()
	radius := 30.0
	future := time.Now().UTC().Add(24 * time.Hour)

	course, err := db.CreateCheckpointCourse(ctx, userID, &model.CheckpointCourseCreate{
		Name:       "checkpoint-rollback-controls",
		SportGroup: "bike",
		Visibility: "private",
		Controls: []model.CheckpointControl{
			func() model.CheckpointControl {
				c := checkpointPoint(0, model.CheckpointControlStart, 8.000, 49.000, radius)
				c.Label = "before"
				return c
			}(),
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
	revision, err := db.UpdateSegmentVersionValidity(ctx, course.ID, versionID, userID, false, model.SegmentVersionValidityUpdate{
		OperationID:  "checkpoint-rollback-validity",
		ValidFromAt:  nil,
		ValidUntilAt: &future,
		Recalculate:  true,
	})
	if err != nil {
		t.Fatalf("UpdateSegmentVersionValidity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		UPDATE course_version_controls
		SET label = 'mutated'
		WHERE segment_version_id = $1 AND sequence_index = 0
	`, versionID); err != nil {
		t.Fatalf("mutate checkpoint control: %v", err)
	}

	if _, err := db.RollbackSegmentVersionRevision(ctx, course.ID, revision.ID, userID, false, "restore controls"); err != nil {
		t.Fatalf("RollbackSegmentVersionRevision: %v", err)
	}
	controls, err := db.GetCheckpointControls(ctx, versionID)
	if err != nil {
		t.Fatalf("GetCheckpointControls: %v", err)
	}
	if len(controls) != 2 || controls[0].Label != "before" {
		t.Fatalf("controls after rollback = %#v, want restored label", controls)
	}
}

func checkpointPoint(index int, role model.CheckpointControlRole, lng, lat, radius float64) model.CheckpointControl {
	return model.CheckpointControl{
		SequenceIndex: index,
		Role:          role,
		Primitive:     model.CheckpointControlPoint,
		GeometryShape: model.CheckpointControlGeometryPoint,
		Geometry:      mustCheckpointJSON(map[string]any{"type": "Point", "coordinates": []float64{lng, lat}}),
		HitRadiusM:    &radius,
		Required:      true,
	}
}

func checkpointCircle(index int, role model.CheckpointControlRole, lng, lat, radius float64) model.CheckpointControl {
	return model.CheckpointControl{
		SequenceIndex: index,
		Role:          role,
		Primitive:     model.CheckpointControlArea,
		GeometryShape: model.CheckpointControlGeometryCircle,
		Geometry:      mustCheckpointJSON(map[string]any{"type": "Point", "coordinates": []float64{lng, lat}}),
		AreaRadiusM:   &radius,
		Required:      true,
	}
}

func checkpointPolygon(index int, role model.CheckpointControlRole, rings [][][]float64) model.CheckpointControl {
	return model.CheckpointControl{
		SequenceIndex: index,
		Role:          role,
		Primitive:     model.CheckpointControlArea,
		GeometryShape: model.CheckpointControlGeometryPolygon,
		Geometry:      mustCheckpointJSON(map[string]any{"type": "Polygon", "coordinates": rings}),
		Required:      true,
	}
}

func mustCheckpointJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
