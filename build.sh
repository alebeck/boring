#!/bin/bash
set -e

mkdir -p ./bin
go build -o ./bin/boring ./cmd/boring
go build -o ./bin/boringd ./cmd/boringd
