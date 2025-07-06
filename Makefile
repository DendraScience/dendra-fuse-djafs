# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS = -X github.com/dendrascience/dendra-archive-fuse/version.Version=$(VERSION) \
          -X github.com/dendrascience/dendra-archive-fuse/version.Commit=$(COMMIT) \
          -X github.com/dendrascience/dendra-archive-fuse/version.Date=$(DATE)

# Build targets
main: djafs-bin

djafs-bin: *.go internal/cmd/*.go version/*.go djafs/*.go util/*.go
	go build -ldflags "$(LDFLAGS)" -o djafs-bin .

djafs: djafs-bin

all: djafs-bin

clean:  
	@printf "Cleaning up \e[32mall binaries\e[39m...\n"
	rm -f djafs-bin converter validator
	rm -f main dendra-archive-fuse
	rm -f *.zip
	@# Only remove djafs if it's a file, not a directory
	@if [ -f djafs ]; then rm -f djafs; fi

install: clean all
	mv djafs-bin "$$GOPATH/bin/djafs"

vet: 
	@echo "Running go vet..."
	@go vet ./... || (printf "\e[31mGo vet failed, exit code $$?\e[39m\n"; exit 1)
	@printf "\e[32mGo vet success!\e[39m\n"

test:
	@echo "Running tests..."
	@go test ./...

# Development targets
dev-build: 
	@echo "Building development versions..."
	@$(MAKE) all VERSION=dev-$(shell date +%Y%m%d-%H%M%S)

# Release targets
release: clean
	@echo "Building release versions..."
	@$(MAKE) all

.PHONY: main djafs all clean install vet test dev-build release