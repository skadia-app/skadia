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
    subject = f"skadia.enrichment.progress.{user_id}" if user_id else "skadia.enrichment.progress"
    await nc.publish(subject, json.dumps(payload).encode())
