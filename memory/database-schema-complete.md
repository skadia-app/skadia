---
name: Skadia Complete Database Schema & Fitness Data Storage
description: Comprehensive map of all tables, columns, computed metrics, enrichment, and sports-specific data
type: reference
---

# Skadia Database Schema & Fitness Data Storage

## 1. Core Activity Tables

### 1.1 `activities` (Main table)
**Purpose:** Core activity record with summary metrics.

**Key Columns:**
- `id` (UUID), `user_id` (UUID), `file_hash` (TEXT)
- `title`, `title_auto`, `title_is_manual`, `description` (TEXT)
- `sport_type` (TEXT), `sub_type`, `fit_sport_id`, `fit_sub_sport_id`
- `is_ebike` (BOOL, GENERATED from sport_type)
- `started_at`, `ended_at` (TIMESTAMPTZ)
- `elapsed_time_s`, `moving_time_s`, `distance_m` (INTEGER/DOUBLE)
- `elevation_gain_m`, `elevation_loss_m`, `max_elevation_m`, `min_elevation_m`
- `avg_speed_ms`, `max_speed_ms`
- `avg_heart_rate`, `max_heart_rate` (HR in bpm)
- `avg_power_w`, `max_power_w`, `normalized_power_w` (Watts)
- `avg_cadence`, `avg_temperature_c`
- **Computed Metrics (000247):**
  - `tss` (activity-level TSS from user-provided threshold)
  - `trimp` (Edwards TRIMP score)
  - `computed_tss`, `computed_np_w`, `computed_ngp_mps`, `computed_if`, `tss_method`
- `start_location`, `end_location` (GEOMETRY Point, EPSG 4326)
- `original_file_*` (bucket, key, format, filename)
- `sidecar_meta_key`, `gear_id` (UUID)
- `tags` (TEXT[], JSON array)
- `enrichment_status`, `visibility`

**Indexes:**
- `(user_id, started_at DESC)`, `GIN(tags)`, `(file_hash)`

---

### 1.2 `activity_streams` (TimescaleDB Hypertable)
**Purpose:** Time-series data at 1 Hz or variable frequency.

**Key Columns:**
- `time` (TIMESTAMPTZ, hypertable key), `activity_id` (UUID)
- `lat`, `lng`, `altitude_m` (DOUBLE)
- `speed_ms`, `distance_m` (DOUBLE)
- `heart_rate`, `power_w`, `cadence`, `temperature_c` (SMALLINT)

**Running Dynamics (000062):**
- `ground_contact_time_ms`, `vertical_oscillation_mm`, `stance_time_percent`, `left_right_balance`

**Extended Metrics (000207):**
- Grade: `avg_grade_pct`, `avg_pos_grade_pct`, `avg_neg_grade_pct`, `max_pos_grade_pct`, `max_neg_grade_pct`
- Vertical Speed: `avg_pos_vertical_speed_ms`, `avg_neg_vertical_speed_ms`, `max_pos_vertical_speed_ms`, `max_neg_vertical_speed_ms`, `avg_vam_ms`
- Altitude: `avg_altitude_m`, `total_fractional_ascent_m`, `total_fractional_descent_m`
- Pedal Analytics (15): `avg_left/right_torque_effectiveness_pct`, `avg_left/right_pedal_smoothness_pct`, `avg_combined_pedal_smoothness_pct`, `avg_left/right_pco_mm`, left/right power phase angles (8 fields)
- Power/Cadence by Position (4 arrays): `avg_power_position_w`, `max_power_position_w`, `avg_cadence_position`, `max_cadence_position`
- Standing/Sitting: `time_standing_s`, `stand_count`

**Swimming Metrics (000207):**
- `stroke_count`, `zone_count` (INTEGER arrays)

**Diving Data (000207):**
- `avg_depth_m`, `max_depth_m` (REAL)

**Compression:** TimescaleDB compression enabled (7-day policy), segmented by `activity_id`.

**Indexes:**
- `(activity_id, time DESC)`

---

### 1.3 `activity_tracks`
**Purpose:** Simplified, full, and privacy-aware track geometries.

**Columns:**
- `activity_id` (PK), `track_simplified`, `track_full`, `track_public` (GEOMETRY LineString)
- `bbox` (GEOMETRY Polygon), `point_count` (INTEGER)

**Indexes:**
- GIST on `track_full`

---

### 1.4 `activity_sensors`
**Purpose:** Device/sensor metadata per activity.

**Columns:**
- `id` (UUID), `activity_id`, `device_index` (SMALLINT)
- `serial_number` (BIGINT), `source_type` (e.g., 'ANT+', 'BLE'), `antplus_device_type`
- `all_device_types` (TEXT array)
- `manufacturer`, `manufacturer_id`, `garmin_product`, `garmin_product_id`
- `product_name`, `software_version`, `hardware_version`
- `battery_status`, `battery_voltage` (NUMERIC 5,3)

**Indexes:**
- `(activity_id)`, `(serial_number) WHERE serial_number IS NOT NULL`

---

## 2. Lap & Split Data

### 2.1 `activity_laps` (FIT Laps)
**Purpose:** Laps from FIT file (auto-lap, manual laps, swim groups).

**Key Columns:**
- `id` (UUID), `activity_id` (UNIQUE with message_index), `message_index`
- `start_time`, `end_time` (TIMESTAMPTZ)
- `lap_trigger`, `sport`, `sub_sport`, `intensity`
- `total_elapsed_time_s`, `total_timer_time_s`, `total_moving_time_s`
- `total_distance_m`, `total_ascent_m`, `total_descent_m`
- `total_calories`, `total_work_j` (INTEGER)
- `avg_speed_ms`, `max_speed_ms`, `avg_heart_rate`, `max_heart_rate`, `min_heart_rate`
- `avg_cadence`, `max_cadence`, `avg_power_w`, `max_power_w`, `normalized_power_w`
- `avg_temperature_c` (INTEGER)
- `start_lat`, `start_lng`, `end_lat`, `end_lng`

**Swimming-Specific (000031):**
- `swim_stroke`, `num_lengths`, `num_active_lengths`

**Extended FIT Lap Fields (000208) [~83 new columns]:**
- Cycles/Strokes: `total_cycles`, `total_fat_calories`, `avg_stroke_distance_m`, `first_length_index`, `avg_fractional_cadence`, `max_fractional_cadence`, `total_fractional_cycles`, `stroke_count[]`, `zone_count[]`
- Running Dynamics: `avg_vertical_oscillation_mm`, `avg_stance_time_ms`, `avg_stance_time_pct`, `avg_stance_time_balance_pct`, `avg_step_length_mm`, `avg_vertical_ratio_pct`
- Grade, Vertical Speed, Temperature (extended), Pedal Analytics (15), Power/Cadence by Position, Standing/Sitting, L/R Balance, E-Bike, Grit/Flow, Jumps, Scores, Hemoglobin arrays, Diving, Respiration, Altitude, Misc (GPS accuracy, etc.)

---

### 2.2 `activity_splits`
**Purpose:** Split data (climbing, skiing, intervals).

**Columns:**
- `id`, `activity_id` (UNIQUE with message_index), `message_index`
- `split_type`, `start_time`, `end_time`
- `total_elapsed_time_s`, `total_timer_time_s`, `total_moving_time_s`, `total_distance_m`
- `total_ascent_m`, `total_descent_m`, `total_calories`
- `avg_speed_ms`, `max_speed_ms`, `avg_vert_speed_ms`
- `start_lat`, `start_lng`, `end_lat`, `end_lng`, `start_elevation_m`

---

### 2.3 `activity_split_summaries`
**Purpose:** Aggregates per split type.

**Columns:**
- `id`, `activity_id`, `message_index`, `split_type` (UNIQUE constraint)
- `num_splits`, `total_timer_time_s`, `total_moving_time_s`, `total_distance_m`
- `total_ascent_m`, `total_descent_m`, `total_calories`
- `avg_speed_ms`, `max_speed_ms`, `avg_vert_speed_ms`
- `avg_heart_rate`, `max_heart_rate`

---

### 2.4 `activity_lengths` (Swimming Pool Traversals)
**Purpose:** Individual pool lengths from swim activity.

**Columns:**
- `id`, `activity_id`, `message_index`
- `start_time`, `end_time`
- `total_elapsed_time_s`, `total_timer_time_s`, `total_strokes`, `total_calories`
- `avg_speed_ms`, `avg_swimming_cadence` (strokes/min)
- `swim_stroke`, `length_type`

---

### 2.5 `activity_detected_laps` (Auto-Detected Laps)
**Purpose:** Algorithmically detected laps (not FIT-level).

**Columns:**
- `id`, `activity_id`, `lap_number` (UNIQUE with activity_id)
- `start_time`, `end_time`, `elapsed_time_s`
- `distance_m`, `avg_speed_ms`, `avg_heart_rate`, `avg_power_w`, `avg_cadence`
- `total_ascent_m`, `total_descent_m`
- `start_lat`, `start_lng`

---

### 2.6 `activity_climb_pro_events` (ClimbPro FIT Messages)
**Purpose:** ClimbPro event tracking during activity.

**Columns:**
- `id`, `activity_id`, `timestamp`
- `event_type`, `climb_number`, `climb_category`
- `current_dist_m`, `lat`, `lng`

---

## 3. Segment & Leaderboard Data

### 3.1 `segments`
**Purpose:** User-defined or auto-created route segments.

**Columns:**
- `id`, `created_by` (user UUID), `name`, `description`
- `sport_group` (bike/foot/swim/ski/paddle/generic), `allowed_sport_types`, `allowed_sub_types` (TEXT arrays)
- `segment_type` (standard, etc.), `segment_type_meta` (JSONB)
- `source_activity_id` (UUID), `osm_lift_id` (BIGINT)
- `distance_m`, `elevation_gain_m`
- `effort_count`, `athlete_count` (summary stats, updated from segment_efforts)
- `visibility` (public/private), `route_hash`

**Indexes:**
- `(segment_type)`, `(sport_group)`, `(created_by)`

---

### 3.2 `segment_versions`
**Purpose:** Versioned geometry (for route changes over time).

**Columns:**
- `id`, `segment_id` (FK), `version_number`
- `track` (GEOMETRY LineString), `start_point`, `end_point`
- `valid_from` (DATE), `change_note`, `created_by`, `created_at`

**Indexes:**
- GIST on `track`, `(segment_id, valid_from DESC)`

---

### 3.3 `segment_efforts` (TimescaleDB Hypertable)
**Purpose:** Leaderboard data per segment/version/activity.

**Columns:**
- `id`, `segment_id`, `segment_version_id`, `activity_id`, `user_id`
- `started_at` (TIMESTAMPTZ, hypertable key)
- `elapsed_time_s`, `is_reverse` (BOOL)
- `sport_type`, `sub_type`, `gear_id`
- `avg_power_w`, `avg_heart_rate`, `avg_speed_ms`
- `rank_overall`, `is_personal_best`, `is_segment_record`, `in_leaderboard`

**Indexes:**
- `(segment_id, sport_type, is_reverse, elapsed_time_s ASC) WHERE in_leaderboard`
- `(segment_id, sport_type, sub_type, is_reverse, elapsed_time_s ASC) WHERE in_leaderboard`
- `(segment_id, gear_id, elapsed_time_s ASC) WHERE in_leaderboard AND gear_id IS NOT NULL`

---

## 4. Training Metrics & Performance

### 4.1 `pmc_daily` (Performance Management Chart)
**Purpose:** Daily performance metrics.

**Columns:**
- `user_id`, `date` (DATE, PK)
- `tss` (daily Training Stress Score), `ctl` (Chronic Training Load)
- `atl` (Acute Training Load), `tsb` (Training Stress Balance)
- `acwr_ra` (ACWR Rolling Average, Acute/Chronic, 7:28 uncoupled, computed by `CalcACWR_RA`)

**Computed Values:**
- `acwr = atl / ctl` (when ctl > 0)

**Calculation Logic:**
- **CTL:** 42-day exponential moving average (decay = e^(-1/42))
- **ATL:** 7-day exponential moving average (decay = e^(-1/7))
- **TSB:** ctl - atl (readiness at start of day)
- **ACWR_RA:** (sum TSS days 0-6) / 7 ÷ (sum TSS days 7-34) / 28 (requires ≥7 training days in chronic window)

---

### 4.2 `training_zones` (FIT SDK v26+)
**Purpose:** Per-user HR and Power zones by sport/sub_sport.

**Columns:**
- `id`, `user_id`, `sport`, `sub_sport` (SMALLINT, UNIQUE constraint)
- `zone_type` (e.g., 'hr', 'power'), `max_heart_rate`, `threshold_heart_rate`
- `functional_threshold_power`, `hr_calc_type`, `pwr_calc_type` (SMALLINT)
- `zone_names`, `zone_highs` (TEXT[], REAL arrays)
- `threshold_speed_mps` (REAL, for running, 000247)
- `source_type` (manual/derived), `created_at`, `updated_at`

---

## 5. Enrichment Data

### 5.1 Enrichment Fields on `activities` (000011)
**Location Enrichment:**
- `start_city`, `end_city`, `start_state`, `start_country`, `start_country_code`

**Weather Enrichment:**
- `weather_temp_c`, `weather_condition`, `weather_humidity`, `weather_wind_speed_ms`, `weather_tags` (TEXT array)

**Activity Classification:**
- `is_round_trip` (BOOL), `intensity` (locker/moderat/intensiv), `time_of_day` (vor-sonnenaufgang/goldene-stunde/tagestour/nach-sonnenuntergang/nacht)

---

### 5.2 `activity_geo_tags` (000011)
**Purpose:** Structured geo-locations detected during activity.

**Columns:**
- `id`, `activity_id`, `category` (TEXT), `name` (TEXT)
- `osm_id`, `lat`, `lng`, `ele`, `dist_m`, `duration_min` (INTEGER)
- `meta` (JSONB, optional context)

---

### 5.3 `activity_climbs` (000011)
**Purpose:** Detected ascents during activity.

**Columns:**
- `id`, `activity_id`, `climb_number`
- `start_distance_m`, `end_distance_m`, `elevation_gain_m`, `length_m`
- `avg_gradient_pct`, `max_gradient_pct`, `category`

---

### 5.4 `activity_stops` (000011)
**Purpose:** Detected stops during activity.

**Columns:**
- `id`, `activity_id`, `started_at`, `ended_at`, `duration_s`
- `location` (GEOMETRY Point), `name`

---

### 5.5 `user_enrichment_settings` (000011)
**Purpose:** Per-user enrichment feature flags and thresholds.

**Columns:**
- `user_id` (PK)
- `enable_summit_detection`, `enable_weather_enrichment`, `enable_stop_detection`, `enable_climb_detection`, `enable_commute_detection`, `enable_auto_naming`, `enable_lap_detection` (BOOL)
- `climb_min_gradient_pct`, `climb_min_elevation_m`, `round_trip_threshold_m` (FLOAT)

---

### 5.6 `commute_clusters` (000011)
**Purpose:** Auto-detected commute start/end locations.

**Columns:**
- `id`, `user_id`, `cluster_label` (INTEGER)
- `center` (GEOMETRY Point), `radius_m`, `point_count`

---

### 5.7 `geo_name_gender_cache` (000011)
**Purpose:** Gender cache for German place names (auto-naming).

**Columns:**
- `name_normalized` (PK), `gender` (CHAR 1: m/w/n), `source`, `created_at`

---

## 6. Health Monitoring Data (FIT SDK Health Messages)

### 6.1 `health_monitoring_samples` (000241, TimescaleDB)
**Purpose:** Per-minute health monitoring samples (rest, sleep, stress, etc.).

**Columns:**
- `user_id`, `ts` (TIMESTAMPTZ, hypertable key)
- `heart_rate`, `activity_type`, `intensity` (SMALLINT)
- `temperature_c`, `temperature_min_c`, `temperature_max_c` (REAL)
- `stress` (SMALLINT, 0-100)
- `body_battery`, `body_battery_delta` (SMALLINT)
- `respiration_rate` (REAL)
- `sleep_level` (SMALLINT: 0=awake, 1=light, 2=deep, 3=rem)
- `spo2_pct`, `spo2_confidence` (SMALLINT, 000245)
- `wrist_temperature_c` (REAL)

**Hypertable:** Chunk interval = 1 day, compression enabled (7-day policy).

---

### 6.2 `health_monitoring_daily` (000242)
**Purpose:** Daily aggregate of health monitoring (Continuous Aggregate).

**Note:** Created by Go app startup code (`garmin.EnsureMonitoringDailyCA`) because TimescaleDB CAs cannot be created inside transactions.

---

### 6.3 `health_weight_entries`
**Purpose:** Body composition, weight history.

**Columns (extended in 000244):**
- Standard: date, weight_kg, percent_fat, percent_water, percent_muscle
- **Extended:**
  - `percent_hydration`, `visceral_fat_mass_g`
  - `basal_met`, `active_met` (metabolic rate)
  - `physique_rating`, `metabolic_age`, `visceral_fat_rating` (SMALLINT)

---

### 6.4 `health_sleep` (with score fields from 000243)
**Purpose:** Sleep sessions and quality scores.

**Score Columns:**
- `score_light`, `score_awake_time`, `score_awakenings`
- `score_combined_awake`, `score_restlessness`, `score_interruptions`, `score_recovery`

---

### 6.5 `health_spo2_snapshots` (000245)
**Purpose:** SpO2 measurements with confidence.

**Columns:**
- `confidence` (SMALLINT, added in 000245)

---

## 7. FIT SDK Full Support Fields (000207, 000208)

### Activity-Level Fields (000207)
**Movement/Cycles (5):**
- `total_cycles`, `total_work_j`, `total_fat_calories`, `metabolic_calories`, `total_moving_time_s` (REAL)

**Swimming (9):**
- `swim_stroke`, `pool_length_m` (REAL), `pool_length_unit`
- `num_lengths`, `num_active_lengths`
- `avg_stroke_count`, `avg_stroke_distance_m` (REAL)
- `stroke_count[]`, `zone_count[]` (INTEGER arrays)

**Cadence Extended (3):**
- `avg_fractional_cadence`, `max_fractional_cadence`, `total_fractional_cycles` (REAL)

**Running Dynamics (6):**
- `avg_vertical_oscillation_mm`, `avg_stance_time_ms`, `avg_stance_time_pct`, `avg_stance_time_balance_pct`, `avg_step_length_mm`, `avg_vertical_ratio_pct` (REAL)

**Grade (5):**
- `avg_grade_pct`, `avg_pos_grade_pct`, `avg_neg_grade_pct`, `max_pos_grade_pct`, `max_neg_grade_pct` (REAL)

**Vertical Speed (5):**
- `avg_pos_vertical_speed_ms`, `avg_neg_vertical_speed_ms`, `max_pos_vertical_speed_ms`, `max_neg_vertical_speed_ms`, `avg_vam_ms` (REAL)

**Altitude Extended (3):**
- `avg_altitude_m`, `total_fractional_ascent_m`, `total_fractional_descent_m` (REAL)

**Pedal Analytics (15):**
- Effectiveness & Smoothness (5): `avg_left/right_torque_effectiveness_pct`, `avg_left/right_pedal_smoothness_pct`, `avg_combined_pedal_smoothness_pct`
- PCO (2): `avg_left/right_pco_mm` (SMALLINT)
- Power Phase Angles (8 per side): left/right start, end, peak_start, peak_end degrees (REAL)

**Power/Cadence by Position (4 arrays):**
- `avg_power_position_w[]`, `max_power_position_w[]` (INTEGER arrays)
- `avg_cadence_position[]`, `max_cadence_position[]` (SMALLINT arrays)

**Standing/Sitting (2):**
- `time_standing_s` (REAL), `stand_count` (INTEGER)

**E-Bike (3):**
- `avg_lev_motor_power_w`, `max_lev_motor_power_w` (INTEGER)
- `lev_battery_consumption_pct` (REAL)

**MTB Grit/Flow (4):**
- `total_grit`, `total_flow`, `avg_grit`, `avg_flow` (REAL)

**Jumps (1):**
- `jump_count` (INTEGER)

**Ball Sports (4):**
- `max_ball_speed_ms`, `avg_ball_speed_ms` (REAL)
- `player_score`, `opponent_score` (INTEGER)
- `opponent_name` (TEXT)

**Hemoglobin (6 arrays):**
- `avg_total_hemoglobin_conc[]`, `min_total_hemoglobin_conc[]`, `max_total_hemoglobin_conc[]` (REAL arrays)
- `avg_saturated_hemoglobin_pct[]`, `min_saturated_hemoglobin_pct[]`, `max_saturated_hemoglobin_pct[]` (REAL arrays)

**Diving (9):**
- `avg_depth_m`, `max_depth_m` (REAL)
- `surface_interval_s`, `dive_number` (INTEGER)
- `start_cns_pct`, `end_cns_pct` (SMALLINT)
- `start_n2_pct`, `end_n2_pct` (INTEGER)
- `o2_toxicity_otu` (INTEGER)

**Respiration/HRV (3):**
- `max_respiration_rate`, `min_respiration_rate` (REAL)
- `sdrr_hrv_ms` (SMALLINT, SDNN in ms)

**Temperature Extended (2):**
- `min_core_temperature_c`, `max_core_temperature_c` (REAL)

**Misc (10):**
- `avg_lap_time_s` (REAL), `best_lap_index`, `num_laps`, `first_lap_index` (SMALLINT/INTEGER)
- `intensity_factor` (REAL), `left_right_balance` (REAL)
- `sport_profile_name`, `sport_index`
- `nec_lat`, `nec_lng`, `swc_lat`, `swc_lng` (NE/SW corners, DOUBLE)

---

## 8. Computed Metrics & TSS Calculation

### 8.1 TSS Computation Methods (pmc/tss.go, pmc/service.go)

**Power-Based TSS (Coggan):**
```
IF (normalizedPowerW available AND ftpW > 0):
  intensityFactor = NP / FTP
  TSS = (movingTimeH * NP * IF) / (FTP * 100)
ELSE:
  Fallback to HR-based TSS
```

**HR-Based TSS (Coggan hrTSS):**
- Second-by-second from HR stream if available
- Falls back to average HR estimation
- Formula: `sum(hrr * 0.64 * exp(1.92 * hrr)) / (3600 * normFactor) * 100`
- Where `hrr = (HR - RestingHR) / (LTHR - RestingHR)`
- 1 hour at exactly LTHR = 100 hrTSS (by design)

**Running TSS (rTSS, Normalized Graded Pace):**
- Calculates NGP using grade-adjusted speeds (Minetti coefficient)
- Then applies NP algorithm to NGP stream
- TSS = (movingTimeH * NGP * IF) / (threshold_speed * 100)

**Swimming TSS (sTSS):**
- `TSS = (movingTimeH * avgSpeed / CSS) ^ 2 * 100`
- CSS = Critical Swim Speed threshold

**Computed Metrics in DB:**
- `computed_tss`: TSS value (which method in `tss_method`)
- `computed_np_w`: Normalized Power (Watts)
- `computed_ngp_mps`: Normalized Graded Pace (m/s, running)
- `computed_if`: Intensity Factor (NP/FTP or similar)
- `tss_method`: 'power', 'hr', 'running', 'swimming', 'strength', 'generic'

---

### 8.2 Additional Metrics

**Normalized Power (NP, pmc/tss.go):**
- 30-second SMA → 4th power → mean → 4th root
- Repairs 1-5 sample dropouts (interpolates if surrounded by >50W)
- Minimum 30 samples required
- Coggan standard algorithm

**Normalized Graded Pace (NGP):**
- Adjusts speed for grade using Minetti et al. energy coefficient
- Grade range clamped to ±45%
- Applies NP algorithm to grade-adjusted speeds
- Fallback: uses Garmin GAP field (140) if available

**Edwards TRIMP:**
- Zone-based: HR ratio [0.50-0.60]=1, [0.60-0.70]=2, [0.70-0.80]=3, [0.80-0.90]=4, [0.90-1.01]=5
- Returns TRIMP-minutes (1 min in Zone 5 = 5.0 TRIMP-minutes)
- Time-series calculation from HR stream

**ACWR (Acute:Chronic Workload Ratio):**
- **Acute:** sum of TSS days 0-6 (current week) / 7
- **Chronic:** sum of TSS days 7-34 (4 weeks before current week) / 28
- **ACWR = Acute / Chronic** (×100 for percentage)
- Uncoupled 7:28 windows
- Requires minimum 7 training days in chronic window to compute
- Stored as `acwr_ra` in `pmc_daily`

---

## 9. Time-In-Zone Data

### 9.1 Integration with Zones
- **FIT SDK:** Sport-specific training zones (HR/Power) stored in `training_zones`
- **Stream-Level:** Zone occupancy arrays in `stroke_count[]`, `zone_count[]` (laps, streams)
- **Time-In-Zone Laps (000216):** Per-lap zone time tracking

---

## 10. Data Enrichment Pipeline

### 10.1 Enrichment Pipeline Stages (activityutil/)

**Stage 1 (stage1.go):**
- GPS filtering, distance/speed derivation (FillMissingDistanceAndSpeed)
- Speed smoothing (SmoothSpeed with median filter)
- Distance reset detection (FixDistanceResets)
- Elevation fill/interpolation from DEM or stream
- Moving time calculation (CalcMovingTime with 5-min gap detection)

**Geo Enrichment (geo_enrich.go):**
- OSM reverse geocoding (start/end locations)
- Summit/peak detection
- Weather enrichment (historical weather API)
- Geo-tags (POIs, landmarks)

**Stream Enrichment (stream_enrich.go):**
- Running dynamics normalization
- Grade calculation (elevation/distance)
- Vertical speed computation
- Power zone assignment (if zones defined)

**Enrichment Settings (user_enrichment_settings):**
- Per-user control over which enrichments run
- Climb min gradient, min elevation, round-trip threshold

---

## 11. Special Data Types

### 11.1 Diving
- **Activity Fields:** `avg_depth_m`, `max_depth_m`, `surface_interval_s`, `dive_number`, `start_cns_pct`, `end_cns_pct`, `start_n2_pct`, `end_n2_pct`, `o2_toxicity_otu`
- **Lap Fields:** `avg_depth_m`, `max_depth_m`
- **No dedicated dive table** — all stored in activities/laps

### 11.2 Swimming
- **Activity Fields:** `swim_stroke`, `pool_length_m`, `num_lengths`, `num_active_lengths`, `avg_stroke_count`, `avg_stroke_distance_m`
- **Lap Fields:** `swim_stroke`, `num_lengths`, `num_active_lengths`
- **Pool Traversals:** `activity_lengths` table with per-length stroke/speed data

### 11.3 Running Dynamics
- **Stream-Level:** `ground_contact_time_ms`, `vertical_oscillation_mm`, `stance_time_percent`, `left_right_balance`
- **Activity-Level:** Aggregates (avg/max) of running dynamics metrics
- **Lap-Level:** Aggregates per lap

### 11.4 Power Metrics
- **Stream:** `power_w` (SMALLINT), pedal analytics if available
- **Activity:** `avg_power_w`, `max_power_w`, `normalized_power_w`, `computed_np_w`
- **Lap:** `avg_power_w`, `max_power_w`, `normalized_power_w`
- **Pedal Analytics:** Torque effectiveness, pedal smoothness, power phase angles (per leg)

### 11.5 Hemoglobin/Blood Metrics
- **Activity/Lap:** 6 REAL[] arrays (avg/min/max total hemoglobin, avg/min/max saturated %)
- Used for sports watches with built-in sensors (Garmin, etc.)

---

## 12. Key Foreign Keys & Relationships

```
activities.user_id → users.id (CASCADE DELETE)
activity_streams.activity_id → activities.id (CASCADE DELETE)
segment_efforts.activity_id → activities.id (CASCADE DELETE)
segment_efforts.user_id → users.id
segment_efforts.segment_id → segments.id (CASCADE DELETE)
segments.created_by → users.id
activities.gear_id → gear.id
training_zones.user_id → users.id (CASCADE DELETE)
pmc_daily.user_id → users.id (CASCADE DELETE)
health_monitoring_samples.user_id → users.id (CASCADE DELETE)
```

---

## 13. Indexes & Performance

**Hot Path Indexes:**
- `activities (user_id, started_at DESC)` — list activities per user
- `activity_streams (activity_id, time DESC)` — hypertable key + activity FK
- `segment_efforts (segment_id, sport_type, is_reverse, elapsed_time_s ASC)` — leaderboard queries
- `pmc_daily (user_id, date)` — PMC range queries
- `training_zones (user_id, sport, sub_sport, zone_type)` — UNIQUE constraint, fast lookups

**Compression:**
- `activity_streams`: TimescaleDB compression (7-day policy, segment by user_id)
- `health_monitoring_samples`: TimescaleDB compression (7-day policy, segment by user_id)

---

## 14. Next Free Migration

Migration **000250** is the next available slot.

---

## 15. Summary: What Gets Stored

| Data Type | Location | Computed | Notes |
|-----------|----------|----------|-------|
| GPS Track | activity_streams, activity_tracks | ✓ (simplified) | Geometry types, compression enabled |
| HR Stream | activity_streams | ✗ | Raw 1Hz or variable |
| Power Stream | activity_streams | ✓ (NP, IF) | SMALLINT in stream; computed metrics in activities |
| Running Dynamics | activity_streams, activities (agg), laps (agg) | ✓ | Ground contact, oscillation, stance time |
| Cadence | activity_streams, activities (avg/max), laps | ✗ | Raw + aggregates |
| Temperature | activity_streams, activities | ✗ | Stream + body/core variants |
| Elevation | activity_streams, activities | ✓ (from DEM) | Ascent/descent calculated |
| Swimming | activity_laps, activity_lengths, activities | ✗ | Pool lengths, strokes per length, CSS if available |
| Diving | activities, laps | ✗ | Depth, CNS%, N2%, surface intervals |
| HRV (SDRR) | activities | ✗ | SDNN in milliseconds if available |
| PMC | pmc_daily (TimescaleDB-agnostic) | ✓ | CTL/ATL/TSB/ACWR computed daily |
| Training Zones | training_zones, segment_efforts (via HR/Power) | ✗ | Per-sport, supports HR and Power |
| Gear | activities.gear_id, segment_efforts.gear_id | ✗ | Links to gear table |
| Segments | segments, segment_versions, segment_efforts | ✓ | Geometry versioned, leaderboard hypertable |
| Enrichment | activity_* tables + user_enrichment_settings | ✓ | Summits, weather, geo-tags, climbs, stops |

---

**Created:** 2026-03-29
**Scope:** Complete database schema mapping for Skadia v0.5+
**Status:** Reference document for DB exploration
