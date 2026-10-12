.PHONY: build test install clean release

# Navescript build configuration
NVS_CONFIG = nvs.config.yaml

# Binaries
NVS_BIN = bin/nvs
# nvm is a shell script, not a Go binary — install it alongside nvs.
NVM_SH = cmd/nvm/nvm.sh
NVM_BIN = bin/nvm

# nasm is only needed for the experimental standalone WASM JIT backend
# (internal/nasm_wasm, not wired into the Go build). Skip gracefully
# when nasm isn't installed so `make build` works on a plain Go toolchain.
NASM := $(shell command -v nasm 2>/dev/null)

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

# nvm is a shell script: ship it as-is, don't try to compile it.
$(NVM_BIN): $(NVM_SH)
	mkdir -p bin
	cp $(NVM_SH) $(NVM_BIN)
	chmod +x $(NVM_BIN)

# Assembly compilation (x86_64) — experimental standalone backend.
# Skipped with a note when nasm isn't available; never blocks the build.
build-asm: $(ASM_OBJS)

%.o: %.asm
ifdef NASM
	nasm -f elf64 $< -o $@
else
	@echo "note: nasm not installed; skipping $< (experimental WASM backend, not part of the Go build)"
endif

test:
	go test ./internal/...

# Release build: same binaries, but version-stamped from git.
# (nvm is skipped when its package is absent — pre-existing upstream gap.)
release: $(GO_SRCS)
	go build -ldflags "$(LDFLAGS)" -o $(NVS_BIN) ./cmd/nvs
	$(MAKE) $(NVM_BIN)
	@echo "built $(GIT_VERSION)"

install: build
	mkdir -p /usr/local/bin
	cp $(NVS_BIN) /usr/local/bin/nvs
	cp $(NVM_BIN) /usr/local/bin/nvm

clean:
	rm -rf bin/
	rm -f $(ASM_OBJS)
