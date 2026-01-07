package convert

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	specschema "github.com/hashicorp/terraform-plugin-codegen-spec/schema"
)

func TestDescription_AppendValidators(t *testing.T) {
	tests := []struct {
		name         string
		initialDesc  *string
		validators   Validators
		expectedDesc string
	}{
		{
			name:        "Empty description, simple OneOf",
			initialDesc: nil,
			validators: NewValidators(ValidatorTypeString, specschema.CustomValidators{
				{
					SchemaDefinition: `stringvalidator.OneOf("a", "b")`,
				},
			}),
			expectedDesc: "Possible values: `a`, `b`",
		},
		{
			name:        "Existing description, simple OneOf",
			initialDesc: stringPtr("Some description."),
			validators: NewValidators(ValidatorTypeString, specschema.CustomValidators{
				{
					SchemaDefinition: `stringvalidator.OneOf("foo", "bar")`,
				},
			}),
			expectedDesc: "Some description.\nPossible values: `foo`, `bar`",
		},
		{
			name:        "Multiline OneOf",
			initialDesc: stringPtr("Desc"),
			validators: NewValidators(ValidatorTypeString, specschema.CustomValidators{
				{
					SchemaDefinition: `stringvalidator.OneOf(
"val1",
"val2",
)`,
				},
			}),
			expectedDesc: "Desc\nPossible values: `val1`, `val2`",
		},
		{
			name:        "Commas inside quotes",
			initialDesc: stringPtr("Desc"),
			validators: NewValidators(ValidatorTypeString, specschema.CustomValidators{
				{
					SchemaDefinition: `stringvalidator.OneOf("a,b", "c")`,
				},
			}),
			expectedDesc: "Desc\nPossible values: `a,b`, `c`",
		},
		{
			name:        "No OneOf",
			initialDesc: stringPtr("Desc"),
			validators: NewValidators(ValidatorTypeString, specschema.CustomValidators{
				{
					SchemaDefinition: `stringvalidator.LengthAtLeast(1)`,
				},
			}),
			expectedDesc: "Desc",
		},
		{
			name:        "Multiple OneOf (should append both)",
			initialDesc: stringPtr("Desc"),
			validators: NewValidators(ValidatorTypeString, specschema.CustomValidators{
				{
					SchemaDefinition: `stringvalidator.OneOf("a", "b")`,
				},
				{
					// This case is artificial, usually there's only one OneOf, but good to test behavior
					SchemaDefinition: `stringvalidator.OneOf("c", "d")`,
				},
			}),
			expectedDesc: "Desc\nPossible values: `a`, `b`\nPossible values: `c`, `d`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDescription(tt.initialDesc)
			d.AppendValidators(tt.validators)

			got := d.Description()
			if diff := cmp.Diff(tt.expectedDesc, got); diff != "" {
				t.Errorf("AppendValidators() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
