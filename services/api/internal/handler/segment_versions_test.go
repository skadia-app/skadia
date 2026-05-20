package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUpdateSegmentVersionValidityRequiresBothBoundKeys_DB(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, userID, TestSegmentOpts{})
	versionID := segmentCurrentVersionID(t, pool, segmentID)

	r := chi.NewRouter()
	r.Patch("/api/v1/segments/{id}/versions/{version_id}/validity", h.UpdateSegmentVersionValidity)
	body := `{"operation_id":"missing-until","valid_from_at":null,"recalculate":true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/segments/"+segmentID+"/versions/"+versionID+"/validity", bytes.NewBufferString(body))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateSegmentVersionValidityRejectsDateOnlyInput_DB(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, userID, TestSegmentOpts{})
	versionID := segmentCurrentVersionID(t, pool, segmentID)

	r := chi.NewRouter()
	r.Patch("/api/v1/segments/{id}/versions/{version_id}/validity", h.UpdateSegmentVersionValidity)
	body := `{"operation_id":"date-only","valid_from_at":"2026-05-20","valid_until_at":null,"recalculate":true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/segments/"+segmentID+"/versions/"+versionID+"/validity", bytes.NewBufferString(body))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateSegmentVersionValidityCanScheduleAndListRevision_DB(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	userID := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, userID, TestSegmentOpts{})
	versionID := segmentCurrentVersionID(t, pool, segmentID)

	r := chi.NewRouter()
	r.Patch("/api/v1/segments/{id}/versions/{version_id}/validity", h.UpdateSegmentVersionValidity)
	r.Get("/api/v1/segments/{id}/versions/revisions", h.ListSegmentVersionRevisions)
	r.Get("/api/v1/segments/{id}/versions/revisions/{revision_id}", h.GetSegmentVersionRevision)
	body := `{"operation_id":"validity-handler-ok","valid_from_at":null,"valid_until_at":null,"recalculate":true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/segments/"+segmentID+"/versions/"+versionID+"/validity", bytes.NewBufferString(body))
	req = req.WithContext(withUserAndAdmin(req.Context(), userID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var revision struct {
		ID                 string  `json:"id"`
		Operation          string  `json:"operation"`
		RecalculationRunID *string `json:"recalculation_run_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &revision); err != nil {
		t.Fatalf("decode revision: %v", err)
	}
	if revision.ID == "" || revision.Operation != "validity_update" || revision.RecalculationRunID == nil {
		t.Fatalf("revision response = %#v, want validity revision with recalculation run", revision)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/segments/"+segmentID+"/versions/revisions", nil)
	listReq = listReq.WithContext(withUserAndAdmin(listReq.Context(), userID, false))
	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body: %s", listRec.Code, listRec.Body.String())
	}
	var listBody struct {
		Revisions []struct {
			ID           string `json:"id"`
			HasSnapshots bool   `json:"has_snapshots"`
		} `json:"revisions"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode revision list: %v", err)
	}
	if len(listBody.Revisions) != 1 || listBody.Revisions[0].ID != revision.ID || !listBody.Revisions[0].HasSnapshots {
		t.Fatalf("revision list = %#v, want summary for created revision", listBody)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/segments/"+segmentID+"/versions/revisions/"+revision.ID, nil)
	detailReq = detailReq.WithContext(withUserAndAdmin(detailReq.Context(), userID, false))
	detailRec := httptest.NewRecorder()
	r.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200; body: %s", detailRec.Code, detailRec.Body.String())
	}
	if !bytes.Contains(detailRec.Body.Bytes(), []byte("before_snapshot")) || !bytes.Contains(detailRec.Body.Bytes(), []byte("after_snapshot")) {
		t.Fatalf("detail response missing snapshots: %s", detailRec.Body.String())
	}
}

func TestUpdateSegmentVersionValidityRequiresOwnerOrAdmin_DB(t *testing.T) {
	pool := fusionTestPool(t)
	h := newTestHandler(t, pool)
	ownerID := insertTestUser(t, pool)
	otherID := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, ownerID, TestSegmentOpts{})
	versionID := segmentCurrentVersionID(t, pool, segmentID)

	r := chi.NewRouter()
	r.Patch("/api/v1/segments/{id}/versions/{version_id}/validity", h.UpdateSegmentVersionValidity)
	body := `{"operation_id":"not-owner","valid_from_at":null,"valid_until_at":null,"recalculate":true}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/segments/"+segmentID+"/versions/"+versionID+"/validity", bytes.NewBufferString(body))
	req = req.WithContext(withUserAndAdmin(req.Context(), otherID, false))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body: %s", rec.Code, rec.Body.String())
	}
}

func segmentCurrentVersionID(t *testing.T, pool *pgxpool.Pool, segmentID string) string {
	t.Helper()
	var versionID string
	if err := pool.QueryRow(context.Background(), `
		SELECT id
		FROM segment_versions
		WHERE segment_id = $1
		ORDER BY version_number DESC
		LIMIT 1
	`, segmentID).Scan(&versionID); err != nil {
		t.Fatalf("load segment version: %v", err)
	}
	return versionID
}
