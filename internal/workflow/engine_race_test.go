package workflow

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/oriys/nova/internal/domain"
	"github.com/oriys/nova/internal/store"
	"github.com/stretchr/testify/require"
)

// TestRaceCondition_MultiplePredsFinishSimultaneously
// Demonstrates the race condition when nodes A and B both finish at the same time,
// and node C depends on both.
//
// Scenario:
//   A --\
//        --> C
//   B --/
//
// Race condition:
// 1. Thread1 (A): succeedNode(A) -> advanceDAG(A) -> propagateInput(A)
// 2. Thread2 (B): succeedNode(B) -> advanceDAG(B) -> propagateInput(B)
//
// Problem: Both threads may call propagateInput and try to set C's input.
// Additionally, propagateInput(B) may fetch run nodes before B's own status
// is committed, causing B's output to be missing from the merged input.
func TestRaceCondition_MultiplePredsFinishSimultaneously(t *testing.T) {
	// This test is conceptual - it requires:
	// 1. A test-friendly store implementation
	// 2. Ability to inject delays/barriers
	// 3. Ability to verify concurrent behavior

	/*
	ctx := context.Background()
	mockStore := setupMockStore()

	// Create workflow with diamond dependency
	workflow := setupWorkflowWithDiamondDeps()
	run := setupRun(workflow)

	// Setup run nodes: A, B, C (C depends on A and B)
	// A.deps = 0 (ready)
	// B.deps = 0 (ready)
	// C.deps = 2 (pending)

	var wg sync.WaitGroup
	errors := make(chan error, 2)

	// Simulate A and B finishing simultaneously
	wg.Add(2)

	go func() {
		defer wg.Done()
		// Simulate A finishing
		nodeA := getRunNode(run, "A")
		nodeA.Status = domain.NodeStatusSucceeded
		nodeA.Output = json.RawMessage(`{"a_result": "data_from_A"}`)

		engine := NewEngine(mockStore, nil)
		engine.succeedNode(ctx, nodeA, &domain.InvokeResponse{
			Output: json.RawMessage(`{"a_result": "data_from_A"}`),
		})
	}()

	go func() {
		defer wg.Done()
		// Simulate B finishing
		nodeB := getRunNode(run, "B")
		nodeB.Status = domain.NodeStatusSucceeded
		nodeB.Output = json.RawMessage(`{"b_result": "data_from_B"}`)

		engine := NewEngine(mockStore, nil)
		engine.succeedNode(ctx, nodeB, &domain.InvokeResponse{
			Output: json.RawMessage(`{"b_result": "data_from_B"}`),
		})
	}()

	wg.Wait()

	// Check that C has both A's and B's outputs
	nodeC := getRunNode(run, "C")
	var input map[string]interface{}
	json.Unmarshal(nodeC.Input, &input)

	// Bug: B's output might be missing if:
	// - Thread B calls propagateInput before its own UpdateRunNode is committed
	// - GetRunNodes in Thread B sees B.Status as Running instead of Succeeded
	// - B's output is excluded from the merged map
	require.Contains(t, input, "A", "Output from A should be in C's input")
	require.Contains(t, input, "B", "Output from B should be in C's input (BUG: might be missing)")
	*/

	// This test is a placeholder to document the race condition.
	// Real reproduction requires:
	// - A controllable Store implementation
	// - Ability to insert delays/barriers between transactions
	// - Thread synchronization hooks

	t.Skip("Race condition test requires mock store with transaction control")
}

// TestPropagateInputMissingPredecessor
// Demonstrates the bug where propagateInput can miss a predecessor's output
// if the predecessor's status is not yet committed to the database.
//
// This happens when:
// 1. B completes and calls succeedNode(B)
// 2. succeedNode(B) commits B.Status = Succeeded
// 3. advanceDAG(B) calls DecrementDeps, which makes C ready
// 4. propagateInput(B) is called
// 5. propagateInput(B) calls GetRunNodes
// 6. But the GetRunNodes call might see B.Status as something other than Succeeded
//    if there's a lag in the database or transaction isolation issues
// 7. B's output is excluded from the merged input for C
func TestPropagateInputMissingPredecessor(t *testing.T) {
	/*
	Conceptual test showing the race:

	Time T0: A completes
	  - UpdateRunNode(A): A.Status = Succeeded, A.Output = {...}
	  - DecrementDeps(C): C.deps = 2 -> 1 (still Pending)
	  - propagateInput(A): C.Status = Pending, SKIP

	Time T1: B completes
	  - UpdateRunNode(B): B.Status = Succeeded, B.Output = {...}
	  - (assume this tx not yet flushed to disk)
	  - DecrementDeps(C): C.deps = 1 -> 0, C.Status = Ready
	  - propagateInput(B):
	    - GetRunNodes(runID): sees A.Status=Succeeded, B.Status=??? (uncommitted read issue)
	    - For each pred in [A, B]:
	      - predNode = rnByKey[pred]
	      - if predNode.Status == Succeeded: merged[pred] = predNode.Output
	    - If B is not seen as Succeeded, B is EXCLUDED from merged map
	    - C.Input = {A: {...}} (MISSING B's output)

	Result: C only has A's output, B's output is lost!
	*/

	t.Skip("Race condition test requires transaction isolation control")
}

// TestMultipleCallsToUpdateRunNode
// Demonstrates that if propagateInput is called multiple times for the same node,
// blind writes can cause data loss.
//
// Scenario:
// 1. Thread 1 (A): propagateInput(A) builds merged input for C
// 2. Thread 2 (B): propagateInput(B) builds merged input for C
// 3. Both threads call UpdateRunNode(C) with different input values
// 4. Last write wins, earlier write is lost (if they contain different data)
//
// In practice, if both calculate the same merged input, it's OK (idempotent).
// But the code structure suggests this could happen:
//
//   for _, edge := range edges {
//       if edge.FromNodeID != completed.NodeID { continue }
//       succKey := nodeIDToKey[edge.ToNodeID]
//       succNode := rnByKey[succKey]
//       if succNode == nil || succNode.Status != domain.NodeStatusReady {
//           continue
//       }
//       // Both Thread1 (A) and Thread2 (B) reach here if:
//       // - edge.FromNodeID = A and edge.To = C
//       // - edge.FromNodeID = B and edge.To = C
//       e.store.UpdateRunNode(ctx, succNode)
//   }
func TestMultipleCallsToUpdateRunNode(t *testing.T) {
	t.Skip("Race condition test requires concurrent store mock")
}

// Helper function to show the expected behavior
func expectCNodeInput(t *testing.T, nodeC *domain.RunNode, expectA bool, expectB bool) {
	var input map[string]json.RawMessage
	err := json.Unmarshal(nodeC.Input, &input)
	require.NoError(t, err)

	if expectA {
		require.Contains(t, input, "A", "A output should be in C input")
	}
	if expectB {
		require.Contains(t, input, "B", "B output should be in C input")
	}
}

// Test showing the correct behavior (non-racy scenario)
func TestNormalFlow_SequentialPredecessors(t *testing.T) {
	// When A and B finish sequentially (not concurrently):
	// 1. A finishes: succeedNode(A) -> advanceDAG(A) -> propagateInput(A)
	//    C.deps = 2 -> 1, stays Pending, skip
	// 2. B finishes: succeedNode(B) -> advanceDAG(B) -> propagateInput(B)
	//    C.deps = 1 -> 0, C becomes Ready
	//    propagateInput(B) fetches all nodes, sees A=Succeeded, B=Succeeded
	//    Merges both outputs correctly
	//    Result: C.Input = {A: {...}, B: {...}} ✓

	t.Skip("Need real store to run this test")
}

// DocumentedBug captures the exact race condition
type DocumentedBug struct {
	Name        string
	Scenario    string
	RootCause   string
	Impact      string
	FixRequired bool
}

var knownRaceConditions = []DocumentedBug{
	{
		Name: "Missing predecessor output in propagateInput",
		Scenario: `
Node C depends on A and B.
Both A and B finish at nearly the same time.
B's succeedNode() and advanceDAG() -> propagateInput(B) run closely together.
When propagateInput(B) calls GetRunNodes(), the database transaction
sees B's status as something other than Succeeded (uncommitted read).
B's output is excluded from C's merged input.
Result: C.Input = {A: {...}} but missing B's output.
		`,
		RootCause: `
propagateInput() calls GetRunNodes() to fetch fresh node data,
but this can see an intermediate state where the current node's own
status hasn't been fully committed/flushed.
The code then checks: if predNode.Status == Succeeded
If the current node doesn't see itself as Succeeded, it won't include its own output.
		`,
		Impact: `
When a node has multiple predecessors and they complete simultaneously,
the successor node might receive incomplete input, missing one or more
predecessor's output. This could cause:
- Silent data loss (inputs not propagated)
- Downstream nodes processing incomplete data
- Workflow producing incorrect results
		`,
		FixRequired: true,
	},
	{
		Name: "Multiple threads calling UpdateRunNode concurrently",
		Scenario: `
If DecrementDeps makes a node Ready, both its predecessors might
call propagateInput for the same node:
- Thread A: propagateInput(A) -> sees C is Ready -> UpdateRunNode(C, ...)
- Thread B: propagateInput(B) -> sees C is Ready -> UpdateRunNode(C, ...)
Both write to the same node. Last write wins (blind write).
		`,
		RootCause: `
The propagateInput() function iterates edges looking only at successors
of the completed node:
  if edge.FromNodeID != completed.NodeID { continue }
If C has two predecessors, both will reach UpdateRunNode() call.
There's no locking or version check to prevent concurrent writes.
		`,
		Impact: `
If both threads calculate the same merged input, it's idempotent (no harm).
But if there's any difference in the data they write (due to timing),
the earlier write is lost. This could corrupt the node's state.
		`,
		FixRequired: true,
	},
}

func TestDocumentBug_PrintRaceConditionDetails(t *testing.T) {
	for _, bug := range knownRaceConditions {
		t.Logf("BUG: %s\n", bug.Name)
		t.Logf("Scenario:\n%s\n", bug.Scenario)
		t.Logf("Root Cause:\n%s\n", bug.RootCause)
		t.Logf("Impact:\n%s\n", bug.Impact)
		t.Logf("Fix Required: %v\n\n", bug.FixRequired)
	}
}
