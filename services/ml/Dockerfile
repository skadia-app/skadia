FROM python:3.14-slim

LABEL org.opencontainers.image.title="Skadia ML"
LABEL org.opencontainers.image.description="Skadia ML service — embeddings, multimodal scoring, analytics"
LABEL org.opencontainers.image.licenses="AGPL-3.0-or-later"
LABEL org.opencontainers.image.source="https://github.com/skadia-app/skadia"

RUN apt-get update -qq && apt-get install -y -qq \
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
RUN pip install --no-cache-dir --index-url "$TORCH_INDEX_URL" "$TORCH_PACKAGE" \
    && pip install --no-cache-dir -r requirements.txt
COPY . .
RUN useradd -r -d /app -s /usr/sbin/nologin mluser
RUN chown -R mluser:mluser /app
USER mluser

HEALTHCHECK --interval=30s --timeout=3s --start-period=60s --retries=3 \
    CMD curl -fsS --max-time 2 http://localhost:8000/livez || exit 1

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
