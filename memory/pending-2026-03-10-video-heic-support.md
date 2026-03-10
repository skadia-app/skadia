## docs/07_DATABASE_SCHEMA.md — activity_photos Video-Felder
**Suche nach:** activity_photos
**Delta:** Migration 000138 fügt 4 neue Spalten hinzu: `is_video BOOLEAN DEFAULT false`, `duration_s NUMERIC`, `video_width INT`, `video_height INT`

## docs/04_ML_AI_SERVICE.md — kein Änderungsbedarf

## docs/02_CORE_BACKEND.md — activity_photos Video/HEIC/JXL Upload
**Suche nach:** activity_photos
**Delta:**
- Upload-Handler unterstützt jetzt `video/*` (mp4/mov/avi/mkv/webm) und HEIC/JXL neben Bildern
- `maxPhotoSize` auf 200 MB erhöht (war 20 MB)
- Content-Type Detection: http.DetectContentType() + Extension-Fallback für HEIC/JXL/Videos
- Thumbnails für Videos/HEIC/JXL via ffmpeg (`imageutil.GenerateThumbnailFFmpeg()`)
- Video-Metadaten (duration_s, width, height) via ffprobe (`imageutil.ProbeVideo()`)
- Videos bekommen kein Preview (s3_key_preview bleibt leer)
- Neue Hilfsfunktionen: `contentTypeFromExtension()`, `fileExtForContentType()`
- Neue Dateien: `services/api/internal/imageutil/ffmpeg.go`, `ffprobe.go`
- Dockerfile: ffmpeg-Paket hinzugefügt

## MEMORY.md — Migrations-Zähler
**Suche nach:** next free
**Delta:** 000138 hinzugefügt (activity_photos Video-Metadaten); next free: 000139
