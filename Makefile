VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
ARCH    ?= aarch64_cortex-a53

.PHONY: test lint build ipk clean

test:
	go test ./... -race -covermode=atomic -coverprofile=cover.out

lint:
	golangci-lint run ./...

build:
	CGO_ENABLED=0 go build ./...

# Cross-compile the router daemon (static, no libc dependency).
dist/geolocd:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-ldflags "-s -w -X main.Version=$(VERSION)" -o dist/geolocd ./cmd/geolocd

ipk: dist/geolocd
	sh packaging/openwrt/build-ipk.sh dist/geolocd $(VERSION) $(ARCH) dist

clean:
	rm -rf dist cover.out
