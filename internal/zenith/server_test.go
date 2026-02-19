package zenith

import "testing"

func TestIsCometOnlyHTTPPath_StateRoute(t *testing.T) {
	t.Parallel()

	if !isCometOnlyHTTPPath("/functions/demo/state") {
		t.Fatalf("expected state route to be treated as comet-only path")
	}
}
