#!/usr/bin/env bash
set -euo pipefail

API_BASE="${SKADIA_API_BASE:-http://localhost:8080/api/v1}"
EMAIL="${SKADIA_DEV_EMAIL:-}"
PASSWORD="${SKADIA_DEV_PASSWORD:-}"

if [[ -z "$EMAIL" || -z "$PASSWORD" ]]; then
  echo "Set SKADIA_DEV_EMAIL and SKADIA_DEV_PASSWORD." >&2
  exit 2
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 2
  fi
}

require_cmd curl
require_cmd jq
require_cmd timeout

json_escape() {
  jq -Rn --arg value "$1" '$value'
}

login_payload="$(jq -n --arg email "$EMAIL" --arg password "$PASSWORD" '{email:$email,password:$password}')"
token="$(
  curl -fsS -X POST "$API_BASE/auth/login" \
    -H 'Content-Type: application/json' \
    --data "$login_payload" |
    jq -r '.token // empty'
)"
if [[ -z "$token" ]]; then
  echo "Login succeeded but no token was returned." >&2
  exit 1
fi

session_id="$(
  curl -fsS -X POST "$API_BASE/chat/sessions" \
    -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' |
    jq -r '.id // empty'
)"
if [[ -z "$session_id" ]]; then
  echo "Session creation failed." >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
cleanup() {
  if [[ -n "${session_id:-}" && -n "${token:-}" ]]; then
    curl -fsS -X DELETE "$API_BASE/chat/sessions/$session_id" \
      -H "Authorization: Bearer $token" >/dev/null 2>&1 || true
  fi
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

send_prompt() {
  local name="$1"
  local prompt="$2"
  local expected_tool_csv="$3"
  local body out json_lines tools writes
  body="$(jq -n --arg id "smoke-$name-$(date +%s%N)" --arg message "$prompt" '{message_id:$id,message:$message}')"
  out="$tmp_dir/$name.sse"

  timeout "${SKADIA_AGENT_SMOKE_TIMEOUT:-180}" curl -fsS -N -X POST "$API_BASE/chat/sessions/$session_id/send" \
    -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' \
    --data "$body" >"$out"

  json_lines="$tmp_dir/$name.jsonl"
  sed -n 's/^data: //p' "$out" | jq -c 'select(type == "object")' >"$json_lines"

  if jq -e 'select(.type == "chat.error")' "$json_lines" >/dev/null; then
    echo "[$name] chat.error:" >&2
    jq -c 'select(.type == "chat.error")' "$json_lines" >&2
    exit 1
  fi
  if ! jq -e 'select(.type == "chat.done")' "$json_lines" >/dev/null; then
    echo "[$name] stream did not finish with chat.done" >&2
    exit 1
  fi

  tools="$(jq -r 'select(.type == "chat.tools_loaded") | .tools[]?' "$json_lines" | sort -u | tr '\n' ',' | sed 's/,$//')"
  writes="$(jq -r 'select(.type == "chat.tools_loaded") | .tools[]?' "$json_lines" |
    grep -E '^(create_|update_|delete_|rename_|retire_|set_|tag_activity|bulk_tag_activities|save_planned_route|record_health_metric|enable_|disable_)' || true)"
  if [[ -n "$writes" ]]; then
    echo "[$name] read-only smoke exposed write tools:" >&2
    echo "$writes" >&2
    exit 1
  fi

  IFS=',' read -ra expected_tools <<<"$expected_tool_csv"
  for expected in "${expected_tools[@]}"; do
    [[ -z "$expected" ]] && continue
    if ! jq -e --arg tool "$expected" 'select(.type == "chat.tools_loaded") | (.tools // []) | index($tool)' "$json_lines" >/dev/null; then
      echo "[$name] expected tool '$expected' missing; loaded: ${tools:-none}" >&2
      exit 1
    fi
  done

  echo "[$name] ok tools=${tools:-none}"
}

send_prompt "last-activity" "Can you tell me the name of my last activity?" "query_activities"
send_prompt "segment-exists" "Existiert ein Segment mit einem frei gewaehlten Testnamen?" "query_segments"
send_prompt "document-search" "Suche in meinen Dokumenten nach einem frei gewaehlten Testbegriff." "search_documents"

if [[ -n "${SKADIA_SMOKE_PLACE_A:-}" && -n "${SKADIA_SMOKE_PLACE_B:-}" ]]; then
  send_prompt "route-distance" "Wie weit ist ${SKADIA_SMOKE_PLACE_A} von ${SKADIA_SMOKE_PLACE_B} entfernt?" "get_agent_service_status,geocode_place,analyze_route_points"
else
  echo "[route-distance] skipped; set SKADIA_SMOKE_PLACE_A and SKADIA_SMOKE_PLACE_B for routing smoke."
fi
