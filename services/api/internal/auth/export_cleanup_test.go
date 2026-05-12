package auth

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartArtifactCleanupRunsStartupSweepOnce(t *testing.T) {
	var sweeps int32
	h := &Handler{
		shutdownCtx: context.Background(),
		artifactMaintenanceHooks: artifactMaintenanceHooks{
			startupSweep: func(context.Context) { atomic.AddInt32(&sweeps, 1) },
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.StartGDPRArtifactMaintenance(ctx)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&sweeps) == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("startup sweep count = %d, want 1", sweeps)
}
