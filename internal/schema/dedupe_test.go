// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"errors"
	"strings"
	"testing"
)

// noChildren is the renderChildren argument for a node with only leaf children.
func noChildren() ([]byte, error) { return nil, nil }

func TestDedupeGenerated_FirstCall(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	got, err := DedupeGenerated("compute", generated, func() ([]byte, error) {
		return []byte("type ComputeValue struct{}"), nil
	}, func() ([]byte, error) {
		return []byte("type NestedValue struct{}"), nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// The caller gets the whole subtree.
	if string(got) != "type ComputeValue struct{}type NestedValue struct{}" {
		t.Errorf("unexpected output: %q", got)
	}

	// The map records the declaration alone, which is what later occurrences
	// are compared against.
	if string(generated["compute"]) != "type ComputeValue struct{}" {
		t.Errorf("declaration was not recorded: %q", generated["compute"])
	}
}

func TestDedupeGenerated_IdenticalRepeat(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	renderDecl := func() ([]byte, error) { return []byte("type MetricValue struct{}"), nil }

	childCalls := 0

	renderChildren := func() ([]byte, error) {
		childCalls++

		return nil, nil
	}

	if _, err := DedupeGenerated("metric", generated, renderDecl, renderChildren); err != nil {
		t.Fatalf("unexpected error on first call: %s", err)
	}

	got, err := DedupeGenerated("metric", generated, renderDecl, renderChildren)

	if err != nil {
		t.Fatalf("unexpected error on second call: %s", err)
	}

	// Emitting nothing is what makes legitimate sharing work: one declaration
	// serves every attribute with this name.
	if got != nil {
		t.Errorf("expected no output for an identical repeat, got %q", got)
	}

	// Once to emit, once to validate. The repeat has to walk the subtree even
	// though it emits nothing, because matching declarations do not prove that
	// the descendants match.
	if childCalls != 2 {
		t.Errorf("expected children to render twice, rendered %d times", childCalls)
	}

	if string(generated["metric"]) != "type MetricValue struct{}" {
		t.Errorf("recorded declaration was disturbed: %q", generated["metric"])
	}
}

// TestDedupeGenerated_SharedSubtreeRepeat is the regression test for
// https://github.com/doitintl/terraform-plugin-codegen-framework/issues/16.
//
// Two sibling attributes of identical shape share one name, and that shape has
// a nested child. The occurrence that renders first introduces the child and so
// carries the child's declaration; later occurrences do not, because the child
// dedupes away. Comparing subtree bytes therefore reported two provably
// identical shapes as a conflict. Comparing declarations does not.
func TestDedupeGenerated_SharedSubtreeRepeat(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	renderCredits := func() ([]byte, error) {
		return DedupeGenerated("credits", generated, func() ([]byte, error) {
			return []byte("type CreditsValue struct{ Cost CostValue }"), nil
		}, func() ([]byte, error) {
			return DedupeGenerated("cost", generated, func() ([]byte, error) {
				return []byte("type CostValue struct{}"), nil
			}, noChildren)
		})
	}

	first, err := renderCredits()

	if err != nil {
		t.Fatalf("unexpected error on first occurrence: %s", err)
	}

	if string(first) != "type CreditsValue struct{ Cost CostValue }type CostValue struct{}" {
		t.Errorf("unexpected output for first occurrence: %q", first)
	}

	second, err := renderCredits()

	if err != nil {
		t.Fatalf("identical shapes sharing a name must not conflict, got: %s", err)
	}

	if second != nil {
		t.Errorf("expected no output for an identical repeat, got %q", second)
	}
}

// TestDedupeGenerated_RepeatDivergesBelowDeclaration covers a conflict that the
// declaration comparison cannot see on its own: two occurrences of "x" declare
// identically, because a declaration names its child "p" without describing it,
// but their copies of "p" have different shapes.
//
// ResolveTypeNameConflicts separates this for attribute-rooted conflicts. It
// cannot for block-rooted ones (issue #17) or for a divergence below the
// fingerprint depth cap, so the walk has to reach "p" and let it report.
func TestDedupeGenerated_RepeatDivergesBelowDeclaration(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	renderX := func(pDecl string) ([]byte, error) {
		return DedupeGenerated("x", generated, func() ([]byte, error) {
			return []byte("type XValue struct{ P PValue }"), nil
		}, func() ([]byte, error) {
			return DedupeGenerated("p", generated, func() ([]byte, error) {
				return []byte(pDecl), nil
			}, noChildren)
		})
	}

	if _, err := renderX("type PValue struct{ Q string }"); err != nil {
		t.Fatalf("unexpected error on first occurrence: %s", err)
	}

	got, err := renderX("type PValue struct{ R string }")

	if err == nil {
		t.Fatal("expected an error for subtrees that diverge below the declaration")
	}

	if got != nil {
		t.Errorf("expected no output alongside the error, got %q", got)
	}

	// The error must name the descendant that actually differs, not its parent.
	if !strings.Contains(err.Error(), `"p"`) {
		t.Errorf("error should name the offending type, got: %s", err)
	}

	// The sentinel reinstated for the walk must be restored, not left behind.
	if string(generated["x"]) != "type XValue struct{ P PValue }" {
		t.Errorf("recorded declaration was not restored: %q", generated["x"])
	}
}

// TestDedupeGenerated_RepeatEmitsFirstRenderOfDescendant covers a descendant
// that the repeat walk is the first to reach. Its declaration gets recorded, so
// it has to be emitted too -- dropping it would leave the schema referencing a
// type that was never declared.
func TestDedupeGenerated_RepeatEmitsFirstRenderOfDescendant(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	// The first occurrence reaches "x" while "x" is still rendering, so the
	// sentinel stops it and "p" is never emitted.
	var renderX func() ([]byte, error)

	first := true

	renderX = func() ([]byte, error) {
		return DedupeGenerated("x", generated, func() ([]byte, error) {
			return []byte("type XValue struct{}"), nil
		}, func() ([]byte, error) {
			if first {
				first = false

				return renderX()
			}

			return DedupeGenerated("p", generated, func() ([]byte, error) {
				return []byte("type PValue struct{}"), nil
			}, noChildren)
		})
	}

	if _, err := renderX(); err != nil {
		t.Fatalf("unexpected error on first occurrence: %s", err)
	}

	got, err := renderX()

	if err != nil {
		t.Fatalf("unexpected error on repeat: %s", err)
	}

	if string(got) != "type PValue struct{}" {
		t.Errorf("a descendant first rendered during a repeat must still be emitted, got %q", got)
	}
}

func TestDedupeGenerated_DifferingRepeat(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	if _, err := DedupeGenerated("compute", generated, func() ([]byte, error) {
		return []byte("shape A"), nil
	}, noChildren); err != nil {
		t.Fatalf("unexpected error on first call: %s", err)
	}

	got, err := DedupeGenerated("compute", generated, func() ([]byte, error) {
		return []byte("shape B"), nil
	}, noChildren)

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
// child render reaches the same name again while it is still being rendered.
func TestDedupeGenerated_RecursionGuard(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	calls := 0

	var renderNode func() ([]byte, error)

	renderNode = func() ([]byte, error) {
		calls++

		if calls > 10 {
			return nil, errors.New("runaway recursion")
		}

		return DedupeGenerated("node", generated, func() ([]byte, error) {
			return []byte("type NodeValue struct{}"), nil
		}, renderNode)
	}

	got, err := renderNode()

	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// The nested reach hits the sentinel and stops, so the node is entered
	// twice in total and rendered once.
	if calls != 2 {
		t.Errorf("expected the node to be entered twice, entered %d times", calls)
	}

	if string(got) != "type NodeValue struct{}" {
		t.Errorf("unexpected output: %q", got)
	}
}

// TestDedupeGenerated_RecursionGuardOnRepeat covers the same self-reference
// reached through a comparison re-render of an already recorded name.
func TestDedupeGenerated_RecursionGuardOnRepeat(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	declCalls := 0

	var renderNode func() ([]byte, error)

	renderNode = func() ([]byte, error) {
		return DedupeGenerated("node", generated, func() ([]byte, error) {
			declCalls++

			if declCalls > 10 {
				return nil, errors.New("runaway recursion")
			}

			return []byte("type NodeValue struct{}"), nil
		}, renderNode)
	}

	if _, err := renderNode(); err != nil {
		t.Fatalf("unexpected error on first call: %s", err)
	}

	got, err := renderNode()

	if err != nil {
		t.Fatalf("unexpected error on second call: %s", err)
	}

	if got != nil {
		t.Errorf("expected no output for an identical repeat, got %q", got)
	}

	// Once for the first occurrence, once for the comparison re-render. The
	// repeat does not descend, so the self-reference is not reached again.
	if declCalls != 2 {
		t.Errorf("expected exactly one re-render, the declaration rendered %d times total", declCalls)
	}

	if string(generated["node"]) != "type NodeValue struct{}" {
		t.Errorf("recorded declaration was disturbed: %q", generated["node"])
	}
}

func TestDedupeGenerated_DeclRenderError(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	sentinelErr := errors.New("boom")

	if _, err := DedupeGenerated("compute", generated, func() ([]byte, error) {
		return nil, sentinelErr
	}, noChildren); !errors.Is(err, sentinelErr) {
		t.Fatalf("expected the render error to propagate, got: %v", err)
	}

	// The recursion sentinel must not be left behind: a later attempt would
	// otherwise be mistaken for an in-progress render and emit nothing.
	if _, ok := generated["compute"]; ok {
		t.Error("sentinel was left in the map after a failed render")
	}

	got, err := DedupeGenerated("compute", generated, func() ([]byte, error) {
		return []byte("type ComputeValue struct{}"), nil
	}, noChildren)

	if err != nil {
		t.Fatalf("unexpected error on retry: %s", err)
	}

	if string(got) != "type ComputeValue struct{}" {
		t.Errorf("unexpected output on retry: %q", got)
	}
}

func TestDedupeGenerated_ChildrenRenderError(t *testing.T) {
	t.Parallel()

	generated := make(map[string][]byte)

	sentinelErr := errors.New("boom")

	renderDecl := func() ([]byte, error) { return []byte("type ComputeValue struct{}"), nil }

	if _, err := DedupeGenerated("compute", generated, renderDecl, func() ([]byte, error) {
		return nil, sentinelErr
	}); !errors.Is(err, sentinelErr) {
		t.Fatalf("expected the render error to propagate, got: %v", err)
	}

	if _, ok := generated["compute"]; ok {
		t.Error("sentinel was left in the map after a failed render")
	}

	got, err := DedupeGenerated("compute", generated, renderDecl, noChildren)

	if err != nil {
		t.Fatalf("unexpected error on retry: %s", err)
	}

	if string(got) != "type ComputeValue struct{}" {
		t.Errorf("unexpected output on retry: %q", got)
	}
}
