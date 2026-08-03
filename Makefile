.PHONY: build test vet fmt check e2e install flash firmware clean

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)

build:                ## build for macOS arm64 (dist/typedeck)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o dist/typedeck ./cmd/typedeck

test:                 ## unit tests with the race detector
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w cmd internal web

check: fmt vet test   ## what must pass before every commit

firmware:             ## build the board firmware
	arduino-cli compile --fqbn arduino:avr:leonardo --output-dir /tmp/typedeck-fw firmware

flash:                ## build and flash the board through the Mac
	scripts/flash.sh

install:              ## install on a Mac over ssh (MAC_HOST, default "mac")
	scripts/install-mac.sh $(VERSION)

e2e:                  ## tests against the real board on the Mac (--typing types into TextEdit)
	scripts/e2e-mac.sh $(ARGS)

clean:
	rm -rf dist
