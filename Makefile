.PHONY: build dist test vet fmt check e2e e2e-linux install flash firmware clean

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS = -s -w -X main.version=$(VERSION)

build:                ## build for this machine (dist/typedeck)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/typedeck ./cmd/typedeck

dist:                 ## build for macOS, Linux and Windows (dist/typedeck-<os>-<arch>) with SHA256 sums
	@mkdir -p dist
	@for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
	  os=$${t%/*}; arch=$${t#*/}; ext=""; flags="$(LDFLAGS)"; \
	  if [ $$os = windows ]; then ext=".exe"; flags="$$flags -H=windowsgui"; fi; \
	  echo "$$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$$flags" -o dist/typedeck-$$os-$$arch$$ext ./cmd/typedeck || exit 1; \
	done
	cd dist && sha256sum typedeck-* > SHA256SUMS

test:                 ## unit tests with the race detector
	go test -race ./...

vet:                  ## static analysis for the three systems
	go vet ./...
	GOOS=windows go vet ./...
	GOOS=darwin go vet ./...

fmt:
	gofmt -l -w cmd internal web

check: fmt vet test   ## what must pass before every commit

firmware:             ## build the board firmware
	arduino-cli compile --fqbn arduino:avr:leonardo --output-dir /tmp/typedeck-fw firmware

flash:                ## build and flash the board through a Mac
	scripts/flash.sh

install:              ## install on a Mac over ssh (MAC_HOST, default "mac")
	scripts/install-mac.sh $(VERSION)

e2e:                  ## tests against the real board on a Mac (--typing types into TextEdit)
	scripts/e2e-mac.sh $(ARGS)

e2e-linux: build      ## end-to-end test on Linux with a simulated board (no hardware needed)
	python3 tests/e2e/fakeboard_linux.py dist/typedeck

clean:
	rm -rf dist
