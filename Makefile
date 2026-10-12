.PHONY: build test install clean release

# Navescript build configuration
NVS_CONFIG = nvs.config.yaml

# Binaries
NVS_BIN = bin/nvs
NVM_BIN = bin/nvm

# Release version: nearest git tag (falls back to short SHA on untagged trees).
# Stamp it into both the CLI (`nvs version`) and the language runtime
# (`nvs_version()` / `nvs_info()` inside scripts).
GIT_VERSION = $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS = -X main.version=$(GIT_VERSION) -X github.com/navescript/nvs/internal/eval.versionStamp=$(GIT_VERSION)

# Sources
GO_SRCS = $(shell find . -name "*.go")
ASM_SRCS = $(shell find internal/nasm_wasm -name "*.asm")
ASM_OBJS = $(ASM_SRCS:.asm=.o)

build: $(NVS_BIN) $(NVM_BIN) build-asm

$(NVS_BIN): $(GO_SRCS)
	go build -o $(NVS_BIN) ./cmd/nvs

$(NVM_BIN): $(GO_SRCS)
	go build -o $(NVM_BIN) ./cmd/nvm

# Assembly compilation (x86_64)
build-asm: $(ASM_OBJS)

%.o: %.asm
	nasm -f elf64 $< -o $@

test:
	go test ./internal/...

# Release build: same binaries, but version-stamped from git.
# (nvm is skipped when its package is absent — pre-existing upstream gap.)
release: $(GO_SRCS)
	go build -ldflags "$(LDFLAGS)" -o $(NVS_BIN) ./cmd/nvs
	@if ls ./cmd/nvm/*.go >/dev/null 2>&1; then \
		go build -ldflags "$(LDFLAGS)" -o $(NVM_BIN) ./cmd/nvm; \
	else \
		echo "note: ./cmd/nvm has no Go sources; skipping nvm (pre-existing)"; \
	fi
	@echo "built $(GIT_VERSION)"

install: build
	cp $(NVS_BIN) /usr/local/bin/nvs
	cp $(NVM_BIN) /usr/local/bin/nvm

clean:
	rm -rf bin/
	rm -f $(ASM_OBJS)
