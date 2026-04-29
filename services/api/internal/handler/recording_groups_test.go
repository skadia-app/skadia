package handler

// recording_groups_test.go — Integration tests for AddGroupMember and
// group-mode FusionMerge.
//
// Requires TEST_DATABASE_URL to be set; the whole suite is skipped otherwise.
//
// Tests:
//   - TestAddGroupMember_HappyPath           — POST with valid group + activity → 200 OK, member added
//   - TestAddGroupMember_OwnershipDenied      — group belongs to different user → 403
//   - TestAddGroupMember_Idempotent           — adding same activity twice → 200 OK both times
//   - TestFusionMerge_GroupMode_Preserves2MemberGroup — group-mode fusion must not dissolve the group (R3-2)
//   - TestAddGroupMember_RejectsFusionSource  — fusion-source activities must be rejected 400 (R3-3)

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
)

// doAddGroupMember sends POST /recording-groups/{groupID}/members with the given
// activityID in the body. Returns the response code.
func doAddGroupMember(t *testing.T, h *Handler, userID, groupID, activityID string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"activity_id": activityID})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recording-groups/"+groupID+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", groupID)
	req = req.WithContext(context.WithValue(
		context.WithValue(req.Context(), middleware.UserIDKey, userID),
		chi.RouteCtxKey, rctx,
	))

	rr := httptest.NewRecorder()
	h.AddGroupMember(rr, req)
	return rr.Code
}

func doSetGroupSportType(t *testing.T, h *Handler, userID, groupID string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/recording-groups/"+groupID+"/sport-type", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", groupID)
	req = req.WithContext(context.WithValue(
		context.WithValue(req.Context(), middleware.UserIDKey, userID),
		chi.RouteCtxKey, rctx,
	))

	rr := httptest.NewRecorder()
	h.SetRecordingGroupSportType(rr, req)
	return rr
}

func TestSetRecordingGroupSportTypePreservesAbsentModifiers(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	act1 := insertMinimalActivity(t, db, userID)
	act2 := insertMinimalActivity(t, db, userID)
	group, err := db.CreateRecordingGroup(ctx, userID, []string{act1, act2}, act1, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE recording_groups
		SET sport_type = 'cycling', sub_type = 'mountain', is_electric = true, is_indoor = true, is_virtual = true
		WHERE id = $1
	`, group.ID); err != nil {
		t.Fatalf("seed recording group sport tuple: %v", err)
	}

	h := &Handler{DB: db}
	rr := doSetGroupSportType(t, h, userID, group.ID, []byte(`{"sport_type":"cycling"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body = %s", rr.Code, rr.Body.String())
	}

	reloaded, err := db.GetRecordingGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetRecordingGroup: %v", err)
	}
	if !reloaded.IsElectric || !reloaded.IsIndoor || !reloaded.IsVirtual {
		t.Fatalf("modifiers = electric:%v indoor:%v virtual:%v, want all preserved true",
			reloaded.IsElectric, reloaded.IsIndoor, reloaded.IsVirtual)
	}
}

// TestAddGroupMember_HappyPath: create a group with two activities, add a third
// unrelated activity → should return 200 and list three members.
func TestAddGroupMember_HappyPath(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	act1 := insertMinimalActivity(t, db, userID)
	act2 := insertMinimalActivity(t, db, userID)
	act3 := insertMinimalActivity(t, db, userID)

	group, err := db.CreateRecordingGroup(ctx, userID, []string{act1, act2}, act1, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}

	h := &Handler{DB: db}
	code := doAddGroupMember(t, h, userID, group.ID, act3)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}

	// Verify three members.
	members, err := db.GetRecordingGroupMembers(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetRecordingGroupMembers: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}
	found := false
	for _, m := range members {
		if m.ActivityID == act3 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected act3 to be a member after AddGroupMember")
	}
}

// TestAddGroupMember_OwnershipDenied: group belongs to userA, but request is made
// by userB → should return 403.
func TestAddGroupMember_OwnershipDenied(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userA := insertTestUser(t, pool)
	userB := insertTestUser(t, pool)

	act1 := insertMinimalActivity(t, db, userA)
	act2 := insertMinimalActivity(t, db, userA)
	actB := insertMinimalActivity(t, db, userB)

	group, err := db.CreateRecordingGroup(ctx, userA, []string{act1, act2}, act1, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}

	h := &Handler{DB: db}
	// userB tries to add to userA's group
	code := doAddGroupMember(t, h, userB, group.ID, actB)
	if code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", code)
	}
}

// TestAddGroupMember_Idempotent: adding the same activity to the same group twice
// should return 200 both times (idempotent).
func TestAddGroupMember_Idempotent(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	act1 := insertMinimalActivity(t, db, userID)
	act2 := insertMinimalActivity(t, db, userID)
	act3 := insertMinimalActivity(t, db, userID)

	group, err := db.CreateRecordingGroup(ctx, userID, []string{act1, act2}, act1, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}

	h := &Handler{DB: db}

	// First call — adds act3.
	code := doAddGroupMember(t, h, userID, group.ID, act3)
	if code != http.StatusOK {
		t.Fatalf("first add: expected 200, got %d", code)
	}

	// Second call — act3 is already in the group, should be idempotent → 200.
	code = doAddGroupMember(t, h, userID, group.ID, act3)
	if code != http.StatusOK {
		t.Fatalf("idempotent add: expected 200, got %d", code)
	}

	// Still three members (not four).
	members, err := db.GetRecordingGroupMembers(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetRecordingGroupMembers: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members after idempotent add, got %d", len(members))
	}
}

// ─── R3-2: group-mode fusion must not dissolve the recording_group ──────────

// TestFusionMerge_GroupMode_Preserves2MemberGroup verifies the R3-2 fix:
//   - A 2-member group {A, B} exists.
//   - FusionMerge is called with group_id = G, sources = [A, B].
//   - After the merge: 201, the merged activity C exists, and group G still
//     exists with A, B (is_fused=false) and C (is_fused=true) as members.
//   - primary_activity_id of G must be C.
//
// Before R3-2: trigger 000313 removed A and B from recording_group_members
// during the TX, causing the deferred sync to dissolve the group at commit.
// The post-commit AddRecordingGroupMemberFused then hit an FK violation
// (non-fatal), leaving the merge without group context.
func TestFusionMerge_GroupMode_Preserves2MemberGroup(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)

	actA := insertMinimalActivity(t, db, userID)
	actB := insertMinimalActivity(t, db, userID)

	// Create the 2-member recording group.
	group, err := db.CreateRecordingGroup(ctx, userID, []string{actA, actB}, actA, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}
	groupID := group.ID

	// Perform the group-mode FusionMerge: sources=[A,B], group_id=G.
	reqBody := fmt.Sprintf(
		`{"sources":[%q,%q],"field_map":{"gps":%q},"tags_merge":false,"group_id":%q}`,
		actA, actB, actA, groupID,
	)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v1/fusion-merge",
		strings.NewReader(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq = httpReq.WithContext(
		context.WithValue(httpReq.Context(), middleware.UserIDKey, userID),
	)
	rr := httptest.NewRecorder()
	h := &Handler{DB: db}
	h.FusionMerge(rr, httpReq)

	if rr.Code != http.StatusCreated {
		t.Fatalf("FusionMerge: expected 201, got %d — body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode FusionMerge response: %v", err)
	}
	mergedID := resp.ID
	if mergedID == "" {
		t.Fatal("FusionMerge response did not include an id")
	}
	t.Logf("FusionMerge: merged activity id = %s", mergedID)

	// ── Assert: recording_group G still exists ────────────────────────────────
	reloadedGroup, err := db.GetRecordingGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("GetRecordingGroup after fusion: %v", err)
	}
	if reloadedGroup == nil {
		t.Fatal("R3-2: recording_group was dissolved by group-mode fusion — expected it to survive")
	}

	// ── Assert: primary_activity_id = C (merged) ─────────────────────────────
	if reloadedGroup.PrimaryActivityID == nil || *reloadedGroup.PrimaryActivityID != mergedID {
		primaryStr := "<nil>"
		if reloadedGroup.PrimaryActivityID != nil {
			primaryStr = *reloadedGroup.PrimaryActivityID
		}
		t.Errorf("R3-2: primary_activity_id = %s, want %s", primaryStr, mergedID)
	}

	// ── Assert: group members contain A, B (sources) and C (merged) ──────────
	members, err := db.GetRecordingGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatalf("GetRecordingGroupMembers after fusion: %v", err)
	}

	byID := make(map[string]database.RecordingGroupMemberDetail, len(members))
	for _, m := range members {
		byID[m.ActivityID] = m
	}

	if _, ok := byID[actA]; !ok {
		t.Errorf("R3-2: actA (%s) missing from group members after fusion", actA)
	}
	if _, ok := byID[actB]; !ok {
		t.Errorf("R3-2: actB (%s) missing from group members after fusion", actB)
	}
	if m, ok := byID[mergedID]; !ok {
		t.Errorf("R3-2: merged activity C (%s) missing from group members", mergedID)
	} else if !m.IsFused {
		t.Errorf("R3-2: merged activity C (%s) must have is_fused=true", mergedID)
	}

	t.Logf("R3-2: group %s preserved, primary=%s, members=%d — OK", groupID, mergedID, len(members))
}

// ─── R3-3: AddGroupMember must reject fusion-source activities ────────────────

// TestAddGroupMember_RejectsFusionSource verifies the R3-3 fix:
//   - Activities A and B are merged → C (A and B become fusion-sources).
//   - POST /recording-groups/G/members {activity_id: A} must return 400.
//   - Group G must have no member A after the rejected call.
func TestAddGroupMember_RejectsFusionSource(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)

	actA := insertMinimalActivity(t, db, userID)
	actB := insertMinimalActivity(t, db, userID)

	// Fuse A + B → C (ad-hoc mode, no group_id).
	reqBody := fmt.Sprintf(
		`{"sources":[%q,%q],"field_map":{"gps":%q},"tags_merge":false}`,
		actA, actB, actA,
	)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v1/fusion-merge",
		strings.NewReader(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq = httpReq.WithContext(
		context.WithValue(httpReq.Context(), middleware.UserIDKey, userID),
	)
	rrFuse := httptest.NewRecorder()
	h := &Handler{DB: db}
	h.FusionMerge(rrFuse, httpReq)
	if rrFuse.Code != http.StatusCreated {
		t.Fatalf("FusionMerge setup: expected 201, got %d — body: %s", rrFuse.Code, rrFuse.Body.String())
	}

	// Create a fresh group with two unrelated activities to hold the group.
	actX := insertMinimalActivity(t, db, userID)
	actY := insertMinimalActivity(t, db, userID)
	group, err := db.CreateRecordingGroup(ctx, userID, []string{actX, actY}, actX, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}

	// Attempt to add actA (a fusion-source) to group G → must fail with 400.
	code := doAddGroupMember(t, h, userID, group.ID, actA)
	if code != http.StatusBadRequest {
		t.Errorf("R3-3: expected 400 for fusion-source, got %d", code)
	}

	// Verify actA is not a member of G.
	members, err := db.GetRecordingGroupMembers(ctx, group.ID)
	if err != nil {
		t.Fatalf("GetRecordingGroupMembers: %v", err)
	}
	for _, m := range members {
		if m.ActivityID == actA {
			t.Errorf("R3-3: actA was added to the group despite being a fusion-source")
		}
	}

	t.Logf("R3-3: AddGroupMember correctly rejected fusion-source actA with 400 — OK")
}

// ─── 409 Conflict: activity already in a different group ─────────────────────

// ─── R4-6: 3-member-group coverage ──────────────────────────────────────────

// TestFusionMerge_GroupMode_Preserves3MemberGroup_MergeSubset verifies that
// group-mode fusion with a 3-member group {A, B, C} and a subset merge {A, B}
// correctly extends the group rather than dissolving it.
//
//   - Group G = {A (primary), B, C}
//   - FusionMerge sources=[A, B], group_id=G → creates D
//   - After commit: G = {A, B, C, D(is_fused=true)}, primary=D
//   - A, B, C must have is_group_secondary=TRUE (set by trg_group_primary_sync)
func TestFusionMerge_GroupMode_Preserves3MemberGroup_MergeSubset(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)

	actA := insertMinimalActivity(t, db, userID)
	actB := insertMinimalActivity(t, db, userID)
	actC := insertMinimalActivity(t, db, userID)

	group, err := db.CreateRecordingGroup(ctx, userID, []string{actA, actB, actC}, actA, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}
	groupID := group.ID

	// Fuse subset {A, B} with group_id = G.
	reqBody := fmt.Sprintf(
		`{"sources":[%q,%q],"field_map":{"gps":%q},"tags_merge":false,"group_id":%q}`,
		actA, actB, actA, groupID,
	)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v1/fusion-merge",
		strings.NewReader(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq = httpReq.WithContext(
		context.WithValue(httpReq.Context(), middleware.UserIDKey, userID),
	)
	rr := httptest.NewRecorder()
	h := &Handler{DB: db}
	h.FusionMerge(rr, httpReq)

	if rr.Code != http.StatusCreated {
		t.Fatalf("FusionMerge: expected 201, got %d — body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode FusionMerge response: %v", err)
	}
	mergedID := resp.ID

	// Group must still exist.
	reloadedGroup, err := db.GetRecordingGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("GetRecordingGroup after fusion: %v", err)
	}
	if reloadedGroup == nil {
		t.Fatal("R4-6: 3-member group was dissolved by group-mode fusion — expected it to survive")
	}

	// primary_activity_id must be the merged activity D.
	if reloadedGroup.PrimaryActivityID == nil || *reloadedGroup.PrimaryActivityID != mergedID {
		primaryStr := "<nil>"
		if reloadedGroup.PrimaryActivityID != nil {
			primaryStr = *reloadedGroup.PrimaryActivityID
		}
		t.Errorf("R4-6: primary_activity_id = %s, want %s (merged)", primaryStr, mergedID)
	}

	members, err := db.GetRecordingGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatalf("GetRecordingGroupMembers after fusion: %v", err)
	}

	byID := make(map[string]database.RecordingGroupMemberDetail, len(members))
	for _, m := range members {
		byID[m.ActivityID] = m
	}

	// Group must contain A, B, C (sources + non-merged member) and D (merged).
	for _, id := range []string{actA, actB, actC} {
		if _, ok := byID[id]; !ok {
			t.Errorf("R4-6: activity %s missing from group after subset merge", id)
		}
	}
	if m, ok := byID[mergedID]; !ok {
		t.Errorf("R4-6: merged activity D (%s) missing from group members", mergedID)
	} else if !m.IsFused {
		t.Errorf("R4-6: merged activity D (%s) must have is_fused=true", mergedID)
	}

	// A, B, C must be is_group_secondary=TRUE (trg_group_primary_sync fired).
	for _, id := range []string{actA, actB, actC} {
		var isSecondary bool
		if err := pool.QueryRow(ctx,
			`SELECT is_group_secondary FROM activities WHERE id = $1`, id,
		).Scan(&isSecondary); err != nil {
			t.Fatalf("R4-6: scan is_group_secondary for %s: %v", id, err)
		}
		if !isSecondary {
			t.Errorf("R4-6: activity %s should have is_group_secondary=TRUE after group-mode merge", id)
		}
	}

	t.Logf("R4-6: 3-member group %s preserved, primary=%s, members=%d — OK", groupID, mergedID, len(members))
}

// TestFusionMerge_AdHocMode_RemovesSourcesFromGroup verifies that ad-hoc fusion
// (no group_id) removes merged sources from their recording group via trigger
// 000313, and that a group dissolves when too few members remain.
//
//   - Group G = {A (primary), B, C}
//   - FusionMerge sources=[A, B], NO group_id → creates D (ad-hoc)
//   - Trigger 000313 removes A and B from recording_group_members
//   - Only C remains → group dissolves (trg_group_members_sync, <2 members)
func TestFusionMerge_AdHocMode_RemovesSourcesFromGroup(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)

	actA := insertMinimalActivity(t, db, userID)
	actB := insertMinimalActivity(t, db, userID)
	actC := insertMinimalActivity(t, db, userID)

	group, err := db.CreateRecordingGroup(ctx, userID, []string{actA, actB, actC}, actA, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup: %v", err)
	}
	groupID := group.ID

	// Fuse {A, B} ad-hoc (no group_id).
	reqBody := fmt.Sprintf(
		`{"sources":[%q,%q],"field_map":{"gps":%q},"tags_merge":false}`,
		actA, actB, actA,
	)
	httpReq := httptest.NewRequest(http.MethodPost, "/api/v1/fusion-merge",
		strings.NewReader(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq = httpReq.WithContext(
		context.WithValue(httpReq.Context(), middleware.UserIDKey, userID),
	)
	rr := httptest.NewRecorder()
	h := &Handler{DB: db}
	h.FusionMerge(rr, httpReq)

	if rr.Code != http.StatusCreated {
		t.Fatalf("FusionMerge ad-hoc: expected 201, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// Group must be dissolved: trigger 000313 removes A+B (2 sources),
	// leaving only C (1 member) → trg_group_members_sync dissolves the group.
	dissolvedGroup, err := db.GetRecordingGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("GetRecordingGroup after ad-hoc fusion: %v", err)
	}
	if dissolvedGroup != nil {
		t.Errorf("R4-6: ad-hoc merge of {A,B} from 3-member group should dissolve the group (only C remains), but it still exists")
	}

	// A and B must no longer be in recording_group_members (trigger removed them).
	for _, id := range []string{actA, actB} {
		var cnt int
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM recording_group_members WHERE activity_id = $1`, id,
		).Scan(&cnt); err != nil {
			t.Fatalf("R4-6: count recording_group_members for %s: %v", id, err)
		}
		if cnt != 0 {
			t.Errorf("R4-6: fusion-source %s should have been removed from recording_group_members by trigger 000313, but still has %d row(s)", id, cnt)
		}
	}

	t.Logf("R4-6: ad-hoc fusion of {A,B} correctly dissolved 3-member group G after sources removed — OK")
}

// ─── 409 Conflict: activity already in a different group ─────────────────────

// TestAddGroupMember_Conflict409 verifies that adding an activity that already
// belongs to a *different* recording group returns 409 Conflict.
//
// Setup:
//   - actX is created and added to group G2.
//   - POST /recording-groups/G1/members {activity_id: actX} → 409.
//   - Response body must contain "already" (substring, wording-change safe).
func TestAddGroupMember_Conflict409(t *testing.T) {
	pool := fusionTestPool(t)
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)

	// G1: two members, actX will NOT be added here in the setup.
	act1 := insertMinimalActivity(t, db, userID)
	act2 := insertMinimalActivity(t, db, userID)
	g1, err := db.CreateRecordingGroup(ctx, userID, []string{act1, act2}, act1, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup G1: %v", err)
	}

	// G2: actX is a member here.
	actX := insertMinimalActivity(t, db, userID)
	act3 := insertMinimalActivity(t, db, userID)
	_, err = db.CreateRecordingGroup(ctx, userID, []string{actX, act3}, actX, "")
	if err != nil {
		t.Fatalf("CreateRecordingGroup G2: %v", err)
	}

	// Attempt to add actX (already in G2) to G1 — must return 409 Conflict.
	h := &Handler{DB: db}

	body, _ := json.Marshal(map[string]string{"activity_id": actX})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recording-groups/"+g1.ID+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", g1.ID)
	req = req.WithContext(context.WithValue(
		context.WithValue(req.Context(), middleware.UserIDKey, userID),
		chi.RouteCtxKey, rctx,
	))

	rr := httptest.NewRecorder()
	h.AddGroupMember(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d — body: %s", rr.Code, rr.Body.String())
	}

	// Response body must mention "already" (wording-stable substring).
	if !strings.Contains(rr.Body.String(), "already") {
		t.Errorf("expected 'already' in conflict response body, got: %s", rr.Body.String())
	}

	// actX must remain only in G2, not added to G1.
	g1Members, err := db.GetRecordingGroupMembers(ctx, g1.ID)
	if err != nil {
		t.Fatalf("GetRecordingGroupMembers G1: %v", err)
	}
	for _, m := range g1Members {
		if m.ActivityID == actX {
			t.Errorf("actX was added to G1 despite 409 response")
		}
	}
}
