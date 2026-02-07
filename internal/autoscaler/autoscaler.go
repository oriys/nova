package autoscaler

import (
	"context"
	"sync"
	"time"

	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/metrics"
	"github.com/oriys/nova/internal/pool"
	"github.com/oriys/nova/internal/store"
)

const emaAlpha = 0.3

func ema(prev, cur float64) float64 {
	if prev == 0 {
		return cur
	}
	return emaAlpha*cur + (1-emaAlpha)*prev
}

type funcSnapshot struct {
	invocations   int64
	coldStarts    int64
	totalMs       int64
	lastScaleUp   time.Time
	lastScaleDown time.Time
	// EMA-smoothed signals (alpha = 0.3 for ~3-tick smoothing)
	emaLatencyMs    float64
	emaColdStartPct float64
	emaConcurrency  float64 // for concurrency-based scaling
	// Hourly invocation rate history (24 slots, one per hour)
	hourlyRates  [24]float64
	lastHourSlot int
}

// Autoscaler dynamically adjusts pool sizing based on load signals
type Autoscaler struct {
	pool      *pool.Pool
	store     *store.Store
	interval  time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
	prevState sync.Map // funcID -> *funcSnapshot
}

// New creates a new Autoscaler
func New(p *pool.Pool, s *store.Store, interval time.Duration) *Autoscaler {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Autoscaler{
		pool:     p,
		store:    s,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start launches the autoscaler background goroutine
func (a *Autoscaler) Start() {
	go a.loop()
	logging.Op().Info("autoscaler started", "interval", a.interval)
}

// Stop shuts down the autoscaler
func (a *Autoscaler) Stop() {
	a.cancel()
}

func (a *Autoscaler) loop() {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.evaluate()
		}
	}
}

func (a *Autoscaler) evaluate() {
	ctx := context.Background()
	funcs, err := a.store.ListFunctions(ctx)
	if err != nil {
		logging.Op().Error("autoscaler: list functions", "error", err)
		return
	}

	m := metrics.Global()

	for _, fn := range funcs {
		policy := fn.AutoScalePolicy
		if policy == nil || !policy.Enabled {
			continue
		}

		funcID := fn.ID

		// Gather signals
		queueDepth := a.pool.QueueDepth(funcID)
		total, busy, idle := a.pool.FunctionPoolStats(funcID)

		// Get metrics delta
		fm := m.GetFunctionMetrics(funcID)
		var coldStartRate float64
		var avgLatencyMs int64
		var deltaInvocations int64

		prev := a.getSnapshot(funcID)
		if fm != nil {
			curInvocations := fm.Invocations.Load()
			curColdStarts := fm.ColdStarts.Load()
			curTotalMs := fm.TotalMs.Load()

			deltaInvocations = curInvocations - prev.invocations
			deltaColdStarts := curColdStarts - prev.coldStarts
			deltaTotalMs := curTotalMs - prev.totalMs

			if deltaInvocations > 0 {
				coldStartRate = float64(deltaColdStarts) / float64(deltaInvocations) * 100
				avgLatencyMs = deltaTotalMs / deltaInvocations
			}

			// Update prev state
			prev.invocations = curInvocations
			prev.coldStarts = curColdStarts
			prev.totalMs = curTotalMs
		}

		// Update EMA-smoothed signals
		prev.emaLatencyMs = ema(prev.emaLatencyMs, float64(avgLatencyMs))
		prev.emaColdStartPct = ema(prev.emaColdStartPct, coldStartRate)

		// Compute concurrency and update EMA
		var avgConcurrency float64
		if total > 0 {
			avgConcurrency = float64(busy) / float64(total)
		}
		prev.emaConcurrency = ema(prev.emaConcurrency, avgConcurrency)

		// Track hourly invocation rates for predictive scaling
		hour := time.Now().Hour()
		if deltaInvocations > 0 {
			intervalSecs := a.interval.Seconds()
			ratePerSec := float64(deltaInvocations) / intervalSecs
			prev.hourlyRates[hour] = ema(prev.hourlyRates[hour], ratePerSec)
			prev.lastHourSlot = hour
		}

		var idlePct float64
		if total > 0 {
			idlePct = float64(idle) / float64(total) * 100
		}

		currentDesired := total
		if currentDesired < policy.MinReplicas {
			currentDesired = policy.MinReplicas
		}

		now := time.Now()

		// Check scale-up conditions
		scaleUp := false
		if policy.ScaleUpThresholds.QueueDepth > 0 && queueDepth > policy.ScaleUpThresholds.QueueDepth {
			scaleUp = true
		}
		if policy.ScaleUpThresholds.ColdStartPct > 0 && prev.emaColdStartPct > policy.ScaleUpThresholds.ColdStartPct {
			scaleUp = true
		}
		if policy.ScaleUpThresholds.AvgLatencyMs > 0 && int64(prev.emaLatencyMs) > policy.ScaleUpThresholds.AvgLatencyMs {
			scaleUp = true
		}
		if policy.ScaleUpThresholds.TargetConcurrency > 0 && prev.emaConcurrency > policy.ScaleUpThresholds.TargetConcurrency {
			scaleUp = true
		}

		// Check scale-down conditions
		scaleDown := false
		if policy.ScaleDownThresholds.IdlePct > 0 && idlePct > policy.ScaleDownThresholds.IdlePct && !scaleUp {
			scaleDown = true
		}

		cooldownUp := time.Duration(policy.CooldownScaleUpS) * time.Second
		if cooldownUp == 0 {
			cooldownUp = 15 * time.Second
		}
		cooldownDown := time.Duration(policy.CooldownScaleDownS) * time.Second
		if cooldownDown == 0 {
			cooldownDown = 60 * time.Second
		}

		newDesired := currentDesired
		if scaleUp && now.Sub(prev.lastScaleUp) >= cooldownUp {
			// Scale up: add 1 or proportional to queue depth
			increment := 1
			if queueDepth > 2 {
				increment = queueDepth / 2
			}
			newDesired = currentDesired + increment
			if newDesired > policy.MaxReplicas {
				newDesired = policy.MaxReplicas
			}
			if newDesired != currentDesired {
				prev.lastScaleUp = now
				logging.Op().Info("autoscaler: scale up",
					"function", fn.Name,
					"from", currentDesired,
					"to", newDesired,
					"queue_depth", queueDepth,
					"cold_start_pct", prev.emaColdStartPct,
					"avg_latency_ms", int64(prev.emaLatencyMs),
					"concurrency", prev.emaConcurrency)
				metrics.RecordAutoscaleDecision(fn.Name, "up")
			}
		} else if scaleDown && now.Sub(prev.lastScaleDown) >= cooldownDown {
			// Scale down: remove configurable step
			step := policy.ScaleDownStep
			if step <= 0 {
				step = 1
			}
			newDesired = currentDesired - step
			if newDesired < policy.MinReplicas {
				newDesired = policy.MinReplicas
			}
			if newDesired != currentDesired {
				prev.lastScaleDown = now
				logging.Op().Info("autoscaler: scale down",
					"function", fn.Name,
					"from", currentDesired,
					"to", newDesired,
					"step", step,
					"idle_pct", idlePct,
					"busy", busy)
				metrics.RecordAutoscaleDecision(fn.Name, "down")
			}
		}

		// Predictive scaling: check if next hour has historically higher load
		nextHour := (time.Now().Hour() + 1) % 24
		currentHour := time.Now().Hour()
		if prev.hourlyRates[nextHour] > 0 && prev.hourlyRates[currentHour] > 0 {
			ratio := prev.hourlyRates[nextHour] / prev.hourlyRates[currentHour]
			if ratio > 1.5 { // Next hour expects 50%+ more traffic
				predictedDesired := int(float64(newDesired) * ratio)
				if predictedDesired > newDesired && predictedDesired <= policy.MaxReplicas {
					logging.Op().Info("autoscaler: predictive pre-warm",
						"function", fn.Name,
						"current_desired", newDesired,
						"predicted_desired", predictedDesired,
						"next_hour_rate", prev.hourlyRates[nextHour],
						"current_hour_rate", prev.hourlyRates[currentHour])
					newDesired = predictedDesired
					metrics.RecordAutoscaleDecision(fn.Name, "predictive")
				}
			}
		}

		// Clamp to policy bounds
		if newDesired < policy.MinReplicas {
			newDesired = policy.MinReplicas
		}
		if newDesired > policy.MaxReplicas {
			newDesired = policy.MaxReplicas
		}

		a.pool.SetDesiredReplicas(funcID, newDesired)
		metrics.SetAutoscaleDesiredReplicas(fn.Name, newDesired)
	}
}

func (a *Autoscaler) getSnapshot(funcID string) *funcSnapshot {
	if v, ok := a.prevState.Load(funcID); ok {
		return v.(*funcSnapshot)
	}
	snap := &funcSnapshot{}
	actual, _ := a.prevState.LoadOrStore(funcID, snap)
	return actual.(*funcSnapshot)
}
