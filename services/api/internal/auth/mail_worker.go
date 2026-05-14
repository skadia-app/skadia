package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const emailRunningJobTimeout = 15 * time.Minute

type EmailWorker struct {
	id        string
	mailer    *Mailer
	batchSize int
}

func NewEmailWorker(id string, mailer *Mailer, batchSize int) *EmailWorker {
	if batchSize <= 0 {
		batchSize = 10
	}
	return &EmailWorker{id: id, mailer: mailer, batchSize: batchSize}
}

func (w *EmailWorker) ProcessOnce(ctx context.Context) error {
	if w == nil || w.mailer == nil || w.mailer.outbox == nil {
		return fmt.Errorf("email worker is not fully configured")
	}
	now := time.Now()
	if err := w.mailer.outbox.ExpirePending(ctx, now); err != nil {
		return err
	}
	if err := w.mailer.outbox.RequeueStaleRunning(ctx, now.Add(-emailRunningJobTimeout), now); err != nil {
		return err
	}
	jobs, err := w.mailer.outbox.Claim(ctx, w.id, w.batchSize, now)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := w.mailer.sendTemplate(job.Recipient, job.TemplateKey, job.Payload); err != nil {
			retryAt := time.Now().Add(emailRetryBackoff(job.Attempts))
			if markErr := w.mailer.outbox.MarkFailed(ctx, job.ID, err, retryAt); markErr != nil {
				return markErr
			}
			slog.Warn("email worker: send failed", "job_id", job.ID, "template", job.TemplateKey, "error", err)
			continue
		}
		if err := w.mailer.outbox.MarkSent(ctx, job.ID); err != nil {
			return err
		}
	}
	return nil
}

func (w *EmailWorker) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := w.ProcessOnce(ctx); err != nil {
			slog.Warn("email worker: process batch failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func emailRetryBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Minute
	}
	if attempt == 2 {
		return 5 * time.Minute
	}
	return 30 * time.Minute
}
