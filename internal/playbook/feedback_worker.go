package playbook

import (
	"context"
	"log/slog"
	"time"

	"github.com/ios9000/PGPulse_01/internal/alert"
)

// FeedbackWorker periodically checks completed playbook runs for implicit feedback signals.
// If the triggering alert auto-resolved within the feedback window, sets feedback_resolved = true.
// If the run was abandoned, sets feedback_resolved = false.
type FeedbackWorker struct {
	store          PlaybookStore
	alertStore     alert.AlertHistoryStore
	feedbackWindow time.Duration
	logger         *slog.Logger
	cancel         context.CancelFunc
}

// NewFeedbackWorker creates a FeedbackWorker.
func NewFeedbackWorker(store PlaybookStore, alertStore alert.AlertHistoryStore, feedbackWindow time.Duration, logger *slog.Logger) *FeedbackWorker {
	if feedbackWindow == 0 {
		feedbackWindow = 5 * time.Minute
	}
	return &FeedbackWorker{
		store:          store,
		alertStore:     alertStore,
		feedbackWindow: feedbackWindow,
		logger:         logger,
	}
}

// Start launches the feedback evaluation loop in a goroutine (60s ticker).
func (w *FeedbackWorker) Start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)
	go w.run(ctx)
}

// Stop cancels the feedback loop.
func (w *FeedbackWorker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *FeedbackWorker) run(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.evaluate(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (w *FeedbackWorker) evaluate(ctx context.Context) {
	// Look at runs completed in the last feedback window + some buffer.
	since := time.Now().Add(-w.feedbackWindow * 2)
	runs, err := w.store.ListCompletedRunsWithoutFeedback(ctx, since)
	if err != nil {
		w.logger.Warn("feedback worker: failed to list runs", "error", err)
		return
	}

	for _, run := range runs {
		if run.CompletedAt == nil {
			continue
		}

		elapsed := time.Since(*run.CompletedAt)

		// Only check implicit feedback within the window.
		if elapsed > w.feedbackWindow {
			continue
		}

		// Check if the triggering alert has resolved.
		if run.TriggerSource == "alert" && run.TriggerID != "" && w.alertStore != nil {
			unresolved, err := w.alertStore.ListUnresolved(ctx)
			if err != nil {
				w.logger.Warn("feedback worker: failed to list unresolved alerts", "error", err)
				continue
			}

			alertStillActive := false
			for _, ae := range unresolved {
				if ae.RuleID == run.TriggerID {
					alertStillActive = true
					break
				}
			}

			if !alertStillActive {
				resolved := true
				if err := w.store.UpdateFeedback(ctx, run.ID, nil, &resolved, "auto-resolved after playbook completion"); err != nil {
					w.logger.Warn("feedback worker: failed to update feedback", "run_id", run.ID, "error", err)
				} else {
					w.logger.Info("feedback worker: auto-resolved feedback set", "run_id", run.ID)
				}
			}
		}
	}
}
