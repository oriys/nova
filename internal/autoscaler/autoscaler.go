package autoscaler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oriys/nova/internal/advisor"
	"github.com/oriys/nova/internal/checkpoint"
	"github.com/oriys/nova/internal/cluster"
	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/executor"
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
	lowLoadSince  time.Time
	// EMA-smoothed signals (alpha = 0.3 for ~3-tick smoothing)
	emaLatencyMs    float64
	emaColdStartPct float64
	emaConcurrency  float64 // for concurrency-based scaling
	emaRatePerSec   float64
	// Hourly invocation rate history (24 slots, one per hour)
	hourlyRates  [24]float64
	lastHourSlot int
}

// Autoscaler dynamically adjusts pool sizing based on load signals
type Autoscaler struct {
	pool            *pool.Pool
	store           *store.Store
	interval        time.Duration
	ctx             context.Context
	cancel          context.CancelFunc
	prevState       sync.Map // funcID -> *funcSnapshot
	predictor       *advisor.PerformanceAdvisor
	checkpointStore *checkpoint.Store
	clusterRouter   *cluster.Router
	localNodeID     string
}

// New creates a new Autoscaler
func New(p *pool.Pool, s *store.Store, interval time.Duration) *Autoscaler {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	localNodeID := strings.TrimSpace(os.Getenv("NOVA_CLUSTER_NODE_ID"))
	as := &Autoscaler{
		pool:            p,
		store:           s,
		interval:        interval,
		ctx:             ctx,
		cancel:          cancel,
		checkpointStore: checkpoint.NewStore(6 * time.Hour),
		localNodeID:     localNodeID,
	}
	if s != nil {
		as.predictor = &advisor.PerformanceAdvisor{Store: s}
	}
	if s != nil && localNodeID != "" {
		registry := cluster.NewRegistry(s, cluster.DefaultConfig(localNodeID))
		scheduler := cluster.NewScheduler(registry, cluster.StrategyLocalityAware)
		as.clusterRouter = cluster.NewRouter(registry, scheduler, cluster.NewProxy(3*time.Second), localNodeID)
	}
	return as
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
	funcs, err := a.store.ListFunctions(ctx, 0, 0)
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
		queueDepth := a.pool.QueueDepth(funcID)
		total, busy, idle := a.pool.FunctionPoolStats(funcID)

		prev := a.getSnapshot(funcID)
		fm := m.GetFunctionMetrics(funcID)

		var (
			coldStartRate    float64
			avgLatencyMs     int64
			deltaInvocations int64
			ratePerSec       float64
		)
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
				ratePerSec = float64(deltaInvocations) / a.interval.Seconds()
			}

			prev.invocations = curInvocations
			prev.coldStarts = curColdStarts
			prev.totalMs = curTotalMs
		}

		prev.emaLatencyMs = ema(prev.emaLatencyMs, float64(avgLatencyMs))
		prev.emaColdStartPct = ema(prev.emaColdStartPct, coldStartRate)
		if ratePerSec > 0 {
			prev.emaRatePerSec = ema(prev.emaRatePerSec, ratePerSec)
		}

		var avgConcurrency float64
		if total > 0 {
			avgConcurrency = float64(busy) / float64(total)
		}
		prev.emaConcurrency = ema(prev.emaConcurrency, avgConcurrency)

		hour := time.Now().Hour()
		if ratePerSec > 0 {
			prev.hourlyRates[hour] = ema(prev.hourlyRates[hour], ratePerSec)
			prev.lastHourSlot = hour
		}

		var idlePct float64
		if total > 0 {
			idlePct = float64(idle) / float64(total) * 100
		}

		estimatedQueueWaitMs := a.pool.FunctionQueueWaitMs(funcID)
		if queueDepth > 0 && prev.emaLatencyMs > 0 {
			workers := max(total, 1)
			heuristicWaitMs := int64(float64(queueDepth) * prev.emaLatencyMs / float64(workers))
			if heuristicWaitMs > estimatedQueueWaitMs {
				estimatedQueueWaitMs = heuristicWaitMs
			}
		}

		currentDesired := max(total, policy.MinReplicas)

		targetUtilization := policy.TargetUtilization
		if targetUtilization <= 0 || targetUtilization > 1 {
			targetUtilization = 0.7
		}
		minSamples := policy.MinSampleCount
		if minSamples <= 0 {
			minSamples = 3
		}
		instanceConcurrency := fn.InstanceConcurrency
		if instanceConcurrency <= 0 {
			instanceConcurrency = 1
		}

		desiredByLoad := currentDesired
		if deltaInvocations >= int64(minSamples) && prev.emaRatePerSec > 0 && prev.emaLatencyMs > 0 {
			serviceTimeSec := prev.emaLatencyMs / 1000.0
			if serviceTimeSec < 0.001 {
				serviceTimeSec = 0.001
			}
			capacityPerReplica := float64(instanceConcurrency) * targetUtilization
			if capacityPerReplica < 0.01 {
				capacityPerReplica = 0.01
			}
			loadReplicas := int((prev.emaRatePerSec*serviceTimeSec)/capacityPerReplica + 0.999)
			if loadReplicas > desiredByLoad {
				desiredByLoad = loadReplicas
			}
		}

		maxReplicas := policy.MaxReplicas
		if maxReplicas <= 0 && fn.MaxReplicas > 0 {
			maxReplicas = fn.MaxReplicas
		}

		scaleUp := desiredByLoad > currentDesired
		if policy.ScaleUpThresholds.QueueDepth > 0 && queueDepth > policy.ScaleUpThresholds.QueueDepth {
			scaleUp = true
		}
		if policy.ScaleUpThresholds.QueueWaitMs > 0 && estimatedQueueWaitMs > policy.ScaleUpThresholds.QueueWaitMs {
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

		scaleDown := false
		if !scaleUp {
			if policy.ScaleDownThresholds.IdlePct > 0 && idlePct > policy.ScaleDownThresholds.IdlePct {
				scaleDown = true
			}
			if queueDepth == 0 && prev.emaConcurrency < targetUtilization*0.5 {
				scaleDown = true
			}
		}

		now := time.Now()
		if scaleDown {
			if prev.lowLoadSince.IsZero() {
				prev.lowLoadSince = now
			}
		} else {
			prev.lowLoadSince = time.Time{}
		}

		cooldownUp := time.Duration(policy.CooldownScaleUpS) * time.Second
		if cooldownUp == 0 {
			cooldownUp = 15 * time.Second
		}
		cooldownDown := time.Duration(policy.CooldownScaleDownS) * time.Second
		if cooldownDown == 0 {
			cooldownDown = 60 * time.Second
		}
		scaleDownStabilization := time.Duration(policy.ScaleDownStabilizationS) * time.Second
		if scaleDownStabilization == 0 {
			scaleDownStabilization = 90 * time.Second
		}

		newDesired := currentDesired
		if scaleUp && now.Sub(prev.lastScaleUp) >= cooldownUp {
			stepMax := policy.ScaleUpStepMax
			if stepMax <= 0 {
				stepMax = 4
			}
			increment := 1
			if queueDepth > 0 {
				increment = max(increment, min(stepMax, max(1, queueDepth/2)))
			}
			if desiredByLoad > currentDesired {
				increment = max(increment, min(stepMax, desiredByLoad-currentDesired))
			}

			newDesired = currentDesired + increment
			if desiredByLoad > newDesired {
				newDesired = desiredByLoad
			}
			if maxReplicas > 0 && newDesired > maxReplicas {
				newDesired = maxReplicas
			}

			if newDesired != currentDesired {
				prev.lastScaleUp = now
				prev.lowLoadSince = time.Time{}
				logging.Op().Info("autoscaler: scale up",
					"function", fn.Name,
					"from", currentDesired,
					"to", newDesired,
					"queue_depth", queueDepth,
					"queue_wait_ms", estimatedQueueWaitMs,
					"cold_start_pct", prev.emaColdStartPct,
					"avg_latency_ms", int64(prev.emaLatencyMs),
					"concurrency", prev.emaConcurrency,
					"ema_rate_per_sec", prev.emaRatePerSec)
				metrics.RecordAutoscaleDecision(fn.Name, "up")
			}
		} else if scaleDown &&
			!prev.lowLoadSince.IsZero() &&
			now.Sub(prev.lowLoadSince) >= scaleDownStabilization &&
			now.Sub(prev.lastScaleDown) >= cooldownDown {
			step := policy.ScaleDownStep
			if step <= 0 {
				step = 1
			}
			floor := policy.MinReplicas
			if desiredByLoad > floor {
				floor = desiredByLoad
			}
			newDesired = currentDesired - step
			if newDesired < floor {
				newDesired = floor
			}

			if newDesired != currentDesired {
				prev.lastScaleDown = now
				logging.Op().Info("autoscaler: scale down",
					"function", fn.Name,
					"from", currentDesired,
					"to", newDesired,
					"step", step,
					"idle_pct", idlePct,
					"busy", busy,
					"ema_rate_per_sec", prev.emaRatePerSec)
				metrics.RecordAutoscaleDecision(fn.Name, "down")
			}
		}

		snapshotAvailable := false
		if snapshotDir := strings.TrimSpace(a.pool.SnapshotDir()); snapshotDir != "" {
			snapshotAvailable = executor.HasSnapshot(snapshotDir, funcID)
		}

		// Predictive scaling #1: hourly seasonality.
		nextHour := (time.Now().Hour() + 1) % 24
		currentHour := time.Now().Hour()
		if prev.hourlyRates[nextHour] > 0 && prev.hourlyRates[currentHour] > 0 {
			ratio := prev.hourlyRates[nextHour] / prev.hourlyRates[currentHour]
			if ratio > 1.5 {
				predictedDesired := int(float64(newDesired) * ratio)
				if maxReplicas > 0 && predictedDesired > maxReplicas {
					predictedDesired = maxReplicas
				}
				if predictedDesired > newDesired {
					targetNodeID := a.resolveLocalNodeID()
					remotePrewarm := false
					if a.clusterRouter != nil {
						if nodeID, routed, routeErr := a.clusterRouter.TryRoutePrewarm(ctx, funcID, fn.Name, predictedDesired); routeErr != nil {
							logging.Op().Warn("autoscaler: remote predictive prewarm failed",
								"function", fn.Name,
								"error", routeErr)
						} else if routed {
							remotePrewarm = true
							targetNodeID = nodeID
						}
					}
					if !remotePrewarm {
						if err := a.prewarmLocal(ctx, fn, predictedDesired); err != nil {
							logging.Op().Warn("autoscaler: local predictive prewarm failed", "function", fn.Name, "error", err)
						}
					}

					logging.Op().Info("autoscaler: predictive pre-warm",
						"function", fn.Name,
						"current_desired", newDesired,
						"predicted_desired", predictedDesired,
						"next_hour_rate", prev.hourlyRates[nextHour],
						"current_hour_rate", prev.hourlyRates[currentHour],
						"snapshot_available", snapshotAvailable,
						"target_node", targetNodeID,
						"remote", remotePrewarm)
					newDesired = predictedDesired
					metrics.RecordAutoscaleDecision(fn.Name, "predictive")
					a.recordPredictiveCheckpoint(funcID, "hourly-seasonality", targetNodeID, nil, predictedDesired, snapshotAvailable, remotePrewarm)
				}
			}
		}

		// Predictive scaling #2: advisor near-term forecast.
		if a.predictor != nil {
			prediction, predErr := a.predictor.PredictTraffic(ctx, funcID, 7)
			if predErr != nil {
				logging.Op().Warn("autoscaler: advisor prediction failed", "function", fn.Name, "error", predErr)
			} else if prediction != nil && prediction.Confidence >= 0.6 {
				advisorDesired := estimateDesiredReplicas(
					prediction.PredictedRatePerSec,
					prev.emaLatencyMs,
					targetUtilization,
					instanceConcurrency,
					policy.MinReplicas,
				)
				if !snapshotAvailable && advisorDesired > newDesired+2 {
					advisorDesired = newDesired + 2
				}
				if maxReplicas > 0 && advisorDesired > maxReplicas {
					advisorDesired = maxReplicas
				}

				if advisorDesired > newDesired {
					targetNodeID := a.resolveLocalNodeID()
					remotePrewarm := false
					if a.clusterRouter != nil {
						if nodeID, routed, routeErr := a.clusterRouter.TryRoutePrewarm(ctx, funcID, fn.Name, advisorDesired); routeErr != nil {
							logging.Op().Warn("autoscaler: advisor remote prewarm failed",
								"function", fn.Name,
								"error", routeErr)
						} else if routed {
							remotePrewarm = true
							targetNodeID = nodeID
						}
					}
					if !remotePrewarm {
						if err := a.prewarmLocal(ctx, fn, advisorDesired); err != nil {
							logging.Op().Warn("autoscaler: advisor local prewarm failed", "function", fn.Name, "error", err)
						}
					}

					logging.Op().Info("autoscaler: advisor predictive pre-warm",
						"function", fn.Name,
						"from", newDesired,
						"to", advisorDesired,
						"predicted_rate_per_sec", prediction.PredictedRatePerSec,
						"confidence", prediction.Confidence,
						"snapshot_available", snapshotAvailable,
						"target_node", targetNodeID,
						"remote", remotePrewarm)
					newDesired = advisorDesired
					metrics.RecordAutoscaleDecision(fn.Name, "advisor_predictive")
					a.recordPredictiveCheckpoint(funcID, "advisor-predictive", targetNodeID, prediction, advisorDesired, snapshotAvailable, remotePrewarm)
				}
			}
		}

		if newDesired < policy.MinReplicas {
			newDesired = policy.MinReplicas
		}
		if maxReplicas > 0 && newDesired > maxReplicas {
			newDesired = maxReplicas
		}
		if newDesired < 0 {
			newDesired = 0
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

func (a *Autoscaler) resolveLocalNodeID() string {
	if a.localNodeID != "" {
		return a.localNodeID
	}
	return "local"
}

func (a *Autoscaler) recordPredictiveCheckpoint(
	functionID string,
	reason string,
	targetNodeID string,
	prediction *advisor.TrafficPrediction,
	targetReplicas int,
	snapshotAvailable bool,
	remote bool,
) {
	if a.checkpointStore == nil || functionID == "" {
		return
	}

	payload := map[string]interface{}{
		"reason":             reason,
		"target_node_id":     targetNodeID,
		"target_replicas":    targetReplicas,
		"snapshot_available": snapshotAvailable,
		"remote_dispatch":    remote,
		"recorded_at":        time.Now().UTC().Format(time.RFC3339),
	}
	if prediction != nil {
		payload["current_rate_per_sec"] = prediction.CurrentRatePerSec
		payload["predicted_rate_per_sec"] = prediction.PredictedRatePerSec
		payload["confidence"] = prediction.Confidence
		payload["lookback_days"] = prediction.LookbackDays
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		logging.Op().Warn("autoscaler: marshal predictive checkpoint failed", "function_id", functionID, "error", err)
		return
	}

	a.checkpointStore.Save("predictive:"+functionID, functionID, "predictive_prewarm", raw)
}

func (a *Autoscaler) prewarmLocal(ctx context.Context, fn *domain.Function, targetReplicas int) error {
	if a.pool == nil || a.store == nil || fn == nil {
		return nil
	}

	codeRecord, err := a.store.GetFunctionCode(ctx, fn.ID)
	if err != nil {
		return err
	}
	if codeRecord == nil {
		return fmt.Errorf("function code not found: %s", fn.Name)
	}

	codeContent := codeRecord.CompiledBinary
	if len(codeContent) == 0 {
		codeContent = []byte(codeRecord.SourceCode)
	}

	a.pool.SetDesiredReplicas(fn.ID, targetReplicas)
	return a.pool.EnsureReady(ctx, fn, codeContent)
}

func estimateDesiredReplicas(ratePerSec, emaLatencyMs, targetUtilization float64, instanceConcurrency, minReplicas int) int {
	if ratePerSec <= 0 {
		return minReplicas
	}

	serviceTimeSec := emaLatencyMs / 1000.0
	if serviceTimeSec <= 0 {
		serviceTimeSec = 0.05
	}
	if targetUtilization <= 0 || targetUtilization > 1 {
		targetUtilization = 0.7
	}
	if instanceConcurrency <= 0 {
		instanceConcurrency = 1
	}

	capacityPerReplica := float64(instanceConcurrency) * targetUtilization
	if capacityPerReplica < 0.01 {
		capacityPerReplica = 0.01
	}

	desired := int((ratePerSec*serviceTimeSec)/capacityPerReplica + 0.999)
	if desired < minReplicas {
		desired = minReplicas
	}
	return desired
}
