#!/usr/bin/env bash
# fix-legacy-paths.sh
# Migrates activity files from /data/shadow-fs/ to /data/library/{userDir}/activities/
# COPY-then-verify-then-update-DB (never deletes source)
#
# Usage: ./scripts/fix-legacy-paths.sh [--dry-run]

SHADOW_FS_HOST="/path/to/skadia/data/shadow-fs"
LIBRARY_HOST="/path/to/skadia/data/library"
DB_CONTAINER="skadia-postgres-1"

DRY_RUN=0
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
  echo "DRY-RUN mode — no files will be copied, no DB updates"
fi

echo "=== fix-legacy-paths.sh ==="
echo "SHADOW_FS: ${SHADOW_FS_HOST}"
echo "LIBRARY:   ${LIBRARY_HOST}"

# Fetch all records with legacy absolute paths into a temp file
TMPFILE=$(mktemp /tmp/fix-legacy-paths.XXXXXX)
trap 'rm -f "${TMPFILE}"' EXIT

echo "Fetching legacy path records from DB..."
distrobox-host-exec podman exec -i "${DB_CONTAINER}" psql -U skadia -d skadia -t -A -F '|' \
  -c "SELECT id, user_id, storage_path, size_bytes, COALESCE(file_hash, '') FROM user_files WHERE storage_backend = 'local' AND storage_path LIKE '/data/shadow-fs/%' ORDER BY storage_path;" \
  > "${TMPFILE}" 2>/dev/null

TOTAL=$(wc -l < "${TMPFILE}")
echo "Found ${TOTAL} records to migrate"

if [[ "${TOTAL}" -eq 0 ]]; then
  echo "Nothing to do."
  exit 0
fi

MIGRATED=0
FAILED=0
SKIPPED=0

while IFS='|' read -r FILE_ID USER_ID STORAGE_PATH SIZE_BYTES FILE_HASH; do
  # Skip empty lines
  [[ -z "${FILE_ID}" ]] && continue

  # Strip leading /data/shadow-fs/ to get relative part like 2024/03/15_0830_cycling.fit
  REL_PATH="${STORAGE_PATH#/data/shadow-fs/}"
  BASENAME=$(basename "${REL_PATH}")
  DIR_PART=$(dirname "${REL_PATH}")  # e.g. 2024/03

  # Source on host filesystem
  SRC="${SHADOW_FS_HOST}/${REL_PATH}"

  # User directory: use user_id as fallback (no library_dir_name set)
  USER_DIR="${USER_ID}"

  # Target: {library}/{userDir}/activities/{year}/{month}/{filename}
  TARGET_DIR="${LIBRARY_HOST}/${USER_DIR}/activities/${DIR_PART}"
  TARGET="${TARGET_DIR}/${BASENAME}"

  # New relative path for DB (relative to library base)
  NEW_DB_PATH="${USER_DIR}/activities/${DIR_PART}/${BASENAME}"

  if [[ ! -f "${SRC}" ]]; then
    echo "MISSING source: ${SRC} (id=${FILE_ID}) — SKIP"
    FAILED=$((FAILED + 1))
    continue
  fi

  if [[ -f "${TARGET}" ]]; then
    # Already exists — check size
    ACTUAL_SIZE=$(stat -c '%s' "${TARGET}" 2>/dev/null || echo 0)
    if [[ "${SIZE_BYTES}" -eq 0 ]] || [[ "${ACTUAL_SIZE}" -eq "${SIZE_BYTES}" ]]; then
      if [[ "${DRY_RUN}" -eq 0 ]]; then
        distrobox-host-exec podman exec -i "${DB_CONTAINER}" psql -U skadia -d skadia -q \
          -c "UPDATE user_files SET storage_path = '${NEW_DB_PATH}', updated_at = NOW() WHERE id = '${FILE_ID}';" </dev/null 2>&1
      fi
      echo "ALREADY EXISTS (ok): ${BASENAME} → ${NEW_DB_PATH}"
      SKIPPED=$((SKIPPED + 1))
      continue
    else
      echo "EXISTS but size mismatch (db=${SIZE_BYTES} disk=${ACTUAL_SIZE}): overwriting ${TARGET}"
    fi
  fi

  if [[ "${DRY_RUN}" -eq 1 ]]; then
    echo "DRY-RUN: cp ${SRC} → ${TARGET}"
    MIGRATED=$((MIGRATED + 1))
    continue
  fi

  # Create target directory
  mkdir -p "${TARGET_DIR}"

  # Copy file
  if ! cp "${SRC}" "${TARGET}" 2>&1; then
    echo "FAILED to copy: ${SRC} → ${TARGET}"
    FAILED=$((FAILED + 1))
    continue
  fi

  # Verify size
  ACTUAL_SIZE=$(stat -c '%s' "${TARGET}" 2>/dev/null || echo 0)
  if [[ "${SIZE_BYTES}" -gt 0 ]] && [[ "${ACTUAL_SIZE}" -ne "${SIZE_BYTES}" ]]; then
    echo "SIZE MISMATCH after copy: expected=${SIZE_BYTES} actual=${ACTUAL_SIZE} — removing ${TARGET}"
    rm -f "${TARGET}"
    FAILED=$((FAILED + 1))
    continue
  fi

  # Update DB (</dev/null prevents podman from consuming the while-loop's stdin)
  distrobox-host-exec podman exec -i "${DB_CONTAINER}" psql -U skadia -d skadia -q \
    -c "UPDATE user_files SET storage_path = '${NEW_DB_PATH}', updated_at = NOW() WHERE id = '${FILE_ID}';" </dev/null 2>&1

  MIGRATED=$((MIGRATED + 1))
  echo "OK [${MIGRATED}/${TOTAL}]: ${BASENAME} (${ACTUAL_SIZE}B) → ${NEW_DB_PATH}"

done < "${TMPFILE}"

echo ""
echo "=== Summary ==="
echo "Migrated: ${MIGRATED}"
echo "Skipped (already existed): ${SKIPPED}"
echo "Failed:   ${FAILED}"

if [[ "${FAILED}" -gt 0 ]]; then
  echo "ERROR: ${FAILED} files failed — check output above"
  exit 1
fi

echo "fix-legacy-paths.sh complete"
