// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/doitintl/terraform-plugin-codegen-framework/internal/model"
)

// fakeNested is a minimal stand-in for a nested attribute generator. The real
// generators live in internal/{datasource,provider,resource}, which import this
// package, so they cannot be used here.
type fakeNested struct {
	attrs    GeneratorAttributes
	typeName string
}

func (f fakeNested) GetAttributes() GeneratorAttributes { return f.attrs }

func (f fakeNested) EffectiveTypeName() string { return f.typeName }

func (f fakeNested) WithResolvedTypeName(name string) GeneratorAttribute {
	f.typeName = name
	return f
}

func (f fakeNested) Equal(GeneratorAttribute) bool { return false }

func (f fakeNested) GeneratorSchemaType() Type { return GeneratorSingleNestedAttribute }

func (f fakeNested) Imports() *Imports { return NewImports() }

func (f fakeNested) ModelField(FrameworkIdentifier) (model.Field, error) {
	return model.Field{}, nil
}

func (f fakeNested) Schema(FrameworkIdentifier) (string, error) { return "", nil }

// fakeString and fakeBool are distinct leaf types so that fingerprinting, which
// keys partly off the Go type of each attribute, has something to tell apart.
type fakeString struct{}

func (f fakeString) Equal(GeneratorAttribute) bool { return false }

func (f fakeString) GeneratorSchemaType() Type { return GeneratorStringAttribute }

func (f fakeString) Imports() *Imports { return NewImports() }

func (f fakeString) ModelField(FrameworkIdentifier) (model.Field, error) {
	return model.Field{}, nil
}

func (f fakeString) Schema(FrameworkIdentifier) (string, error) { return "", nil }

type fakeBool struct{}

func (f fakeBool) Equal(GeneratorAttribute) bool { return false }

func (f fakeBool) GeneratorSchemaType() Type { return GeneratorBoolAttribute }

func (f fakeBool) Imports() *Imports { return NewImports() }

func (f fakeBool) ModelField(FrameworkIdentifier) (model.Field, error) {
	return model.Field{}, nil
}

func (f fakeBool) Schema(FrameworkIdentifier) (string, error) { return "", nil }

// nest builds a nested attribute holding the given children.
func nest(attrs GeneratorAttributes) GeneratorAttribute {
	return fakeNested{attrs: attrs}
}

// leaves builds a set of string leaf attributes with the given keys.
func leaves(keys ...string) GeneratorAttributes {
	attrs := make(GeneratorAttributes, len(keys))
	for _, k := range keys {
		attrs[k] = fakeString{}
	}
	return attrs
}

// typeNames walks the resolved tree and returns a map of dotted attribute path
// to the type name that will be generated for it, applying the same bare-name
// fallback as CustomTypeValueBytes.
func typeNames(attrs GeneratorAttributes, prefix string, out map[string]string) {
	for _, k := range attrs.SortedKeys() {
		nested, ok := attrs[k].(Attributes)
		if !ok {
			continue
		}

		path := k
		if prefix != "" {
			path = prefix + "." + k
		}

		name := k
		if t, ok := attrs[k].(EffectiveTypeName); ok {
			if tn := t.EffectiveTypeName(); tn != "" {
				name = tn
			}
		}

		out[path] = name

		typeNames(nested.GetAttributes(), path, out)
	}
}

func resolveAndCollect(attrs GeneratorAttributes) map[string]string {
	g := GeneratorSchema{Attributes: attrs}
	g.ResolveTypeNameConflicts()

	out := make(map[string]string)
	typeNames(g.Attributes, "", out)

	return out
}

func TestGeneratorSchema_ResolveTypeNameConflicts(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		attributes GeneratorAttributes
		expected   map[string]string
	}{
		// The PS4C case: one product-line key reused by several siblings, each
		// with a different shape. Before grouping replaced the depth rule these
		// were all at the same depth, so none of them were renamed.
		"same depth, different shapes": {
			attributes: GeneratorAttributes{
				"onboarding_status": nest(GeneratorAttributes{"compute": nest(leaves("status"))}),
				"savings_totals":    nest(GeneratorAttributes{"compute": nest(leaves("lifetime", "ytd"))}),
				"stats30d":          nest(GeneratorAttributes{"compute": nest(leaves("savings"))}),
			},
			expected: map[string]string{
				"onboarding_status":         "onboarding_status",
				"onboarding_status.compute": "compute",
				"savings_totals":            "savings_totals",
				"savings_totals.compute":    "savings_totals_compute",
				"stats30d":                  "stats30d",
				"stats30d.compute":          "stats30d_compute",
			},
		},

		// Genuine sharing: the same name for the same shape must keep sharing a
		// single generated type.
		"same depth, identical shapes": {
			attributes: GeneratorAttributes{
				"cost":  nest(GeneratorAttributes{"metric": nest(leaves("value"))}),
				"usage": nest(GeneratorAttributes{"metric": nest(leaves("value"))}),
			},
			expected: map[string]string{
				"cost":         "cost",
				"cost.metric":  "metric",
				"usage":        "usage",
				"usage.metric": "metric",
			},
		},

		// Reproduces the naming the depth-based rule already produced for the
		// three distinct statussheet shapes in datasource_cloud_diagrams_schemes.
		"cross depth": {
			attributes: GeneratorAttributes{
				"statussheet": nest(GeneratorAttributes{
					"node":        fakeString{},
					"statussheet": nest(leaves("id", "updated_at")),
					"scheme":      nest(GeneratorAttributes{"statussheet": nest(leaves("ssid", "color"))}),
				}),
			},
			expected: map[string]string{
				"statussheet":                    "statussheet",
				"statussheet.scheme":             "scheme",
				"statussheet.scheme.statussheet": "scheme_statussheet",
				"statussheet.statussheet":        "statussheet_statussheet",
			},
		},

		// Four occurrences, two shapes: two names, not four.
		"mixed groups": {
			attributes: GeneratorAttributes{
				"a": nest(GeneratorAttributes{"item": nest(leaves("x"))}),
				"b": nest(GeneratorAttributes{"item": nest(leaves("y"))}),
				"c": nest(GeneratorAttributes{"item": nest(leaves("x"))}),
				"d": nest(GeneratorAttributes{"item": nest(leaves("y"))}),
			},
			expected: map[string]string{
				"a": "a", "a.item": "item",
				"b": "b", "b.item": "b_item",
				"c": "c", "c.item": "item",
				"d": "d", "d.item": "b_item",
			},
		},

		// The nearest-ancestor candidate collides with a real attribute name, so
		// disambiguation escalates to the grandparent.
		"prefix escalation": {
			attributes: GeneratorAttributes{
				"alpha_compute": nest(leaves("x")),
				"beta":          nest(GeneratorAttributes{"compute": nest(leaves("p"))}),
				"outer": nest(GeneratorAttributes{
					"alpha": nest(GeneratorAttributes{"compute": nest(leaves("q"))}),
				}),
			},
			expected: map[string]string{
				"alpha_compute":       "alpha_compute",
				"beta":                "beta",
				"beta.compute":        "compute",
				"outer":               "outer",
				"outer.alpha":         "alpha",
				"outer.alpha.compute": "outer_alpha_compute",
			},
		},

		// The two compute objects have identical immediate children and differ
		// only one level further down. A non-recursive fingerprint would call
		// them interchangeable and silently emit one shape for both.
		"differs only at depth two": {
			attributes: GeneratorAttributes{
				"p1": nest(GeneratorAttributes{"compute": nest(GeneratorAttributes{"inner": nest(leaves("a"))})}),
				"p2": nest(GeneratorAttributes{"compute": nest(GeneratorAttributes{"inner": nest(leaves("b"))})}),
			},
			expected: map[string]string{
				"p1":               "p1",
				"p1.compute":       "compute",
				"p1.compute.inner": "inner",
				"p2":               "p2",
				"p2.compute":       "p2_compute",
				"p2.compute.inner": "compute_inner",
			},
		},

		// Same keys, different leaf types: still a conflict.
		"differs only by leaf type": {
			attributes: GeneratorAttributes{
				"a": nest(GeneratorAttributes{"flag": nest(GeneratorAttributes{"v": fakeString{}})}),
				"b": nest(GeneratorAttributes{"flag": nest(GeneratorAttributes{"v": fakeBool{}})}),
			},
			expected: map[string]string{
				"a": "a", "a.flag": "flag",
				"b": "b", "b.flag": "b_flag",
			},
		},

		"no conflict": {
			attributes: GeneratorAttributes{
				"a": nest(leaves("x")),
				"b": nest(leaves("y")),
			},
			expected: map[string]string{"a": "a", "b": "b"},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := resolveAndCollect(testCase.attributes)

			if diff := cmp.Diff(got, testCase.expected); diff != "" {
				t.Errorf("unexpected difference: %s", diff)
			}
		})
	}
}

// TestGeneratorSchema_ResolveTypeNameConflicts_Deterministic guards against Go's
// randomised map iteration leaking into the names that get assigned.
func TestGeneratorSchema_ResolveTypeNameConflicts_Deterministic(t *testing.T) {
	t.Parallel()

	build := func() GeneratorAttributes {
		return GeneratorAttributes{
			"onboarding_status": nest(GeneratorAttributes{"compute": nest(leaves("status"))}),
			"savings_totals":    nest(GeneratorAttributes{"compute": nest(leaves("lifetime", "ytd"))}),
			"stats30d":          nest(GeneratorAttributes{"compute": nest(leaves("savings"))}),
			"monthly_stats":     nest(GeneratorAttributes{"compute": nest(leaves("cost_with_savings"))}),
			"daily_coverage":    nest(GeneratorAttributes{"compute": nest(leaves("on_demand_cost"))}),
		}
	}

	want := resolveAndCollect(build())

	for i := 0; i < 50; i++ {
		if diff := cmp.Diff(resolveAndCollect(build()), want); diff != "" {
			t.Fatalf("run %d differed: %s", i, diff)
		}
	}

	// Every distinct shape must end up with its own name.
	seen := make(map[string]struct{})
	for path, name := range want {
		if !hasSuffix(path, ".compute") {
			continue
		}
		if _, ok := seen[name]; ok {
			t.Errorf("type name %q reused for two distinct compute shapes", name)
		}
		seen[name] = struct{}{}
	}

	if len(seen) != 5 {
		t.Errorf("expected 5 distinct compute type names, got %d: %v", len(seen), seen)
	}
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
