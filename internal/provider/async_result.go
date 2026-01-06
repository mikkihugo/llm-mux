package provider

import (
	"context"
	"sync"
	"time"

	"github.com/nghyane/llm-mux/internal/registry"
)

// resultWorkerConfig configures the async result worker.
type resultWorkerConfig struct {
	QueueSize int
	Workers   int
}

// asyncResultWorker processes MarkResult calls asynchronously.
// This reduces lock contention in the hot path by moving persistence
// and registry updates to background workers.
type asyncResultWorker struct {
	manager *Manager
	queue   chan resultJob
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

type resultJob struct {
	ctx    context.Context
	result Result
}

// newAsyncResultWorker creates a new async result worker.
func newAsyncResultWorker(m *Manager, cfg resultWorkerConfig) *asyncResultWorker {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 2048
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}

	w := &asyncResultWorker{
		manager: m,
		queue:   make(chan resultJob, cfg.QueueSize),
		stopCh:  make(chan struct{}),
	}

	w.wg.Add(cfg.Workers)
	for i := 0; i < cfg.Workers; i++ {
		go w.worker()
	}

	return w
}

// Submit queues a result for async processing.
// Returns immediately without blocking the caller.
func (w *asyncResultWorker) Submit(ctx context.Context, result Result) {
	select {
	case w.queue <- resultJob{ctx: ctx, result: result}:
	default:
		// Queue full - process synchronously as fallback
		w.processResult(ctx, result)
	}
}

// Stop stops the worker and waits for pending jobs.
func (w *asyncResultWorker) Stop() {
	close(w.stopCh)
	close(w.queue)
	w.wg.Wait()
}

func (w *asyncResultWorker) worker() {
	defer w.wg.Done()
	for {
		select {
		case job, ok := <-w.queue:
			if !ok {
				return
			}
			w.processResult(job.ctx, job.result)
		case <-w.stopCh:
			// Drain remaining
			for job := range w.queue {
				w.processResult(job.ctx, job.result)
			}
			return
		}
	}
}

// processResult is the core result processing logic (extracted from MarkResult).
// It handles state updates, registry notifications, and persistence.
func (w *asyncResultWorker) processResult(ctx context.Context, result Result) {
	if result.AuthID == "" {
		return
	}

	m := w.manager
	shouldResumeModel := false
	shouldSuspendModel := false
	suspendReason := ""
	clearModelQuota := false
	setModelQuota := false
	var authToStore *Auth

	// Fast path: minimal time under lock
	m.mu.Lock()
	if auth, ok := m.auths[result.AuthID]; ok && auth != nil {
		now := time.Now()
		w.updateAuthState(auth, result, now, &shouldResumeModel, &shouldSuspendModel, &suspendReason, &clearModelQuota, &setModelQuota)
		// Clone for async persistence
		authToStore = auth.Clone()
	}
	m.mu.Unlock()

	// These operations are done OUTSIDE the lock
	if clearModelQuota && result.Model != "" {
		registry.GetGlobalRegistry().ClearModelQuotaExceeded(result.AuthID, result.Model)
	}
	if setModelQuota && result.Model != "" {
		registry.GetGlobalRegistry().SetModelQuotaExceeded(result.AuthID, result.Model)
	}
	if shouldResumeModel {
		registry.GetGlobalRegistry().ResumeClientModel(result.AuthID, result.Model)
	} else if shouldSuspendModel {
		registry.GetGlobalRegistry().SuspendClientModel(result.AuthID, result.Model, suspendReason)
	}

	if result.Error != nil && result.Error.HTTPStatus == 429 {
		if qm, ok := m.selector.(*QuotaManager); ok {
			qm.RecordQuotaHit(result.AuthID, result.Provider, result.Model, result.RetryAfter)
		}
	}

	// Async persistence (outside lock)
	if authToStore != nil {
		_ = m.persist(ctx, authToStore)
	}

	m.hook.OnResult(ctx, result)
}

// updateAuthState updates auth state (called under lock).
// Extracted for cleaner code organization.
func (w *asyncResultWorker) updateAuthState(
	auth *Auth,
	result Result,
	now time.Time,
	shouldResumeModel *bool,
	shouldSuspendModel *bool,
	suspendReason *string,
	clearModelQuota *bool,
	setModelQuota *bool,
) {
	if result.Success {
		if result.Model != "" {
			state := ensureModelState(auth, result.Model)
			resetModelState(state, now)

			clearedModels := clearQuotaGroupOnSuccess(auth, result.Model, now)
			for _, clearedModel := range clearedModels {
				registry.GetGlobalRegistry().ClearModelQuotaExceeded(result.AuthID, clearedModel)
				registry.GetGlobalRegistry().ResumeClientModel(result.AuthID, clearedModel)
			}

			updateAggregatedAvailability(auth, now)
			if !hasModelError(auth, now) {
				auth.LastError = nil
				auth.StatusMessage = ""
				auth.Status = StatusActive
			}
			auth.UpdatedAt = now
			*shouldResumeModel = true
			*clearModelQuota = true
		} else {
			clearAuthStateOnSuccess(auth, now)
		}
	} else {
		if result.Model != "" {
			state := ensureModelState(auth, result.Model)
			statusCode := statusCodeFromResult(result.Error)

			var errMsg string
			if result.Error != nil {
				errMsg = result.Error.Message
			}
			category := CategorizeError(statusCode, errMsg)

			if category != CategoryUserError {
				state.Unavailable = true
				state.Status = StatusError
			}
			state.UpdatedAt = now

			if result.Error != nil && category != CategoryUserError {
				state.LastError = cloneError(result.Error)
				state.StatusMessage = result.Error.Message
				auth.LastError = cloneError(result.Error)
				auth.StatusMessage = result.Error.Message
			}

			switch statusCode {
			case 401:
				next := now.Add(30 * time.Minute)
				state.NextRetryAfter = next
				*suspendReason = "unauthorized"
				*shouldSuspendModel = true
			case 402, 403:
				next := now.Add(30 * time.Minute)
				state.NextRetryAfter = next
				*suspendReason = "payment_required"
				*shouldSuspendModel = true
			case 404:
				next := now.Add(12 * time.Hour)
				state.NextRetryAfter = next
				*suspendReason = "not_found"
				*shouldSuspendModel = true
			case 429:
				var next time.Time
				backoffLevel := state.Quota.BackoffLevel
				if result.RetryAfter != nil {
					next = now.Add(*result.RetryAfter)
				} else {
					cooldown, nextLevel := nextQuotaCooldown(backoffLevel)
					if cooldown > 0 {
						next = now.Add(cooldown)
					}
					backoffLevel = nextLevel
				}
				state.NextRetryAfter = next
				state.Quota = QuotaState{
					Exceeded:      true,
					Reason:        "quota",
					NextRecoverAt: next,
					BackoffLevel:  backoffLevel,
				}
				*suspendReason = "quota"
				*shouldSuspendModel = true
				*setModelQuota = true

				affectedModels := propagateQuotaToGroup(auth, result.Model, state.Quota, next, now)
				for _, affectedModel := range affectedModels {
					registry.GetGlobalRegistry().SetModelQuotaExceeded(result.AuthID, affectedModel)
					registry.GetGlobalRegistry().SuspendClientModel(result.AuthID, affectedModel, "quota_group")
				}
			case 408, 500, 502, 503, 504:
				next := now.Add(1 * time.Minute)
				state.NextRetryAfter = next
			default:
				state.NextRetryAfter = now.Add(30 * time.Second)
			}

			if category != CategoryUserError {
				auth.Status = StatusError
			}
			auth.UpdatedAt = now
			updateAggregatedAvailability(auth, now)
		} else {
			applyAuthFailureState(auth, result.Error, result.RetryAfter, now)
		}
	}
}
