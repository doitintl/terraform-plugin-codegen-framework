package convert

import (
	"fmt"
	"regexp"
	"strings"
)

// oneOfRegex matches "stringvalidator.OneOf(...)" and captures the content inside.
var oneOfRegex = regexp.MustCompile(`stringvalidator\.OneOf\(([\s\S]*?)\)`)

// AppendValidators appends possible values from validators to the description.
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
		matches := oneOfRegex.FindStringSubmatch(custom.SchemaDefinition)
		if len(matches) > 1 {
			// matches[1] contains the arguments inside OneOf(...)
			// e.g. " \"a\", \"b\", " or "\n\"a\",\n\"b\",\n"

			// Simple extraction: verify it looks like a list of strings
			args := matches[1]

			// Split by comma
			parts := strings.Split(args, ",")
			var values []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				p = strings.Trim(p, "\"") // remove quotes
				if p != "" {
					values = append(values, fmt.Sprintf("`%s`", p))
				}
			}

			if len(values) > 0 {
				suffix := fmt.Sprintf("Possible values: %s", strings.Join(values, ", "))
				if *d.description != "" {
					*d.description += "\n"
				}
				*d.description += suffix
			}
		}
	}
}
