.PHONY: build test race lint install install-glue uninstall-glue smoke

# Where the glue script + tmux config get installed by `make install-glue`.
GLUE_BIN  ?= $(HOME)/.local/bin
GLUE_CONF ?= $(HOME)/.config/fvmux

build:
	go build ./...

test:
	go test ./...

race:
	go test -race ./...

lint:
	go vet ./...

install:
	go install ./cmd/fvmux

# install-glue lays down the fvmuxa wrapper + its private tmux config.
# Run `make install` first so `fvmux` itself is on PATH; the wrapper
# checks for it at launch. The same files are embedded into the fvmux
# binary and the first-run wizard offers to install them — this target
# is just the non-interactive equivalent for source checkouts.
install-glue:
	@mkdir -p $(GLUE_BIN) $(GLUE_CONF)
	@install -m 0755 internal/glue/fvmuxa $(GLUE_BIN)/fvmuxa
	@install -m 0644 internal/glue/fvmux.tmux.conf $(GLUE_CONF)/fvmux.tmux.conf
	@echo "installed:"
	@echo "  $(GLUE_BIN)/fvmuxa"
	@echo "  $(GLUE_CONF)/fvmux.tmux.conf"
	@echo
	@echo "Add $(GLUE_BIN) to PATH if it isn't already, then run 'fvmuxa'."

uninstall-glue:
	@rm -f $(GLUE_BIN)/fvmuxa $(GLUE_CONF)/fvmux.tmux.conf
	@echo "uninstalled fvmuxa + fvmux.tmux.conf"

smoke:
	@if [ -x test/smoke/run.sh ]; then test/smoke/run.sh; else echo "smoke scripts not yet present (sub-step 12)"; fi
