// Command protoc-gen-twirp_csharp is a protoc plugin that generates C# clients for Twirp services:
//
//	protoc --csharp_out=out --twirp_csharp_out=out service.proto
package main

import "github.com/pabloFuente/twirp-csharp/internal/generator"

// version is stamped by release builds; go install builds read the module version instead.
var version string

func main() {
	generator.Main("protoc-gen-twirp_csharp", version)
}
