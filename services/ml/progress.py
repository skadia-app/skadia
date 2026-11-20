import json
import re
import nats

_SAFE_SUBJECT_TOKEN_RE = re.compile(r"^[A-Za-z0-9_-]+$")


def _progress_subject(user_id: str) -> str:
    """Build a NATS-valid progress subject (nats-py >= 2.16 validates strictly)."""
    token = (user_id or "").strip()
    if token and _SAFE_SUBJECT_TOKEN_RE.match(token):
        return f"skadia.enrichment.progress.{token}"
    return "skadia.enrichment.progress"


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
    subject = _progress_subject(user_id)
    await nc.publish(subject, json.dumps(payload).encode())
