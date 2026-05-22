package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

type CheckpointActivitySample struct {
	Index     int
	Time      time.Time
	Lat       float64
	Lng       float64
	DistanceM *float64
	AscentM   *float64
}

type CheckpointAttempt struct {
	StartedAt       time.Time
	FinishedAt      time.Time
	ElapsedTimeS    int
	OccurrenceIndex int
	OccurrenceHash  string
	DistanceM       *float64
	ElevationGainM  *float64
	ControlHits     []model.CheckpointControlHit
}

type CollectedSegmentEfforts struct {
	Efforts             []*model.SegmentEffort
	CheckpointEfforts   []model.CheckpointMatchedEffort
	MatchedSegmentIDs   []string
	CandidateSegmentIDs []string
}

func MatchCheckpointSequenceV1(samples []CheckpointActivitySample, controls []model.CheckpointControl) ([]CheckpointAttempt, error) {
	if len(samples) < 2 {
		return nil, nil
	}
	if err := ValidateCheckpointControls(controls, true); err != nil {
		return nil, err
	}

	effective := make([]checkpointEffectiveGeometry, len(controls))
	for i, control := range controls {
		geom, err := validateCheckpointControlGeometry(control)
		if err != nil {
			return nil, err
		}
		effective[i] = geom
	}

	var attempts []CheckpointAttempt
	for startIdx := range samples {
		_, startHit := checkpointSampleHitsControl(samples[startIdx], effective[0])
		if !startHit {
			continue
		}
		if startIdx > 0 {
			_, previousStartHit := checkpointSampleHitsControl(samples[startIdx-1], effective[0])
			if previousStartHit {
				continue
			}
		}

		hits := make([]model.CheckpointControlHit, 0, len(controls))
		startSample := samples[startIdx]
		startDistance, _ := checkpointSampleHitsControl(startSample, effective[0])
		var nearest *float64
		if startDistance >= 0 {
			nearest = &startDistance
		}
		hits = append(hits, model.CheckpointControlHit{
			ControlID:           controls[0].ID,
			SequenceIndex:       controls[0].SequenceIndex,
			EnteredAt:           startSample.Time,
			ActivityStreamIndex: startSample.Index,
			NearestDistanceM:    nearest,
			EvidenceHash:        checkpointControlEvidenceHash(controls[0].SequenceIndex, startSample.Index, startSample.Time),
		})

		lastIdx := startIdx
		searchFrom := startIdx + 1
		previousHitAt := startSample.Time
		for controlIdx := 1; controlIdx < len(controls); controlIdx++ {
			control := controls[controlIdx]
			foundIdx := -1
			var nearest *float64
			for sampleIdx := searchFrom; sampleIdx < len(samples); sampleIdx++ {
				if !samples[sampleIdx].Time.After(previousHitAt) {
					continue
				}
				distance, hit := checkpointSampleHitsControl(samples[sampleIdx], effective[controlIdx])
				if !hit {
					continue
				}
				foundIdx = sampleIdx
				if distance >= 0 {
					nearest = &distance
				}
				break
			}
			if foundIdx < 0 {
				break
			}
			sample := samples[foundIdx]
			hits = append(hits, model.CheckpointControlHit{
				ControlID:           control.ID,
				SequenceIndex:       control.SequenceIndex,
				EnteredAt:           sample.Time,
				ActivityStreamIndex: sample.Index,
				NearestDistanceM:    nearest,
				EvidenceHash:        checkpointControlEvidenceHash(control.SequenceIndex, sample.Index, sample.Time),
			})
			lastIdx = foundIdx
			previousHitAt = sample.Time
			searchFrom = foundIdx + 1
		}
		if len(hits) != len(controls) {
			continue
		}
		finishSample := samples[lastIdx]
		elapsed := int(finishSample.Time.Sub(startSample.Time).Seconds())
		if elapsed <= 0 {
			continue
		}
		attempt := CheckpointAttempt{
			StartedAt:    startSample.Time,
			FinishedAt:   finishSample.Time,
			ElapsedTimeS: elapsed,
			ControlHits:  hits,
		}
		if startSample.DistanceM != nil && finishSample.DistanceM != nil {
			d := *finishSample.DistanceM - *startSample.DistanceM
			if d >= 0 {
				attempt.DistanceM = &d
			}
		}
		if startSample.AscentM != nil && finishSample.AscentM != nil {
			gain := *finishSample.AscentM - *startSample.AscentM
			if gain >= 0 {
				attempt.ElevationGainM = &gain
			}
		}
		attempts = append(attempts, attempt)
	}
	sort.SliceStable(attempts, func(i, j int) bool {
		if attempts[i].StartedAt.Equal(attempts[j].StartedAt) {
			return attempts[i].ControlHits[0].ActivityStreamIndex < attempts[j].ControlHits[0].ActivityStreamIndex
		}
		return attempts[i].StartedAt.Before(attempts[j].StartedAt)
	})
	for i := range attempts {
		attempts[i].OccurrenceIndex = i
		attempts[i].OccurrenceHash = checkpointAttemptHash(attempts[i])
	}
	return attempts, nil
}

func checkpointSampleHitsControl(sample CheckpointActivitySample, geom checkpointEffectiveGeometry) (float64, bool) {
	switch geom.kind {
	case "circle":
		distance := checkpointDistanceM(sample.Lat, sample.Lng, geom.lat, geom.lng)
		return distance, distance <= geom.radiusM
	case "polygon", "multipolygon":
		return -1, checkpointPointInEffectiveArea(sample.Lng, sample.Lat, geom)
	default:
		return -1, false
	}
}

func checkpointAttemptHash(attempt CheckpointAttempt) string {
	parts := make([]string, 0, len(attempt.ControlHits)+1)
	for _, hit := range attempt.ControlHits {
		parts = append(parts, fmt.Sprintf("%d:%d:%s", hit.SequenceIndex, hit.ActivityStreamIndex, hit.EnteredAt.UTC().Format(time.RFC3339Nano)))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func checkpointControlEvidenceHash(sequenceIndex, streamIndex int, at time.Time) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", sequenceIndex, streamIndex, at.UTC().Format(time.RFC3339Nano))))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func BuildCheckpointSequenceV1MatchIdentity(activityID, segmentID, versionID string, startedAt time.Time, occurrenceIndex int, occurrenceHash string) string {
	return fmt.Sprintf(
		"platform_activity:%s:segment:%s:version:%s:contract:%s:%d:start:%s:occurrence:%d:evidence:%s",
		activityID,
		segmentID,
		versionID,
		CheckpointSequenceV1Key,
		CheckpointSequenceV1Version,
		FormatCourseGPSV1Instant(startedAt),
		occurrenceIndex,
		occurrenceHash,
	)
}

func PrepareCheckpointSequenceV1EffortMetadata(e *model.SegmentEffort, ctx SegmentMatchActivityContext) {
	e.MatchingContractKey = CheckpointSequenceV1Key
	e.MatchingContractVersion = CheckpointSequenceV1Version
	e.MatchSourceType = "platform_activity"
	e.MatchSourceID = e.ActivityID
	occurrenceHash := ""
	if e.OccurrenceHash != nil {
		occurrenceHash = *e.OccurrenceHash
	}
	e.MatchIdentityKey = BuildCheckpointSequenceV1MatchIdentity(e.ActivityID, e.SegmentID, e.SegmentVersionID, e.StartedAt, e.OccurrenceIndex, occurrenceHash)

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
	e.LeaderboardVersionPolicyKey = CheckpointSequencePolicyKey
	e.LeaderboardContractKey = CheckpointSequenceV1Key
	e.LeaderboardContractVersion = CheckpointSequenceV1Version
}

func InsertCheckpointMatchedEffortsTx(ctx context.Context, tx pgx.Tx, matches []model.CheckpointMatchedEffort) error {
	for _, match := range matches {
		if match.Effort == nil {
			return fmt.Errorf("%w: checkpoint match has no effort", ErrSegmentValidation)
		}
		effort := match.Effort
		var effortID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO segment_efforts (
				segment_id, segment_version_id, activity_id, user_id,
				started_at, elapsed_time_s, is_reverse,
				sport_type, sub_type, gear_id,
				avg_speed_ms, avg_heart_rate, avg_power_w,
				distance_m, elevation_gain_m,
				matching_contract_key, matching_contract_version,
				match_source_type, match_source_id, match_identity_key,
				occurrence_index, occurrence_hash,
				leaderboard_equivalence_group_id,
				leaderboard_sport_type, leaderboard_sub_type,
				leaderboard_equipment_class, leaderboard_assist_class,
				leaderboard_indoor_policy_state, leaderboard_virtual_policy_state,
				leaderboard_electric_policy_state, leaderboard_direction,
				leaderboard_version_policy_key,
				leaderboard_contract_key, leaderboard_contract_version,
				activity_trait_bucket_key, eligibility_policy_key,
				eligibility_policy_version, eligibility_source_hash,
				in_leaderboard
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7,
				$8, $9, $10,
				$11, $12, $13,
				$14, $15,
				$16, $17,
				$18, $19, $20,
				$21, $22,
				$23,
				$24, $25,
				$26, $27,
				$28, $29,
				$30, $31,
				$32,
				$33, $34,
				$35, $36,
				$37, $38,
				$39
			)
			ON CONFLICT (match_identity_key, started_at) WHERE superseded_at IS NULL
			DO UPDATE SET
				elapsed_time_s = EXCLUDED.elapsed_time_s,
				occurrence_hash = EXCLUDED.occurrence_hash,
				distance_m = EXCLUDED.distance_m,
				elevation_gain_m = EXCLUDED.elevation_gain_m,
				in_leaderboard = EXCLUDED.in_leaderboard
			RETURNING id::text
		`,
			effort.SegmentID, effort.SegmentVersionID, effort.ActivityID, effort.UserID,
			effort.StartedAt, effort.ElapsedTimeS, effort.IsReverse,
			effort.SportType, effort.SubType, effort.GearID,
			effort.AvgSpeedMs, effort.AvgHeartRate, effort.AvgPowerW,
			effort.DistanceM, effort.ElevationGainM,
			nullableString(effort.MatchingContractKey), nullableInt(effort.MatchingContractVersion),
			nullableString(effort.MatchSourceType), nullableString(effort.MatchSourceID), nullableString(effort.MatchIdentityKey),
			effort.OccurrenceIndex, effort.OccurrenceHash,
			nullableString(effort.LeaderboardEquivalenceGroupID),
			nullableString(effort.LeaderboardSportType), nullableString(effort.LeaderboardSubType),
			nullableString(effort.LeaderboardEquipmentClass), nullableString(effort.LeaderboardAssistClass),
			nullableString(effort.LeaderboardIndoorPolicyState), nullableString(effort.LeaderboardVirtualPolicyState),
			nullableString(effort.LeaderboardElectricPolicyState), nullableString(effort.LeaderboardDirection),
			nullableString(effort.LeaderboardVersionPolicyKey),
			nullableString(effort.LeaderboardContractKey), nullableInt(effort.LeaderboardContractVersion),
			nullableString(effort.ActivityTraitBucketKey), segmentEffortEligibilityPolicyKey(effort),
			segmentEffortEligibilityPolicyVersion(effort), nullableString(effort.EligibilitySourceHash),
			segmentEffortInLeaderboard(effort),
		).Scan(&effortID); err != nil {
			return fmt.Errorf("insert checkpoint segment effort: %w", err)
		}
		for _, hit := range match.ControlHits {
			if _, err := tx.Exec(ctx, `
				INSERT INTO checkpoint_course_effort_controls (
					segment_effort_id, segment_effort_started_at, segment_version_id,
					control_id, sequence_index, entered_at, exited_at,
					activity_stream_index, nearest_distance_m, evidence_hash
				) VALUES (
					$1, $2, $3,
					$4, $5, $6, $7,
					$8, $9, $10
				)
				ON CONFLICT (segment_effort_id, segment_effort_started_at, sequence_index)
				DO UPDATE SET
					control_id = EXCLUDED.control_id,
					segment_version_id = EXCLUDED.segment_version_id,
					entered_at = EXCLUDED.entered_at,
					exited_at = EXCLUDED.exited_at,
					activity_stream_index = EXCLUDED.activity_stream_index,
					nearest_distance_m = EXCLUDED.nearest_distance_m,
					evidence_hash = EXCLUDED.evidence_hash
			`, effortID, effort.StartedAt, effort.SegmentVersionID,
				hit.ControlID, hit.SequenceIndex, hit.EnteredAt, hit.ExitedAt,
				hit.ActivityStreamIndex, hit.NearestDistanceM, hit.EvidenceHash); err != nil {
				return fmt.Errorf("insert checkpoint control evidence: %w", err)
			}
		}
	}
	return nil
}
