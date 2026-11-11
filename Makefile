.PHONY: e2e-windows-capture e2e-windows-physical e2e-macos-capture build winres macos-helpers dist archives packages release-files test vet fmt check e2e-linux e2e-linux-capture e2e-desktop e2e-windows e2e-linux-real selftest web-test screenshots flash firmware clean

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS = -s -w -X main.version=$(VERSION)

build:                ## build for this machine (dist/typedeck)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/typedeck ./cmd/typedeck

winres:               ## manifest, version info and icon for the Windows executables (cmd/typedeck/rsrc_windows_<arch>.syso)
	@for arch in amd64 arm64; do \
	  go run ./packaging/windows/mkres -arch $$arch -version $(VERSION) -manifest packaging/windows/typedeck.manifest -icon packaging/windows/typedeck.ico -o cmd/typedeck/rsrc_windows_$$arch.syso || exit 1; \
	done

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

dist: winres          ## build for macOS, Linux and Windows (dist/typedeck-<os>-<arch>, plus typedeck-cli-windows-<arch>.exe for terminals) with SHA256 sums
	@mkdir -p dist
	@for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
	  os=$${t%/*}; arch=$${t#*/}; ext=""; flags="$(LDFLAGS)"; \
	  if [ $$os = windows ]; then ext=".exe"; flags="$$flags -H=windowsgui"; fi; \
	  echo "$$os/$$arch"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$$flags" -o dist/typedeck-$$os-$$arch$$ext ./cmd/typedeck || exit 1; \
	  if [ $$os = windows ]; then CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o dist/typedeck-cli-$$os-$$arch.exe ./cmd/typedeck || exit 1; fi; \
	done
	cd dist && sha256sum typedeck-* > SHA256SUMS

NFPM ?= nfpm
PKGVERSION = $(patsubst v%,%,$(VERSION))

archives: $(if $(PREBUILT),,dist)        ## tar.gz and zip archives with the binary, README, LICENSE and NOTICE (dist/typedeck_<version>_<os>_<arch>)
	@for t in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do \
	  os=$${t%/*}; arch=$${t#*/}; ext=""; [ $$os = windows ] && ext=".exe"; \
	  name=typedeck_$(PKGVERSION)_$${os}_$${arch}; dir=dist/$$name; \
	  rm -rf $$dir; mkdir -p $$dir; cp dist/typedeck-$$os-$$arch$$ext $$dir/typedeck$$ext; cp README.md LICENSE NOTICE $$dir/; \
	  if [ $$os = windows ]; then cp dist/typedeck-cli-$$os-$$arch.exe $$dir/typedeck-cli.exe; else chmod 755 $$dir/typedeck; fi; \
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
	  $(NFPM) package -p $$fmt -f dist/nfpm-$$arch.yaml -t dist/ || exit 1; \
	done; done

release-files: archives packages   ## everything a release publishes, with one SHA256SUMS (PREBUILT=1 keeps the binaries already in dist/)
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

e2e-linux: build      ## end-to-end test on Linux with a simulated board (no hardware needed)
	python3 tests/e2e/fakeboard_linux.py dist/typedeck

e2e-linux-capture: build  ## software capture on Linux against uinput virtual keyboards (needs a throwaway Linux machine or VM, access to /dev/uinput and /dev/input, never your desktop)
	python3 tests/e2e/linux_capture.py dist/typedeck

e2e-desktop: build    ## Linux desktop features on a virtual X11 (needs Xvfb, openbox, xdotool, xclip, xterm, x11-utils)
	python3 tests/e2e/linux_desktop.py dist/typedeck
	python3 tests/e2e/linux_focus.py dist/typedeck

e2e-windows-capture: build  ## software capture on a Windows virtual machine with keys sent by the hypervisor (WIN_HOST, WIN_VMID)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o dist/typedeck-win.exe ./cmd/typedeck
	python3 tests/e2e/windows_capture.py dist/typedeck-win.exe

e2e-windows-physical:  ## software capture on a real Windows PC with its own keyboard, lock and suspend (WIN_HOST; asks you to press F6; SUSPEND=1 adds suspend)
	python3 tests/e2e/windows_physical.py

e2e-macos-capture:    ## software capture on a real Mac with its own keyboard (MAC_HOST; asks you to press F6; the helper needs Accessibility and Input Monitoring)
	python3 tests/e2e/macos_capture.py

e2e-windows:          ## full test run on a Windows machine with the board (WIN_HOST, WIN_DIR, WIN_COM_PORT)
	scripts/e2e-windows.sh

e2e-linux-real:       ## full test run on a Linux machine with the board (LINUX_HOST)
	scripts/e2e-linux-real.sh

selftest:             ## editor self test in a headless Chromium
	scripts/selftest.sh

web-test:             ## unit tests of the editor modules (Node 20 or newer)
	node --test tests/web/*.test.mjs

screenshots:          ## regenerate docs/images
	scripts/screenshots.sh

clean:
	rm -rf dist cmd/typedeck/*.syso
