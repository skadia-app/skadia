package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/crypto"
	"github.com/skadia-app/skadia/services/api/internal/database"
)

type EmailJob struct {
	ID          string
	UserID      string
	TemplateKey EmailTemplateKey
	Recipient   string
	Payload     map[string]string
	ExpiresAt   time.Time
	Attempts    int
}

type EmailOutboxStore struct {
	db            *database.DB
	encryptionKey string
}

func NewEmailOutboxStore(db *database.DB, encryptionKey string) *EmailOutboxStore {
	return &EmailOutboxStore{db: db, encryptionKey: encryptionKey}
}

func (s *EmailOutboxStore) Enqueue(ctx context.Context, job EmailJob) (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("email outbox has no db")
	}
	if s.encryptionKey == "" {
		return "", fmt.Errorf("email outbox encryption key not configured")
	}
	if _, ok := emailTemplateRegistry[job.TemplateKey]; !ok {
		return "", fmt.Errorf("%w: %s", ErrEmailTemplateUnknown, job.TemplateKey)
	}
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return "", fmt.Errorf("marshal email payload: %w", err)
	}
	recipientCiphertext, err := crypto.Encrypt(job.Recipient, s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("encrypt email recipient: %w", err)
	}
	payloadCiphertext, err := crypto.Encrypt(string(payload), s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("encrypt email payload: %w", err)
	}

	var id string
	var userID any
	if job.UserID != "" {
		userID = job.UserID
	}
	var expiresAt any
	if !job.ExpiresAt.IsZero() {
		expiresAt = job.ExpiresAt
	}
	if err := s.db.Pool.QueryRow(ctx,
		`INSERT INTO email_outbox (user_id, template_key, recipient, payload, recipient_ciphertext, payload_ciphertext, expires_at)
		 VALUES ($1, $2, '', '{}'::jsonb, $3, $4, COALESCE($5, now() + interval '24 hours'))
		 RETURNING id`,
		userID, string(job.TemplateKey), recipientCiphertext, payloadCiphertext, expiresAt,
	).Scan(&id); err != nil {
		return "", fmt.Errorf("enqueue email: %w", err)
	}
	return id, nil
}

func (s *EmailOutboxStore) Claim(ctx context.Context, workerID string, limit int, now time.Time) ([]EmailJob, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.db.Pool.Query(ctx,
		`WITH picked AS (
		    SELECT id
			      FROM email_outbox
			     WHERE status = 'pending'
			       AND next_attempt_at <= $3
			       AND expires_at > $3
			     ORDER BY created_at, id
		     LIMIT $2
		     FOR UPDATE SKIP LOCKED
		 )
		 UPDATE email_outbox o
		    SET status = 'running',
		        claimed_by = $1,
		        claimed_at = now(),
		        attempt_count = attempt_count + 1,
		        updated_at = now()
		   FROM picked
		  WHERE o.id = picked.id
				  RETURNING o.id, COALESCE(o.user_id::text, ''), o.template_key, o.recipient_ciphertext, o.payload_ciphertext, o.attempt_count`,
		workerID, limit, now,
	)
	if err != nil {
		return nil, fmt.Errorf("claim email jobs: %w", err)
	}
	defer rows.Close()

	var jobs []EmailJob
	for rows.Next() {
		var job EmailJob
		var recipientCiphertext string
		var payloadCiphertext string
		if err := rows.Scan(&job.ID, &job.UserID, &job.TemplateKey, &recipientCiphertext, &payloadCiphertext, &job.Attempts); err != nil {
			return nil, fmt.Errorf("scan claimed email job: %w", err)
		}
		recipient, err := crypto.Decrypt(recipientCiphertext, s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt email recipient: %w", err)
		}
		rawPayload, err := crypto.Decrypt(payloadCiphertext, s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt email payload: %w", err)
		}
		job.Recipient = recipient
		if err := json.Unmarshal([]byte(rawPayload), &job.Payload); err != nil {
			return nil, fmt.Errorf("decode email payload: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed email jobs: %w", err)
	}
	return jobs, nil
}

func (s *EmailOutboxStore) ExpirePending(ctx context.Context, now time.Time) error {
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE email_outbox
		    SET status = 'failed',
		        recipient = '',
		        payload = '{}'::jsonb,
		        recipient_ciphertext = '',
		        payload_ciphertext = '',
		        last_error = 'email job expired before delivery',
		        updated_at = now()
		  WHERE status IN ('pending', 'running')
		    AND expires_at <= $1`,
		now,
	)
	if err != nil {
		return fmt.Errorf("expire pending email jobs: %w", err)
	}
	return nil
}

func (s *EmailOutboxStore) RequeueStaleRunning(ctx context.Context, staleBefore time.Time, nextAttemptAt time.Time) error {
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE email_outbox
		    SET status = 'pending',
		        claimed_by = NULL,
		        claimed_at = NULL,
		        next_attempt_at = $2,
		        last_error = COALESCE(last_error, 'requeued stale running email job'),
		        updated_at = now()
		  WHERE status = 'running'
		    AND claimed_at < $1
		    AND expires_at > $2`,
		staleBefore, nextAttemptAt,
	)
	if err != nil {
		return fmt.Errorf("requeue stale email jobs: %w", err)
	}
	return nil
}

func (s *EmailOutboxStore) MarkSent(ctx context.Context, id string) error {
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE email_outbox
		    SET status = 'sent',
		        recipient = '',
		        payload = '{}'::jsonb,
		        recipient_ciphertext = '',
		        payload_ciphertext = '',
		        sent_at = now(),
		        updated_at = now(),
		        last_error = NULL
		  WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("mark email sent: %w", err)
	}
	return nil
}

func (s *EmailOutboxStore) MarkFailed(ctx context.Context, id string, sendErr error, retryAt time.Time) error {
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE email_outbox
		    SET status = CASE WHEN expires_at <= now() THEN 'failed' ELSE 'pending' END,
		        recipient = '',
		        payload = '{}'::jsonb,
		        recipient_ciphertext = CASE WHEN expires_at <= now() THEN '' ELSE recipient_ciphertext END,
		        payload_ciphertext = CASE WHEN expires_at <= now() THEN '' ELSE payload_ciphertext END,
		        claimed_by = CASE WHEN expires_at <= now() THEN claimed_by ELSE NULL END,
		        claimed_at = CASE WHEN expires_at <= now() THEN claimed_at ELSE NULL END,
		        next_attempt_at = CASE WHEN expires_at <= now() THEN next_attempt_at ELSE $2 END,
		        last_error = $3,
		        updated_at = now()
		  WHERE id = $1`,
		id, retryAt, sendErr.Error(),
	)
	if err != nil {
		return fmt.Errorf("mark email failed: %w", err)
	}
	return nil
}
