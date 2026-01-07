package convert

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// AppendValidators parses stringvalidator.OneOf validators from the provided Validators object
// and appends the possible values to the Description.
//
// It uses go/parser to safely extract values from the schema definition, handling
// escaped quotes, commas, and other Go syntax correctly.
func (d *Description) AppendValidators(v Validators) {
	if d.description == nil {
		empty := ""
		d.description = &empty
	}

	for _, custom := range v.custom {
		if custom.SchemaDefinition == "" {
			continue
		}

		// Parse the expression
		expr, err := parser.ParseExpr(custom.SchemaDefinition)
		if err != nil {
			// If we can't parse it, we can't extract values safely.
			// Just skip description augmentation.
			continue
		}

		// Inspect the AST to find OneOf calls
		ast.Inspect(expr, func(n ast.Node) bool {
			// We look for function calls
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check if function is stringvalidator.OneOf
			// This could be a SelectorExpr (pkg.Func)
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// Check package name
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Name != "stringvalidator" {
				return true
			}

			// Check function name
			if sel.Sel.Name != "OneOf" {
				return true
			}

			// Extract arguments
			var values []string
			for _, arg := range call.Args {
				// We expect string literals
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}

				// Unquote the string value
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}

				values = append(values, fmt.Sprintf("`%s`", val))
			}

			if len(values) > 0 {
				suffix := fmt.Sprintf("Possible values: %s", strings.Join(values, ", "))

				// Avoid appending if already present
				if strings.Contains(*d.description, suffix) {
					return true
				}

				if *d.description != "" {
					*d.description += "\n"
				}
				*d.description += suffix
			}

			return true
		})
	}
}
