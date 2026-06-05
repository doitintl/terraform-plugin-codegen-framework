// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package convert

import (
	"testing"

	specschema "github.com/hashicorp/terraform-plugin-codegen-spec/schema"
)

func TestCustomTypePrimitive_TypeString(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		input    CustomTypePrimitive
		expected string
	}{
		"no-custom-type": {
			input:    NewCustomTypePrimitive(nil, nil, "attr"),
			expected: "",
		},
		"custom-type": {
			input: NewCustomTypePrimitive(
				&specschema.CustomType{
					Type:      "jsontypes.NormalizedType{}",
					ValueType: "jsontypes.Normalized",
				},
				nil,
				"attr",
			),
			expected: "jsontypes.NormalizedType{}",
		},
		"associated-external-type-only": {
			input: NewCustomTypePrimitive(
				nil,
				&specschema.AssociatedExternalType{
					Type: "*api.ExtString",
				},
				"attr",
			),
			expected: "",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := testCase.input.TypeString()

			if got != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestCustomTypePrimitive_ValueType(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		input    CustomTypePrimitive
		expected string
	}{
		"no-custom-type": {
			input:    NewCustomTypePrimitive(nil, nil, "attr"),
			expected: "",
		},
		"custom-type": {
			input: NewCustomTypePrimitive(
				&specschema.CustomType{
					Type:      "jsontypes.NormalizedType{}",
					ValueType: "jsontypes.Normalized",
				},
				nil,
				"attr",
			),
			expected: "jsontypes.Normalized",
		},
		"associated-external-type-only": {
			input: NewCustomTypePrimitive(
				nil,
				&specschema.AssociatedExternalType{
					Type: "*api.ExtString",
				},
				"attr",
			),
			expected: "AttrValue",
		},
		"custom-type-overrides-associated": {
			input: NewCustomTypePrimitive(
				&specschema.CustomType{
					ValueType: "my_custom_value",
				},
				&specschema.AssociatedExternalType{
					Type: "*api.ExtString",
				},
				"attr",
			),
			expected: "my_custom_value",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := testCase.input.ValueType()

			if got != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}
