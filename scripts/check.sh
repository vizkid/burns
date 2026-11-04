#!/bin/sh
# Format, vet, test, and keep every Go file small enough to read in one sitting.
set -eu
cd "$(dirname "$0")/.."

unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then
	echo "gofmt needed:"; echo "$unformatted"; exit 1
fi
go vet ./...
go test -race -count=1 -timeout 2m ./...

limit=300
over=$(find cmd internal -name '*.go' -exec wc -l {} + | awk -v l=$limit '$2 != "total" && $1 >= l {print}')
if [ -n "$over" ]; then
	echo "Files of $limit lines or more:"; echo "$over"; exit 1
fi

# The New Agent Session Rex action, where Rex runs: Rex is the only Lua it runs under.
if command -v rex >/dev/null; then
	rex do scripts/rex/new-agent-session_test.lua >/dev/null
fi

# The apps' shared Swift package, where a Swift toolchain exists (macOS).
if command -v swift >/dev/null && [ "$(uname -s)" = Darwin ]; then
	(cd apps/BurnsKit && swift test --quiet)
fi
