VERSION ?= $(shell git describe --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/vizkid/burns/internal/usage.Version=$(VERSION)

.PHONY: build check install

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/smithersd ./cmd/smithersd
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/smithers ./cmd/smithers

check:
	scripts/check.sh

install: build
	scripts/install.sh
