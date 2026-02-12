package cost

import (
	"math"
	"testing"
)

func TestCalcInvocationWarmStart(t *testing.T) {
	calc := NewDefaultCalculator()

	result := calc.CalcInvocation(128, 500, false)

	if result.InvocationCost != DefaultPricing.InvocationUnit {
		t.Errorf("expected invocation cost %v, got %v", DefaultPricing.InvocationUnit, result.InvocationCost)
	}
	if result.ColdStartCost != 0 {
		t.Errorf("expected zero cold start cost for warm start, got %v", result.ColdStartCost)
	}
	if result.TotalCost <= 0 {
		t.Error("expected positive total cost")
	}
	if result.TotalCost != result.InvocationCost+result.ComputeCost {
		t.Error("total cost should equal invocation + compute for warm starts")
	}
}

func TestCalcInvocationColdStart(t *testing.T) {
	calc := NewDefaultCalculator()

	result := calc.CalcInvocation(256, 1000, true)

	if result.ColdStartCost != DefaultPricing.ColdStartUnit {
		t.Errorf("expected cold start cost %v, got %v", DefaultPricing.ColdStartUnit, result.ColdStartCost)
	}
	if result.TotalCost != result.InvocationCost+result.ComputeCost+result.ColdStartCost {
		t.Error("total cost should equal invocation + compute + cold start")
	}
}

func TestCalcInvocationScalesWithMemory(t *testing.T) {
	calc := NewDefaultCalculator()

	small := calc.CalcInvocation(128, 1000, false)
	large := calc.CalcInvocation(1024, 1000, false)

	if large.ComputeCost <= small.ComputeCost {
		t.Error("higher memory should result in higher compute cost")
	}

	ratio := large.ComputeCost / small.ComputeCost
	expected := 1024.0 / 128.0
	if math.Abs(ratio-expected) > 0.01 {
		t.Errorf("compute cost should scale linearly with memory, got ratio %v, expected %v", ratio, expected)
	}
}

func TestCalcInvocationScalesWithDuration(t *testing.T) {
	calc := NewDefaultCalculator()

	short := calc.CalcInvocation(128, 100, false)
	long := calc.CalcInvocation(128, 1000, false)

	if long.ComputeCost <= short.ComputeCost {
		t.Error("longer duration should result in higher compute cost")
	}
}

func TestAggregateFunctionCost(t *testing.T) {
	summary := AggregateFunctionCost(
		"func-1", "test-function",
		100, 50000, 10, 256,
		DefaultPricing,
	)

	if summary.FunctionID != "func-1" {
		t.Error("unexpected function ID")
	}
	if summary.FunctionName != "test-function" {
		t.Error("unexpected function name")
	}
	if summary.Invocations != 100 {
		t.Errorf("expected 100 invocations, got %d", summary.Invocations)
	}
	if summary.ColdStarts != 10 {
		t.Errorf("expected 10 cold starts, got %d", summary.ColdStarts)
	}
	if summary.TotalCost <= 0 {
		t.Error("expected positive total cost")
	}
	if summary.ColdStartCost <= 0 {
		t.Error("expected positive cold start cost")
	}
	if summary.AvgCost <= 0 {
		t.Error("expected positive average cost")
	}
	if math.Abs(summary.AvgCost-summary.TotalCost/100) > 1e-10 {
		t.Error("average cost should equal total / invocations")
	}
}

func TestAggregateFunctionCostZeroInvocations(t *testing.T) {
	summary := AggregateFunctionCost(
		"func-1", "test-function",
		0, 0, 0, 256,
		DefaultPricing,
	)

	if summary.TotalCost != 0 {
		t.Errorf("expected zero total cost, got %v", summary.TotalCost)
	}
	if summary.AvgCost != 0 {
		t.Errorf("expected zero avg cost, got %v", summary.AvgCost)
	}
}

func TestCustomPricing(t *testing.T) {
	pricing := Pricing{
		InvocationUnit: 0.001,
		DurationUnit:   0.01,
		ColdStartUnit:  0.005,
		IdleVMUnit:     0.0001,
	}
	calc := NewCalculator(pricing)

	result := calc.CalcInvocation(1024, 1000, true)

	if result.InvocationCost != 0.001 {
		t.Errorf("expected invocation cost 0.001, got %v", result.InvocationCost)
	}
	if result.ColdStartCost != 0.005 {
		t.Errorf("expected cold start cost 0.005, got %v", result.ColdStartCost)
	}

	got := calc.GetPricing()
	if got.InvocationUnit != pricing.InvocationUnit {
		t.Error("GetPricing should return configured pricing")
	}
}
