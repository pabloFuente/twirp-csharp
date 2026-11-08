#!/usr/bin/env bash
# Generates C# from test/protos with both plugin binaries, compiles it at the lowest level the
# generated code supports (netstandard2.0, C# 7.3, warnings as errors) and runs the C# tests against
# it. Needs Go, protoc and the .NET SDK on PATH; extra arguments go to dotnet test.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
GEN="$ROOT/test/csharp/Acme.Generated/Generated"
BIN=$(mktemp -d)
trap 'rm -rf "$BIN"' EXIT

(cd "$ROOT" && go build -o "$BIN/" ./cmd/...)

rm -rf "$GEN"
mkdir -p "$GEN/messages" "$GEN/clients" "$GEN/legacy"
PROTOS=(acme/shop/v1/shop.proto acme/inventory.proto acme/editions.proto acme/system.proto)
cd "$ROOT/test/protos"

# Next to the namespace Acme.System, where every System.X not written as global::System.X would point
protoc --csharp_out="$GEN/messages" \
    --plugin=protoc-gen-twirp_csharp="$BIN/protoc-gen-twirp_csharp" \
    --twirp_csharp_opt=namespace=Acme.Clients,className=AcmeApi \
    --twirp_csharp_out="$GEN/clients" \
    "${PROTOS[@]}"

# The short name of the plugin, with the other options: global namespace, an exception type of the
# project and routes relative to the base address
protoc --plugin=protoc-gen-twirpcs="$BIN/protoc-gen-twirpcs" \
    --twirpcs_opt=className=LegacyApi,pathPrefix=rpc,errorFactory=Acme.Errors.CustomErrors.FromResponse \
    --twirpcs_out="$GEN/legacy" \
    "${PROTOS[@]}"

dotnet test "$ROOT/test/csharp/Acme.Tests" "$@"
