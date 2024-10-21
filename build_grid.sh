#!/bin/bash

set -e

oss="darwin linux"
archs="amd64 arm64"

tag=$(git describe --tags --exact-match)

for os in $oss; do
	for arch in $archs; do
		GOOS=$os GOARCH=$arch ./build.sh ./dist
		tar -czf ./dist/boring-$tag-$os-$arch.tar.gz LICENSE -C ./dist boring
		rm ./dist/boring
	done
done
