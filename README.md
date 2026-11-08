# twirp-csharp

A C# client generator for [Twirp](https://github.com/twitchtv/twirp) services: `protoc-gen-twirp_csharp`, a `protoc` plugin that turns the services of your `.proto` files into a C# class that calls them with an `HttpClient`.

## Install

Download the archive for your platform from the [releases](https://github.com/pabloFuente/twirp-csharp/releases) and put `protoc-gen-twirp_csharp` on your `PATH`. `checksums.txt` lists the SHA-256 of every archive, and each one carries a build provenance attestation: `gh attestation verify <archive> --repo pabloFuente/twirp-csharp`.

With Go 1.23 or newer:

```sh
go install github.com/pabloFuente/twirp-csharp/cmd/protoc-gen-twirp_csharp@latest
```

The archives also contain `protoc-gen-twirpcs`, the same plugin under a shorter name, used as `--twirpcs_out` and `--twirpcs_opt` (with Go, `.../cmd/protoc-gen-twirpcs@latest`).

## Generate

`protoc`'s own `--csharp_out` generates the messages, and `--twirp_csharp_out` the client:

```sh
protoc --csharp_out=Generated --twirp_csharp_out=Generated shop.proto
```

The client is one file with the services of every `.proto` given to `protoc` (not those of the files they import). When none declares a service, there is no file.

Options go in `--twirp_csharp_opt`:

```sh
protoc --csharp_out=Generated \
    --twirp_csharp_opt=namespace=Acme.Clients,className=ShopApi \
    --twirp_csharp_out=Generated shop.proto
```

| Option         | Default    |                                                                                                                                                                     |
| -------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `className`    | `TwirpApi` | Name of the class, and of its file.                                                                                                                                 |
| `namespace`    | (global)   | Namespace of the class.                                                                                                                                             |
| `pathPrefix`   | `/twirp`   | Start of every route, the Twirp default. Without a leading `/` (`twirp`), routes are relative to the base address of the `HttpClient`, subpath included.            |
| `errorFactory` | (none)     | Fully qualified static method, without `global::`, that builds the exception of a failed call: `Exception Factory(string body, int status)`. See [Errors](#errors). |

The generated code targets C# 7.3 and `netstandard2.0` (it builds in Unity too), and only needs the [Google.Protobuf](https://www.nuget.org/packages/Google.Protobuf) package, at least as new as the `protoc` that generated the messages.

## Call

For this service:

```protobuf
package acme.shop.v1;

service Shop {
  rpc GetItem(GetItemRequest) returns (Item);
}
```

`ShopApi` has a static method per RPC, named after it in PascalCase, which posts the protobuf-encoded request to `<base address>/twirp/acme.shop.v1.Shop/GetItem` (the service and the method as written in the proto, as the Twirp specification says):

```csharp
using var http = new HttpClient { BaseAddress = new Uri("https://api.example.com") };

var item = await ShopApi.GetItem(http, new GetItemRequest { Id = "42" });

// Headers for this request only, such as a token, and cancellation
item = await ShopApi.GetItem(
    http,
    new GetItemRequest { Id = "42" },
    configureRequest: request => request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token),
    cancellationToken: cancellationToken);
```

The `HttpClient` is never modified, so it can be shared. Every `await` uses `ConfigureAwait(false)`, so blocking on a call from a UI thread does not deadlock. Request and response types have the names `protoc` gives them in C#: the `csharp_namespace` of their own file (or its package in PascalCase), with nested messages under `Types`. Every type in the generated code is written with `global::`, so it compiles next to any namespace or type of your project, such as the namespace `Acme.System` of a proto package `acme.system`. The class also has an internal constant with the route of every RPC, `ShopApi.Routes.Shop.GetItem`, for code of the same assembly that sends requests itself.

## Errors

A failed call throws `ShopApi.Exception`:

```csharp
try
{
    await ShopApi.GetItem(http, new GetItemRequest { Id = "missing" });
}
catch (ShopApi.Exception e)
{
    Console.WriteLine($"{e.Type} ({e.Status}): {e.Message}"); // Not_Found (404): item not found
    if (e.Meta.TryGetValue("retry_after", out var retryAfter)) { /* ... */ }
}
```

`Type` is the Twirp error code (`ShopApi.ErrorCode`), `Status` the HTTP status and `Meta` the `meta` map of the error. A response that is not a Twirp error, such as a proxy's `503` page, gets the code the [Twirp specification](https://twitchtv.github.io/twirp/docs/errors.html#http-errors-from-intermediary-proxies) maps its HTTP status to (`Unavailable` for a 503), with `http_error_from_intermediary`, `status_code` and `body` (or `location` for a redirect) in `Meta`. An unknown error code becomes `Internal`. This is what the Go client of Twirp does.

With `errorFactory`, the generated class calls your method instead, and its `Exception` keeps only `Type`. Return a subclass of it with whatever your API needs:

```sh
--twirp_csharp_opt=errorFactory=Acme.Errors.ApiException.FromResponse
```

## Limitations

- Clients only: there is no server generator.
- Protobuf encoding only, no JSON.
- No streaming RPCs, which Twirp does not have: a service that declares one fails the generation.
- All the services of one `protoc` run go into one class, so two RPCs with the same name and request type fail the generation. Generate them in separate runs with different class names.

## Development

```sh
go test ./...   # the generator
test/run.sh     # generates C# from test/protos and runs the C# tests (needs protoc and the .NET SDK)
```

A GitHub release with a `vX.Y.Z` tag builds the archives (`scripts/build-release.sh`), attests them and attaches them to the release.

## License

[Apache 2.0](LICENSE). Twirp is a project of Twitch; this is an independent implementation.
