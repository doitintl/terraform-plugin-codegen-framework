// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package convert

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-codegen-spec/code"
	specschema "github.com/hashicorp/terraform-plugin-codegen-spec/schema"

	"github.com/doitintl/terraform-plugin-codegen-framework/internal/format"
	"github.com/doitintl/terraform-plugin-codegen-framework/internal/schema"
)

type CustomTypeNestedObject struct {
	customType *specschema.CustomType
	name       string
}

// NewCustomTypeNestedObject constructs an CustomTypeNestedObject which is used to determine whether a CustomType
// should be assigned to a nested attribute object in the schema.
//
// If a CustomType has been declared in the spec, then the CustomType.Type will be used as
// the CustomType in the schema.
//
// If the spec CustomType is nil, the generator will create custom Type and Value types using the attribute
// name, and the generated custom Type type will be used as the CustomType in the schema.
func NewCustomTypeNestedObject(c *specschema.CustomType, name string) CustomTypeNestedObject {
	return CustomTypeNestedObject{
		customType: c,
		name:       name,
	}
}

func (c CustomTypeNestedObject) Equal(other CustomTypeNestedObject) bool {
	if !c.customType.Equal(other.customType) {
		return false
	}

	return c.name == other.name
}

func (c CustomTypeNestedObject) Imports() *schema.Imports {
	imports := schema.NewImports()

	if c.customType != nil {
		if c.customType.HasImport() {
			imports.Add(*c.customType.Import)
		}
	} else {
		imports.Add(code.Import{
			Path: schema.TypesImport,
		})
	}

	return imports
}

func (c CustomTypeNestedObject) Schema() []byte {
	var customTypeType string

	switch {
	case c.customType != nil:
		customTypeType = c.customType.Type
	default:
		customTypeType = fmt.Sprintf("%sType{\nObjectType: types.ObjectType{\nAttrTypes: %sValue{}.AttributeTypes(ctx),\n},\n}", format.ToPascalCase(c.name), format.ToPascalCase(c.name))
	}

	if customTypeType != "" {
		return fmt.Appendf(nil, "CustomType: %s,\n", customTypeType)
	}

	return nil
}

func (c CustomTypeNestedObject) ValueType() string {
	if c.customType != nil {
		return c.customType.ValueType
	}

	return ""
}

// TypeName returns the name used for generating the custom type.
func (c CustomTypeNestedObject) TypeName() string {
	return c.name
}

// WithTypeName returns a copy of the CustomTypeNestedObject with the given name.
// This is used for resolving type name conflicts when the same attribute name
// appears at different nesting levels with different schemas.
func (c CustomTypeNestedObject) WithTypeName(name string) CustomTypeNestedObject {
	c.name = name
	return c
}
