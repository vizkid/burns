VERSION ?= dev
LDFLAGS := -X github.com/vizkid/burns/internal/usage.Version=$(VERSION)

.PHONY: build check

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/smithersd ./cmd/smithersd
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/smithers ./cmd/smithers

check:
	scripts/check.sh
