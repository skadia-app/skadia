package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/jackc/pgx/v5"
)

func TestGDPRTransferAdmissionLockSerializesTransactions(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)

	tx1, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	defer tx1.Rollback(ctx)
	if err := lockGDPRTransferAdmission(ctx, tx1); err != nil {
		t.Fatalf("lock tx1: %v", err)
	}

	tx2, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	defer tx2.Rollback(ctx)
	blockedCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if err := lockGDPRTransferAdmission(blockedCtx, tx2); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second admission lock err = %v, want context deadline exceeded", err)
	}
	_ = tx2.Rollback(ctx)

	if err := tx1.Rollback(ctx); err != nil {
		t.Fatalf("rollback tx1: %v", err)
	}
	tx3, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("begin tx3: %v", err)
	}
	defer tx3.Rollback(ctx)
	retryCtx, retryCancel := context.WithTimeout(ctx, time.Second)
	defer retryCancel()
	if err := lockGDPRTransferAdmission(retryCtx, tx3); err != nil {
		t.Fatalf("lock tx3 after release: %v", err)
	}
}
