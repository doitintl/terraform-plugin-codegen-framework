// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"errors"
	"strings"
	"testing"
)

func TestDedupeGenerated_FirstCall(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	got, err := DedupeGenerated("compute", generated, func() ([]byte, error) {
		return []byte("type ComputeValue struct{}"), nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if string(got) != "type ComputeValue struct{}" {
		t.Errorf("unexpected output: %q", got)
	}

	if string(generated["compute"]) != "type ComputeValue struct{}" {
		t.Errorf("declaration was not recorded: %q", generated["compute"])
	}
}

func TestDedupeGenerated_IdenticalRepeat(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	render := func() ([]byte, error) { return []byte("type MetricValue struct{}"), nil }

	if _, err := DedupeGenerated("metric", generated, render); err != nil {
		t.Fatalf("unexpected error on first call: %s", err)
	}

	got, err := DedupeGenerated("metric", generated, render)

	if err != nil {
		t.Fatalf("unexpected error on second call: %s", err)
	}

	// Emitting nothing is what makes legitimate sharing work: one declaration
	// serves every attribute with this name.
	if got != nil {
		t.Errorf("expected no output for an identical repeat, got %q", got)
	}

	if string(generated["metric"]) != "type MetricValue struct{}" {
		t.Errorf("recorded declaration was disturbed: %q", generated["metric"])
	}
}

func TestDedupeGenerated_DifferingRepeat(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	if _, err := DedupeGenerated("compute", generated, func() ([]byte, error) {
		return []byte("shape A"), nil
	}); err != nil {
		t.Fatalf("unexpected error on first call: %s", err)
	}

	got, err := DedupeGenerated("compute", generated, func() ([]byte, error) {
		return []byte("shape B"), nil
	})

	if err == nil {
		t.Fatal("expected an error for two distinct shapes sharing a name")
	}

	if got != nil {
		t.Errorf("expected no output alongside the error, got %q", got)
	}

	if !strings.Contains(err.Error(), `"compute"`) {
		t.Errorf("error should name the offending type, got: %s", err)
	}

	// The first declaration must survive so the caller can report and stop
	// rather than emit a half-built file.
	if string(generated["compute"]) != "shape A" {
		t.Errorf("first declaration was lost: %q", generated["compute"])
	}
}

// TestDedupeGenerated_RecursionGuard covers a self-referential subtree: the
// render function reaches the same name again while it is still being rendered.
func TestDedupeGenerated_RecursionGuard(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	calls := 0

	var render func() ([]byte, error)

	render = func() ([]byte, error) {
		calls++

		if calls > 10 {
			return nil, errors.New("runaway recursion")
		}

		inner, err := DedupeGenerated("node", generated, render)

		if err != nil {
			return nil, err
		}

		return append([]byte("type NodeValue struct{}"), inner...), nil
	}

	got, err := DedupeGenerated("node", generated, render)

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if calls != 1 {
		t.Errorf("expected the render function to run once, ran %d times", calls)
	}

	if string(got) != "type NodeValue struct{}" {
		t.Errorf("unexpected output: %q", got)
	}
}

// TestDedupeGenerated_RecursionGuardOnRepeat covers the same self-reference on a
// comparison re-render, where the sentinel has to be reinstated and the already
// recorded declaration restored afterwards.
func TestDedupeGenerated_RecursionGuardOnRepeat(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	calls := 0

	var render func() ([]byte, error)

	render = func() ([]byte, error) {
		calls++

		if calls > 10 {
			return nil, errors.New("runaway recursion")
		}

		inner, err := DedupeGenerated("node", generated, render)

		if err != nil {
			return nil, err
		}

		return append([]byte("type NodeValue struct{}"), inner...), nil
	}

	if _, err := DedupeGenerated("node", generated, render); err != nil {
		t.Fatalf("unexpected error on first call: %s", err)
	}

	got, err := DedupeGenerated("node", generated, render)

	if err != nil {
		t.Fatalf("unexpected error on second call: %s", err)
	}

	if got != nil {
		t.Errorf("expected no output for an identical repeat, got %q", got)
	}

	if calls != 2 {
		t.Errorf("expected exactly one re-render, render ran %d times total", calls)
	}

	if string(generated["node"]) != "type NodeValue struct{}" {
		t.Errorf("recorded declaration was not restored: %q", generated["node"])
	}
}

func TestDedupeGenerated_RenderError(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	sentinelErr := errors.New("boom")

	if _, err := DedupeGenerated("compute", generated, func() ([]byte, error) {
		return nil, sentinelErr
	}); !errors.Is(err, sentinelErr) {
		t.Fatalf("expected the render error to propagate, got: %v", err)
	}

	// The recursion sentinel must not be left behind: a later attempt would
	// otherwise be mistaken for an in-progress render and emit nothing.
	if _, ok := generated["compute"]; ok {
		t.Error("sentinel was left in the map after a failed render")
	}

	got, err := DedupeGenerated("compute", generated, func() ([]byte, error) {
		return []byte("type ComputeValue struct{}"), nil
	})

	if err != nil {
		t.Fatalf("unexpected error on retry: %s", err)
	}

	if string(got) != "type ComputeValue struct{}" {
		t.Errorf("unexpected output on retry: %q", got)
	}
}
