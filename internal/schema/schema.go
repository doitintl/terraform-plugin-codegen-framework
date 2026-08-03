// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
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
	// attrPath tracks the path of attribute keys to reach this type in the tree.
	// Its last element is the attribute's own key; everything before it is the
	// ancestor chain used to disambiguate a conflicting name.
	attrPath []string
}

// ResolveTypeNameConflicts walks all nested attributes and blocks recursively,
// detecting name conflicts where the same attribute name is used for two or more
// differently-shaped objects. Occurrences are grouped by schema fingerprint: one
// group keeps the bare name and every other group is given a name qualified with
// enough of its ancestor chain to be unique.
//
// Grouping, rather than renaming each occurrence individually, is what preserves
// legitimate type sharing: when the same name is used for the same shape in
// several places, those occurrences stay in one group and continue to share a
// single generated type.
//
// This method mutates the GeneratorSchema's Attributes map in place and must be
// called before Schema() and CustomTypeValueBytes().
func (g *GeneratorSchema) ResolveTypeNameConflicts() {
	// Collect all type fingerprints
	fingerprints := make(map[string][]typeFingerprint)
	collectFingerprints(g.Attributes, fingerprints, 0, nil)
	collectBlockFingerprints(g.Blocks, fingerprints, 0, nil)

	// taken records every type name that is already spoken for, keyed by the
	// PascalCase form that actually appears in the generated Go. It is seeded
	// with every bare nested-object name so that a disambiguated name can never
	// shadow a real one.
	taken := make(map[string]struct{}, len(fingerprints))

	names := make([]string, 0, len(fingerprints))

	for name := range fingerprints {
		names = append(names, name)
		taken[FrameworkIdentifier(name).ToPascalCase()] = struct{}{}
	}

	// Sorted so that the names assigned are stable across runs.
	sort.Strings(names)

	for _, name := range names {
		fps := fingerprints[name]

		if len(fps) <= 1 {
			continue
		}

		groups := groupByFingerprint(fps)

		if len(groups) == 1 {
			// No conflict: every occurrence of this name has the same schema,
			// so sharing a single generated type is correct.
			continue
		}

		canonical := canonicalGroup(groups)

		for i, group := range groups {
			if i == canonical {
				continue // Keeps the bare name.
			}

			newName := disambiguate(group[0], name, taken)

			taken[FrameworkIdentifier(newName).ToPascalCase()] = struct{}{}

			// Applied to every occurrence in the group so that occurrences
			// sharing a shape keep sharing a single generated type.
			for _, fp := range group {
				resolveAtPath(g.Attributes, fp.attrPath, newName)
			}
		}
	}
}

// groupByFingerprint partitions occurrences by schema fingerprint, preserving
// the order in which each distinct fingerprint was first seen.
func groupByFingerprint(fps []typeFingerprint) [][]typeFingerprint {
	var groups [][]typeFingerprint

	index := make(map[string]int)

	for _, fp := range fps {
		if i, ok := index[fp.fingerprint]; ok {
			groups[i] = append(groups[i], fp)
			continue
		}

		index[fp.fingerprint] = len(groups)
		groups = append(groups, []typeFingerprint{fp})
	}

	return groups
}

// canonicalGroup returns the index of the group that keeps the bare name: the
// one containing the shallowest occurrence, breaking ties on the attribute path.
func canonicalGroup(groups [][]typeFingerprint) int {
	canonical := 0

	var best *typeFingerprint

	for i := range groups {
		for j := range groups[i] {
			fp := &groups[i][j]

			if best == nil || occurrenceLess(fp, best) {
				best = fp
				canonical = i
			}
		}
	}

	return canonical
}

// occurrenceLess orders occurrences by nesting depth, then by attribute path.
func occurrenceLess(a, b *typeFingerprint) bool {
	if a.depth != b.depth {
		return a.depth < b.depth
	}

	for i := 0; i < len(a.attrPath) && i < len(b.attrPath); i++ {
		if a.attrPath[i] != b.attrPath[i] {
			return a.attrPath[i] < b.attrPath[i]
		}
	}

	return len(a.attrPath) < len(b.attrPath)
}

// disambiguate returns a type name for an occurrence that cannot keep the bare
// name. It walks up the ancestor chain, trying "parent_name", then
// "grandparent_parent_name", and so on, returning the first candidate that is
// not already taken. A numeric suffix is the deterministic last resort.
func disambiguate(fp typeFingerprint, name string, taken map[string]struct{}) string {
	// attrPath is [root, ..., parent, name], so everything but the last element
	// is the ancestor chain.
	ancestors := fp.attrPath

	if len(ancestors) > 0 {
		ancestors = ancestors[:len(ancestors)-1]
	}

	for i := len(ancestors) - 1; i >= 0; i-- {
		candidate := strings.Join(append(append([]string{}, ancestors[i:]...), name), "_")

		if _, ok := taken[FrameworkIdentifier(candidate).ToPascalCase()]; !ok {
			return candidate
		}
	}

	base := strings.Join(append(append([]string{}, ancestors...), name), "_")

	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s_%d", base, n)

		if _, ok := taken[FrameworkIdentifier(candidate).ToPascalCase()]; !ok {
			return candidate
		}
	}
}

// collectFingerprints recursively walks attributes and collects type fingerprints.
func collectFingerprints(attrs GeneratorAttributes, fps map[string][]typeFingerprint, depth int, path []string) {
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
				attrPath:    currentPath,
			})

			// Recurse into nested attributes
			collectFingerprints(nestedAttrs, fps, depth+1, currentPath)
		}
	}
}

// collectBlockFingerprints recursively walks blocks and collects type fingerprints.
func collectBlockFingerprints(blocks GeneratorBlocks, fps map[string][]typeFingerprint, depth int, path []string) {
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
				attrPath:    currentPath,
			})

			collectFingerprints(nestedAttrs, fps, depth+1, currentPath)
		}

		if nested, ok := block.(Blocks); ok {
			nestedBlocks := nested.GetBlocks()
			collectBlockFingerprints(nestedBlocks, fps, depth+1, currentPath)
		}
	}
}

// fingerprintMaxDepth caps how far computeAttrFingerprint recurses. The IR is a
// tree so there are no true cycles; the cap is insurance against a pathologically
// deep schema, and is far beyond any depth the framework can usefully generate.
const fingerprintMaxDepth = 10

// computeAttrFingerprint computes a string fingerprint from a GeneratorAttributes
// map. Two attributes with the same fingerprint have the same schema structure,
// recursively, up to fingerprintMaxDepth levels of nesting.
//
// The fingerprint must capture nested structure, not just the immediate children:
// two same-named objects that differ only in a grandchild would otherwise be
// treated as interchangeable and one would silently win, producing a schema that
// compiles but describes the wrong shape.
func computeAttrFingerprint(attrs GeneratorAttributes) string {
	return computeAttrFingerprintDepth(attrs, fingerprintMaxDepth)
}

func computeAttrFingerprintDepth(attrs GeneratorAttributes, remaining int) string {
	keys := attrs.SortedKeys()
	var parts []string
	for _, k := range keys {
		a := attrs[k]
		if a == nil {
			continue
		}

		part := fmt.Sprintf("%s:%T", k, a)

		if remaining > 0 {
			if nested, ok := a.(Attributes); ok {
				part += "{" + computeAttrFingerprintDepth(nested.GetAttributes(), remaining-1) + "}"
			}
		}

		parts = append(parts, part)
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
