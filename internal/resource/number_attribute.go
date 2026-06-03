// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resource

import (
	"bytes"
	"fmt"

	"github.com/hashicorp/terraform-plugin-codegen-spec/resource"

	"github.com/doitintl/terraform-plugin-codegen-framework/internal/convert"
	"github.com/doitintl/terraform-plugin-codegen-framework/internal/model"
	generatorschema "github.com/doitintl/terraform-plugin-codegen-framework/internal/schema"
)

type GeneratorNumberAttribute struct {
	AssociatedExternalType   *generatorschema.AssocExtType
	ComputedOptionalRequired convert.ComputedOptionalRequired
	CustomType               convert.CustomTypePrimitive
	Default                  convert.DefaultCustom
	DeprecationMessage       convert.DeprecationMessage
	Description              convert.Description
	PlanModifiers            convert.PlanModifiers
	Sensitive                convert.Sensitive
	Validators               convert.Validators
}

func NewGeneratorNumberAttribute(name string, a *resource.NumberAttribute) (GeneratorNumberAttribute, error) {
	if a == nil {
		return GeneratorNumberAttribute{}, fmt.Errorf("*resource.NumberAttribute is nil")
	}

	c := convert.NewComputedOptionalRequired(a.ComputedOptionalRequired)

	ctp := convert.NewCustomTypePrimitive(a.CustomType, a.AssociatedExternalType, name)

	dc := convert.NewDefaultCustom(a.Default.CustomDefault())

	dm := convert.NewDeprecationMessage(a.DeprecationMessage)

	d := convert.NewDescription(a.Description)

	pm := convert.NewPlanModifiers(convert.PlanModifierTypeNumber, a.PlanModifiers.CustomPlanModifiers())

	s := convert.NewSensitive(a.Sensitive)

	v := convert.NewValidators(convert.ValidatorTypeNumber, a.Validators.CustomValidators())

	return GeneratorNumberAttribute{
		AssociatedExternalType:   generatorschema.NewAssocExtType(a.AssociatedExternalType),
		ComputedOptionalRequired: c,
		CustomType:               ctp,
		Default:                  dc,
		Description:              d,
		DeprecationMessage:       dm,
		PlanModifiers:            pm,
		Sensitive:                s,
		Validators:               v,
	}, nil
}

func (g GeneratorNumberAttribute) GeneratorSchemaType() generatorschema.Type {
	return generatorschema.GeneratorNumberAttribute
}

func (g GeneratorNumberAttribute) Imports() *generatorschema.Imports {
	imports := generatorschema.NewImports()

	imports.Append(g.CustomType.Imports())

	imports.Append(g.Default.Imports())

	imports.Append(g.PlanModifiers.Imports())

	imports.Append(g.Validators.Imports())

	if g.AssociatedExternalType != nil {
		imports.Append(generatorschema.AssociatedExternalTypeImports())
	}

	imports.Append(g.AssociatedExternalType.Imports())

	return imports
}

func (g GeneratorNumberAttribute) Equal(ga generatorschema.GeneratorAttribute) bool {
	h, ok := ga.(GeneratorNumberAttribute)

	if !ok {
		return false
	}

	if !g.AssociatedExternalType.Equal(h.AssociatedExternalType) {
		return false
	}

	if !g.ComputedOptionalRequired.Equal(h.ComputedOptionalRequired) {
		return false
	}

	if !g.CustomType.Equal(h.CustomType) {
		return false
	}

	if !g.Default.Equal(h.Default) {
		return false
	}

	if !g.DeprecationMessage.Equal(h.DeprecationMessage) {
		return false
	}

	if !g.Description.Equal(h.Description) {
		return false
	}

	if !g.PlanModifiers.Equal(h.PlanModifiers) {
		return false
	}

	if !g.Sensitive.Equal(h.Sensitive) {
		return false
	}

	return g.Validators.Equal(h.Validators)
}

func (g GeneratorNumberAttribute) Schema(name generatorschema.FrameworkIdentifier) (string, error) {
	var b bytes.Buffer

	b.WriteString(fmt.Sprintf("%q: schema.NumberAttribute{\n", name))
	b.Write(g.CustomType.Schema())
	b.Write(g.ComputedOptionalRequired.Schema())
	b.Write(g.Sensitive.Schema())
	b.Write(g.Description.Schema())
	b.Write(g.DeprecationMessage.Schema())
	b.Write(g.PlanModifiers.Schema())
	b.Write(g.Validators.Schema())
	b.Write(g.Default.Schema())
	b.WriteString("},")

	return b.String(), nil
}

func (g GeneratorNumberAttribute) ModelField(name generatorschema.FrameworkIdentifier) (model.Field, error) {
	field := model.Field{
		Name:      name.ToPascalCase(),
		TfsdkName: name.ToString(),
		ValueType: model.NumberValueType,
	}

	customValueType := g.CustomType.ValueType()

	if customValueType != "" {
		field.ValueType = customValueType
	}

	return field, nil
}

func (g GeneratorNumberAttribute) CustomTypeAndValue(name string, generated map[string][]byte) ([]byte, error) {
	// Check if this name was already generated with identical content.
	// If so, skip to avoid duplicate declarations. If the name exists but
	// with nil content (sentinel for recursion guard), we proceed.
	if prev, ok := generated[name]; ok && prev != nil {
		return nil, nil
	}

	// Set a nil sentinel to prevent infinite recursion if a nested
	// attribute references the same name.
	generated[name] = nil

	if g.AssociatedExternalType == nil {
		return nil, nil
	}

	var buf bytes.Buffer

	numberType := generatorschema.NewCustomNumberType(name)

	b, err := numberType.Render()

	if err != nil {
		return nil, err
	}

	buf.Write(b)

	numberValue := generatorschema.NewCustomNumberValue(name)

	b, err = numberValue.Render()

	if err != nil {
		return nil, err
	}

	buf.Write(b)

	// Store the generated bytes so future calls with the same name
	// and identical content will be skipped.
	result := buf.Bytes()
	generated[name] = result

	return result, nil
}

func (g GeneratorNumberAttribute) ToFromFunctions(name string) ([]byte, error) {
	if g.AssociatedExternalType == nil {
		return nil, nil
	}

	toFrom := generatorschema.NewToFromNumber(name, g.AssociatedExternalType)

	b, err := toFrom.Render()

	if err != nil {
		return nil, err
	}

	return b, nil
}

// AttrType returns a string representation of a basetypes.NumberTypable type.
func (g GeneratorNumberAttribute) AttrType(name generatorschema.FrameworkIdentifier) (string, error) {
	if g.AssociatedExternalType != nil {
		return fmt.Sprintf("%sType{}", name.ToPascalCase()), nil
	}

	return "basetypes.NumberType{}", nil
}

// AttrValue returns a string representation of a basetypes.NumberValuable type.
func (g GeneratorNumberAttribute) AttrValue(name generatorschema.FrameworkIdentifier) string {
	if g.AssociatedExternalType != nil {
		return fmt.Sprintf("%sValue", name.ToPascalCase())
	}

	return "basetypes.NumberValue"
}

func (g GeneratorNumberAttribute) To() (generatorschema.ToFromConversion, error) {
	if g.AssociatedExternalType != nil {
		return generatorschema.ToFromConversion{
			AssocExtType: g.AssociatedExternalType,
		}, nil
	}

	return generatorschema.ToFromConversion{
		Default: "ValueBigFloat",
	}, nil
}

func (g GeneratorNumberAttribute) From() (generatorschema.ToFromConversion, error) {
	if g.AssociatedExternalType != nil {
		return generatorschema.ToFromConversion{
			AssocExtType: g.AssociatedExternalType,
		}, nil
	}

	return generatorschema.ToFromConversion{
		Default: "NumberValue",
	}, nil
}
