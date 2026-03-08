import json
import nats


async def publish_progress(
    nc: nats.NATS, activity_id: str, stage: str, status: str, detail: str = "", user_id: str = ""
) -> None:
    """Publish enrichment progress to NATS for frontend updates."""
    payload = {
        "activity_id": activity_id,
        "stage": stage,
        "status": status,
        "detail": detail,
        "user_id": user_id,
    }
    await nc.publish("skadia.enrichment.progress", json.dumps(payload).encode())
