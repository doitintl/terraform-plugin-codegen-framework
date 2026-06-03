// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"text/template"

	"github.com/hashicorp/terraform-plugin-codegen-spec/code"
	specschema "github.com/hashicorp/terraform-plugin-codegen-spec/schema"

	"github.com/doitintl/terraform-plugin-codegen-framework/internal/logging"
	"github.com/doitintl/terraform-plugin-codegen-framework/internal/model"
)

type GeneratorSchema struct {
	Attributes          GeneratorAttributes
	Blocks              GeneratorBlocks
	Description         *string
	MarkdownDescription *string
	DeprecationMessage  *string
}

func (g GeneratorSchema) Imports() (string, error) {
	imports := NewImports()

	imports.Add(
		code.Import{
			Path: ContextImport,
		},
	)

	for _, a := range g.Attributes {
		if a == nil {
			continue
		}

		if _, ok := a.(Attributes); ok {
			imports.Add([]code.Import{
				{
					Path: FmtImport,
				},
				{
					Path: StringsImport,
				},
				{
					Path: DiagImport,
				},
				{
					Path: AttrImport,
				},
				{
					Path: TfTypesImport,
				},
				{
					Path: BaseTypesImport,
				},
			}...)
		}
	}

	for _, b := range g.Blocks {
		if b == nil {
			continue
		}

		imports.Add([]code.Import{
			{
				Path: ContextImport,
			},
		}...)

		if _, ok := b.(Blocks); ok {
			imports.Add([]code.Import{
				{
					Path: FmtImport,
				},
				{
					Path: StringsImport,
				},
				{
					Path: DiagImport,
				},
				{
					Path: AttrImport,
				},
				{
					Path: TfTypesImport,
				},
				{
					Path: BaseTypesImport,
				},
			}...)
		}
	}

	for _, v := range g.Attributes {
		imports.Add(v.Imports().All()...)
	}

	for _, v := range g.Blocks {
		imports.Add(v.Imports().All()...)
	}

	var sb strings.Builder

	for _, i := range imports.All() {
		var alias string

		if i.Alias != nil {
			alias = *i.Alias + " "
		}

		sb.WriteString(fmt.Sprintf("%s%q\n", alias, i.Path))
	}

	return sb.String(), nil
}

func (g GeneratorSchema) Schema(name, packageName, generatorType string) ([]byte, error) {
	attributes, err := g.Attributes.Schema()

	if err != nil {
		return nil, err
	}

	blocks, err := g.Blocks.Schema()

	if err != nil {
		return nil, err
	}

	imports, err := g.Imports()

	if err != nil {
		return nil, err
	}

	description := ""
	if g.Description != nil {
		description = *g.Description
	}

	markdownDescription := ""
	if g.MarkdownDescription != nil {
		markdownDescription = *g.MarkdownDescription
	}

	deprecationMessage := ""
	if g.DeprecationMessage != nil {
		deprecationMessage = *g.DeprecationMessage
	}

	templateData := struct {
		Name                string
		PackageName         string
		GeneratorType       string
		Attributes          string
		Blocks              string
		Description         string
		Imports             string
		MarkdownDescription string
		DeprecationMessage  string
	}{
		Name:                FrameworkIdentifier(name).ToPascalCase(),
		PackageName:         packageName,
		GeneratorType:       generatorType,
		Attributes:          attributes,
		Blocks:              blocks,
		Description:         description,
		Imports:             imports,
		MarkdownDescription: markdownDescription,
		DeprecationMessage:  deprecationMessage,
	}

	t, err := template.New("schema").Parse(SchemaGoTemplate)

	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	err = t.Execute(&buf, templateData)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (g GeneratorSchema) Models(name string) ([]model.Model, error) {
	var models []model.Model

	var modelFields []model.Field

	attributeKeys := g.Attributes.SortedKeys()

	for _, k := range attributeKeys {
		if g.Attributes[k] == nil {
			continue
		}

		modelField, err := g.Attributes[k].ModelField(FrameworkIdentifier(k))

		if err != nil {
			return nil, err
		}

		// If the attribute's type name was changed by conflict resolution,
		// update the model field's ValueType to match the resolved name.
		// The Go field name and tfsdk tag must keep the original attribute name.
		if t, ok := g.Attributes[k].(EffectiveTypeName); ok {
			if tn := t.EffectiveTypeName(); tn != "" && tn != k {
				modelField.ValueType = FrameworkIdentifier(tn).ToPascalCase() + "Value"
			}
		}

		modelFields = append(modelFields, modelField)
	}

	blockKeys := g.Blocks.SortedKeys()

	for _, k := range blockKeys {
		if g.Blocks[k] == nil {
			continue
		}

		modelField, err := g.Blocks[k].ModelField(FrameworkIdentifier(k))

		if err != nil {
			return nil, err
		}

		modelFields = append(modelFields, modelField)
	}

	m := model.Model{
		Name:   FrameworkIdentifier(name).ToPascalCase(),
		Fields: modelFields,
	}

	models = append(models, m)

	return models, nil
}

// typeFingerprint represents a collected type's schema fingerprint at a specific nesting depth.
type typeFingerprint struct {
	fingerprint string
	depth       int
	parentName  string
	// attrPath tracks the path of attribute keys to reach this type in the tree
	attrPath []string
}

// ResolveTypeNameConflicts walks all nested attributes and blocks recursively,
// detecting name conflicts where the same attribute name appears at different
// nesting levels with different schemas. When a conflict is found, the deeper
// attribute gets its type name prefixed with the parent name to disambiguate.
//
// This method mutates the GeneratorSchema's Attributes map in place and must be
// called before Schema() and CustomTypeValueBytes().
func (g *GeneratorSchema) ResolveTypeNameConflicts() {
	// Collect all type fingerprints
	fingerprints := make(map[string][]typeFingerprint)
	collectFingerprints(g.Attributes, fingerprints, 0, "", nil)
	collectBlockFingerprints(g.Blocks, fingerprints, 0, "", nil)

	// Find conflicts: same name, different fingerprints
	for name, fps := range fingerprints {
		if len(fps) <= 1 {
			continue
		}

		// Check if all fingerprints are the same (same schema, just shared)
		allSame := true
		for i := 1; i < len(fps); i++ {
			if fps[i].fingerprint != fps[0].fingerprint {
				allSame = false
				break
			}
		}

		if allSame {
			continue // No conflict: all types with this name have the same schema
		}

		// Conflict detected: disambiguate the deeper ones by prefixing with parent name.
		// The shallowest occurrence keeps the bare name.
		minDepth := fps[0].depth
		for _, fp := range fps[1:] {
			if fp.depth < minDepth {
				minDepth = fp.depth
			}
		}

		for _, fp := range fps {
			if fp.depth == minDepth {
				continue // Keep the shallowest occurrence with the bare name
			}

			// Prefix with parent name to disambiguate
			newName := fp.parentName + "_" + name
			resolveAtPath(g.Attributes, fp.attrPath, newName)
		}
	}
}

// collectFingerprints recursively walks attributes and collects type fingerprints.
func collectFingerprints(attrs GeneratorAttributes, fps map[string][]typeFingerprint, depth int, parentName string, path []string) {
	for _, k := range attrs.SortedKeys() {
		attr := attrs[k]
		if attr == nil {
			continue
		}

		currentPath := append(append([]string{}, path...), k)

		// Check if this attribute has nested attributes (implements Attributes interface)
		if nested, ok := attr.(Attributes); ok {
			// Compute fingerprint from the nested attributes' sorted keys
			nestedAttrs := nested.GetAttributes()
			fp := computeAttrFingerprint(nestedAttrs)

			fps[k] = append(fps[k], typeFingerprint{
				fingerprint: fp,
				depth:       depth,
				parentName:  parentName,
				attrPath:    currentPath,
			})

			// Recurse into nested attributes
			collectFingerprints(nestedAttrs, fps, depth+1, k, currentPath)
		}
	}
}

// collectBlockFingerprints recursively walks blocks and collects type fingerprints.
func collectBlockFingerprints(blocks GeneratorBlocks, fps map[string][]typeFingerprint, depth int, parentName string, path []string) {
	for _, k := range blocks.SortedKeys() {
		block := blocks[k]
		if block == nil {
			continue
		}

		currentPath := append(append([]string{}, path...), k)

		if nested, ok := block.(Attributes); ok {
			nestedAttrs := nested.GetAttributes()
			fp := computeAttrFingerprint(nestedAttrs)

			fps[k] = append(fps[k], typeFingerprint{
				fingerprint: fp,
				depth:       depth,
				parentName:  parentName,
				attrPath:    currentPath,
			})

			collectFingerprints(nestedAttrs, fps, depth+1, k, currentPath)
		}

		if nested, ok := block.(Blocks); ok {
			nestedBlocks := nested.GetBlocks()
			collectBlockFingerprints(nestedBlocks, fps, depth+1, k, currentPath)
		}
	}
}

// computeAttrFingerprint computes a string fingerprint from a GeneratorAttributes map.
// Two attributes with the same fingerprint have the same schema structure at depth 1.
func computeAttrFingerprint(attrs GeneratorAttributes) string {
	keys := attrs.SortedKeys()
	var parts []string
	for _, k := range keys {
		a := attrs[k]
		if a == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%T", k, a))
	}
	return strings.Join(parts, ",")
}

// resolveAtPath walks the attribute tree following the given path and renames the
// type at the leaf. The path is a sequence of attribute keys from the root.
func resolveAtPath(attrs GeneratorAttributes, path []string, newName string) {
	if len(path) == 0 {
		return
	}

	if len(path) == 1 {
		// Leaf: rename this attribute's type
		attr := attrs[path[0]]
		if resolver, ok := attr.(TypeNameConflictResolver); ok {
			attrs[path[0]] = resolver.WithResolvedTypeName(newName)
		}
		return
	}

	// Intermediate: recurse into nested attributes
	attr := attrs[path[0]]
	if nested, ok := attr.(Attributes); ok {
		resolveAtPath(nested.GetAttributes(), path[1:], newName)
	}
}

// CustomTypeValueBytes iterates over all the attributes and blocks to generate code
// for custom type and value types for use in the schema and data models.
func (g GeneratorSchema) CustomTypeValueBytes() ([]byte, error) {
	var buf bytes.Buffer

	generated := make(map[string][]byte)

	attributeKeys := g.Attributes.SortedKeys()

	for _, k := range attributeKeys {
		if g.Attributes[k] == nil {
			continue
		}

		if c, ok := g.Attributes[k].(CustomTypeAndValue); ok {
			// Use the effective type name if available (may have been
			// changed by ResolveTypeNameConflicts).
			effectiveName := k
			if t, ok := g.Attributes[k].(EffectiveTypeName); ok {
				if tn := t.EffectiveTypeName(); tn != "" {
					effectiveName = tn
				}
			}

			b, err := c.CustomTypeAndValue(effectiveName, generated)

			if err != nil {
				return nil, err
			}

			buf.Write(b)
		}
	}

	blockKeys := g.Blocks.SortedKeys()

	for _, k := range blockKeys {
		if g.Blocks[k] == nil {
			continue
		}

		if c, ok := g.Blocks[k].(CustomTypeAndValue); ok {
			effectiveName := k
			if t, ok := g.Blocks[k].(EffectiveTypeName); ok {
				if tn := t.EffectiveTypeName(); tn != "" {
					effectiveName = tn
				}
			}

			b, err := c.CustomTypeAndValue(effectiveName, generated)

			if err != nil {
				return nil, err
			}

			buf.Write(b)
		}
	}
	if buf.Len() > 0 {
		buf.WriteString("\n")
	}

	return buf.Bytes(), nil
}

// ToFromFunctions generates code for converting to an associated
// external type from a framework type, and from an associated
// external type to a framework type.
func (g GeneratorSchema) ToFromFunctions(ctx context.Context, logger *slog.Logger) ([]byte, error) {
	var buf bytes.Buffer

	attributeKeys := g.Attributes.SortedKeys()

	for _, k := range attributeKeys {
		if g.Attributes[k] == nil {
			continue
		}

		if t, ok := g.Attributes[k].(ToFrom); ok {
			b, err := t.ToFromFunctions(k)

			var unimplErr *UnimplementedError

			if errors.As(err, &unimplErr) {
				logger.Error("error generating to/from methods", "path", fmt.Sprintf("%s.%s.%s", logging.GetPathFromContext(ctx), k, unimplErr.Path()), "err", err)
			} else if err != nil {
				return nil, err
			}

			buf.Write(b)
		}
	}

	blockKeys := g.Blocks.SortedKeys()

	for _, k := range blockKeys {
		if g.Blocks[k] == nil {
			continue
		}

		if g.Blocks[k] == nil {
			continue
		}

		if t, ok := g.Blocks[k].(ToFrom); ok {
			b, err := t.ToFromFunctions(k)

			var unimplErr *UnimplementedError

			if errors.As(err, &unimplErr) {
				logger.Error("error generating to/from methods", "path", fmt.Sprintf("%s.%s.%s", logging.GetPathFromContext(ctx), k, unimplErr.Path()), "err", err)
			} else if err != nil {
				return nil, err
			}

			buf.Write(b)
		}
	}

	return buf.Bytes(), nil
}

func ElementTypeString(elementType specschema.ElementType) (string, error) {
	switch {
	case elementType.Bool != nil:
		return "types.BoolType", nil
	case elementType.Float64 != nil:
		return "types.Float64Type", nil
	case elementType.Int64 != nil:
		return "types.Int64Type", nil
	case elementType.List != nil:
		elemType, err := ElementTypeString(elementType.List.ElementType)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("types.ListType{\nElemType: %s,\n}", elemType), nil
	case elementType.Map != nil:
		elemType, err := ElementTypeString(elementType.Map.ElementType)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("types.MapType{\nElemType: %s,\n}", elemType), nil
	case elementType.Number != nil:
		return "types.NumberType", nil
	case elementType.Object != nil:
		attrTypesStr, err := AttrTypesString(elementType.Object.AttributeTypes)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("types.ObjectType{\nAttrTypes: map[string]attr.Type{\n%s,\n},\n}", attrTypesStr), nil
	case elementType.Set != nil:
		elemType, err := ElementTypeString(elementType.Set.ElementType)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("types.SetType{\nElemType: %s,\n}", elemType), nil
	case elementType.String != nil:
		return "types.StringType", nil
	}

	return "", errors.New("no matching element type found")
}

// ElementTypeGoType defaults to the defined pointer types on the basis of the
// supplied elementType.
// TODO: Provide a mechanism to allow mapping to be configured. For instance elementType.Float64 => float32
// TODO: Implement for list, map, object, and set.
func ElementTypeGoType(elementType specschema.ElementType) (string, error) {
	switch {
	case elementType.Bool != nil:
		return "*bool", nil
	case elementType.Float64 != nil:
		return "*float64", nil
	case elementType.Int64 != nil:
		return "*int64", nil
	case elementType.List != nil:
		return "", NewUnimplementedError(errors.New("list element type is not yet implemented"))
	case elementType.Map != nil:
		return "", NewUnimplementedError(errors.New("map element type is not yet implemented"))
	case elementType.Number != nil:
		return "*big.Float", nil
	case elementType.Object != nil:
		return "", NewUnimplementedError(errors.New("object element type is not yet implemented"))
	case elementType.Set != nil:
		return "", NewUnimplementedError(errors.New("set element type is not yet implemented"))
	case elementType.String != nil:
		return "*string", nil
	}

	return "", errors.New("no matching element type found")
}

func AttrTypesString(attrTypes specschema.ObjectAttributeTypes) (string, error) {
	var attrTypesStr []string

	for _, v := range attrTypes {
		switch {
		case v.Bool != nil:
			attrTypesStr = append(attrTypesStr, fmt.Sprintf("%q: types.BoolType", v.Name))
		case v.Float64 != nil:
			attrTypesStr = append(attrTypesStr, fmt.Sprintf("%q: types.Float64Type", v.Name))
		case v.Int64 != nil:
			attrTypesStr = append(attrTypesStr, fmt.Sprintf("%q: types.Int64Type", v.Name))
		case v.List != nil:
			elemType, err := ElementTypeString(v.List.ElementType)
			if err != nil {
				return "", err
			}
			attrTypesStr = append(attrTypesStr, fmt.Sprintf("%q: types.ListType{\nElemType: %s,\n}", v.Name, elemType))
		case v.Map != nil:
			elemType, err := ElementTypeString(v.Map.ElementType)
			if err != nil {
				return "", err
			}
			attrTypesStr = append(attrTypesStr, fmt.Sprintf("%q: types.MapType{\nElemType: %s,\n}", v.Name, elemType))
		case v.Number != nil:
			attrTypesStr = append(attrTypesStr, fmt.Sprintf("%q: types.NumberType", v.Name))
		case v.Object != nil:
			if v.Object.CustomType != nil {
				attrTypesStr = append(attrTypesStr, fmt.Sprintf("%q: %s", v.Name, v.Object.CustomType.Type))
			} else {
				typeName := FrameworkIdentifier(v.Name).ToPascalCase()
				attrTypesStr = append(attrTypesStr, fmt.Sprintf("%q: %sType{\nbasetypes.ObjectType{\nAttrTypes: %sValue{}.AttributeTypes(ctx),\n},\n}", v.Name, typeName, typeName))
			}
		case v.Set != nil:
			elemType, err := ElementTypeString(v.Set.ElementType)
			if err != nil {
				return "", err
			}
			attrTypesStr = append(attrTypesStr, fmt.Sprintf("%q: types.SetType{\nElemType: %s,\n}", v.Name, elemType))
		case v.String != nil:
			attrTypesStr = append(attrTypesStr, fmt.Sprintf("%q: types.StringType", v.Name))
		}
	}

	return strings.Join(attrTypesStr, ",\n"), nil
}

func ObjectFieldTo(o specschema.ObjectAttributeType) (ObjectField, error) {
	switch {
	case o.Bool != nil:
		return ObjectField{
			GoType: "*bool",
			Type:   "types.Bool",
			ToFunc: "ValueBoolPointer",
		}, nil
	case o.Float64 != nil:
		return ObjectField{
			GoType: "*float64",
			Type:   "types.Float64",
			ToFunc: "ValueFloat64Pointer",
		}, nil
	case o.Int64 != nil:
		return ObjectField{
			GoType: "*int64",
			Type:   "types.Int64",
			ToFunc: "ValueInt64Pointer",
		}, nil
	case o.List != nil:
		return ObjectField{}, NewUnimplementedError(errors.New("list attribute type is not yet implemented"))
	case o.Map != nil:
		return ObjectField{}, NewUnimplementedError(errors.New("map attribute type is not yet implemented"))
	case o.Number != nil:
		return ObjectField{
			GoType: "*big.Float",
			Type:   "types.Number",
			ToFunc: "ValueBigFloat",
		}, nil
	case o.Object != nil:
		return ObjectField{}, NewUnimplementedError(errors.New("object attribute type is not yet implemented"))
	case o.Set != nil:
		return ObjectField{}, NewUnimplementedError(errors.New("set attribute type is not yet implemented"))
	case o.String != nil:
		return ObjectField{
			GoType: "*string",
			Type:   "types.String",
			ToFunc: "ValueStringPointer",
		}, nil
	}

	return ObjectField{}, errors.New("no matching object attribute type found")
}

func ObjectFieldFrom(o specschema.ObjectAttributeType) (ObjectField, error) {
	switch {
	case o.Bool != nil:
		return ObjectField{
			Type:     "types.BoolType",
			FromFunc: "BoolPointerValue",
		}, nil
	case o.Float64 != nil:
		return ObjectField{
			Type:     "types.Float64Type",
			FromFunc: "Float64PointerValue",
		}, nil
	case o.Int64 != nil:
		return ObjectField{
			Type:     "types.Int64Type",
			FromFunc: "Int64PointerValue",
		}, nil
	case o.List != nil:
		return ObjectField{}, NewUnimplementedError(errors.New("list attribute type is not yet implemented"))
	case o.Map != nil:
		return ObjectField{}, NewUnimplementedError(errors.New("map attribute type is not yet implemented"))
	case o.Number != nil:
		return ObjectField{
			Type:     "types.NumberType",
			FromFunc: "NumberValue",
		}, nil
	case o.Object != nil:
		return ObjectField{}, NewUnimplementedError(errors.New("object attribute type is not yet implemented"))
	case o.Set != nil:
		return ObjectField{}, NewUnimplementedError(errors.New("set attribute type is not yet implemented"))
	case o.String != nil:
		return ObjectField{
			Type:     "types.StringType",
			FromFunc: "StringPointerValue",
		}, nil
	}

	return ObjectField{}, errors.New("no matching object attribute type found")
}
