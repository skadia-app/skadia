"""Tool definitions for LLM tool-use (function calling).

Contains 11 domain-specific tools with scenario-oriented descriptions,
enum parameters, and compact result formatting.
"""
import json
import logging
import os
import re
from datetime import date, timedelta
from uuid import UUID

# Compiled once at module level — used for BRouter profile param validation
_SAFE_PROFILE_PARAM = re.compile(r"^[a-zA-Z0-9_.\-]{1,50}$")

logger = logging.getLogger(__name__)

# Canonical sport types as stored in DB (after migration 000037 normalization).
# Sub-types (mountain_biking, trail_running, etc.) are stored as sub_type column,
# NOT as sport_type. Use run_sql for sub_type filtering.
_SPORT_TYPES = [
    "running", "cycling", "hiking", "walking", "swimming",
    "alpine_skiing", "cross_country_skiing", "strength_training",
    "mountaineering", "rowing", "paddle_boarding", "kayaking",
    "training", "fitness_equipment", "rock_climbing",
    "stand_up_paddleboarding", "multisport", "racket", "generic",
    "unknown", "other",
]

ALL_TOOL_DEFINITIONS = [
    # ── 1. query_activities ────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "query_activities",
            "description": (
                "Fetch individual activity records (title, date, distance, pace/speed, "
                "HR, elevation, city). Use when the user asks about SPECIFIC activities "
                "(e.g. 'mein letzter Lauf', 'schnellste Radtour', 'alle Bergtouren im März'). "
                "Do NOT use for totals/counts — use aggregate_stats instead. "
                "Do NOT use for health/HRV questions — use get_health_status. "
                "sport_type must be an exact DB value from the enum."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "sport_type": {
                        "type": "string",
                        "description": "Filter by sport type (exact DB value)",
                        "enum": _SPORT_TYPES,
                    },
                    "sub_type": {
                        "type": "string",
                        "description": (
                            "Filter by sub_type for more specific sport: "
                            "mountain (MTB), gravel_cycling, indoor_cycling, road, "
                            "unicycling, trail (Trail Running), open_water, bouldering"
                        ),
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 30; use 3650 for 'aller Zeiten')",
                        "default": 30,
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max results to return (default 10, max 50)",
                        "default": 10,
                    },
                    "sort_by": {
                        "type": "string",
                        "enum": ["date", "distance", "duration", "hr", "elevation"],
                        "description": "Sort order (default: date)",
                        "default": "date",
                    },
                    "min_distance_km": {
                        "type": "number",
                        "description": "Minimum distance in km",
                    },
                    "title_search": {
                        "type": "string",
                        "description": "Partial match on activity title (case-insensitive)",
                    },
                    "gear_id": {
                        "type": "string",
                        "description": "UUID of gear item — filters activities that used this gear",
                    },
                },
            },
        },
    },

    # ── 2. aggregate_stats ─────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "aggregate_stats",
            "description": (
                "Compute aggregate statistics over a ROLLING time window (last N days): "
                "total distance, duration, count, avg HR, elevation, weekly/monthly breakdowns. "
                "Use for 'Wie weit?', 'Wie oft?', 'Wie viel insgesamt?' with relative time. "
                "Do NOT use for calendar years ('letztes Jahr 2025', '2024') — use run_sql with "
                "EXTRACT(YEAR FROM started_at) instead. "
                "Do NOT use for sub-types (MTB, Gravel, Trail) — use run_sql with sub_type filter. "
                "Sub-types: cycling has sub_type mountain/gravel_cycling/indoor_cycling, "
                "running has trail/treadmill, swimming has open_water/lap_swimming. "
                "Do NOT use to list individual activities — use query_activities."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "metric": {
                        "type": "string",
                        "enum": [
                            "total_distance", "total_duration", "count",
                            "avg_heart_rate", "total_elevation", "avg_distance",
                        ],
                        "description": "Which aggregate metric to compute",
                    },
                    "sport_type": {
                        "type": "string",
                        "enum": _SPORT_TYPES,
                        "description": "Limit to a specific sport (omit for all sports)",
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 30)",
                        "default": 30,
                    },
                    "group_by": {
                        "type": "string",
                        "enum": ["none", "week", "month", "sport"],
                        "description": "Break down results by time period or sport (default: none)",
                        "default": "none",
                    },
                },
            },
        },
    },

    # ── 3. get_health_status ───────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_health_status",
            "description": (
                "Retrieve health and recovery data: HRV trend (RMSSD, SDRR, status), "
                "sleep metrics (duration, deep sleep, score), resting HR, readiness score, "
                "body weight, SpO2. "
                "Use for 'Wie ist mein HRV?', 'Wie habe ich geschlafen?', "
                "'Bin ich erholt?', 'Wie ist mein Ruhepuls?' questions. "
                "Do NOT use for training load/fatigue — use get_training_load. "
                "Do NOT use for activity statistics — use aggregate_stats."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "aspect": {
                        "type": "string",
                        "enum": ["hrv", "sleep", "readiness", "weight", "resting_hr", "all"],
                        "description": "Which health aspect to retrieve (default: all)",
                        "default": "all",
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 30)",
                        "default": 30,
                    },
                },
            },
        },
    },

    # ── 4. get_training_load ───────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_training_load",
            "description": (
                "Fetch Performance Management Chart (PMC) data: CTL (Fitness/42-day avg), "
                "ATL (Fatigue/7-day avg), TSB (Form = CTL - ATL), ACWR (ATL/CTL ratio). "
                "Use for 'Wie ist meine Form?', 'Trainiere ich zu viel?', "
                "'Wie hoch ist meine Fitness?', 'Bin ich überlastet?' questions. "
                "TSB interpretation: <-30 überlastet, -10..+5 optimal, >+15 Formverlust. "
                "ACWR: >1.5 = erhöhtes Verletzungsrisiko. "
                "Do NOT use for HRV/sleep — use get_health_status instead."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 30, use 42 for full CTL view)",
                        "default": 30,
                    },
                },
            },
        },
    },

    # ── 5. compare_periods ─────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "compare_periods",
            "description": (
                "Compare two date ranges on activity volume metrics: count, total distance, "
                "total duration, average HR, total elevation. "
                "Use for 'Vergleich März vs. Februar', 'Dieses Jahr vs. letztes Jahr' questions. "
                "All dates in YYYY-MM-DD format. "
                "Do NOT use for health/HRV comparisons — use get_health_status with different day windows."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "period1_start": {"type": "string", "description": "YYYY-MM-DD"},
                    "period1_end": {"type": "string", "description": "YYYY-MM-DD"},
                    "period2_start": {"type": "string", "description": "YYYY-MM-DD"},
                    "period2_end": {"type": "string", "description": "YYYY-MM-DD"},
                    "sport_type": {
                        "type": "string",
                        "enum": _SPORT_TYPES,
                        "description": "Limit comparison to one sport (optional)",
                    },
                },
                "required": ["period1_start", "period1_end", "period2_start", "period2_end"],
            },
        },
    },

    # ── 6. get_gear_stats ─────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_gear_stats",
            "description": (
                "Retrieve gear/equipment statistics: name, type, brand, model, "
                "total distance, total activities, status (active/stored/retired). "
                "Use for 'Wie viele km haben meine Schuhe?', 'Welche Ausrüstung nutze ich?', "
                "'Muss ich bald neue Schuhe kaufen?' questions. "
                "Do NOT use for activity statistics — use aggregate_stats for that. "
                "Without gear_id returns all active gear sorted by total distance."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "gear_id": {
                        "type": "string",
                        "description": "UUID of specific gear item (optional; omit for all active gear)",
                    },
                    "gear_type": {
                        "type": "string",
                        "enum": ["shoes", "bike", "wetsuit", "helmet", "watch", "other"],
                        "description": "Filter by gear type (optional)",
                    },
                },
            },
        },
    },

    # ── 7. query_geo_features ──────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "query_geo_features",
            "description": (
                "Query geographic features tagged in activities: peaks (Gipfel), passes (Pässe), "
                "lakes, rivers, forests, settlements, borders, and gastronomy stops (Hütten, Restaurants). "
                "Use for 'Wie viele Gipfel?', 'Welche Pässe?', 'Wo war ich essen?', "
                "'Auf welchen Hütten?' questions. "
                "Do NOT use for route/map queries — use search_routes for that. "
                "Do NOT use for segment PRs — use query_segments."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "feature_type": {
                        "type": "string",
                        "enum": [
                            "summit", "pass", "lake", "river", "forest",
                            "settlement", "border", "gastronomy",
                        ],
                        "description": "Type of geo feature to query",
                    },
                    "min_elevation": {
                        "type": "number",
                        "description": "Minimum elevation in meters (for peaks/passes)",
                    },
                    "days": {
                        "type": "integer",
                        "description": "Rolling lookback window in days from today (default 365). For specific year queries use date_from/date_to instead.",
                        "default": 365,
                    },
                    "date_from": {
                        "type": "string",
                        "description": "Start date filter ISO 8601 (YYYY-MM-DD). Overrides 'days' if set. Use for specific year/period queries.",
                    },
                    "date_to": {
                        "type": "string",
                        "description": "End date filter ISO 8601 (YYYY-MM-DD, inclusive). Use together with date_from for specific year/period queries.",
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max features returned (default 20). Does NOT affect total_count in the response — total_count always reflects ALL matching features.",
                        "default": 20,
                    },
                    "sport_type": {
                        "type": "string",
                        "enum": _SPORT_TYPES,
                        "description": "Limit to activities of this sport (optional)",
                    },
                    "activity_ids": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Filter to specific activity UUIDs (for multi-hop: pass IDs from a prior query_activities call)",
                    },
                },
            },
        },
    },

    # ── 8. query_segments ─────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "query_segments",
            "description": (
                "Search segments and their effort history: personal records (PR), "
                "recent attempts, segment distance and elevation. "
                "Use for 'Bestzeit auf X', 'PR auf Hausberg', 'Wie oft habe ich Segment Y gefahren?' questions. "
                "Do NOT use for geo features like peaks — use query_geo_features. "
                "Do NOT use for activity lists — use query_activities."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Segment name search (partial match)",
                    },
                    "include_efforts": {
                        "type": "boolean",
                        "description": "Include effort history and PR (default true)",
                        "default": True,
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window for efforts in days (default 365)",
                        "default": 365,
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max segments to return (default 5)",
                        "default": 5,
                    },
                },
            },
        },
    },

    # ── 9. get_analytics ──────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_analytics",
            "description": (
                "Retrieve ML-computed analytics from the cache: anomaly detection results, "
                "activity clustering, route similarity groups, readiness score, "
                "HRV-performance correlations, DFA alpha1 (aerobic threshold). "
                "Use for 'Gibt es Anomalien?', 'Welche Aktivitäten sind ähnlich?', "
                "'Wie korreliert HRV mit meiner Leistung?' questions. "
                "Do NOT use for raw HRV data — use get_health_status instead. "
                "Results may be cached up to 24h old."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "analysis": {
                        "type": "string",
                        "enum": [
                            "anomaly", "clustering", "readiness", "correlations",
                            "dfa_alpha1", "route_similarity",
                        ],
                        "description": "Which ML analysis result to retrieve",
                    },
                },
                "required": ["analysis"],
            },
        },
    },

    # ── 10. search_routes ─────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "search_routes",
            "description": (
                "Search for activities by geographic area or route similarity. "
                "Use for 'Welche Aktivitäten in der Nähe von X?', "
                "'Ähnliche Routen wie heute?', 'War ich schon mal in Y?' questions. "
                "Do NOT use for segment queries — use query_segments. "
                "Do NOT use for geo feature counts (peaks/huts) — use query_geo_features."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "location_name": {
                        "type": "string",
                        "description": "City or region name to search near",
                    },
                    "activity_id": {
                        "type": "string",
                        "description": "UUID of reference activity for similarity search",
                    },
                    "sport_type": {
                        "type": "string",
                        "enum": _SPORT_TYPES,
                        "description": "Limit to sport type (optional)",
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 365)",
                        "default": 365,
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max results (default 10)",
                        "default": 10,
                    },
                },
            },
        },
    },

    # ── 12. get_gear_for_activity ──────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_gear_for_activity",
            "description": (
                "Retrieve the gear/equipment used in a specific activity. "
                "Use for 'Welche Schuhe hatte ich bei dieser Wanderung?', "
                "'Was habe ich bei meinem letzten Lauf getragen?'. "
                "Requires an activity_id — first call query_activities to get the ID if unknown."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "activity_id": {
                        "type": "string",
                        "description": "UUID of the activity",
                    },
                },
                "required": ["activity_id"],
            },
        },
    },

    # ── 13. get_gear_documents ─────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_gear_documents",
            "description": (
                "Retrieve documents (invoices, receipts, manuals) for a specific gear item. "
                "Use for 'Was haben meine Schuhe gekostet?', 'Rechnung für mein Rad', "
                "'Garantiebeleg für Uhr'. "
                "extracted_data JSONB contains parsed invoice data (price, date, vendor). "
                "Requires a gear_id — first call get_gear_stats to find it."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "gear_id": {
                        "type": "string",
                        "description": "UUID of the gear item",
                    },
                },
                "required": ["gear_id"],
            },
        },
    },

    # ── 14. get_activity_detail ────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_activity_detail",
            "description": (
                "Get full details for a specific activity: all metrics, gear name, "
                "photo count, geo-tag count, notes, weather. "
                "Use for drill-down after finding an activity: 'Erzähl mir mehr über diese Aktivität', "
                "'Details zu meinem letzten Lauf'. "
                "IMPORTANT: activity_id must come from a PRIOR query_activities or run_sql result. "
                "NEVER invent or guess UUIDs. "
                "Do NOT use for lists — use query_activities for that."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "activity_id": {
                        "type": "string",
                        "description": "UUID of the activity",
                    },
                },
                "required": ["activity_id"],
            },
        },
    },

    # ── 15. search_photos_by_content ───────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "search_photos_by_content",
            "description": (
                "Semantic photo search using SigLIP2 embeddings. Finds photos by visual content. "
                "Use for 'Hab ich Fotos von Steinböcken?', 'Zeig mir Sonnenuntergang-Fotos', "
                "'Bilder vom Gipfel', 'Fotos mit Tieren'. "
                "Supports German and English queries. Returns photos with similarity scores."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Visual content description (e.g. 'Steinbock auf Felsen', 'Sonnenuntergang über Bergen')",
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max results (default 10)",
                        "default": 10,
                    },
                },
                "required": ["query"],
            },
        },
    },

    # ── 11. run_sql ──────────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "run_sql",
            "description": (
                "Run a read-only SQL query when specialized tools can't answer the question. "
                "Use for complex queries: year-over-year comparisons, JOINs, GROUP BY, window functions. "
                "user_id filtering is AUTOMATIC (RLS) — NEVER add WHERE user_id=... or current_user. "
                "NEVER use $1/$2 parameter placeholders. SELECT only.\n"
                "IMPORTANT sport_type mapping: MTB/Mountainbike→sport_type='cycling' AND sub_type='mountain', "
                "Gravel→sub_type='gravel_cycling', Trail→sport_type='running' AND sub_type='trail', "
                "E-Bike→sport_type='cycling' AND is_ebike=true, Bouldern→sport_type='rock_climbing' AND sub_type='bouldering', "
                "Schwimmen open water→sub_type='open_water', Indoor-Rad→sub_type='indoor_cycling'.\n"
                "IMPORTANT SQL rules: Use ROUND(x::numeric, n) not ROUND(float8, n). "
                "gear.type values: 'bike','shoes','other' (NOT 'bicycle'). "
                "activity_geo_tags.category values: 'summit','pass','still_water','flowing_water','forest','settlement','border'.\n"
                "Tables: "
                "activities(id UUID,title,sport_type,sub_type,started_at TIMESTAMPTZ,elapsed_time_s,moving_time_s,"
                "distance_m,elevation_gain_m,avg_speed_ms,avg_heart_rate,avg_power_w,tss,trimp,"
                "is_indoor,start_city,gear_id UUID) | "
                "segments(id UUID,created_by UUID,name,sport_group,distance_m,elevation_gain_m,effort_count) | "
                "segment_efforts(segment_id UUID,activity_id UUID,started_at,elapsed_time_s,"
                "sport_type,avg_power_w,avg_heart_rate,avg_speed_ms,is_personal_best,is_reverse) | "
                "activity_laps(activity_id UUID,message_index,start_time,total_elapsed_time_s,"
                "total_distance_m,avg_speed_ms,avg_heart_rate) | "
                "gear(id UUID,name,type TEXT['bike','shoes','other'],brand,model,total_distance_m,status) | "
                "pmc_daily(date DATE,tss,ctl,atl,tsb) | "
                "health_snapshots(measured_at_utc,rmssd_ms,sdrr_ms) | "
                "health_sleep(calendar_date DATE,deep_seconds,light_seconds,rem_seconds,score_overall) | "
                "health_weight_entries(measured_at_utc,weight_g,body_fat_pct) | "
                "tags(id UUID,name,color) | activity_tags(activity_id UUID,tag_id UUID) | "
                "activity_geo_tags(activity_id UUID,category TEXT,name,osm_id,ele FLOAT8,lat,lng) | "
                "activity_stops(activity_id UUID,name,duration_s,poi_type,poi_name) | "
                "activity_photos(activity_id UUID,taken_at,lat,lng,is_video)"
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "sql": {
                        "type": "string",
                        "description": "SELECT query (user_id filter is automatic via RLS)",
                    },
                },
                "required": ["sql"],
            },
        },
    },
    # ── 16. search_pois_nearby ──────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "search_pois_nearby",
            "description": (
                "Search POIs (huts, gastronomy, peaks, shelters, etc.) near a coordinate. "
                "Use for 'Hütten in der Nähe', 'nächster Gipfel', 'Gasthäuser um Innsbruck'."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "lat": {"type": "number", "description": "Latitude"},
                    "lng": {"type": "number", "description": "Longitude"},
                    "radius_m": {"type": "integer", "default": 5000, "description": "Search radius in meters"},
                    "category": {
                        "type": "string",
                        "description": "POI category filter",
                        "enum": ["summit", "pass", "settlement", "gastronomy", "accommodation",
                                 "shelter", "viewpoint", "camping", "transport", "shop",
                                 "sport", "information", "landmark", "religious"],
                    },
                    "limit": {"type": "integer", "default": 20},
                },
                "required": ["lat", "lng"],
            },
        },
    },
    # ── 17. search_trails_by_difficulty ─────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "search_trails_by_difficulty",
            "description": (
                "Search hiking/MTB trails by difficulty in an area. "
                "Use for 'T2-T3 Wanderwege rund um Garmisch', 'MTB S0-S2 Trails bei Innsbruck'."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "lat": {"type": "number"},
                    "lng": {"type": "number"},
                    "radius_m": {"type": "integer", "default": 10000},
                    "max_sac_scale": {
                        "type": "string",
                        "description": "Max SAC scale (hiking_trail, mountain_hiking, demanding_mountain_hiking, alpine_hiking, demanding_alpine_hiking, difficult_alpine_hiking)",
                    },
                    "max_mtb_scale": {"type": "string", "description": "Max MTB scale: 0-5"},
                    "surface": {"type": "string", "description": "Surface filter: asphalt, gravel, ground, etc."},
                    "limit": {"type": "integer", "default": 20},
                },
                "required": ["lat", "lng"],
            },
        },
    },
    # ── 18. plan_route ──────────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "plan_route",
            "description": (
                "Plan a route between waypoints via BRouter. Select the appropriate profile "
                "for the sport. Use for 'plane eine Wanderung von A nach B', 'Radtour Innsbruck-Brenner'. "
                "Needs at least 2 waypoints as [lng, lat] pairs."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "waypoints": {
                        "type": "array",
                        "items": {"type": "array", "items": {"type": "number"}, "minItems": 2, "maxItems": 2},
                        "description": "Waypoints as [lng, lat] pairs (minimum 2)",
                        "minItems": 2,
                    },
                    "profile": {
                        "type": "string",
                        "enum": ["hiking-mountain", "hiking-sac2", "trekking", "cycling-road",
                                 "cycling-mountain", "cycling-gravel", "running", "walking"],
                        "description": "BRouter profile",
                    },
                    "profile_params": {
                        "type": "object",
                        "description": "Optional profile variable overrides: SAC_scale_limit (0-6), consider_elevation (true/false), iswet (0/1)",
                    },
                },
                "required": ["waypoints"],
            },
        },
    },
    # ── 19. search_pois_on_route ────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "search_pois_on_route",
            "description": (
                "Find POIs along a planned route (huts, peaks, gastronomy on the way). "
                "Requires a route geometry (GeoJSON LineString). "
                "Use AFTER plan_route to find interesting stops."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "route_geometry": {"type": "string", "description": "GeoJSON LineString of the route"},
                    "buffer_m": {"type": "integer", "default": 500, "description": "Buffer distance around route in meters"},
                    "category": {"type": "string", "description": "POI category filter"},
                    "limit": {"type": "integer", "default": 50},
                },
                "required": ["route_geometry"],
            },
        },
    },
    # ── 20. get_isochrone ───────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_isochrone",
            "description": (
                "Calculate a reachability area (isochrone) from a point via BadRouter graph. "
                "Use for 'was erreiche ich in 2 Stunden ab Innsbruck zu Fuß'. "
                "Requires routing capability (osm_ways with topology)."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "lat": {"type": "number"},
                    "lng": {"type": "number"},
                    "time_minutes": {"type": "integer", "default": 60, "description": "Travel time in minutes"},
                    "profile": {"type": "string", "default": "hiking-mountain"},
                },
                "required": ["lat", "lng"],
            },
        },
    },
    # ── 21. query_coverage ──────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "query_coverage",
            "description": (
                "Query spatial coverage data at a location. Returns 0–255 values from "
                "Coverage Grid layers. Use for 'Wie viel Wald?', 'Ist es laut hier?', "
                "'Wie urbanisiert ist diese Gegend?', 'Gibt es Gewässer in der Nähe?'. "
                "Values are uint8 (0=none, 255=maximum); -1 means layer unavailable. "
                "Use this instead of run_sql on osm_ways coverage columns "
                "(noise_score, forest_pct, etc.) — those columns may be NULL when "
                "Phase 1 enrichment is disabled."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "lat": {"type": "number", "description": "Latitude (WGS84)"},
                    "lon": {"type": "number", "description": "Longitude (WGS84)"},
                    "layers": {
                        "type": "string",
                        "description": (
                            "Comma-separated layer names: "
                            "noise, forest, traffic, protected, urbanization, "
                            "river_proximity, water, landuse"
                        ),
                    },
                },
                "required": ["lat", "lon", "layers"],
            },
        },
    },
]

# Backward-compatible alias used by existing call sites
TOOL_DEFINITIONS = ALL_TOOL_DEFINITIONS


def get_tool_definitions(tool_names: list[str] | None = None) -> list[dict]:
    """Return tool definitions, optionally filtered by name.

    Args:
        tool_names: List of tool names to include. Pass None to get all tools.

    Returns:
        List of tool definition dicts ready for LLM function-calling API.
    """
    if tool_names is None:
        return ALL_TOOL_DEFINITIONS
    name_set = set(tool_names)
    return [t for t in ALL_TOOL_DEFINITIONS if t["function"]["name"] in name_set]


# ── Helper formatters ──────────────────────────────────────────────────────────

def _format_duration(seconds: int | float) -> str:
    """Format seconds as human-readable duration string."""
    if not seconds:
        return "0min"
    mins = int(seconds) // 60
    if mins >= 60:
        return f"{mins // 60}h{mins % 60:02d}min"
    return f"{mins}min"


def _format_activity_compact(row: dict) -> dict:
    """Format activity record for LLM consumption (short keys, pre-computed values)."""
    distance_km = round((row.get("distance_m") or 0) / 1000, 1)
    duration_s = row.get("elapsed_time_s") or row.get("duration_s", 0)
    speed = row.get("avg_speed_ms") or 0

    sport_label = row.get("sport_type", "")
    if row.get("sub_type"):
        sport_label = f"{sport_label}/{row['sub_type']}"

    result = {
        "id": str(row["id"]) if row.get("id") else "",
        "date": row["started_at"].strftime("%d.%m.%Y") if row.get("started_at") else "",
        "title": row.get("title", ""),
        "sport": sport_label,
        "km": distance_km,
        "duration": _format_duration(duration_s),
        "hr": row.get("avg_heart_rate"),
        "elevation_m": round(row.get("elevation_gain_m", 0) or 0),
        "city": row.get("start_city", ""),
    }

    # Add pace or speed depending on sport
    if row.get("sport_type") in ("running", "trail_running", "hiking", "walking"):
        if speed and speed > 0:
            pace_min_km = (1000.0 / speed) / 60.0
            result["pace"] = f"{int(pace_min_km)}:{int((pace_min_km % 1) * 60):02d} min/km"
    elif speed and speed > 0:
        result["speed"] = f"{round(speed * 3.6, 1)} km/h"

    return result


# ── Tool execution ─────────────────────────────────────────────────────────────

async def execute_tool(pool, user_id: UUID, tool_name: str, args: dict) -> dict:
    """Execute a tool call and return results."""
    dispatch = {
        "query_activities": _query_activities,
        "aggregate_stats": _aggregate_stats,
        "get_health_status": _get_health_status,
        "get_training_load": _get_training_load,
        "compare_periods": _compare_periods,
        "get_gear_stats": _get_gear_stats,
        "query_geo_features": _query_geo_features,
        "query_segments": _query_segments,
        "get_analytics": _get_analytics,
        "search_routes": _search_routes,
        "run_sql": _run_sql,
        "get_gear_for_activity": _get_gear_for_activity,
        "get_gear_documents": _get_gear_documents,
        "get_activity_detail": _get_activity_detail,
        "search_photos_by_content": _search_photos_by_content,
        "search_pois_nearby": _search_pois_nearby,
        "search_trails_by_difficulty": _search_trails_by_difficulty,
        "plan_route": _plan_route,
        "search_pois_on_route": _search_pois_on_route,
        "get_isochrone": _get_isochrone,
        "query_coverage": _query_coverage,
        # Legacy aliases kept for backward compatibility
        "get_hrv_trend": _get_hrv_trend_legacy,
        "get_readiness": _get_readiness_legacy,
    }
    fn = dispatch.get(tool_name)
    if fn is None:
        return {"error": f"Unknown tool: {tool_name}"}
    try:
        return await fn(pool, user_id, **args)
    except Exception as exc:
        import logging
        err_msg = str(exc).strip() or type(exc).__name__
        logging.getLogger(__name__).error("Tool %s failed: %s: %s", tool_name, type(exc).__name__, err_msg, exc_info=True)
        return {"error": f"Tool '{tool_name}' konnte nicht ausgeführt werden: {err_msg}"}


# ── Implementations ────────────────────────────────────────────────────────────

_VALID_SORTS = {
    "date": "started_at",
    "distance": "distance_m",
    "duration": "elapsed_time_s",
    "hr": "avg_heart_rate",
    "elevation": "elevation_gain_m",
}


async def _query_activities(
    pool, user_id,
    sport_type=None, sub_type=None, days=30, limit=10, sort_by="date",
    min_distance_km=None, title_search=None, gear_id=None,
):
    from uuid import UUID as UUIDType
    since = date.today() - timedelta(days=days)
    sort_col = _VALID_SORTS.get(sort_by, "started_at")

    conditions = ["a.user_id = $1", "a.started_at >= $2"]
    params: list = [user_id, since]

    if sport_type:
        params.append(sport_type)
        conditions.append(f"a.sport_type = ${len(params)}")
    if sub_type:
        params.append(sub_type)
        conditions.append(f"a.sub_type = ${len(params)}")
    if min_distance_km is not None:
        params.append(min_distance_km * 1000)
        conditions.append(f"a.distance_m >= ${len(params)}")
    if title_search:
        params.append(f"%{title_search}%")
        conditions.append(f"a.title ILIKE ${len(params)}")
    if gear_id:
        params.append(UUIDType(gear_id))
        conditions.append(f"a.gear_id = ${len(params)}")

    where = " AND ".join(conditions)
    query = (
        "SELECT a.id, a.title, a.sport_type, a.sub_type, a.started_at, "
        "a.distance_m, a.elapsed_time_s, a.avg_heart_rate, a.elevation_gain_m, "
        "a.avg_speed_ms, a.start_city "
        f"FROM activities a WHERE {where} "
        f"ORDER BY a.{sort_col} DESC NULLS LAST LIMIT {min(int(limit), 50)}"
    )
    rows = await pool.fetch(query, *params)
    return {"activities": [_format_activity_compact(dict(r)) for r in rows]}


async def _aggregate_stats(pool, user_id, metric="count", sport_type=None, days=30, group_by="none"):
    since = date.today() - timedelta(days=days)

    _METRIC_SQL = {
        "total_distance": "COALESCE(SUM(distance_m), 0) / 1000.0 AS value",
        "total_duration": "COALESCE(SUM(elapsed_time_s), 0) / 3600.0 AS value",
        "count": "COUNT(*) AS value",
        "avg_heart_rate": "ROUND(AVG(avg_heart_rate)::numeric, 1) AS value",
        "total_elevation": "COALESCE(SUM(elevation_gain_m), 0) AS value",
        "avg_distance": "ROUND((COALESCE(AVG(distance_m), 0) / 1000.0)::numeric, 2) AS value",
    }
    agg_expr = _METRIC_SQL.get(metric, "COUNT(*) AS value")

    conditions = ["user_id = $1", "started_at >= $2"]
    params: list = [user_id, since]

    if sport_type:
        params.append(sport_type)
        conditions.append(f"sport_type = ${len(params)}")

    where = " AND ".join(conditions)

    if group_by == "week":
        group_expr = "date_trunc('week', started_at)::date AS period"
        query = f"SELECT {group_expr}, {agg_expr} FROM activities WHERE {where} GROUP BY period ORDER BY period"
    elif group_by == "month":
        group_expr = "date_trunc('month', started_at)::date AS period"
        query = f"SELECT {group_expr}, {agg_expr} FROM activities WHERE {where} GROUP BY period ORDER BY period"
    elif group_by == "sport":
        query = f"SELECT sport_type AS period, {agg_expr} FROM activities WHERE {where} GROUP BY sport_type ORDER BY value DESC"
    else:
        query = f"SELECT {agg_expr}, COUNT(*) AS activity_count FROM activities WHERE {where}"

    rows = await pool.fetch(query, *params)
    data = [dict(r) for r in rows]

    _METRIC_UNITS = {
        "total_distance": "km",
        "total_duration": "h",
        "count": "activities",
        "avg_heart_rate": "bpm",
        "total_elevation": "m",
        "avg_distance": "km",
    }

    return {
        "metric": metric,
        "unit": _METRIC_UNITS.get(metric, ""),
        "sport_type": sport_type,
        "days": days,
        "group_by": group_by,
        "data": data,
    }


async def _get_health_status(pool, user_id, aspect="all", days=30):
    since = date.today() - timedelta(days=days)
    result: dict = {}

    if aspect in ("hrv", "all"):
        rows = await pool.fetch(
            "SELECT measured_at_utc::date AS date, rmssd_ms, sdrr_ms, "
            "extra_data "
            "FROM health_snapshots WHERE user_id = $1 AND measured_at_utc >= $2 "
            "ORDER BY measured_at_utc",
            user_id, since,
        )
        hrv_data = []
        for r in rows:
            entry = {"date": r["date"], "rmssd_ms": r["rmssd_ms"], "sdrr_ms": r["sdrr_ms"]}
            # HRV status fields (weekly_avg, baseline, status) live in extra_data JSONB
            extra = r["extra_data"]
            if extra:
                if isinstance(extra, str):
                    extra = json.loads(extra)
                for key in ("weekly_avg", "baseline_low", "baseline_high", "status"):
                    if key in extra:
                        entry[key] = extra[key]
            hrv_data.append(entry)
        result["hrv"] = {
            "data": hrv_data,
            "latest": hrv_data[-1] if hrv_data else None,
            "avg_rmssd": round(sum(r["rmssd_ms"] or 0 for r in hrv_data) / len(hrv_data), 1) if hrv_data else None,
        }

    if aspect in ("sleep", "all"):
        rows = await pool.fetch(
            "SELECT calendar_date AS date, "
            "(COALESCE(deep_seconds, 0) + COALESCE(light_seconds, 0) + COALESCE(rem_seconds, 0)) AS total_sleep_s, "
            "deep_seconds, light_seconds, rem_seconds, awake_seconds, score_overall AS score "
            "FROM health_sleep WHERE user_id = $1 AND calendar_date >= $2 ORDER BY calendar_date",
            user_id, since,
        )
        sleep_data = [dict(r) for r in rows]
        result["sleep"] = {
            "data": sleep_data,
            "avg_score": round(sum(r.get("score") or 0 for r in sleep_data) / len(sleep_data), 1) if sleep_data else None,
            "avg_total_h": round(sum((r.get("total_sleep_s") or 0) for r in sleep_data) / 3600 / len(sleep_data), 1) if sleep_data else None,
        }

    if aspect in ("readiness", "all"):
        row = await pool.fetchrow(
            "SELECT data FROM analytics_cache "
            "WHERE user_id = $1 AND cache_key = 'readiness' "
            "ORDER BY computed_at DESC LIMIT 1",
            user_id,
        )
        if row and row["data"]:
            result["readiness"] = json.loads(row["data"]) if isinstance(row["data"], str) else row["data"]
        else:
            result["readiness"] = None

    if aspect in ("weight", "all"):
        rows = await pool.fetch(
            "SELECT measured_at_utc::date AS date, "
            "ROUND((weight_g / 1000.0)::numeric, 1) AS weight_kg, body_fat_pct "
            "FROM health_weight_entries WHERE user_id = $1 AND measured_at_utc >= $2 "
            "ORDER BY measured_at_utc",
            user_id, since,
        )
        result["weight"] = [dict(r) for r in rows]

    if aspect in ("resting_hr", "all"):
        rows = await pool.fetch(
            "SELECT calendar_date AS date, resting_heart_rate AS resting_hr_bpm "
            "FROM health_daily_summary WHERE user_id = $1 AND calendar_date >= $2 "
            "AND resting_heart_rate IS NOT NULL ORDER BY calendar_date",
            user_id, since,
        )
        result["resting_hr"] = [dict(r) for r in rows]

    return result


async def _get_training_load(pool, user_id, days=30):
    since = date.today() - timedelta(days=days)
    rows = await pool.fetch(
        "SELECT date, tss, ctl, atl, tsb FROM pmc_daily "
        "WHERE user_id = $1 AND date >= $2 ORDER BY date",
        user_id, since,
    )
    data = [dict(r) for r in rows]
    if data:
        latest = data[-1]
        acwr = round(latest["atl"] / latest["ctl"], 2) if latest.get("ctl") and latest["ctl"] > 0 else None
        return {"pmc_data": data, "current": {**latest, "acwr": acwr}}
    return {"pmc_data": [], "current": None}


async def _compare_periods(
    pool, user_id,
    period1_start, period1_end,
    period2_start, period2_end,
    sport_type=None,
):
    async def _period_stats(start, end):
        conditions = ["user_id = $1", "started_at::date BETWEEN $2 AND $3"]
        params: list = [user_id, date.fromisoformat(start), date.fromisoformat(end)]
        if sport_type:
            params.append(sport_type)
            conditions.append(f"sport_type = ${len(params)}")
        where = " AND ".join(conditions)
        row = await pool.fetchrow(
            f"SELECT COUNT(*) AS count, "
            f"ROUND((COALESCE(SUM(distance_m), 0) / 1000.0)::numeric, 1) AS total_distance_km, "
            f"COALESCE(SUM(elapsed_time_s), 0) AS total_duration_s, "
            f"ROUND(COALESCE(AVG(avg_heart_rate), 0)::numeric, 1) AS avg_heart_rate, "
            f"ROUND(COALESCE(SUM(elevation_gain_m), 0)::numeric, 0) AS total_elevation_m "
            f"FROM activities WHERE {where}",
            *params,
        )
        return dict(row) if row else {}

    p1 = await _period_stats(period1_start, period1_end)
    p2 = await _period_stats(period2_start, period2_end)
    return {
        "period1": {"range": f"{period1_start} – {period1_end}", **p1},
        "period2": {"range": f"{period2_start} – {period2_end}", **p2},
    }


async def _get_gear_stats(pool, user_id, gear_id=None, gear_type=None):
    from uuid import UUID as UUIDType
    conditions = ["user_id = $1"]
    params: list = [user_id]

    if gear_id:
        params.append(UUIDType(gear_id))
        conditions.append(f"id = ${len(params)}")
    else:
        conditions.append("status = 'active'")

    if gear_type:
        params.append(gear_type)
        conditions.append(f"type = ${len(params)}")

    where = " AND ".join(conditions)
    rows = await pool.fetch(
        f"SELECT id, name, type, brand, model, "
        f"ROUND((total_distance_m / 1000.0)::numeric, 0) AS total_distance_km, "
        f"total_activities, status "
        f"FROM gear WHERE {where} ORDER BY total_distance_m DESC",
        *params,
    )
    return {"gear": [dict(r) for r in rows]}


async def _query_geo_features(
    pool, user_id,
    feature_type=None, min_elevation=None, days=365, limit=20, sport_type=None,
    activity_ids=None, date_from=None, date_to=None,
):
    if date_from:
        since = date.fromisoformat(date_from)
    else:
        since = date.today() - timedelta(days=days)

    # Map user-facing feature_type to DB category names
    _FEATURE_TO_CATEGORY = {
        "lake": "still_water",
        "river": "flowing_water",
        # These map 1:1
        "summit": "summit",
        "pass": "pass",
        "forest": "forest",
        "settlement": "settlement",
        "border": "border",
    }
    if feature_type and feature_type != "gastronomy":
        feature_type = _FEATURE_TO_CATEGORY.get(feature_type, feature_type)

    if feature_type == "gastronomy":
        gastro_conditions = [
            "a.user_id = $1", "a.started_at >= $2",
            "s.poi_type IN ('restaurant','alpine_hut','cafe','biergarten','fast_food','pub','bar')"
        ]
        params: list = [user_id, since]
        if date_to:
            params.append(date.fromisoformat(date_to))
            gastro_conditions.append(f"a.started_at <= ${len(params)}")
        table_join = (
            "JOIN activities a ON a.id = s.activity_id WHERE "
            + " AND ".join(gastro_conditions)
        )
        select = "s.poi_name AS name, s.poi_type AS subtype, ST_Y(s.location) AS lat, ST_X(s.location) AS lng, s.duration_s"
        from_clause = "activity_stops s"
    else:
        conditions = ["a.user_id = $1", "a.started_at >= $2"]
        params = [user_id, since]

        if date_to:
            params.append(date.fromisoformat(date_to))
            conditions.append(f"a.started_at <= ${len(params)}")
        if feature_type:
            params.append(feature_type)
            conditions.append(f"gt.category = ${len(params)}")
        if min_elevation is not None:
            params.append(min_elevation)
            conditions.append(f"gt.ele >= ${len(params)}")
        if sport_type:
            params.append(sport_type)
            conditions.append(f"a.sport_type = ${len(params)}")
        if activity_ids:
            from uuid import UUID as UUIDType
            params.append([UUIDType(aid) for aid in activity_ids])
            conditions.append(f"a.id = ANY(${len(params)}::uuid[])")

        where = " AND ".join(conditions)
        table_join = f"JOIN activities a ON a.id = gt.activity_id WHERE {where}"
        select = "gt.name, gt.category, gt.ele, gt.lat, gt.lng, a.started_at::date AS activity_date, a.title AS activity_title"
        from_clause = "activity_geo_tags gt"

    count_query = f"SELECT COUNT(*) FROM {from_clause} {table_join}"
    total_count = await pool.fetchval(count_query, *params)

    query = (
        f"SELECT {select} FROM {from_clause} {table_join} "
        f"ORDER BY {'gt.ele DESC NULLS LAST' if feature_type != 'gastronomy' else 's.duration_s DESC'} "
        f"LIMIT {min(int(limit), 100)}"
    )
    rows = await pool.fetch(query, *params)
    data = [dict(r) for r in rows]
    return {"feature_type": feature_type, "total_count": total_count, "features": data, "note": f"Showing {len(data)} of {total_count} total matches"}


async def _query_segments(pool, user_id, query=None, include_efforts=True, days=365, limit=5):
    since = date.today() - timedelta(days=days)

    conditions = ["se.user_id = $1"]
    params: list = [user_id]

    if query:
        params.append(f"%{query}%")
        conditions.append(f"s.name ILIKE ${len(params)}")

    where = " AND ".join(conditions)
    sql = (
        "SELECT s.id, s.name, s.sport_group, "
        "ROUND((s.distance_m / 1000.0)::numeric, 2) AS distance_km, "
        "s.elevation_gain_m, "
        "COUNT(se.id) AS effort_count, "
        "MIN(se.elapsed_time_s) AS pr_seconds, "
        "MAX(se.started_at)::date AS last_attempt "
        "FROM segments s "
        "JOIN segment_efforts se ON se.segment_id = s.id "
        f"WHERE {where} "
        "GROUP BY s.id, s.name, s.sport_group, s.distance_m, s.elevation_gain_m "
        f"ORDER BY last_attempt DESC LIMIT {min(int(limit), 20)}"
    )
    rows = await pool.fetch(sql, *params)

    segments = []
    for r in rows:
        seg = dict(r)
        if seg.get("pr_seconds"):
            seg["pr_formatted"] = _format_duration(seg["pr_seconds"])
        segments.append(seg)

    if include_efforts and segments:
        # Fetch last 3 efforts for each segment
        seg_ids = [r["id"] for r in rows]
        effort_rows = await pool.fetch(
            "SELECT segment_id, started_at::date AS date, elapsed_time_s, avg_heart_rate "
            "FROM segment_efforts "
            "WHERE user_id = $1 AND segment_id = ANY($2) AND started_at >= $3 "
            "ORDER BY started_at DESC",
            user_id, seg_ids, since,
        )
        efforts_by_seg: dict = {}
        for e in effort_rows:
            sid = str(e["segment_id"])
            if sid not in efforts_by_seg:
                efforts_by_seg[sid] = []
            if len(efforts_by_seg[sid]) < 3:
                efforts_by_seg[sid].append({
                    "date": str(e["date"]),
                    "time": _format_duration(e["elapsed_time_s"]),
                    "hr": e["avg_heart_rate"],
                })
        for seg in segments:
            seg["recent_efforts"] = efforts_by_seg.get(str(seg["id"]), [])

    return {"segments": segments}


async def _get_analytics(pool, user_id, analysis):
    row = await pool.fetchrow(
        "SELECT data, computed_at FROM analytics_cache "
        "WHERE user_id = $1 AND cache_key = $2 "
        "ORDER BY computed_at DESC LIMIT 1",
        user_id, analysis,
    )
    if row:
        data = json.loads(row["data"]) if isinstance(row["data"], str) else row["data"]
        return {"analysis": analysis, "computed_at": str(row["computed_at"])[:19], "data": data}
    return {"analysis": analysis, "data": None, "note": "No cached result available"}


async def _search_routes(pool, user_id, location_name=None, activity_id=None, sport_type=None, days=365, limit=10):
    since = date.today() - timedelta(days=days)

    conditions = ["a.user_id = $1", "a.started_at >= $2"]
    params: list = [user_id, since]

    if location_name:
        params.append(f"%{location_name}%")
        conditions.append(
            f"(a.start_city ILIKE ${len(params)} OR a.start_state ILIKE ${len(params)})"
        )
    if sport_type:
        params.append(sport_type)
        conditions.append(f"a.sport_type = ${len(params)}")

    where = " AND ".join(conditions)
    rows = await pool.fetch(
        f"SELECT a.id, a.title, a.sport_type, a.started_at::date AS date, "
        f"ROUND((a.distance_m / 1000.0)::numeric, 1) AS distance_km, "
        f"a.elevation_gain_m, a.start_city "
        f"FROM activities a WHERE {where} "
        f"ORDER BY a.started_at DESC LIMIT {min(int(limit), 50)}",
        *params,
    )
    return {"routes": [dict(r) for r in rows]}


async def _run_sql(pool, user_id, sql):
    """Execute a validated read-only SQL query via the SQL agent."""
    from agent.sql_agent import execute_sql
    rows = await execute_sql(sql, user_id, max_rows=50)
    return {"rows": rows, "count": len(rows)}


async def _get_gear_for_activity(pool, user_id, activity_id):
    from uuid import UUID as UUIDType
    row = await pool.fetchrow(
        "SELECT g.id, g.name, g.type, g.brand, g.model, "
        "ROUND((g.total_distance_m / 1000.0)::numeric, 0) AS total_distance_km, "
        "g.total_activities, g.status "
        "FROM gear g "
        "JOIN activities a ON a.gear_id = g.id "
        "WHERE a.id = $1 AND a.user_id = $2",
        UUIDType(activity_id), user_id,
    )
    if row:
        return {"gear": dict(row)}
    return {"gear": None, "note": "No gear assigned to this activity"}


async def _get_gear_documents(pool, user_id, gear_id):
    from uuid import UUID as UUIDType
    rows = await pool.fetch(
        "SELECT doc_type, original_filename, extracted_data, processing_status, created_at::date AS date "
        "FROM gear_documents "
        "WHERE gear_id = $1 AND user_id = $2 "
        "ORDER BY created_at",
        UUIDType(gear_id), user_id,
    )
    docs = []
    total_cost = 0.0
    total_cost_currency = None
    for r in rows:
        doc = dict(r)
        # Parse extracted_data if string
        if doc.get("extracted_data") and isinstance(doc["extracted_data"], str):
            doc["extracted_data"] = json.loads(doc["extracted_data"])
        # Accumulate cost summary so LLM doesn't need to sum across documents
        ed = doc.get("extracted_data") or {}
        if isinstance(ed, dict):
            try:
                amount = float(ed.get("total_amount") or ed.get("amount") or 0)
                currency = ed.get("currency", "")
                if amount:
                    total_cost += amount
                    if currency and not total_cost_currency:
                        total_cost_currency = currency
            except (TypeError, ValueError):
                pass
        docs.append(doc)
    result: dict = {"documents": docs, "count": len(docs)}
    if total_cost:
        result["total_cost"] = round(total_cost, 2)
        if total_cost_currency:
            result["total_cost_currency"] = total_cost_currency
    return result


async def _get_activity_detail(pool, user_id, activity_id):
    from uuid import UUID as UUIDType
    row = await pool.fetchrow(
        "SELECT a.id, a.title, a.sport_type, a.sub_type, a.started_at, a.elapsed_time_s, "
        "a.distance_m, a.elevation_gain_m, "
        "a.avg_heart_rate, a.max_heart_rate, a.avg_speed_ms, a.avg_power_w, "
        "a.start_city, a.start_state, a.start_country, "
        "a.tss, a.description, a.weather_temp_c, a.weather_condition, "
        "g.name AS gear_name, "
        "(SELECT COUNT(*) FROM activity_photos ap WHERE ap.activity_id = a.id) AS photo_count, "
        "(SELECT COUNT(*) FROM activity_geo_tags gt WHERE gt.activity_id = a.id) AS geo_tag_count "
        "FROM activities a "
        "LEFT JOIN gear g ON g.id = a.gear_id "
        "WHERE a.id = $1 AND a.user_id = $2",
        UUIDType(activity_id), user_id,
    )
    if not row:
        return {"error": "Activity not found"}
    result = dict(row)
    # Format distance + duration for readability
    result["distance_km"] = round((result.pop("distance_m") or 0) / 1000, 2)
    result["duration"] = _format_duration(result.pop("elapsed_time_s") or 0)
    return {"activity": result}


async def _search_photos_by_content(pool, user_id, query: str, limit: int = 10):
    # Requires SigLIP2 embedding for the query text
    try:
        from models.clip import embed_text
        text_vec = await embed_text(query)
    except ImportError:
        return {"error": "Photo search not available — SigLIP2 model not loaded", "photos": []}
    except Exception as e:
        logger.warning("embed_text failed: %s", e)
        return {"error": f"Embedding error: {e}", "photos": []}

    rows = await pool.fetch(
        "SELECT ap.id, ap.activity_id, ap.s3_key_thumb, ap.lat, ap.lng, "
        "ap.taken_at::date AS date, "
        "1 - (ap.clip_embedding <=> $2::vector) AS similarity, "
        "a.title AS activity_title "
        "FROM activity_photos ap "
        "JOIN activities a ON a.id = ap.activity_id "
        "WHERE ap.user_id = $1 AND ap.clip_embedding IS NOT NULL "
        "ORDER BY ap.clip_embedding <=> $2::vector "
        "LIMIT $3",
        user_id, str(text_vec), min(int(limit), 50),
    )
    photos = []
    for r in rows:
        p = dict(r)
        p["similarity"] = round(float(p["similarity"]), 3)
        photos.append(p)
    return {"query": query, "photos": photos, "count": len(photos)}


# ── Legacy compatibility wrappers ──────────────────────────────────────────────

async def _get_hrv_trend_legacy(pool, user_id, days=30):
    """Legacy: get_hrv_trend → get_health_status(aspect='hrv')."""
    result = await _get_health_status(pool, user_id, aspect="hrv", days=days)
    return {"hrv_data": result.get("hrv", {}).get("data", [])}


async def _get_readiness_legacy(pool, user_id):
    """Legacy: get_readiness → get_health_status(aspect='readiness')."""
    result = await _get_health_status(pool, user_id, aspect="readiness")
    return {"readiness": result.get("readiness")}


# ── Routing & POI tools ────────────────────────────────────────────────────────

async def _search_pois_nearby(
    pool, user_id,
    lat: float, lng: float, radius_m: int = 5000,
    category: str | None = None, limit: int = 20,
) -> dict:
    """Search POIs near a coordinate point."""
    query = """
        SELECT name, category, subtype, ele,
               ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
               ST_Distance(geom::geography, ST_Point($1,$2)::geography) AS dist_m
        FROM osm_pois
        WHERE ST_DWithin(geom::geography, ST_Point($1,$2)::geography, $3)
    """
    params: list = [lng, lat, radius_m]
    if category:
        query += " AND category = $4"
        params.append(category)
    query += f" ORDER BY dist_m LIMIT ${len(params) + 1}"
    params.append(limit)
    rows = await pool.fetch(query, *params)
    return {"pois": [dict(r) for r in rows], "count": len(rows)}


async def _search_trails_by_difficulty(
    pool, user_id,
    lat: float, lng: float, radius_m: int = 10000,
    max_sac_scale: str | None = None, max_mtb_scale: str | None = None,
    surface: str | None = None, limit: int = 20,
) -> dict:
    """Search trails by difficulty near a coordinate."""
    query = """
        SELECT name, highway, sac_scale, mtb_scale, surface, tracktype,
               length_m, avg_slope,
               ST_Distance(geom::geography, ST_Point($1,$2)::geography) AS dist_m
        FROM osm_ways
        WHERE ST_DWithin(geom::geography, ST_Point($1,$2)::geography, $3)
          AND highway IN ('path', 'footway', 'track', 'bridleway', 'cycleway', 'steps')
    """
    params: list = [lng, lat, radius_m]
    _SAC_ORDER = {
        "hiking": 1, "mountain_hiking": 2, "demanding_mountain_hiking": 3,
        "alpine_hiking": 4, "demanding_alpine_hiking": 5, "difficult_alpine_hiking": 6,
    }
    if max_sac_scale:
        max_level = _SAC_ORDER.get(max_sac_scale, 6)
        allowed = [k for k, v in _SAC_ORDER.items() if v <= max_level]
        params.append(allowed)
        query += f" AND (sac_scale IS NULL OR sac_scale = ANY(${len(params)}))"
    if max_mtb_scale:
        try:
            mtb_level = int(float(max_mtb_scale))  # handle "2", "2.0", "S2" → strip prefix
        except (ValueError, TypeError):
            mtb_level = 5  # fallback: allow all levels
        mtb_level = max(0, min(5, mtb_level))  # clamp to valid range 0-5
        allowed_mtb = [str(i) for i in range(mtb_level + 1)]
        params.append(allowed_mtb)
        query += f" AND (mtb_scale IS NULL OR mtb_scale = ANY(${len(params)}))"
    if surface:
        params.append(surface)
        query += f" AND surface = ${len(params)}"
    params.append(limit)
    query += f" ORDER BY dist_m LIMIT ${len(params)}"
    rows = await pool.fetch(query, *params)
    return {"trails": [dict(r) for r in rows], "count": len(rows)}


async def _plan_route(
    pool, user_id,
    waypoints: list, profile: str = "hiking-mountain",
    profile_params: dict | None = None,
) -> dict:
    """Plan a route via BRouter directly."""
    import httpx

    brouter_url = os.environ.get("BROUTER_URL", "http://brouter:17777")
    lonlats = "|".join(f"{wp[0]:.6f},{wp[1]:.6f}" for wp in waypoints)
    params = f"lonlats={lonlats}&profile={profile}&format=geojson"

    if profile_params:
        for k, v in profile_params.items():
            if _SAFE_PROFILE_PARAM.match(str(k)) and _SAFE_PROFILE_PARAM.match(str(v)):
                params += f"&profile:{k}={v}"

    try:
        async with httpx.AsyncClient() as client:
            resp = await client.get(
                f"{brouter_url}/brouter?{params}", timeout=30.0,
            )
            if resp.status_code != 200:
                return {"error": f"BRouter returned HTTP {resp.status_code}", "fallback": True}
            return resp.json()
    except Exception as e:
        logger.warning("plan_route failed: %s", e)
        return {"error": "Routing nicht verfügbar — BRouter-Segmente geladen?", "fallback": True}


async def _search_pois_on_route(
    pool, user_id,
    route_geometry: str, buffer_m: int = 500,
    category: str | None = None, limit: int = 50,
) -> dict:
    """Find POIs along a route geometry."""
    if len(route_geometry) > 500_000:
        return {"error": "Route geometry too large (max 500KB GeoJSON)", "pois": [], "count": 0}
    query = """
        SELECT name, category, subtype, ele,
               ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
               ST_Distance(geom::geography, ST_GeomFromGeoJSON($1)::geography) AS dist_m
        FROM osm_pois
        WHERE ST_DWithin(geom::geography, ST_GeomFromGeoJSON($1)::geography, $2)
    """
    params: list = [route_geometry, buffer_m]
    if category:
        params.append(category)
        query += f" AND category = ${len(params)}"
    params.append(limit)
    query += f" ORDER BY dist_m LIMIT ${len(params)}"
    rows = await pool.fetch(query, *params)
    return {"pois": [dict(r) for r in rows], "count": len(rows)}


async def _get_isochrone(
    pool, user_id,
    lat: float, lng: float, time_minutes: int = 60,
    profile: str = "hiking-mountain",
) -> dict:
    """Calculate reachability area via pgRouting driving_distance."""
    try:
        nearest = await pool.fetchrow("""
            SELECT id FROM osm_ways_vertices_pgr
            ORDER BY the_geom <-> ST_SetSRID(ST_Point($1, $2), 4326)
            LIMIT 1
        """, lng, lat)
    except Exception:
        return {"error": "Keine Topologie verfügbar — Trail-Topologie muss zuerst aufgebaut werden"}
    if not nearest:
        return {"error": "Keine Topologie verfügbar — OSM-Import mit Trail-Topologie nötig"}

    # pgr_drivingDistance: max_cost in Minuten (entspricht der cost-Einheit aus der Topologie,
    # die auf tobler_speed-basiertem Zeitaufwand beruht — siehe osm_import.go Phase 5b)
    rows = await pool.fetch("""
        SELECT DISTINCT ON (node)
               node, agg_cost,
               ST_Y(v.the_geom) AS lat, ST_X(v.the_geom) AS lng
        FROM pgr_drivingDistance(
            'SELECT id, source, target, cost, reverse_cost FROM osm_ways WHERE cost >= 0',
            $1, $2, false
        ) dd
        JOIN osm_ways_vertices_pgr v ON dd.node = v.id
    """, nearest["id"], float(time_minutes))
    points = [{"lat": r["lat"], "lng": r["lng"], "minutes": r["agg_cost"]} for r in rows]
    hull = None
    if len(points) >= 3:
        hull_row = await pool.fetchrow("""
            SELECT ST_AsGeoJSON(ST_ConvexHull(ST_Collect(
                ST_SetSRID(ST_Point(lng, lat), 4326)
            ))) AS geojson
            FROM unnest($1::float8[], $2::float8[]) AS t(lng, lat)
        """, [p["lng"] for p in points], [p["lat"] for p in points])
        if hull_row:
            hull = hull_row["geojson"]
    return {"points": points, "hull_geojson": hull, "count": len(points), "center": {"lat": lat, "lng": lng}}


async def _query_coverage(
    pool, user_id,
    lat: float, lon: float, layers: str,
) -> dict:
    """Query coverage grid values at a geographic point from the Go API."""
    import httpx

    api_url = os.environ.get("API_INTERNAL_URL", "http://api:8080")
    try:
        async with httpx.AsyncClient() as client:
            resp = await client.get(
                f"{api_url}/internal/coverage/query",
                params={"lat": lat, "lon": lon, "layers": layers},
                timeout=30.0,
            )
            if resp.status_code != 200:
                return {"error": f"Coverage query returned HTTP {resp.status_code}", "values": {}}
            return {"lat": lat, "lon": lon, "layers": resp.json()}
    except Exception as e:
        logger.warning("query_coverage failed: %s", e)
        return {"error": f"Coverage query nicht verfügbar: {e}", "values": {}}
