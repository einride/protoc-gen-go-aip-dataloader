SHELL := /bin/bash

all: \
	commitlint \
	prettier-markdown \
	go-lint \
	go-review \
	go-test \
	go-mod-tidy \
	buf-generate-example \
	git-verify-nodiff

include tools/buf/rules.mk
include tools/commitlint/rules.mk
include tools/git-verify-nodiff/rules.mk
include tools/golangci-lint/rules.mk
include tools/goreview/rules.mk
include tools/prettier/rules.mk
include tools/semantic-release/rules.mk

build/protoc-gen-go: go.mod
	$(info [$@] building binary...)
	@go build -o $@ google.golang.org/protobuf/cmd/protoc-gen-go

build/protoc-gen-go-grpc: $(abspath $(lastword $(MAKEFILE_LIST)))
	$(info [$@] building binary...)
	@GOBIN=$(abspath $(dir $@)) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1.0

.PHONY: build/protoc-gen-go-aip-dataloader
build/protoc-gen-go-aip-dataloader:
	$(info [$@] building binary...)
	@go build -o $@ .

.PHONY: go-mod-tidy
go-mod-tidy:
	$(info [$@] tidying Go module files...)
	@go mod tidy -v

.PHONY: go-test
go-test:
	$(info [$@] running Go tests...)
	@mkdir -p build/coverage
	@go test -short -race -coverprofile=build/coverage/$@.txt -covermode=atomic ./...

example_plugins := \
	build/protoc-gen-go \
	build/protoc-gen-go-grpc \
	build/protoc-gen-go-aip-dataloader \

.PHONY: buf-generate-example
buf-generate-example: $(buf) $(example_plugins)
	$(info [$@] generating example CLI...)
	@rm -rf example/internal/proto/gen
	@$(buf) generate \
		buf.build/einride/aip \
		--template buf.gen.example.yaml \
		--path einride/example/freight
