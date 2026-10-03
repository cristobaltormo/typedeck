.PHONY: build dist archives packages release-files test vet fmt check e2e e2e-linux e2e-desktop selftest web-test screenshots install flash firmware clean

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

NFPM ?= nfpm
PKGVERSION = $(patsubst v%,%,$(VERSION))

archives: dist        ## tar.gz and zip archives with the binary, README, LICENSE and NOTICE (dist/typedeck_<version>_<os>_<arch>)
	@for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
	  os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=".exe"; \
	  name=typedeck_$(PKGVERSION)_$${os}_$${arch}; dir=dist/$$name; \
	  rm -rf $$dir; mkdir -p $$dir; cp dist/typedeck-$$os-$$arch$$ext $$dir/typedeck$$ext; cp README.md LICENSE NOTICE $$dir/; \
	  if [ $$os = windows ]; then (cd dist && python3 -m zipfile -c $$name.zip $$name) || exit 1; else tar -C dist -czf dist/$$name.tar.gz $$name || exit 1; fi; \
	  rm -rf $$dir; \
	done

packages: dist        ## .deb and .rpm for Linux amd64 and arm64 (needs nfpm)
	@for arch in amd64 arm64; do for fmt in deb rpm; do \
	  sed -e "s/@ARCH@/$$arch/g" -e "s/@VERSION@/$(PKGVERSION)/g" packaging/nfpm.yaml > dist/nfpm-$$arch.yaml; \
	  $(NFPM) package -p $$fmt -f dist/nfpm-$$arch.yaml -t dist/ || exit 1; \
	done; done

release-files: archives packages   ## everything a release publishes, with one SHA256SUMS
	cd dist && rm -f SHA256SUMS && sha256sum typedeck-* typedeck_* > SHA256SUMS

test:                 ## unit tests with the race detector
	go test -race ./...

vet:                  ## static analysis for the three systems
	go vet ./...
	GOOS=windows go vet ./...
	GOOS=darwin go vet ./...

fmt:
	gofmt -l -w cmd internal web

check: fmt vet test web-test   ## what must pass before every commit

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

e2e-desktop: build    ## Linux desktop features on a virtual X11 (needs Xvfb, openbox, xdotool, xclip, xterm, x11-utils)
	python3 tests/e2e/linux_desktop.py dist/typedeck
	python3 tests/e2e/linux_focus.py dist/typedeck

selftest:             ## editor self test in a headless Chromium
	scripts/selftest.sh

web-test:             ## unit tests of the editor modules (Node 20 or newer)
	node --test tests/web/*.test.mjs

screenshots:          ## regenerate docs/images
	scripts/screenshots.sh

clean:
	rm -rf dist
