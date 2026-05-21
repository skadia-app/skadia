package database

import (
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

const (
	CourseGPSV1Key              = "course_gps_v1"
	CourseGPSV1Version          = 1
	CheckpointSequenceV1Key     = "checkpoint_sequence_v1"
	CheckpointSequenceV1Version = 1
	CheckpointSequencePolicyKey = "checkpoint_sequence_current_version"
	courseGPSV1InstantLayout    = "2006-01-02T15:04:05.000000Z"
)

type SegmentMatchActivityContext struct {
	IsIndoor   bool
	IsVirtual  bool
	IsElectric bool
}

func BuildCourseGPSV1OccurrenceBase(activityID, segmentID, versionID string, startedAt time.Time, isReverse bool) string {
	direction := "forward"
	if isReverse {
		direction = "reverse"
	}
	return fmt.Sprintf(
		"platform_activity:%s:segment:%s:version:%s:contract:%s:%d:direction:%s:start:%s",
		activityID,
		segmentID,
		versionID,
		CourseGPSV1Key,
		CourseGPSV1Version,
		direction,
		FormatCourseGPSV1Instant(startedAt),
	)
}

func FormatCourseGPSV1Instant(t time.Time) string {
	return t.UTC().Round(time.Microsecond).Format(courseGPSV1InstantLayout)
}

func BuildCourseGPSV1MatchIdentity(activityID, segmentID, versionID string, startedAt time.Time, isReverse bool, occurrenceIndex int) string {
	return fmt.Sprintf("%s:occurrence:%d",
		BuildCourseGPSV1OccurrenceBase(activityID, segmentID, versionID, startedAt, isReverse),
		occurrenceIndex,
	)
}

func PrepareCourseGPSV1EffortMetadata(e *model.SegmentEffort, ctx SegmentMatchActivityContext) {
	e.MatchingContractKey = CourseGPSV1Key
	e.MatchingContractVersion = CourseGPSV1Version
	e.MatchSourceType = "platform_activity"
	e.MatchSourceID = e.ActivityID
	e.MatchIdentityKey = BuildCourseGPSV1MatchIdentity(e.ActivityID, e.SegmentID, e.SegmentVersionID, e.StartedAt, e.IsReverse, e.OccurrenceIndex)

	e.LeaderboardSportType = nonEmpty(e.SportType, "generic")
	e.LeaderboardSubType = nonEmpty(e.SubType, "generic")
	e.LeaderboardEquipmentClass = "unspecified"
	e.LeaderboardAssistClass = "none"
	if ctx.IsElectric {
		e.LeaderboardAssistClass = "electric"
	}
	e.LeaderboardIndoorPolicyState = "outdoor"
	if ctx.IsIndoor {
		e.LeaderboardIndoorPolicyState = "indoor"
	}
	e.LeaderboardVirtualPolicyState = "real"
	if ctx.IsVirtual {
		e.LeaderboardVirtualPolicyState = "virtual"
	}
	e.LeaderboardElectricPolicyState = "none"
	if ctx.IsElectric {
		e.LeaderboardElectricPolicyState = "electric"
	}
	e.LeaderboardDirection = "forward"
	if e.IsReverse {
		e.LeaderboardDirection = "reverse"
	}
	e.LeaderboardVersionPolicyKey = "legacy_segment_wide"
	e.LeaderboardContractKey = CourseGPSV1Key
	e.LeaderboardContractVersion = CourseGPSV1Version
}

func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}
