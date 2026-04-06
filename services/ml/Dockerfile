FROM python:3.13-slim
RUN apt-get update -qq && apt-get install -y -qq \
    git \
    ffmpeg \
    libheif-dev \
    libjxl-dev \
    build-essential \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu && \
    pip install --no-cache-dir -r requirements.txt
COPY . .
RUN useradd -r -d /app -s /usr/sbin/nologin mluser
RUN chown -R mluser:mluser /app
USER mluser
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
