#!/usr/bin/env bash
# llama-server.sh - Startet llama.cpp Server mit Vulkan-Backend (AMD Vega 56)
#
# Aufruf aus dem Skadia-Projektverzeichnis:
#   ./scripts/llama-server.sh
#
# Umgebungsvariablen:
#   LLAMA_MODEL   - Pfad zum GGUF-Modell (relativ zum Projektverzeichnis)
#                   Default: data/models/Qwen3.5-9B-Q4_K_M.gguf
#   LLAMA_MMPROJ  - Pfad zum Vision-Encoder (mmproj), leer = kein VLM
#                   Default: data/models/mmproj-Qwen3.5-9B-BF16.gguf
#   LLAMA_PORT    - Server-Port (Default: 8181)
#   LLAMA_CTX     - Context Size (Default: 16384, mind. 128K fuer Thinking-Mode)
#   LLAMA_THREADS - CPU-Threads fuer Dekodierung (Default: nproc)
#
# Modelle (unsloth/Qwen3.5-9B-GGUF):
#   LLM:    Qwen3.5-9B-Q4_K_M.gguf  (~5.68 GB)
#   mmproj: mmproj-BF16.gguf (~922 MB)
#   Gesamt: ~6.6 GB auf Vega 56 (8 GB HBM2)
#
# Download (einmalig):
#   huggingface-cli download unsloth/Qwen3.5-9B-GGUF \
#     Qwen3.5-9B-Q4_K_M.gguf mmproj-BF16.gguf \
#     --local-dir data/models/
#
# Sampling-Empfehlungen (Qwen-Autoren):
#   Thinking-Mode:     --temp 0.6 --top-p 0.95 --top-k 20
#   Non-Thinking-Mode: --temp 0.7 --top-p 0.8  --top-k 20

set -euo pipefail

LLAMA_SERVER="$HOME/llama.cpp/build/bin/llama-server"
LLAMA_MODEL="${LLAMA_MODEL:-data/models/Qwen3.5-9B-Q4_K_M.gguf}"
LLAMA_MMPROJ="${LLAMA_MMPROJ:-data/models/mmproj-BF16.gguf}"
LLAMA_PORT="${LLAMA_PORT:-8181}"
LLAMA_CTX="${LLAMA_CTX:-16384}"
LLAMA_THREADS="${LLAMA_THREADS:-$(nproc)}"

# Vulkan Device 0 = AMD Vega 56
export GGML_VULKAN_DEVICE=0

# Pruefen ob llama-server Binary existiert
if [[ ! -x "$LLAMA_SERVER" ]]; then
    echo "FEHLER: llama-server nicht gefunden: $LLAMA_SERVER"
    echo ""
    echo "llama.cpp muss kompiliert sein mit Vulkan-Backend:"
    echo "  cd ~/llama.cpp && cmake -B build -DGGML_VULKAN=ON && cmake --build build -j\$(nproc)"
    exit 1
fi

# Pruefen ob Modell-Datei existiert
if [[ ! -f "$LLAMA_MODEL" ]]; then
    echo "FEHLER: Modell nicht gefunden: $LLAMA_MODEL"
    exit 1
fi

echo "=== llama-server ==="
echo "Modell:   $LLAMA_MODEL"
echo "Port:     $LLAMA_PORT"
echo "Context:  $LLAMA_CTX"
echo "Vulkan:   Device $GGML_VULKAN_DEVICE"
echo "GPU-Layer: 99 (alle)"
echo "Threads:  $LLAMA_THREADS"

# mmproj-Argument nur wenn Datei vorhanden
MMPROJ_ARG=""
if [[ -n "$LLAMA_MMPROJ" && -f "$LLAMA_MMPROJ" ]]; then
    MMPROJ_ARG="--mmproj $LLAMA_MMPROJ"
    echo "mmproj:   $LLAMA_MMPROJ"
fi
echo "===================="
echo ""

exec "$LLAMA_SERVER" \
    --model "$LLAMA_MODEL" \
    $MMPROJ_ARG \
    --host 0.0.0.0 \
    --port "$LLAMA_PORT" \
    --jinja \
    --reasoning-format deepseek \
    -ngl 99 \
    --ctx-size "$LLAMA_CTX" \
    --threads "$LLAMA_THREADS"
