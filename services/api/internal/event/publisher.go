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

// JetStream returns the JetStream context for creating consumers.
func (p *Publisher) JetStream() natsgo.JetStreamContext {
	return p.js
}

func (p *Publisher) Close() {
	if p.nc != nil {
		p.nc.Close()
	}
}
