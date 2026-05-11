# Lift Detector Test Inventory

This subtree contains the lift detector heuristic and its privacy-safe test
fixtures. Code comments and commit messages for changes in this subtree stay in
English. Do not add real coordinates, real activity timestamps, real device or
region identifiers, private source filenames, production IDs, or local training
paths to code, tests, docs, or commit messages.

For every detector behavior change:

1. Add a failing property test or canonical-fixture test first.
2. Keep `DetectHeuristic` behavior changes separate from fixture/tool changes.
3. Run the lift package tests and the private aggregate eval before committing.
4. If a canonical fixture tolerance fails intentionally, re-promote or update
   that fixture in the same commit and explain the behavioral reason.
5. Review fixture diffs manually. A green promotion gate is not a substitute for
   checking whether the detector behavior change is intended.

`lift_detect_eval` is an aggregate smoke check over the private local corpus. It
is not a substitute for TDD and must not be referenced with hardcoded local
paths in committed files.

## Gate Inventory

| Gate | Anchor | Branch | Inputs | Class | Test Mapping |
|---|---|---|---|---|---|
| Initial window sample/time gate | `window.go:10` | reject | sample count, elapsed seconds, recording gap | property-testable | `TestStraightLiftHigh`, `TestCanonicalFixtureSmoke` |
| Effective speed sensor floor | `window.go:51` | admit/fallback | `SpeedMs`, GPS delta, sample gap | property-testable | `TestEffectiveSpeedFloor`, `TestSpeedZeroFallback` |
| Smart-recording gap speed guard | `window.go:70` | reject/fallback | sample gap, GPS delta | property-testable | `TestParseGPXLiftStreamsDeriveSpeedAndPreserveGapHazard`, `TestSmartRecordingGapRejectsInitialWindowAndSpeedFallback` |
| Public indexed detector parity | `detect.go:44` | API contract | ride output, start/end sample indices | property-testable | `TestDetectHeuristicWithIndicesMatchesPublicDetector` |
| Initial classify sinuosity ceiling | `detect.go:663` | reject | window sinuosity | property-testable | `TestNegativeSteepHikingPath`, `TestNegativePassRoad` |
| Initial classify speed band and CV | `detect.go:673` | admit/reject | mean speed, speed CV | property-testable | `TestStraightLiftHigh`, `TestSynthCableTrackAdmitsCableLockStep` |
| Initial classify medium fallback | `detect.go:692` | admit | altitude rate, duration, missing speed | property-testable | `TestClassifyMediumFallbackWithoutUsableSpeed` |
| Initial classify bidirectional fallback | `detect.go:699` | admit | cumulative altitude movement, low net altitude, sinuosity | property-testable | `TestClassifyAndValidateBidirectionalFallback` |
| Sinuosity tail-peel retry | `detect.go:97` | admit retry | final-window sinuosity, retry window | property-testable | `TestSinuosityTailPeelAcceptsLeadingSubWindow` |
| Boundary speed-drop optional gate | `detect.go:153` and `detect.go:2501` | reject | mean speed before/after boundary | property-testable | `TestBoundarySpeedDropRequiresFlankingStop` |
| Backward extension relaxed CV | `detect.go:177` | admit | extended stats, confidence class, CV cap | property-testable | `TestBackwardExtensionRelaxedCVAdmitsBoardingAcceleration` |
| Post-extension ascent floor | `detect.go:223` | reject | total ascent after extension | property-testable | `TestNegativeSteepHikingPath`, `TestCanonicalFixtureSmoke` |
| Mid-lift split merge | `detect.go:278` | admit/reject | gap duration, direction, merged validation | property-testable | `TestMergeMidLiftSplitSubclasses` |
| Stats distance/speed/CV core | `detect.go:426` | metric | Haversine distance, altitude deltas, chunk speeds, trimmed CV | property-testable | `TestStraightLiftHigh`, `TestSpeedZeroFallback`, `TestCanonicalFixtureSmoke` |
| Speed trim for CV | `detect.go:633` | metric | trim seconds, effective speeds | property-testable | `TestSynthCableTrackAdmitsCableLockStep`, `TestSpeedCVTrimIgnoresBoundaryAcceleration` |
| Forward extension stop overflow | `detect.go:720` | close | stop duration, effective speed | property-testable | `TestExtendCandidateStopsOnStopOverflow` |
| Forward extension bidirectional sinuosity loop | `detect.go:989` | close/reject | rolling window sinuosity | property-testable | `TestExtendCandidateBidirStopsOnCurvedTail` |
| Trailing disembark discontinuity trim | `detect.go:1113` | trim | end-window median speed, altitude rate | property-testable | `TestTrimTrailingDisembarkDiscontinuityFindsAltRateStep` |
| Trailing low-gradient trim | `detect.go:1287` | trim | trailing gradient, speed, CV | property-testable | `TestTrimTrailingLowGradientTrimsWalkOffButKeepsCableContinuation` |
| Trail approach stop anchor | `detect.go:1448` | anchor admit | pre-window descent, sustained stop, boundary stats | property-testable | `TestTrailApproachBeforeBoardingStopStartsAtStop` |
| Road-climb boundary guard | `detect.go:1530` | reject | sport config, road-like stats, boundary stop/speed | property-testable | `TestContinuousRoadClimbWithoutBoundaryCueRejected` |
| Road-climb exit guard | `detect.go:1572` | reject/admit | post-end sustained stop and max effective speed | property-testable | `TestContinuousRoadClimbWithoutBoundaryCueRejected`, `TestRoadClimbExitBoundaryRequiresStopOrFastDeparture` |
| Leading low-gradient trim | `detect.go:1684` | trim | leading gradient, speed, sinuosity | property-testable | `TestTrimLeadingLowGradientTrimsBoardingShuffleButKeepsCableCruise` |
| Trailing high-sinuosity trim | `detect.go:1811` | trim | tail-window sinuosity | property-testable | `TestTrimTrailingHighSinuosityTrimsCurvedWalkOff` |
| Backward extension flat-section bypass | `detect.go:1879` | admit/reject | altitude advance, stall duration, mean speed, local sinuosity | property-testable | `TestExtendCandidateBackwardUsesCablePlateauBypass` |
| Backward stall mean speed | `detect.go:2099` | metric | effective speed in stall window | property-testable | `TestBackwardStallMeanSpeedDistinguishesCablePlateauFromQueue` |
| Trailing low-speed sample trim | `detect.go:2171` | trim | low effective speed at end | property-testable | `TestTrimTrailingLowSpeedSamplesDropsStationaryTail` |
| Trailing altitude flat-section trim | `detect.go:2211` | trim | altitude rate, minimum speed in window | property-testable | `TestTrimTrailingAltPlateauTrimsWalkOffButKeepsCableCruise` |
| Trailing chunk trim | `detect.go:2275` | trim | chunked speeds, median speed | property-testable | `TestTrimTrailingChunksDropsSlowTail` |
| Validate duration/CV/ascent floors | `detect.go:2311` | reject | duration, CV, ascent, vertical rate, gradient | property-testable | `TestStraightLiftHigh`, `TestNegativePassRoad`, `TestCanonicalFixtureSmoke` |
| Hike-band reject and cable-lock-step bypass | `detect.go:2366` | reject/admit | mean speed, altitude rate, sinuosity, CV | property-testable | `TestSynthCableTrackAdmitsCableLockStep`, `TestValidateHighRejectsHikeBandWithoutCableLockStep` |
| Slow-band tightening | `detect.go:2400` | reject | mean speed, gradient | property-testable | `TestNegativeSteepHikingPath`, `TestValidateHighRejectsSlowBandLowGradient` |
| Ascending monotonicity check | `detect.go:2411` | reject | ascent/descent ratio | property-testable | `TestValidateHighRejectsAscendingMonotonicityRegression` |
| Bidirectional validation | `detect.go:2425` | reject/admit | duration, ascent/descent, cumulative altitude, speed, CV, sinuosity | property-testable | `TestClassifyAndValidateBidirectionalFallback` |
