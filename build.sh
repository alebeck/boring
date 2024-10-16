#!/bin/bash

tag=$(git describe --tags --exact-match 2>/dev/null)
commit=$(git rev-parse --short HEAD 2>/dev/null)

mkdir -p ./bin

go build \
    -ldflags "-X main.version=$tag -X main.commit=$commit" \
    -o ./bin/boring \
    ./cmd/boring

