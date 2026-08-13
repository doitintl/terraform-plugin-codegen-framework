// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cmd_test

import (
	"testing"

	"github.com/doitintl/terraform-plugin-codegen-framework/internal/cmd"
	"github.com/hashicorp/cli"
)

func TestGenerateDataSourcesCommand(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		irInputPath   string
		goldenFileDir string
	}{
		"custom_and_external": {
			irInputPath:   "testdata/custom_and_external/ir.json",
			goldenFileDir: "testdata/custom_and_external/data_sources_output",
		},
		// Several siblings share the attribute name "compute" with a different
		// shape each, while "metric" is shared by two attributes with the same
		// shape. The golden output pins both halves of the behaviour: distinct
		// shapes get distinct type names, identical shapes keep sharing one.
		"sibling_collision": {
			irInputPath:   "testdata/sibling_collision/ir.json",
			goldenFileDir: "testdata/sibling_collision/data_sources_output",
		},
		// Two siblings, "aws" and "doit", have the identical shape, so the
		// "credits" object nested inside both legitimately shares one generated
		// type. "credits" in turn nests a "cost" object, which only the
		// occurrence rendered first emits. The golden output pins that this is
		// treated as sharing rather than as a conflict.
		"shared_nested_subtree": {
			irInputPath:   "testdata/shared_nested_subtree/ir.json",
			goldenFileDir: "testdata/shared_nested_subtree/data_sources_output",
		},
	}
	for name, testCase := range testCases {

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			testOutputDir := t.TempDir()
			mockUi := cli.NewMockUi()
			c := cmd.GenerateDataSourcesCommand{
				UI: mockUi,
			}

			args := []string{
				"--input", testCase.irInputPath,
				"--package", "generated",
				"--output", testOutputDir,
			}

			exitCode := c.Run(args)
			if exitCode != 0 {
				t.Fatalf("unexpected error running `generate data-sources` cmd: %s", mockUi.ErrorWriter.String())
			}

			compareDirectories(t, testCase.goldenFileDir, testOutputDir)
		})
	}
}
