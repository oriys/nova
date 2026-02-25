package fairqueue

import (
	"context"
	"time"
)

// budgetKeyType is the context key for timeout budget metadata.
type budgetKeyType struct{}

var budgetKey = budgetKeyType{}

// BudgetInfo tracks remaining timeout budget through the pipeline.
type BudgetInfo struct {
	OriginalDeadline time.Time    `json:"original_deadline"`
	RemainingMs      int64        `json:"remaining_ms"`
	Stages           []StageEntry `json:"stages"`
}

// StageEntry records time spent at each pipeline stage.
type StageEntry struct {
	Name      string        `json:"name"`
	EnteredAt time.Time     `json:"entered_at"`
	Duration  time.Duration `json:"duration"`
}

// WithBudget attaches a timeout budget to the context.
// If the function has a timeout, this creates a context with that deadline
// and embeds budget tracking info.
func WithBudget(ctx context.Context, timeoutS int) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(time.Duration(timeoutS) * time.Second)
	info := &BudgetInfo{
		OriginalDeadline: deadline,
		RemainingMs:      int64(timeoutS) * 1000,
	}
	ctx = context.WithValue(ctx, budgetKey, info)
	return context.WithDeadline(ctx, deadline)
}

// EnterStage records entry into a named pipeline stage.
func EnterStage(ctx context.Context, name string) context.Context {
	info, ok := ctx.Value(budgetKey).(*BudgetInfo)
	if !ok {
		return ctx
	}
	// Close previous stage
	if len(info.Stages) > 0 {
		last := &info.Stages[len(info.Stages)-1]
		if last.Duration == 0 {
			last.Duration = time.Since(last.EnteredAt)
		}
	}
	info.Stages = append(info.Stages, StageEntry{
		Name:      name,
		EnteredAt: time.Now(),
	})
	// Update remaining
	if dl, ok := ctx.Deadline(); ok {
		info.RemainingMs = time.Until(dl).Milliseconds()
	}
	return ctx
}

// RemainingBudget returns the remaining timeout budget in milliseconds.
// Returns -1 if no budget is set.
func RemainingBudget(ctx context.Context) int64 {
	if dl, ok := ctx.Deadline(); ok {
		remaining := time.Until(dl).Milliseconds()
		if remaining < 0 {
			return 0
		}
		return remaining
	}
	return -1
}

// BudgetExhausted returns true if the timeout budget is exhausted.
func BudgetExhausted(ctx context.Context) bool {
	if dl, ok := ctx.Deadline(); ok {
		return time.Now().After(dl)
	}
	return false
}

// GetBudgetInfo returns the budget tracking info from context, if present.
func GetBudgetInfo(ctx context.Context) (*BudgetInfo, bool) {
	info, ok := ctx.Value(budgetKey).(*BudgetInfo)
	return info, ok
}
