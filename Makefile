build:
	go build ./cmd/tfplugingen-framework

lint:
	golangci-lint run

fmt:
	gofmt -s -w -e .

test:
	go test $$(go list ./... | grep -v /output) -v -cover -timeout=120s -parallel=4

# Generate copywrite headers.
#
# Do not run this without checking what it would do first (append --plan to the
# go:generate directive in tools/copywrite.go). It generates no code -- it only
# stamps license headers -- and current copywrite releases default the copyright
# holder to "IBM Corp." following the HashiCorp acquisition, so running it here
# rewrites the header of every file in the repo. This fork keeps the upstream
# HashiCorp MPL-2.0 notices; new files should copy the two-line header from a
# neighbouring file by hand.
#
# The target also does not build as-is: this module's resolved knadh/koanf
# versions are incompatible with copywrite v0.25.3. Invoking copywrite with its
# own pins (go run github.com/hashicorp/copywrite@v0.25.3) works around that.
generate:
	cd tools; go generate ./...

# Regenerate testdata folder
testdata:
	go run ./cmd/tfplugingen-framework generate all \
		--input ./internal/cmd/testdata/custom_and_external/ir.json \
		--package specified \
		--output ./internal/cmd/testdata/custom_and_external/all_output/specified_pkg_name

	go run ./cmd/tfplugingen-framework generate all \
		--input ./internal/cmd/testdata/custom_and_external/ir.json \
		--output ./internal/cmd/testdata/custom_and_external/all_output/default_pkg_name

	go run ./cmd/tfplugingen-framework generate resources \
		--input ./internal/cmd/testdata/custom_and_external/ir.json \
		--package generated \
		--output ./internal/cmd/testdata/custom_and_external/resources_output

	go run ./cmd/tfplugingen-framework generate data-sources \
		--input ./internal/cmd/testdata/custom_and_external/ir.json \
		--package generated \
		--output ./internal/cmd/testdata/custom_and_external/data_sources_output

	go run ./cmd/tfplugingen-framework generate data-sources \
		--input ./internal/cmd/testdata/sibling_collision/ir.json \
		--package generated \
		--output ./internal/cmd/testdata/sibling_collision/data_sources_output

	go run ./cmd/tfplugingen-framework generate data-sources \
		--input ./internal/cmd/testdata/shared_nested_subtree/ir.json \
		--package generated \
		--output ./internal/cmd/testdata/shared_nested_subtree/data_sources_output

	go run ./cmd/tfplugingen-framework generate provider \
		--input ./internal/cmd/testdata/custom_and_external/ir.json \
		--package generated \
		--output ./internal/cmd/testdata/custom_and_external/provider_output

	go run ./cmd/tfplugingen-framework scaffold resource \
		--name thing \
		--force \
		--package scaffold \
		--output-dir ./internal/cmd/testdata/scaffold/resource

	go run ./cmd/tfplugingen-framework scaffold data-source \
		--name thing \
		--force \
		--package scaffold \
		--output-dir ./internal/cmd/testdata/scaffold/data_source

	go run ./cmd/tfplugingen-framework scaffold provider \
		--name examplecloud \
		--force \
		--package scaffold \
		--output-dir ./internal/cmd/testdata/scaffold/provider

.PHONY: lint fmt test
