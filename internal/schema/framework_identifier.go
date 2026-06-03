// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FrameworkIdentifier is a string that implements helpful methods for validating and converting identifier names that are valid in Terraform Plugin Framework
type FrameworkIdentifier string

// frameworkIdentifierRegex is used to validate that FrameworkIdentifier adheres to the same naming conventions as
// [Terraform Plugin Framework identifiers].
//
// [Terraform Plugin Framework identifiers]: https://github.com/hashicorp/terraform-plugin-framework/blob/e036d9fbab4b72f8ec671a9d3e94649040e3eeb5/internal/fwschema/attribute_name_validation.go#L61
var frameworkIdentifierRegex = regexp.MustCompile("^[a-z_][a-z0-9_]*$")

// snakeLetters will match to the first letter and an underscore followed by a letter
var snakeLetters = regexp.MustCompile("(^[a-z])|_[a-z0-9]")

// Valid will return whether the identifier string is a valid identifier in Terraform Plugin Framework
func (identifier FrameworkIdentifier) Valid() bool {
	return frameworkIdentifierRegex.MatchString(string(identifier))
}

// ToCamelCase will return a camel case formatted string of the identifier.
// Example:
//   - example_resource_thing -> exampleResourceThing
func (identifier FrameworkIdentifier) ToCamelCase() string {
	pascal := identifier.ToPascalCase()

	// Grab first rune and lower case it
	firstLetter, size := utf8.DecodeRuneInString(pascal)
	if firstLetter == utf8.RuneError && size <= 1 {
		return pascal
	}

	return string(unicode.ToLower(firstLetter)) + pascal[size:]
}

// ToSafeCamelCase returns the camel case identifier, but appends an underscore
// if the result is a Go reserved keyword. Use this instead of ToCamelCase when
// the result will be used as a standalone Go identifier (e.g., a variable name),
// rather than concatenated with a suffix like "Val" or "Attribute".
func (identifier FrameworkIdentifier) ToSafeCamelCase() string {
	result := identifier.ToCamelCase()

	if isGoKeyword(result) {
		return result + "_"
	}

	return result
}

// ToPrefixCamelCase will return a camel case formatted string of the identifier,
// prefixed with a camel-cased version of the supplied name if the identifier is
// a generated custom value method name.
// Example:
//   - equal(something) -> somethingEqual
//   - type(something) -> somethingType
func (identifier FrameworkIdentifier) ToPrefixCamelCase(prefix string) string {
	pascalCase := identifier.ToPascalCase()

	methodNames := identifier.methodNames()

	if slices.Contains(methodNames, pascalCase) {
		return FrameworkIdentifier(prefix + pascalCase).ToCamelCase()
	}

	return FrameworkIdentifier(pascalCase).ToSafeCamelCase()
}

// ToPascalCase will return a pascal case formatted string of the identifier.
// Example:
//   - example_resource_thing -> ExampleResourceThing
func (identifier FrameworkIdentifier) ToPascalCase() string {
	return snakeLetters.ReplaceAllStringFunc(string(identifier), func(s string) string {
		return strings.ToUpper(strings.Replace(s, "_", "", -1))
	})
}

// ToPrefixPascalCase will return a pascal case formatted string of the identifier,
// prefixed with a pascal-cased version of the supplied name if the identifier is
// a generated custom value method name.
// Example:
//   - equal(something) -> SomethingEqual
//   - type(something) -> SomethingType
func (identifier FrameworkIdentifier) ToPrefixPascalCase(prefix string) string {
	pascalCase := identifier.ToPascalCase()

	methodNames := identifier.methodNames()

	if slices.Contains(methodNames, pascalCase) {
		return FrameworkIdentifier(prefix).ToPascalCase() + pascalCase
	}

	return pascalCase
}

// ToString returns the FrameworkIdentifier as a string without any formatting.
// Example:
//   - example_resource_thing -> example_resource_thing
func (identifier FrameworkIdentifier) ToString() string {
	return string(identifier)
}

// methodNames returns a slice containing generated method names for custom value
// types.
func (identifier FrameworkIdentifier) methodNames() []string {
	return []string{
		"AttributeTypes",
		"Equal",
		"IsNull",
		"IsUnknown",
		"String",
		"ToObjectValue",
		"ToTerraformValue",
		"Type",
	}
}

// goKeywords contains all Go reserved keywords that cannot be used as identifiers.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// isGoKeyword returns true if the given string is a Go reserved keyword.
func isGoKeyword(s string) bool {
	return goKeywords[s]
}
