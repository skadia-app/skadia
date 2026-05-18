package database

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestFormatCourseGPSV1InstantUsesCanonicalMicroseconds(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "exact second keeps six fractional digits",
			in:   time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC),
			want: "2026-05-14T09:30:00.000000Z",
		},
		{
			name: "microsecond value keeps six fractional digits",
			in:   time.Date(2026, 5, 14, 9, 30, 0, 123456789, time.FixedZone("offset", 2*60*60)),
			want: "2026-05-14T07:30:00.123457Z",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatCourseGPSV1Instant(tc.in); got != tc.want {
				t.Fatalf("FormatCourseGPSV1Instant() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildCourseGPSV1MatchIdentityIsStable(t *testing.T) {
	startedAt := time.Date(2026, 5, 14, 9, 30, 0, 123456000, time.UTC)
	got := BuildCourseGPSV1MatchIdentity("act-1", "seg-1", "ver-1", startedAt, true, 2)
	want := "platform_activity:act-1:segment:seg-1:version:ver-1:contract:course_gps_v1:1:direction:reverse:start:2026-05-14T09:30:00.123456Z:occurrence:2"
	if got != want {
		t.Fatalf("identity = %q, want %q", got, want)
	}
}

func TestPrepareCourseGPSV1EffortMetadataFreezesPartition(t *testing.T) {
	effort := &model.SegmentEffort{
		SegmentID:        "seg-1",
		SegmentVersionID: "ver-1",
		ActivityID:       "act-1",
		StartedAt:        time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC),
		IsReverse:        false,
		SportType:        "cycling",
		SubType:          "mountain",
	}
	ctx := SegmentMatchActivityContext{
		IsIndoor:   true,
		IsVirtual:  false,
		IsElectric: true,
	}

	PrepareCourseGPSV1EffortMetadata(effort, ctx)

	if effort.MatchingContractKey != CourseGPSV1Key || effort.MatchingContractVersion != CourseGPSV1Version {
		t.Fatalf("contract = %s/%d", effort.MatchingContractKey, effort.MatchingContractVersion)
	}
	if effort.MatchSourceType != "platform_activity" || effort.MatchSourceID != "act-1" {
		t.Fatalf("source = %s/%s", effort.MatchSourceType, effort.MatchSourceID)
	}
	if effort.LeaderboardSportType != "cycling" || effort.LeaderboardSubType != "mountain" {
		t.Fatalf("leaderboard sport tuple = %s/%s", effort.LeaderboardSportType, effort.LeaderboardSubType)
	}
	if effort.LeaderboardAssistClass != "electric" {
		t.Fatalf("assist class = %s, want electric", effort.LeaderboardAssistClass)
	}
	if effort.LeaderboardIndoorPolicyState != "indoor" {
		t.Fatalf("indoor state = %s, want indoor", effort.LeaderboardIndoorPolicyState)
	}
	if effort.LeaderboardVirtualPolicyState != "real" {
		t.Fatalf("virtual state = %s, want real", effort.LeaderboardVirtualPolicyState)
	}
	if effort.LeaderboardDirection != "forward" {
		t.Fatalf("direction = %s, want forward", effort.LeaderboardDirection)
	}
	if effort.LeaderboardVersionPolicyKey != "legacy_segment_wide" {
		t.Fatalf("version policy = %s, want legacy_segment_wide", effort.LeaderboardVersionPolicyKey)
	}
}
