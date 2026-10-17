package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
)

type fakePruner struct {
	remaining int64
	calls     int
	failAt    int
	gotPeriod time.Duration
}

func (p *fakePruner) DeleteExpiredEvents(_ context.Context, retention time.Duration, batch int32) (int64, error) {
	p.calls++
	p.gotPeriod = retention
	if p.failAt == p.calls {
		return 0, errors.New("db down")
	}
	n := min(p.remaining, int64(batch))
	p.remaining -= n
	return n, nil
}

func TestRetentionPrunesInBatchesUntilShortBatch(t *testing.T) {
	pruner := &fakePruner{remaining: 2500}
	r, err := application.NewRetention(pruner, 720*time.Hour, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	var reported int64
	r.Deleted = func(n int64) { reported += n }
	n, err := r.PruneOnce(context.Background())
	if err != nil || n != 2500 || reported != 2500 || pruner.calls != 3 || pruner.gotPeriod != 720*time.Hour {
		t.Fatalf("n=%d reported=%d calls=%d period=%s err=%v", n, reported, pruner.calls, pruner.gotPeriod, err)
	}
}

func TestRetentionStopsOnError(t *testing.T) {
	pruner := &fakePruner{remaining: 5000, failAt: 2}
	r, _ := application.NewRetention(pruner, 24*time.Hour, quietLogger())
	n, err := r.PruneOnce(context.Background())
	if err == nil || n != 1000 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestRetentionRejectsTinyPeriods(t *testing.T) {
	if _, err := application.NewRetention(&fakePruner{}, time.Minute, nil); err == nil {
		t.Fatal("periods under an hour must be rejected")
	}
}
