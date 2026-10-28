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
over=$(find cmd internal -name '*.go' -exec wc -l {} + | awk -v l=$limit '$2 != "total" && $1 > l {print}')
if [ -n "$over" ]; then
	echo "Files over $limit lines:"; echo "$over"; exit 1
fi
