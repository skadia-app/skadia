package event

import (
	"encoding/json"
	"fmt"
	"log/slog"

	natsgo "github.com/nats-io/nats.go"
)

type Publisher struct {
	nc *natsgo.Conn
	js natsgo.JetStreamContext
}

type ActivityUploadedEvent struct {
	ActivityID string `json:"activity_id"`
	UserID     string `json:"user_id"`
	SportType  string `json:"sport_type"`
	HasGPS     bool   `json:"has_gps"`
}

func NewPublisher(natsURL string) (*Publisher, error) {
	nc, err := natsgo.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create JetStream context: %w", err)
	}

	// Create stream if it doesn't exist
	_, err = js.AddStream(&natsgo.StreamConfig{
		Name:     "SKADIA",
		Subjects: []string{"skadia.>"},
		Storage:  natsgo.FileStorage,
	})
	if err != nil {
		// Stream might already exist
		slog.Warn("NATS stream creation", "error", err)
	}

	slog.Info("NATS publisher connected", "url", natsURL)
	return &Publisher{nc: nc, js: js}, nil
}

func (p *Publisher) PublishActivityUploaded(event ActivityUploadedEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	_, err = p.js.Publish("skadia.activity.uploaded", data)
	if err != nil {
		return fmt.Errorf("publish event: %w", err)
	}

	slog.Info("event published",
		"subject", "skadia.activity.uploaded",
		"activity_id", event.ActivityID,
	)
	return nil
}

// Stage2ReadyEvent is emitted after Stage 1 enrichment completes,
// signalling the ML service to begin Stage 2 processing.
//
// EnabledModules lists the Stage-2 module names that should run.
// An empty (nil) slice means every module is enabled — the ML service
// applies that as its default for backwards compatibility.
// When one or more modules are disabled in the user's enrichment settings
// only the remaining module names are included in this slice.
type Stage2ReadyEvent struct {
	ActivityID     string   `json:"activity_id"`
	UserID         string   `json:"user_id"`
	EnabledModules []string `json:"enabled_modules,omitempty"`
}

func (p *Publisher) PublishStage2Ready(event Stage2ReadyEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal stage2 event: %w", err)
	}

	for _, subject := range []string{"skadia.stage2.high", "skadia.stage2.normal", "skadia.stage2.low"} {
		if _, err = p.js.Publish(subject, data); err != nil {
			return fmt.Errorf("publish stage2 event to %s: %w", subject, err)
		}
		slog.Info("event published", "subject", subject, "activity_id", event.ActivityID)
	}
	return nil
}

// DocumentUploadedEvent is emitted when a gear document is uploaded,
// signalling the ML service to process the document with VLM.
type DocumentUploadedEvent struct {
	DocID         string  `json:"doc_id"`
	GearID        *string `json:"gear_id,omitempty"`
	ComponentID   *string `json:"component_id,omitempty"`
	UserID        string  `json:"user_id"`
	ContentType   string  `json:"content_type"`
	IsTextPDF     bool    `json:"is_text_pdf"`
	ExtractedText *string `json:"extracted_text,omitempty"`
	FileID        string  `json:"file_id"`
}

func (p *Publisher) PublishDocumentUploaded(event DocumentUploadedEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal document event: %w", err)
	}
	_, err = p.js.Publish("skadia.document.uploaded", data)
	if err != nil {
		return fmt.Errorf("publish document event: %w", err)
	}
	slog.Info("event published", "subject", "skadia.document.uploaded", "doc_id", event.DocID)
	return nil
}

// AnalyticsInvalidateEvent signals the ML service to recompute analytics for a user.
type AnalyticsInvalidateEvent struct {
	UserID string `json:"user_id"`
}

// PublishAnalyticsInvalidate publishes a skadia.analytics.invalidate event.
// The ML service listens on this subject and recomputes all analytics cache entries for the user.
func (p *Publisher) PublishAnalyticsInvalidate(userID string) {
	data, err := json.Marshal(AnalyticsInvalidateEvent{UserID: userID})
	if err != nil {
		slog.Warn("analytics invalidate: marshal failed", "error", err)
		return
	}
	_, err = p.js.Publish("skadia.analytics.invalidate", data)
	if err != nil {
		slog.Warn("analytics invalidate: publish failed", "error", err)
		return
	}
	slog.Info("event published", "subject", "skadia.analytics.invalidate", "user_id", userID)
}

// PhotoEmbedEvent is emitted after every photo upload so the ML service
// can compute a CLIP embedding for the image.
type PhotoEmbedEvent struct {
	PhotoID         string `json:"photo_id"`
	UserID          string `json:"user_id"`
	FileID          string `json:"file_id"`
	ContentType     string `json:"content_type,omitempty"`
	VideoClipFrames int    `json:"video_clip_frames,omitempty"` // 0 = use default (4)
}

func (p *Publisher) PublishPhotoEmbed(ev PhotoEmbedEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal photo embed event: %w", err)
	}
	_, err = p.js.Publish("skadia.photo.embed", data)
	if err != nil {
		return fmt.Errorf("publish photo embed event: %w", err)
	}
	slog.Info("event published", "subject", "skadia.photo.embed", "photo_id", ev.PhotoID)
	return nil
}

// JetStream returns the JetStream context for creating consumers.
func (p *Publisher) JetStream() natsgo.JetStreamContext {
	return p.js
}

// Subscribe creates a NATS core subscription on the given subject.
// The returned subscription must be Unsubscribed by the caller when no longer needed.
func (p *Publisher) Subscribe(subject string, handler natsgo.MsgHandler) (*natsgo.Subscription, error) {
	return p.nc.Subscribe(subject, handler)
}

func (p *Publisher) Close() {
	if p.nc != nil {
		p.nc.Close()
	}
}
