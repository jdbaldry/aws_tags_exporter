GO       ?= go
GOFMT    ?= $(GO)fmt
GOLINT   ?= $(GO)lint
packages ?= $(shell find collector . -maxdepth 1 -name "*.go")

all: build
.PHONY: all

build:
	go build -o _output/bin/aws_tags_exporter.go
.PHONY: build

clean:
	rm -rf _output
.PHONY: clean

format:
	@echo ">> Formatting code: " $(packages)
	$(GOFMT) -s -w . 
.PHONY: format

lint:
	@echo ">> Linting code: " $(packages)
	$(GOLINT) $(packages)
.PHONY: format

update-dependencies:
	dep ensure -update
.PHONY: update-dependencies
