# Changelog

## 1.0.0

First release of `protoc-gen-twirp_csharp`, a protoc plugin that generates C# clients for Twirp
services, also available as `protoc-gen-twirpcs`.

- One class per `protoc` run with a static method per RPC, posting the protobuf-encoded request
  with an `HttpClient`, plus an overload with `configureRequest`, to set headers on that request,
  and `cancellationToken`. Every `await` uses `ConfigureAwait(false)`.
- Request and response types named as `protoc`'s C# generator names them, and every type written
  with `global::`, so the class compiles next to any namespace or type of the project.
- Errors as the Twirp specification and its Go client define them: `Exception` with the Twirp
  error code, the HTTP status and the `meta` map, and non-Twirp responses mapped from their HTTP
  status. Or an exception of your own through the `errorFactory` option.
- Options `className`, `namespace`, `pathPrefix` and `errorFactory`.
- An internal constant with the route of every RPC, `<class>.Routes.<Service>.<Method>`.
- Generated code for C# 7.3 and `netstandard2.0`, depending only on `Google.Protobuf`.
- Protos need no `go_package`; editions up to 2023 are supported; streaming RPCs and clashing names
  fail the generation with a clear message.
- `--version`, and prebuilt binaries for Linux, macOS and Windows on x64 and arm64.
