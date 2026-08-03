// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package schema

import (
	"bytes"
	"fmt"
)

// DedupeGenerated coordinates the generation of a named custom Type/Value pair,
// ensuring each name is declared exactly once and that every request for a given
// name asks for the same declaration.
//
// The generated map is shared across a whole schema. It serves two purposes:
//
//   - A recursion guard. Before render is called the name is recorded with a nil
//     value, so a subtree that reaches itself again does not recurse forever.
//   - A deduplication record. Two attributes may legitimately share a name when
//     they have the same shape, in which case only the first declaration is
//     emitted and later requests return nothing.
//
// When a name is requested a second time, the declaration is re-rendered and
// compared byte-for-byte against the one already emitted. Identical output means
// genuine sharing and nothing further is emitted. Differing output means two
// distinct object shapes are competing for one Go identifier, which
// ResolveTypeNameConflicts should have separated; that is reported as an error
// rather than silently dropping the second subtree, which would leave the schema
// referencing types that were never declared.
func DedupeGenerated(name string, generated map[string][]byte, render func() ([]byte, error)) ([]byte, error) {
	prev, seen := generated[name]

	// A nil entry is the recursion sentinel: we are already inside a render of
	// this name, so emit nothing and let the in-progress call finish.
	if seen && prev == nil {
		return nil, nil
	}

	if seen {
		// Re-render to compare. The sentinel must be reinstated for the duration
		// so that a self-referential subtree terminates, and the previously
		// emitted bytes restored afterwards.
		generated[name] = nil

		current, err := render()

		generated[name] = prev

		if err != nil {
			return nil, err
		}

		if !bytes.Equal(prev, current) {
			return nil, fmt.Errorf("duplicate custom type %q generated with differing schemas: "+
				"two distinct object shapes share this attribute name and could not be disambiguated", name)
		}

		// Same shape, already declared.
		return nil, nil
	}

	generated[name] = nil

	b, err := render()

	if err != nil {
		// Drop the sentinel so a later attempt is not mistaken for a
		// completed declaration.
		delete(generated, name)

		return nil, err
	}

	generated[name] = b

	return b, nil
}
