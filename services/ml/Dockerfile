FROM python:3.13-slim

LABEL org.opencontainers.image.title="Skadia ML"
LABEL org.opencontainers.image.description="Skadia ML service — embeddings, multimodal scoring, analytics"
LABEL org.opencontainers.image.licenses="AGPL-3.0-or-later"
LABEL org.opencontainers.image.source="https://github.com/skadia-app/skadia"

RUN apt-get update -qq && apt-get install -y -qq --no-install-recommends \
    ca-certificates \
    git \
    ffmpeg \
    poppler-utils \
    libheif-dev \
    libjxl-dev \
    build-essential \
    curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY requirements.txt .
ARG TORCH_PACKAGE="torch==2.10.0+cpu"
ARG TORCH_INDEX_URL="https://download.pytorch.org/whl/cpu"
ARG SKADIA_GIT_COMMIT=unknown
ARG SKADIA_GIT_COMMIT_TIME=unknown
ARG SKADIA_BUILD_TIME=unknown
ARG SKADIA_IMAGE_DIGEST=
ENV SKADIA_GIT_COMMIT=${SKADIA_GIT_COMMIT}
ENV SKADIA_GIT_COMMIT_TIME=${SKADIA_GIT_COMMIT_TIME}
ENV SKADIA_BUILD_TIME=${SKADIA_BUILD_TIME}
ENV SKADIA_IMAGE_DIGEST=${SKADIA_IMAGE_DIGEST}
ENV SKADIA_CONFIG_VERSION=1
# TORCH_PACKAGE defaults to a pinned version (torch==2.10.0+cpu) via ARG so
# builds can override the accelerator wheel without duplicating the Dockerfile.
# hadolint ignore=DL3013
RUN pip install --no-cache-dir --index-url "$TORCH_INDEX_URL" "$TORCH_PACKAGE" \
    && pip install --no-cache-dir -r requirements.txt
COPY . .
RUN groupadd -r -g 10001 mluser \
    && useradd -r -u 10001 -g mluser -d /app -s /usr/sbin/nologin mluser \
    && chown -R 10001:10001 /app
USER 10001:10001

HEALTHCHECK --interval=30s --timeout=3s --start-period=60s --retries=3 \
    CMD ["curl", "-fsS", "-o", "/dev/null", "--max-time", "2", "http://localhost:8000/livez"]

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
