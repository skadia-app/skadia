package event

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

// ActivityHandler processes activity events.
type ActivityHandler interface {
	RecalculatePMC(ctx context.Context, userID string, activityDate time.Time) error
}

// Consumer subscribes to NATS subjects and handles events.
type Consumer struct {
	js      natsgo.JetStreamContext
	handler ActivityHandler
	sub     *natsgo.Subscription
}

// NewConsumer creates a NATS JetStream consumer for activity events.
func NewConsumer(js natsgo.JetStreamContext, handler ActivityHandler) *Consumer {
	return &Consumer{js: js, handler: handler}
}

// Start begins listening on the skadia.activity.uploaded subject.
func (c *Consumer) Start() error {
	sub, err := c.js.Subscribe("skadia.activity.uploaded", c.handleActivityUploaded,
		natsgo.Durable("pmc-calculator"),
		natsgo.DeliverAll(),
		natsgo.AckExplicit(),
	)
	if err != nil {
		return err
	}
	c.sub = sub
	slog.Info("NATS consumer started", "subject", "skadia.activity.uploaded", "durable", "pmc-calculator")
	return nil
}

func (c *Consumer) handleActivityUploaded(msg *natsgo.Msg) {
	var evt ActivityUploadedEvent
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		slog.Error("failed to unmarshal activity event", "error", err)
		msg.Nak()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.handler.RecalculatePMC(ctx, evt.UserID, time.Now()); err != nil {
		slog.Error("PMC recalculation failed", "user_id", evt.UserID, "error", err)
		msg.Nak()
		return
	}

	msg.Ack()
	slog.Info("PMC recalculated after activity upload",
		"activity_id", evt.ActivityID,
		"user_id", evt.UserID,
	)
}

// Stop unsubscribes the consumer.
func (c *Consumer) Stop() {
	if c.sub != nil {
		c.sub.Unsubscribe()
	}
}
