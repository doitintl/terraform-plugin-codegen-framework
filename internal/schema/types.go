// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	specschema "github.com/hashicorp/terraform-plugin-codegen-spec/schema"

	"github.com/doitintl/terraform-plugin-codegen-framework/internal/model"
)

type Attributes interface {
	GetAttributes() GeneratorAttributes
}

type Attrs interface {
	AttrTypes() specschema.ObjectAttributeTypes
}

type Blocks interface {
	Attributes
	GetBlocks() GeneratorBlocks
}

type CustomTypeAndValue interface {
	CustomTypeAndValue(name string, generated map[string][]byte) ([]byte, error)
}

// EffectiveTypeName is implemented by nested attribute/block generators that support
// type name override for conflict resolution. When two nested attributes at different
// nesting depths share the same name but have different schemas, one of them needs
// a disambiguated name. This interface allows reading the resolved name.
type EffectiveTypeName interface {
	EffectiveTypeName() string
}

// TypeNameConflictResolver is implemented by nested attribute/block generators
// that can have their type name changed for conflict resolution.
type TypeNameConflictResolver interface {
	// WithResolvedTypeName returns a copy of the generator with the type name
	// updated. The returned value should be stored back into the GeneratorAttributes map.
	WithResolvedTypeName(name string) GeneratorAttribute
}

type Elements interface {
	ElemType() specschema.ElementType
}

type GeneratorAttribute interface {
	Equal(GeneratorAttribute) bool
	GeneratorSchemaType() Type
	Imports() *Imports
	ModelField(FrameworkIdentifier) (model.Field, error)
	Schema(FrameworkIdentifier) (string, error)
}

type AttrType interface {
	AttrType(FrameworkIdentifier) (string, error)
}

type AttrValue interface {
	AttrValue(FrameworkIdentifier) string
}

type CollectionType interface {
	CollectionType() (map[string]string, error)
}

type GeneratorBlock interface {
	Equal(GeneratorBlock) bool
	GeneratorSchemaType() Type
	Imports() *Imports
	ModelField(FrameworkIdentifier) (model.Field, error)
	Schema(FrameworkIdentifier) (string, error)
}

type ToFrom interface {
	ToFromFunctions(name string) ([]byte, error)
}

type ToFromConversion struct {
	Default        string
	AssocExtType   *AssocExtType
	CollectionType CollectionFields
	ObjectType     map[FrameworkIdentifier]ObjectField
}

type CollectionFields struct {
	ElementType   string
	GoType        string
	TypeValueFrom string
}

type ObjectField struct {
	FromFunc string
	GoType   string
	Type     string
	ToFunc   string
}

type To interface {
	To() (ToFromConversion, error)
}

type From interface {
	From() (ToFromConversion, error)
}

type Type int64

const (
	InvalidGeneratorSchemaType Type = iota
	GeneratorBoolAttribute
	GeneratorFloat64Attribute
	GeneratorInt64Attribute
	GeneratorListAttribute
	GeneratorListNestedAttribute
	GeneratorListNestedBlock
	GeneratorMapAttribute
	GeneratorMapNestedAttribute
	GeneratorNumberAttribute
	GeneratorObjectAttribute
	GeneratorSetAttribute
	GeneratorSetNestedAttribute
	GeneratorSetNestedBlock
	GeneratorSingleNestedAttribute
	GeneratorSingleNestedBlock
	GeneratorStringAttribute
)
