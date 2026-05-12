#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <image-ref>" >&2
  exit 2
fi

image_ref="$1"
key_url="https://raw.githubusercontent.com/BerriAI/litellm/0112e53046018d726492c814b3644b7d376029d0/cosign.pub"

if ! command -v cosign >/dev/null 2>&1; then
  echo "cosign is required to verify LiteLLM image signatures" >&2
  exit 127
fi

cosign verify --key "$key_url" "$image_ref"
