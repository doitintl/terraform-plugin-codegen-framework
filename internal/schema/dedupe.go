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
// Rendering is split in two so that the two halves can be treated differently:
//
//   - renderDecl produces this node's own Type and Value declarations. It does
//     not recurse, so its output depends only on the node's own shape and on the
//     effective type names of its immediate children.
//   - renderChildren recurses into the node's children, each of which routes
//     back through DedupeGenerated.
//
// The generated map is shared across a whole schema. It serves two purposes:
//
//   - A recursion guard. Before children are rendered the name is recorded with a
//     nil value, so a subtree that reaches itself again does not recurse forever.
//   - A deduplication record. Two attributes may legitimately share a name when
//     they have the same shape, in which case only the first declaration is
//     emitted and later requests return nothing.
//
// Only the declaration bytes are recorded, never the subtree. A subtree's bytes
// depend on which of its descendants happened to be emitted already, so the same
// shape rendered at two points in the walk does not produce the same subtree
// bytes: the occurrence that first introduces a nested child carries that child's
// declaration, later occurrences do not. Comparing declarations instead is
// order-independent — two occupancies of one name that ResolveTypeNameConflicts
// grouped together resolve their children to the same effective type names, so
// their declarations match byte-for-byte.
//
// When a name is requested a second time the declaration is re-rendered and
// compared against the one already emitted. Differing output means two distinct
// object shapes are competing for one Go identifier, which
// ResolveTypeNameConflicts should have separated; that is reported as an error
// rather than silently dropping the second subtree, which would leave the schema
// referencing types that were never declared.
//
// Identical output is not by itself proof that the two subtrees match, because a
// declaration names its immediate children without describing them, so the walk
// continues into the subtree on a repeat and every descendant reaches the same
// comparison. That is what catches a divergence deeper than the fingerprint
// depth cap, or one rooted in a block, where ResolveTypeNameConflicts cannot
// currently rename either occurrence (issue #17). The walk normally emits
// nothing, since the first occurrence already emitted the subtree.
func DedupeGenerated(name string, generated map[string][]byte, renderDecl func() ([]byte, error), renderChildren func() ([]byte, error)) ([]byte, error) {
	prev, seen := generated[name]

	// A nil entry is the recursion sentinel: we are already inside a render of
	// this name, so emit nothing and let the in-progress call finish.
	if seen && prev == nil {
		return nil, nil
	}

	decl, err := renderDecl()

	if err != nil {
		return nil, err
	}

	// A declaration is never empty in practice, but a nil slice would be
	// indistinguishable from the recursion sentinel, so normalise it.
	if decl == nil {
		decl = []byte{}
	}

	if seen {
		if !bytes.Equal(prev, decl) {
			return nil, fmt.Errorf("duplicate custom type %q generated with differing schemas: "+
				"two distinct object shapes share this attribute name and could not be disambiguated", name)
		}

		// Matching declarations are not on their own proof that the two subtrees
		// match. A declaration names its immediate children and their framework
		// types, but says nothing about what those children contain, so two
		// occurrences can declare identically and still diverge further down --
		// below the fingerprint depth cap, or anywhere at all when the conflict
		// is rooted in a block, which ResolveTypeNameConflicts cannot currently
		// rename (see issue #17). Walk the subtree so each descendant reaches
		// its own comparison and a divergence is reported rather than dropped.
		//
		// The sentinel is reinstated for the walk so that a self-referential
		// subtree still terminates, and restored afterwards.
		generated[name] = nil

		children, err := renderChildren()

		generated[name] = prev

		if err != nil {
			return nil, err
		}

		// Normally empty: every descendant was emitted by the first occurrence
		// and dedupes away here. Anything the walk does turn out to be the first
		// to render is returned rather than dropped, so the declaration cannot
		// be recorded without also being emitted.
		if len(children) == 0 {
			return nil, nil
		}

		return children, nil
	}

	// Guard against a descendant reaching this name again while children render.
	generated[name] = nil

	children, err := renderChildren()

	if err != nil {
		// Drop the sentinel so a later attempt is not mistaken for a
		// completed declaration.
		delete(generated, name)

		return nil, err
	}

	generated[name] = decl

	var buf bytes.Buffer

	buf.Write(decl)
	buf.Write(children)

	return buf.Bytes(), nil
}
