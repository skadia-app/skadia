#!/usr/bin/env bash
# llama-server.sh - Startet llama.cpp Server mit Vulkan-Backend (AMD Vega 56)
#
# Aufruf aus dem Skadia-Projektverzeichnis:
#   ./scripts/llama-server.sh
#
# Umgebungsvariablen:
#   LLAMA_MODEL   - Pfad zum GGUF-Modell (relativ zum Projektverzeichnis)
#                   Default: data/models/qwen3-8b-q4_k_m.gguf
#   LLAMA_PORT    - Server-Port (Default: 8080)
#   LLAMA_CTX     - Context Size (Default: 4096)
#
# Modelle herunterladen (Hugging Face):
#   LLM:  Qwen3-8B-Instruct-Q4_K_M.gguf (~5 GB)
#         https://huggingface.co/Qwen/Qwen3-8B-Instruct-GGUF
#         -> data/models/qwen3-8b-q4_k_m.gguf
#
#   VLM:  Qwen3-VL-30B-A3B-Instruct-Q4_K_M.gguf + mmproj-Qwen3VL-30B-A3B-Instruct-Q8_0.gguf
#         https://huggingface.co/Qwen/Qwen3-VL-30B-A3B-Instruct-GGUF
#         -> data/models/qwen3-vl-30b-a3b-q4_k_m.gguf
#         -> data/models/mmproj-qwen3vl-30b-a3b-q8_0.gguf

set -euo pipefail

LLAMA_SERVER="$HOME/llama.cpp/build/bin/llama-server"
LLAMA_MODEL="${LLAMA_MODEL:-data/models/Qwen3-8B-Instruct-Q4_K_M.gguf}"
LLAMA_PORT="${LLAMA_PORT:-8181}"
LLAMA_CTX="${LLAMA_CTX:-4096}"

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
    echo ""
    echo "Modelle muessen in data/models/ abgelegt werden."
    echo ""
    echo "Download-Beispiele (huggingface-cli oder wget):"
    echo ""
    echo "  LLM (Qwen3-8B, ~5 GB):"
    echo "    huggingface-cli download Qwen/Qwen3-8B-Instruct-GGUF \\"
    echo "      qwen3-8b-instruct-q4_k_m.gguf \\"
    echo "      --local-dir data/models/"
    echo ""
    echo "  VLM (Qwen3-VL-30B-A3B, ~18 GB + ~1 GB Projektor):"
    echo "    huggingface-cli download Qwen/Qwen3-VL-30B-A3B-Instruct-GGUF \\"
    echo "      qwen3-vl-30b-a3b-instruct-q4_k_m.gguf \\"
    echo "      mmproj-qwen3vl-30b-a3b-instruct-q8_0.gguf \\"
    echo "      --local-dir data/models/"
    exit 1
fi

echo "=== llama-server ==="
echo "Modell:   $LLAMA_MODEL"
echo "Port:     $LLAMA_PORT"
echo "Context:  $LLAMA_CTX"
echo "Vulkan:   Device $GGML_VULKAN_DEVICE"
echo "GPU-Layer: 99 (alle)"
echo "===================="
echo ""

exec "$LLAMA_SERVER" \
    --model "$LLAMA_MODEL" \
    --host 0.0.0.0 \
    --port "$LLAMA_PORT" \
    -ngl 99 \
    --ctx-size "$LLAMA_CTX"

# --- VLM-Modus (Multimodal) ---
# Fuer Vision-Language-Modelle (z.B. Qwen3-VL) zusaetzlich --mmproj angeben:
#
# LLAMA_MODEL=data/models/qwen3-vl-30b-a3b-q4_k_m.gguf \
#   ./scripts/llama-server.sh
#
# Oder manuell:
#   ~/llama.cpp/build/bin/llama-server \
#     --model data/models/qwen3-vl-30b-a3b-q4_k_m.gguf \
#     --mmproj data/models/mmproj-qwen3vl-30b-a3b-q8_0.gguf \
#     --host 0.0.0.0 --port 8080 -ngl 99 --ctx-size 4096
