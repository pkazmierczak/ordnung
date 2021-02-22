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
CTIMEVAR=-X github.com/pkazmierczak/photoordnung/version.GITCOMMIT=$(GITCOMMIT) -X github.com/pkazmierczak/photoordnung/version.VERSION=$(VERSION)
GO_LDFLAGS=-ldflags "-w $(CTIMEVAR)"

release:
	CGO_ENABLED=0 go build ${GO_LDFLAGS} -o photoordnung cmd/photoordnung/main.go
install:
	cp photoordnung /usr/local/bin