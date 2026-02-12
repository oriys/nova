package cost

import (
	"time"
)

// DefaultPricing provides reasonable defaults for serverless cost calculation.
// Prices are in abstract cost units (ACU) per unit of resource.
var DefaultPricing = Pricing{
	InvocationUnit: 0.0000002,  // per invocation
	DurationUnit:   0.00001667, // per GB-second
	ColdStartUnit:  0.000001,   // per cold start penalty
	IdleVMUnit:     0.000005,   // per idle VM per second
}

// Pricing holds cost rate configuration.
type Pricing struct {
	InvocationUnit float64 `json:"invocation_unit"` // Cost per invocation
	DurationUnit   float64 `json:"duration_unit"`   // Cost per GB-second
	ColdStartUnit  float64 `json:"cold_start_unit"` // Cost per cold start
	IdleVMUnit     float64 `json:"idle_vm_unit"`     // Cost per idle VM per second
}

// InvocationCost represents the cost breakdown for a single invocation.
type InvocationCost struct {
	InvocationCost float64 `json:"invocation_cost"` // Base invocation fee
	ComputeCost    float64 `json:"compute_cost"`    // Memory × duration
	ColdStartCost  float64 `json:"cold_start_cost"` // Cold start penalty (0 if warm)
	TotalCost      float64 `json:"total_cost"`
}

// FunctionCostSummary aggregates costs for a function over a period.
type FunctionCostSummary struct {
	FunctionID      string  `json:"function_id"`
	FunctionName    string  `json:"function_name"`
	TotalCost       float64 `json:"total_cost"`
	InvocationsCost float64 `json:"invocations_cost"`
	ComputeCost     float64 `json:"compute_cost"`
	ColdStartCost   float64 `json:"cold_start_cost"`
	Invocations     int64   `json:"invocations"`
	TotalDurationMs int64   `json:"total_duration_ms"`
	ColdStarts      int64   `json:"cold_starts"`
	AvgCost         float64 `json:"avg_cost"`
}

// TenantCostSummary aggregates costs for a tenant.
type TenantCostSummary struct {
	TenantID   string                 `json:"tenant_id"`
	TotalCost  float64                `json:"total_cost"`
	Functions  []*FunctionCostSummary `json:"functions"`
	PeriodFrom time.Time              `json:"period_from"`
	PeriodTo   time.Time              `json:"period_to"`
}

// Calculator computes invocation costs given a pricing model.
type Calculator struct {
	pricing Pricing
}

// NewCalculator creates a cost calculator with the given pricing.
func NewCalculator(pricing Pricing) *Calculator {
	return &Calculator{pricing: pricing}
}

// NewDefaultCalculator creates a cost calculator with default pricing.
func NewDefaultCalculator() *Calculator {
	return &Calculator{pricing: DefaultPricing}
}

// CalcInvocation calculates the cost of a single invocation.
func (c *Calculator) CalcInvocation(memoryMB int, durationMs int64, coldStart bool) InvocationCost {
	if memoryMB < 0 {
		memoryMB = 0
	}
	if durationMs < 0 {
		durationMs = 0
	}

	// Convert memory MB to GB
	memoryGB := float64(memoryMB) / 1024.0
	// Convert duration ms to seconds
	durationS := float64(durationMs) / 1000.0

	invCost := c.pricing.InvocationUnit
	computeCost := memoryGB * durationS * c.pricing.DurationUnit

	var coldStartCost float64
	if coldStart {
		coldStartCost = c.pricing.ColdStartUnit
	}

	return InvocationCost{
		InvocationCost: invCost,
		ComputeCost:    computeCost,
		ColdStartCost:  coldStartCost,
		TotalCost:      invCost + computeCost + coldStartCost,
	}
}

// GetPricing returns the current pricing configuration.
func (c *Calculator) GetPricing() Pricing {
	return c.pricing
}

// AggregateFunctionCost computes a cost summary from raw invocation data.
func AggregateFunctionCost(funcID, funcName string, invocations int64, totalDurationMs int64, coldStarts int64, memoryMB int, pricing Pricing) *FunctionCostSummary {
	calc := NewCalculator(pricing)

	// Compute cost for warm invocations
	warmCount := invocations - coldStarts
	avgDurationMs := int64(0)
	if invocations > 0 {
		avgDurationMs = totalDurationMs / invocations
	}

	var totalColdStartCost float64
	var totalInvocationCost float64
	var totalComputeCost float64

	if warmCount > 0 {
		warmCost := calc.CalcInvocation(memoryMB, avgDurationMs, false)
		totalInvocationCost += warmCost.InvocationCost * float64(warmCount)
		totalComputeCost += warmCost.ComputeCost * float64(warmCount)
	}

	if coldStarts > 0 {
		coldCost := calc.CalcInvocation(memoryMB, avgDurationMs, true)
		totalInvocationCost += coldCost.InvocationCost * float64(coldStarts)
		totalComputeCost += coldCost.ComputeCost * float64(coldStarts)
		totalColdStartCost = coldCost.ColdStartCost * float64(coldStarts)
	}

	total := totalInvocationCost + totalComputeCost + totalColdStartCost
	var avgCost float64
	if invocations > 0 {
		avgCost = total / float64(invocations)
	}

	return &FunctionCostSummary{
		FunctionID:      funcID,
		FunctionName:    funcName,
		TotalCost:       total,
		InvocationsCost: totalInvocationCost,
		ComputeCost:     totalComputeCost,
		ColdStartCost:   totalColdStartCost,
		Invocations:     invocations,
		TotalDurationMs: totalDurationMs,
		ColdStarts:      coldStarts,
		AvgCost:         avgCost,
	}
}
