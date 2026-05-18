package database

import (
	"context"
	"slices"
	"strings"
)

func normalizeCountryCodes(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		code := strings.ToUpper(strings.TrimSpace(raw))
		if len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' || seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	slices.Sort(out)
	return out
}

func countryCodeFromRegion(region string) string {
	if iso, ok := regionToISO[strings.ToLower(strings.TrimSpace(region))]; ok {
		return iso
	}
	return ""
}

func (db *DB) RefreshActivityVisitedCountries(ctx context.Context, activityID string) error {
	var pointCodes []string
	if err := db.Pool.QueryRow(ctx, `
		SELECT COALESCE(array_agg(code ORDER BY code), '{}')::text[]
		FROM (
			SELECT DISTINCT upper(trim(code)) AS code
			FROM activities a,
			     unnest(ARRAY[a.start_country_code, a.end_country_code, a.location_country_code]) AS code
			WHERE a.id = $1
			  AND code IS NOT NULL
			  AND length(trim(code)) = 2
		) c
	`, activityID).Scan(&pointCodes); err != nil {
		return err
	}

	codes := normalizeCountryCodes(pointCodes)

	var hasBoundaries bool
	if err := db.Pool.QueryRow(ctx, `SELECT to_regclass('public.osm_boundaries') IS NOT NULL`).Scan(&hasBoundaries); err != nil {
		return err
	}
	if hasBoundaries {
		rows, err := db.Pool.Query(ctx, `
			SELECT DISTINCT COALESCE(NULLIF(b.country_code, ''), b.source_region) AS code_or_region
			FROM activity_tracks t
			JOIN osm_boundaries b
			  ON b.admin_level = 2
			 AND t.track_full && b.geom
			 AND ST_Intersects(t.track_full, b.geom)
			WHERE t.activity_id = $1
		`, activityID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw *string
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			if raw == nil {
				continue
			}
			code := *raw
			if len(strings.TrimSpace(*raw)) != 2 {
				code = countryCodeFromRegion(*raw)
			}
			codes = append(codes, code)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		codes = normalizeCountryCodes(codes)
	}

	_, err := db.Pool.Exec(ctx, `
		UPDATE activities
		SET visited_country_codes = $2
		WHERE id = $1
	`, activityID, codes)
	return err
}

func (db *DB) UpdateActivityLocationCountryFields(ctx context.Context, activityID, country, countryCode string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE activities SET
			location_country      = NULLIF($2, ''),
			location_country_code = NULLIF($3, '')
		WHERE id = $1`,
		activityID, country, countryCode,
	)
	return err
}
