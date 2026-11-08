// Command protoc-gen-twirpcs is protoc-gen-twirp_csharp under a shorter name, for the flag
// --twirpcs_out:
//
//	protoc --csharp_out=out --twirpcs_out=out service.proto
package main

import "github.com/pabloFuente/twirp-csharp/internal/generator"

// version is stamped by release builds; go install builds read the module version instead.
var version string

func main() {
	generator.Main("protoc-gen-twirpcs", version)
}
