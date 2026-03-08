import asyncpg
import logging
import numpy as np
from sklearn.cluster import DBSCAN

logger = logging.getLogger(__name__)

# DBSCAN parameters
EPS_DEGREES = 0.003  # ~300m at mid-latitudes
MIN_SAMPLES = 5


async def enrich_commute(pool: asyncpg.Pool, activity_id: str, user_id: str) -> bool:
    """Detect commute patterns using DBSCAN on start/end points.

    Clusters the start points and end points of the user's last 30 activities.
    Home = largest cluster of start points.
    Work = largest cluster of end points.
    If the current activity starts near Home and ends near Work (or vice versa),
    it is marked as is_commute=true.

    Returns True if the activity was marked as commute.
    """
    async with pool.acquire() as conn:
        # Fetch last 30 activities with GPS data for this user
        recent = await conn.fetch(
            """
            SELECT id,
                   ST_Y(start_location) AS start_lat, ST_X(start_location) AS start_lng,
                   ST_Y(end_location) AS end_lat, ST_X(end_location) AS end_lng
            FROM activities
            WHERE user_id = $1
              AND start_location IS NOT NULL
              AND end_location IS NOT NULL
            ORDER BY started_at DESC
            LIMIT 30
            """,
            user_id,
        )

        if len(recent) < MIN_SAMPLES:
            return False

        start_points = np.array(
            [[r["start_lat"], r["start_lng"]] for r in recent]
        )
        end_points = np.array(
            [[r["end_lat"], r["end_lng"]] for r in recent]
        )

        # Cluster start points → home
        start_clusters = DBSCAN(eps=EPS_DEGREES, min_samples=MIN_SAMPLES).fit(
            start_points
        )
        home_center = _largest_cluster_center(start_points, start_clusters.labels_)

        # Cluster end points → work
        end_clusters = DBSCAN(eps=EPS_DEGREES, min_samples=MIN_SAMPLES).fit(
            end_points
        )
        work_center = _largest_cluster_center(end_points, end_clusters.labels_)

        if home_center is None or work_center is None:
            return False

        # Update commute_clusters table
        await conn.execute(
            "DELETE FROM commute_clusters WHERE user_id = $1", user_id
        )
        await conn.execute(
            """
            INSERT INTO commute_clusters
                (user_id, cluster_label, center, radius_m, point_count)
            VALUES ($1, 0, ST_SetSRID(ST_MakePoint($3, $2), 4326), 300, $4)
            """,
            user_id,
            home_center[0],
            home_center[1],
            int(np.sum(start_clusters.labels_ == _largest_label(start_clusters.labels_))),
        )
        await conn.execute(
            """
            INSERT INTO commute_clusters
                (user_id, cluster_label, center, radius_m, point_count)
            VALUES ($1, 1, ST_SetSRID(ST_MakePoint($3, $2), 4326), 300, $4)
            """,
            user_id,
            work_center[0],
            work_center[1],
            int(np.sum(end_clusters.labels_ == _largest_label(end_clusters.labels_))),
        )

        # Check current activity
        current = await conn.fetchrow(
            """
            SELECT ST_Y(start_location) AS start_lat, ST_X(start_location) AS start_lng,
                   ST_Y(end_location) AS end_lat, ST_X(end_location) AS end_lng
            FROM activities WHERE id = $1
            """,
            activity_id,
        )
        if current is None:
            return False

        start_near_home = _is_near(
            current["start_lat"], current["start_lng"], home_center, EPS_DEGREES
        )
        end_near_work = _is_near(
            current["end_lat"], current["end_lng"], work_center, EPS_DEGREES
        )
        start_near_work = _is_near(
            current["start_lat"], current["start_lng"], work_center, EPS_DEGREES
        )
        end_near_home = _is_near(
            current["end_lat"], current["end_lng"], home_center, EPS_DEGREES
        )

        is_commute = (start_near_home and end_near_work) or (
            start_near_work and end_near_home
        )

        if is_commute:
            # Ensure "Pendeln" auto-tag exists for this user
            tag_row = await conn.fetchrow(
                """
                INSERT INTO tags (user_id, name, color, text_color, is_auto, category)
                VALUES ($1, 'Pendeln', '#6B7280', '#FFFFFF', true, 'activity')
                ON CONFLICT (user_id, name) DO UPDATE SET name = tags.name
                RETURNING id
                """,
                user_id,
            )
            tag_id = tag_row["id"]

            # Assign tag to activity (junction table)
            await conn.execute(
                """
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
                VALUES ($1, $2, 'auto')
                ON CONFLICT (activity_id, tag_id) DO NOTHING
                """,
                activity_id,
                tag_id,
            )

            # Sync denormalized activities.tags TEXT[] cache
            await conn.execute(
                """
                UPDATE activities
                SET tags = array_append(tags, 'Pendeln'),
                    updated_at = NOW()
                WHERE id = $1 AND NOT ('Pendeln' = ANY(COALESCE(tags, '{}')))
                """,
                activity_id,
            )

            logger.info("Activity %s tagged as Pendeln", activity_id)

        return is_commute


def _largest_label(labels: np.ndarray) -> int:
    """Return the label of the largest cluster (ignoring noise label -1)."""
    unique, counts = np.unique(labels[labels >= 0], return_counts=True)
    if len(unique) == 0:
        return -1
    return int(unique[np.argmax(counts)])


def _largest_cluster_center(
    points: np.ndarray, labels: np.ndarray
) -> tuple[float, float] | None:
    """Return the centroid of the largest cluster."""
    label = _largest_label(labels)
    if label < 0:
        return None
    mask = labels == label
    center = points[mask].mean(axis=0)
    return (float(center[0]), float(center[1]))


def _is_near(
    lat: float, lng: float, center: tuple[float, float], threshold: float
) -> bool:
    """Check if a point is within threshold degrees of a center."""
    return abs(lat - center[0]) < threshold and abs(lng - center[1]) < threshold
