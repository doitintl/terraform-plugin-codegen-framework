package convert

import (
	"fmt"
	"regexp"
	"strings"
)

// oneOfRegex matches "stringvalidator.OneOf(...)" and captures the content inside.
var oneOfRegex = regexp.MustCompile(`stringvalidator\.OneOf\(([\s\S]*?)\)`)

// stringLiteralRegex matches a Go string literal (double-quoted).
// It handles escaped quotes.
var stringLiteralRegex = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)

// AppendValidators parses stringvalidator.OneOf validators from the provided Validators object
// and appends the possible values to the Description.
//
// It currently supports:
// - stringvalidator.OneOf
//
// Arguments inside OneOf are expected to be string literals.
// Commas inside quotes are preserved.
func (d *Description) AppendValidators(v Validators) {
	if d.description == nil {
		empty := ""
		d.description = &empty
	}

	for _, custom := range v.custom {
		if custom.SchemaDefinition == "" {
			continue
		}

		// Check for stringvalidator.OneOf
		// Note: This regex might match multiple times if there are multiple OneOf calls,
		// though typically there's only one per validator set.
		allMatches := oneOfRegex.FindAllStringSubmatch(custom.SchemaDefinition, -1)

		for _, matches := range allMatches {
			if len(matches) > 1 {
				// matches[1] contains the arguments inside OneOf(...)
				args := matches[1]

				// Parse string literals from the arguments
				// This avoids issues with commas inside quotes
				literalMatches := stringLiteralRegex.FindAllStringSubmatch(args, -1)

				var values []string
				for _, lm := range literalMatches {
					if len(lm) > 1 {
						// lm[1] is the content inside the quotes
						// We re-wrap it in backticks for markdown display, escaping backticks if needed
						val := lm[1]
						val = strings.ReplaceAll(val, "`", "` + \"`\" + `") // unlikely but safe
						values = append(values, fmt.Sprintf("`%s`", val))
					}
				}

				if len(values) > 0 {
					suffix := fmt.Sprintf("Possible values: %s", strings.Join(values, ", "))

					// Avoid appending if already present (simple duplicate check)
					if strings.Contains(*d.description, suffix) {
						continue
					}

					if *d.description != "" {
						*d.description += "\n"
					}
					*d.description += suffix
				}
			}
		}
	}
}
