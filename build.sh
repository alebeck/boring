#!/bin/bash

tag=$(git describe --tags --exact-match 2>/dev/null)
commit=$(git rev-parse --short HEAD 2>/dev/null)

dir=${1:-./bin}
mkdir -p $dir

go build \
    -ldflags "-s -w -X main.version=$tag -X main.commit=$commit" \
    -o $dir/boring \
    ./cmd/boring
