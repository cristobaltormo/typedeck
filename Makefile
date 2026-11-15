.PHONY: build platform macos-helpers dist archives packages release-files sums test lint fmt audit check firmware flash e2e-linux e2e-linux-capture e2e-desktop e2e-wine e2e-windows-capture e2e-windows-physical e2e-macos-capture e2e-windows e2e-linux-real measure selftest web-test screenshots clean

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
export TYPEDECK_VERSION := $(VERSION)
CARGO ?= cargo
ZIGBUILD ?= cargo zigbuild
PLATFORMS := linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64

triple.linux-amd64 := x86_64-unknown-linux-musl
triple.linux-arm64 := aarch64-unknown-linux-musl
triple.darwin-amd64 := x86_64-apple-darwin
triple.darwin-arm64 := aarch64-apple-darwin
triple.windows-amd64 := x86_64-pc-windows-msvc
triple.windows-arm64 := aarch64-pc-windows-msvc

TRIPLE = $(triple.$(PLATFORM))
EXE = $(if $(filter windows-%,$(PLATFORM)),.exe)
BUILD = $(if $(filter linux-%,$(PLATFORM)),$(ZIGBUILD),$(CARGO) build)

build:                ## build for this machine (dist/typedeck)
	$(CARGO) build --release --locked --bin typedeck
	@mkdir -p dist
	cp target/release/typedeck dist/typedeck

platform:             ## build one release target, for example PLATFORM=linux-amd64 (dist/typedeck-<os>-<arch>, plus typedeck-cli on Windows)
	@test -n "$(TRIPLE)" || { echo "PLATFORM must be one of: $(PLATFORMS)" >&2; exit 2; }
	$(BUILD) --release --locked --target $(TRIPLE) --bin typedeck $(if $(EXE),--bin typedeck-cli)
	@mkdir -p dist
	cp target/$(TRIPLE)/release/typedeck$(EXE) dist/typedeck-$(PLATFORM)$(EXE)
	$(if $(EXE),cp target/$(TRIPLE)/release/typedeck-cli.exe dist/typedeck-cli-$(PLATFORM).exe)

MACOS_MIN ?= 12.0

macos-helpers:        ## build the two Swift helpers for macOS (dist/typedeck-capture-macos, dist/typedeck-hud-macos), universal and ad-hoc signed; needs a Mac with swiftc
	@mkdir -p dist
	@for tool in capture hud; do \
	  for arch in arm64 x86_64; do \
	    swiftc -O -target $$arch-apple-macos$(MACOS_MIN) macos/$$tool/$$tool.swift -o dist/typedeck-$$tool-$$arch || exit 1; \
	  done; \
	  lipo -create -output dist/typedeck-$$tool-macos dist/typedeck-$$tool-arm64 dist/typedeck-$$tool-x86_64 || exit 1; \
	  rm -f dist/typedeck-$$tool-arm64 dist/typedeck-$$tool-x86_64; \
	  codesign --force --sign - --identifier io.github.cristobaltormo.typedeck.$$tool dist/typedeck-$$tool-macos || exit 1; \
	done
	dist/typedeck-capture-macos --selftest

dist:                 ## build every release target this machine can build (in CI each one is built on its own system)
	@for p in $(PLATFORMS); do $(MAKE) --no-print-directory platform PLATFORM=$$p || exit 1; done

NFPM ?= nfpm
PKGVERSION = $(patsubst v%,%,$(VERSION))

archives: $(if $(PREBUILT),,dist)        ## tar.gz and zip archives with the binary, README, LICENSE and NOTICE (dist/typedeck_<version>_<os>_<arch>)
	@for p in $(PLATFORMS); do \
	  os=$${p%-*}; arch=$${p#*-}; ext=""; [ $$os = windows ] && ext=".exe"; \
	  name=typedeck_$(PKGVERSION)_$${os}_$${arch}; dir=dist/$$name; \
	  rm -rf $$dir; mkdir -p $$dir; cp dist/typedeck-$$p$$ext $$dir/typedeck$$ext; cp README.md LICENSE NOTICE $$dir/; \
	  if [ $$os = windows ]; then cp dist/typedeck-cli-$$p.exe $$dir/typedeck-cli.exe; else chmod 755 $$dir/typedeck; fi; \
	  if [ $$os = darwin ]; then for tool in capture hud; do \
	    if [ -f dist/typedeck-$$tool-macos ]; then cp dist/typedeck-$$tool-macos $$dir/typedeck-$$tool && chmod 755 $$dir/typedeck-$$tool; \
	    elif [ -n "$(REQUIRE_MACOS_HELPERS)" ]; then echo "dist/typedeck-$$tool-macos is missing"; exit 1; fi; \
	  done; fi; \
	  if [ $$os = windows ]; then (cd dist && python3 -m zipfile -c $$name.zip $$name) || exit 1; else tar -C dist -czf dist/$$name.tar.gz $$name || exit 1; fi; \
	  if [ $$os = darwin ]; then for tool in capture hud; do \
	    [ ! -f $$dir/typedeck-$$tool ] || tar -tvzf dist/$$name.tar.gz | grep -Eq "^-[rwx-]{2}x.* $$name/typedeck-$$tool\$$" || { echo "$$name: typedeck-$$tool is not executable in the archive"; exit 1; }; \
	  done; fi; \
	  rm -rf $$dir; \
	done

packages: $(if $(PREBUILT),,dist)        ## .deb and .rpm for Linux amd64 and arm64 (needs nfpm)
	@for arch in amd64 arm64; do for fmt in deb rpm; do \
	  sed -e "s/@ARCH@/$$arch/g" -e "s/@VERSION@/$(PKGVERSION)/g" packaging/nfpm.yaml > dist/nfpm-$$arch.yaml; \
	  if [ $$fmt = deb ]; then out=typedeck_$(PKGVERSION)_$$arch.deb; else rarch=x86_64; [ $$arch = arm64 ] && rarch=aarch64; out=typedeck-$(PKGVERSION)-1.$$rarch.rpm; fi; \
	  $(NFPM) package -p $$fmt -f dist/nfpm-$$arch.yaml -t dist/$$out || exit 1; \
	  [ -f dist/$$out ] || { echo "nfpm did not write dist/$$out"; exit 1; }; \
	done; done

release-files: archives packages   ## everything a release publishes, with one SHA256SUMS (PREBUILT=1 keeps the binaries already in dist/)
	cd dist && rm -f SHA256SUMS nfpm-*.yaml && sha256sum typedeck-* typedeck_* > SHA256SUMS

test:                 ## unit tests (debug profile)
	$(CARGO) test --locked

lint:                 ## formatting and clippy for this machine
	$(CARGO) fmt --check
	$(CARGO) clippy --all-targets --locked -- -D warnings

fmt:
	$(CARGO) fmt

audit:                ## known vulnerabilities in the dependencies (needs cargo-audit)
	$(CARGO) audit

check: lint test web-test   ## what must pass before every commit

firmware:             ## build the board firmware
	arduino-cli compile --fqbn arduino:avr:leonardo --output-dir /tmp/typedeck-fw firmware

flash:                ## build and flash the board through a Mac
	scripts/flash.sh

e2e-linux: build      ## end-to-end test on Linux with a simulated board (no hardware needed)
	python3 tests/e2e/fakeboard_linux.py dist/typedeck

e2e-linux-capture: build  ## software capture on Linux against uinput virtual keyboards (needs a throwaway Linux machine or VM, access to /dev/uinput and /dev/input, never your desktop)
	python3 tests/e2e/linux_capture.py dist/typedeck

e2e-desktop: build    ## Linux desktop features on a virtual X11 (needs Xvfb, openbox, xdotool, xclip, xterm, x11-utils)
	python3 tests/e2e/linux_desktop.py dist/typedeck
	python3 tests/e2e/linux_focus.py dist/typedeck

e2e-wine:             ## the Windows build under Wine with a simulated board on COM3 (needs wine and the mingw target)
	$(CARGO) build --release --locked --target x86_64-pc-windows-gnu --bin typedeck-cli
	python3 tests/e2e/fakeboard_wine.py target/x86_64-pc-windows-gnu/release/typedeck-cli.exe

e2e-windows-capture:  ## software capture on a Windows virtual machine with keys sent by the hypervisor (WIN_HOST, WIN_VMID)
	$(CARGO) build --release --locked --target x86_64-pc-windows-gnu --bin typedeck-cli
	python3 tests/e2e/windows_capture.py target/x86_64-pc-windows-gnu/release/typedeck-cli.exe

e2e-windows-physical:  ## software capture on a real Windows PC with its own keyboard, lock and suspend (WIN_HOST; asks you to press F6; SUSPEND=1 adds suspend)
	python3 tests/e2e/windows_physical.py

e2e-macos-capture:    ## software capture on a real Mac with its own keyboard (MAC_HOST; asks you to press F6; the helper needs Accessibility and Input Monitoring)
	python3 tests/e2e/macos_capture.py

e2e-windows:          ## full test run on a Windows machine with the board (WIN_HOST, WIN_DIR, WIN_COM_PORT)
	scripts/e2e-windows.sh

e2e-linux-real:       ## full test run on a Linux machine with the board (LINUX_HOST)
	scripts/e2e-linux-real.sh

measure: build        ## memory, idle CPU, start-up time and macro latency against the simulated board (IDLE=seconds)
	python3 tests/e2e/measure.py dist/typedeck $(or $(IDLE),120)

selftest:             ## editor self test in a headless Chromium
	scripts/selftest.sh

web-test:             ## unit tests of the editor modules (Node 20 or newer)
	node --test tests/web/*.test.mjs

screenshots:          ## regenerate docs/images
	scripts/screenshots.sh

clean:
	$(CARGO) clean
	rm -rf dist
