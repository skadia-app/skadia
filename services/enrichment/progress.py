import json
import nats


async def publish_progress(
    nc: nats.NATS, activity_id: str, stage: str, status: str, detail: str = ""
) -> None:
    """Publish enrichment progress to NATS for frontend updates."""
    payload = {
        "activity_id": activity_id,
        "stage": stage,
        "status": status,
        "detail": detail,
    }
    await nc.publish("enrichment.progress", json.dumps(payload).encode())
