# Populate version variables
# Add to compile time flags
VERSION := $(shell cat VERSION.txt)
GITCOMMIT := $(shell git rev-parse --short HEAD)
GITUNTRACKEDCHANGES := $(shell git status --porcelain --untracked-files=no)
ifneq ($(GITUNTRACKEDCHANGES),)
	GITCOMMIT := $(GITCOMMIT)-dirty
endif
ifeq ($(GITCOMMIT),)
    GITCOMMIT := ${GITHUB_SHA}
endif

# set LDFLAGS and inject version information
CTIMEVAR=-X github.com/pkazmierczak/ordnung/internal/version.GITCOMMIT=$(GITCOMMIT) -X github.com/pkazmierczak/ordnung/internal/version.VERSION=$(VERSION)
GO_LDFLAGS=-ldflags "-w $(CTIMEVAR)"

.PHONY: release install test vet fmt check

release:
	CGO_ENABLED=0 go build ${GO_LDFLAGS} -o ordnung ./cmd/ordnung

install:
	cp ordnung /usr/local/bin

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

# Run the full set of checks (formatting, vet, tests) the way CI does.
check: vet test
