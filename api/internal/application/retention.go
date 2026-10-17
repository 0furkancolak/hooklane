package application

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// EventPruner deletes finished events older than the retention period, at
// most batch rows per call, and reports how many it deleted.
type EventPruner interface {
	DeleteExpiredEvents(ctx context.Context, retention time.Duration, batch int32) (int64, error)
}

// Retention removes events (with their deliveries and attempts) once they are
// older than Period and none of their deliveries is still pending or in
// flight. It is safe to run from several workers at once.
type Retention struct {
	pruner EventPruner
	period time.Duration
	batch  int32
	log    *slog.Logger
	// Deleted is called with each batch's count, for metrics; may be nil.
	Deleted func(n int64)
}

const defaultRetentionBatch = 1000

func NewRetention(pruner EventPruner, period time.Duration, log *slog.Logger) (*Retention, error) {
	if period < time.Hour {
		return nil, errors.New("retention period must be at least 1h")
	}
	if log == nil {
		log = slog.Default()
	}
	return &Retention{pruner: pruner, period: period, batch: defaultRetentionBatch, log: log}, nil
}

// PruneOnce deletes expired events in batches until a batch comes back short.
// Short batches keep each transaction small so deletes do not block ingestion.
func (r *Retention) PruneOnce(ctx context.Context) (int64, error) {
	var total int64
	for {
		n, err := r.pruner.DeleteExpiredEvents(ctx, r.period, r.batch)
		total += n
		if n > 0 && r.Deleted != nil {
			r.Deleted(n)
		}
		if err != nil {
			return total, err
		}
		if n < int64(r.batch) || ctx.Err() != nil {
			return total, ctx.Err()
		}
	}
}

// Run prunes once shortly after start and then every interval until ctx ends.
func (r *Retention) Run(ctx context.Context, interval time.Duration) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		started := time.Now()
		n, err := r.PruneOnce(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			r.log.Error("retention prune failed", "error", err, "deleted_events", n)
		case n > 0:
			r.log.Info("retention prune", "deleted_events", n, "period", r.period.String(), "duration_ms", time.Since(started).Milliseconds())
		}
		timer.Reset(interval)
	}
}
