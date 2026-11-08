#!/usr/bin/env bash
# Builds the release archives: both plugin binaries, the LICENSE and the README for every supported
# platform (.tar.gz, .zip for Windows), and checksums.txt with their SHA-256 digests.
# Usage: scripts/build-release.sh <version> <output directory>
set -euo pipefail

VERSION=${1:?usage: $0 <version, e.g. v1.0.0> <output directory>}
OUT=${2:?usage: $0 <version, e.g. v1.0.0> <output directory>}
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: the version must look like v1.2.3, not $VERSION" >&2
    exit 1
fi

ROOT=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
    os=${target%/*}
    arch=${target#*/}
    ext=""
    [ "$os" = windows ] && ext=.exe
    name="protoc-gen-twirp_csharp_${VERSION#v}_${os}_${arch}"
    mkdir -p "$STAGE/$name"
    for cmd in protoc-gen-twirp_csharp protoc-gen-twirpcs; do
        (cd "$ROOT" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
            -ldflags "-s -w -X main.version=$VERSION" -o "$STAGE/$name/$cmd$ext" "./cmd/$cmd")
    done
    cp "$ROOT/LICENSE" "$ROOT/README.md" "$STAGE/$name/"
    if [ "$os" = windows ]; then
        (cd "$STAGE" && zip -qr "$OUT/$name.zip" "$name")
    else
        tar -czf "$OUT/$name.tar.gz" -C "$STAGE" "$name"
    fi
    echo "Built $name"
done

(cd "$OUT" && sha256sum protoc-gen-twirp_csharp_* >checksums.txt)
echo "Wrote $OUT/checksums.txt"
