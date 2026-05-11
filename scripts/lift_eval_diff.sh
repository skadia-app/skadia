#!/usr/bin/env bash
# Pre/Post HEAD corpus drift check for the lift-detection heuristic.
#
# Builds two copies of `lift_detect_eval` — one from the working tree, one
# from a baseline git ref (default HEAD) with `services/api/internal/enrich-
# ment/lifts/detect.go` patched in via `git show`. Runs both against the
# user-supplied corpus, emits JSON eval reports, and diffs them through
# `lift_detect_eval -diff`. Exit code 0 when no tolerance-band flips or
# per-lift count changes appear; 1 otherwise.
#
# This is a template for any algorithm with a reproducible eval corpus —
# keep paths argument-driven, never hard-code corpus locations.
#
# Usage:
#   scripts/lift_eval_diff.sh --root <corpus-root> [options]
#
# Required:
#   --root <dir>          corpus root passed to lift_detect_eval -root
#
# Optional:
#   --head <ref>          git ref for the baseline detect.go (default: HEAD)
#   --gt <file>           station-marker YAML relative to --root; enables
#                         per-case boundaries diff (default: _gt_lifts.yaml
#                         if present in --root, else skipped)
#   --manifest <file>     ground-truth manifest relative to --root; enables
#                         counts-level diff (default: ground_truth.yaml if
#                         present, else skipped)
#   --cases <regex>       limit boundaries diff to cases matching regex
#   --marginal-band-m <m> in-band offset drift tolerance in metres (default 10)
#   --workdir <dir>       scratch directory (default: mktemp)
#   --keep-workdir        do not delete --workdir on exit (debug)
#   -h, --help            this message

set -uo pipefail

usage() {
  sed -n '2,/^$/p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)"
if [[ -z "$repo_root" ]]; then
  echo "lift_eval_diff: not inside a git working tree" >&2
  exit 2
fi

head_ref="HEAD"
corpus_root=""
gt_file=""
manifest_file=""
case_regex=""
marginal_band="10"
workdir=""
keep_workdir=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --head) head_ref="$2"; shift 2 ;;
    --root) corpus_root="$2"; shift 2 ;;
    --gt) gt_file="$2"; shift 2 ;;
    --manifest) manifest_file="$2"; shift 2 ;;
    --cases) case_regex="$2"; shift 2 ;;
    --marginal-band-m) marginal_band="$2"; shift 2 ;;
    --workdir) workdir="$2"; shift 2 ;;
    --keep-workdir) keep_workdir=1; shift ;;
    -h|--help) usage 0 ;;
    *) echo "lift_eval_diff: unknown option $1" >&2; usage 2 ;;
  esac
done

if [[ -z "$corpus_root" ]]; then
  echo "lift_eval_diff: --root is required" >&2
  usage 2
fi
if [[ ! -d "$corpus_root" ]]; then
  echo "lift_eval_diff: corpus root '$corpus_root' is not a directory" >&2
  exit 2
fi

# Auto-detect optional GT/manifest files when not explicitly given.
if [[ -z "$gt_file" && -f "$corpus_root/_gt_lifts.yaml" ]]; then
  gt_file="_gt_lifts.yaml"
fi
if [[ -z "$manifest_file" && -f "$corpus_root/ground_truth.yaml" ]]; then
  manifest_file="ground_truth.yaml"
fi

if [[ -z "$gt_file" && -z "$manifest_file" ]]; then
  echo "lift_eval_diff: corpus has neither _gt_lifts.yaml nor ground_truth.yaml — nothing to diff" >&2
  exit 2
fi

# Workdir: scratch space for the reflink copy and JSON outputs.
if [[ -z "$workdir" ]]; then
  workdir="$(mktemp -d -t lift_eval_diff.XXXXXX)"
fi
mkdir -p "$workdir"
cleanup() {
  if [[ "$keep_workdir" -eq 0 ]]; then
    rm -rf "$workdir"
  else
    echo "lift_eval_diff: keeping workdir $workdir" >&2
  fi
}
trap cleanup EXIT

orig_tree="$workdir/orig/api"
detect_path="services/api/internal/enrichment/lifts/detect.go"

mkdir -p "$workdir/orig"
cp -r --reflink=auto "$repo_root/services/api" "$workdir/orig/api"
git -C "$repo_root" show "$head_ref:$detect_path" \
  > "$orig_tree/internal/enrichment/lifts/detect.go"

current_bin="$workdir/lift_detect_eval"
orig_bin="$workdir/lift_detect_eval_orig"

echo "lift_eval_diff: building current binary…" >&2
( cd "$repo_root/services/api" && go build -o "$current_bin" ./cmd/lift_detect_eval/ ) || exit 2
echo "lift_eval_diff: building $head_ref baseline binary…" >&2
( cd "$orig_tree" && go build -o "$orig_bin" ./cmd/lift_detect_eval/ ) || exit 2

# Sanity: hashes must differ when detect.go changed; equal hash means the
# reflink copy was re-patched but produced an identical binary (possible
# if the only change was outside detect.go). We don't fail here — diff
# tooling still runs; result is 0-drift.
if [[ "$(md5sum "$current_bin" | awk '{print $1}')" == "$(md5sum "$orig_bin" | awk '{print $1}')" ]]; then
  echo "lift_eval_diff: current and $head_ref binaries are byte-identical (no detect.go diff?)" >&2
fi

global_exit=0
manifest_exit=0
boundaries_exit=0

# Manifest diff (counts/recall/precision/confidence buckets).
if [[ -n "$manifest_file" ]]; then
  echo
  echo "=== manifest diff ($manifest_file) ==="
  manifest_path="$corpus_root/$manifest_file"
  base_json="$workdir/manifest_baseline.json"
  cur_json="$workdir/manifest_current.json"
  "$orig_bin"   -manifest "$manifest_path" -root "$corpus_root" -format json > "$base_json" || true
  "$current_bin" -manifest "$manifest_path" -root "$corpus_root" -format json > "$cur_json"  || true
  set +e
  "$current_bin" -diff -baseline "$base_json" -current "$cur_json" -marginal-band-m "$marginal_band"
  manifest_exit=$?
  set -e
  if [[ $manifest_exit -ne 0 ]]; then
    global_exit=1
  fi
fi

# Boundaries diff per case with GT endpoints.
if [[ -n "$gt_file" ]]; then
  echo
  echo "=== boundaries diff ($gt_file) ==="
  gt_path="$corpus_root/$gt_file"
  cases=$(grep -E "^  - file:" "$gt_path" | sed 's/^  - file: //; s/^"//; s/"$//')
  for c in $cases; do
    if [[ -n "$case_regex" && ! "$c" =~ $case_regex ]]; then
      continue
    fi
    safe_name=$(echo "$c" | tr '/' '_' | tr ' ' '_')
    base_json="$workdir/bounds_baseline_${safe_name}.json"
    cur_json="$workdir/bounds_current_${safe_name}.json"
    if ! "$orig_bin" -boundaries "$c" -gt "$gt_path" -gt-key "$c" \
        -root "$corpus_root" -format json 2>/dev/null > "$base_json"; then
      # Case has no GT endpoints (`-boundaries` produces "empty lifts list");
      # there is nothing to diff. Counts-only cases are covered above.
      continue
    fi
    "$current_bin" -boundaries "$c" -gt "$gt_path" -gt-key "$c" \
        -root "$corpus_root" -format json 2>/dev/null > "$cur_json" || continue
    set +e
    out=$("$current_bin" -diff -baseline "$base_json" -current "$cur_json" -marginal-band-m "$marginal_band")
    rc=$?
    set -e
    if [[ "$out" == "no drift" ]]; then
      printf "  %-50s =\n" "$c"
    elif [[ "$out" == marginal* ]]; then
      # diff returned exit 0 — same band, all Δ ≤ marginalBandM. Show but
      # don't fail the gate.
      printf "  %-50s ~\n" "$c"
      echo "$out" | sed 's/^/    /'
    else
      printf "  %-50s ×\n" "$c"
      echo "$out" | sed 's/^/    /'
      if [[ $rc -ne 0 ]]; then
        boundaries_exit=1
      fi
    fi
  done
  if [[ $boundaries_exit -ne 0 ]]; then
    global_exit=1
  fi
fi

echo
if [[ $global_exit -eq 0 ]]; then
  echo "lift_eval_diff: 0 drift (manifest=$manifest_exit boundaries=$boundaries_exit)"
else
  echo "lift_eval_diff: drift detected (manifest=$manifest_exit boundaries=$boundaries_exit)"
fi
exit "$global_exit"
