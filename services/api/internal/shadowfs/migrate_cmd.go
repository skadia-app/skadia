package shadowfs

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// extractTagsForFile queries domain tables for schema rendering tags.
// Used by S3→Local migration to compute schema-based local paths.
//
// Returns nil when parentID is nil/empty — callers should then fall back to
// a flat path of the form {userDir}/{dir}/{hash}{ext}.
func extractTagsForFile(ctx context.Context, pool *pgxpool.Pool, fileType, parentType string, parentID *string) map[string]string {
	if parentID == nil || *parentID == "" {
		return nil
	}
	tags := make(map[string]string)

	switch parentType {
	case "activity":
		var startedAt time.Time
		var sportType, subType, title, city string
		var distanceM float64
		var elapsedTimeS int
		var format string
		err := pool.QueryRow(ctx,
			`SELECT started_at, sport_type,
			        COALESCE(sub_type, ''), COALESCE(title, ''),
			        COALESCE(city, ''), COALESCE(distance_m, 0),
			        COALESCE(elapsed_time_s, 0),
			        COALESCE(original_file_format, '')
			 FROM activities WHERE id = $1`, *parentID,
		).Scan(&startedAt, &sportType, &subType, &title, &city,
			&distanceM, &elapsedTimeS, &format)
		if err == nil {
			tags["year"] = startedAt.Format("2006")
			tags["month"] = startedAt.Format("01")
			tags["day"] = startedAt.Format("02")
			tags["time"] = startedAt.Format("1504")
			tags["sport"] = sportType
			if subType != "" {
				tags["subCategory"] = subType
			}
			if title != "" {
				tags["title"] = title
			}
			if city != "" {
				tags["city"] = city
			}
			if distanceM > 0 {
				tags["distance_km"] = strconv.FormatFloat(distanceM/1000.0, 'f', 1, 64)
			}
			if elapsedTimeS > 0 {
				h := elapsedTimeS / 3600
				m := (elapsedTimeS % 3600) / 60
				tags["duration_h"] = strconv.Itoa(h) + "-" + strconv.Itoa(m)
			}
			if format != "" {
				tags["format"] = format
			}
		}

	case "route":
		var title, sportType string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(title, ''), COALESCE(sport_type, '')
			 FROM routes WHERE id = $1`, *parentID,
		).Scan(&title, &sportType)
		if err == nil {
			if title != "" {
				tags["title"] = title
			}
			if sportType != "" {
				tags["sport"] = sportType
			}
		}

	case "gear":
		var name, category string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(name, ''), COALESCE(category, '')
			 FROM gear WHERE id = $1`, *parentID,
		).Scan(&name, &category)
		if err == nil {
			if name != "" {
				tags["title"] = name
			}
			if category != "" {
				tags["category"] = category
			}
		}

	case "gear_component":
		var name, category string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(name, ''), COALESCE(category, '')
			 FROM gear_components WHERE id = $1`, *parentID,
		).Scan(&name, &category)
		if err == nil {
			if name != "" {
				tags["title"] = name
			}
			if category != "" {
				tags["category"] = category
			}
		}

	case "tag":
		var name string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(name, '') FROM tags WHERE id = $1`, *parentID,
		).Scan(&name)
		if err == nil && name != "" {
			tags["title"] = name
		}

	case "marker":
		var name string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(name, '') FROM markers WHERE id = $1`, *parentID,
		).Scan(&name)
		if err == nil && name != "" {
			tags["title"] = name
		}

	case "photo":
		// Photos are media attached to an activity; pull tags from the activity.
		var activityID string
		err := pool.QueryRow(ctx,
			`SELECT COALESCE(activity_id::text, '') FROM activity_photos WHERE id = $1`, *parentID,
		).Scan(&activityID)
		if err == nil && activityID != "" {
			return extractTagsForFile(ctx, pool, fileType, "activity", &activityID)
		}
	}

	// Always include the current year/month as fallback so schema variables
	// never render as "unknown" for parent types we have partial data on.
	if _, ok := tags["year"]; !ok {
		now := time.Now()
		tags["year"] = now.Format("2006")
		tags["month"] = now.Format("01")
		tags["day"] = now.Format("02")
	}

	return tags
}
