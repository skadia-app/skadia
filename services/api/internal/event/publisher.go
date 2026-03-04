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
type Stage2ReadyEvent struct {
	ActivityID string `json:"activity_id"`
	UserID     string `json:"user_id"`
}

func (p *Publisher) PublishStage2Ready(event Stage2ReadyEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal stage2 event: %w", err)
	}

	_, err = p.js.Publish("skadia.stage2.high", data)
	if err != nil {
		return fmt.Errorf("publish stage2 event: %w", err)
	}

	slog.Info("event published",
		"subject", "skadia.stage2.high",
		"activity_id", event.ActivityID,
	)
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
	S3Bucket      string  `json:"s3_bucket"`
	S3Key         string  `json:"s3_key"`
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

// JetStream returns the JetStream context for creating consumers.
func (p *Publisher) JetStream() natsgo.JetStreamContext {
	return p.js
}

func (p *Publisher) Close() {
	if p.nc != nil {
		p.nc.Close()
	}
}
