package generator

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/pluginpb"
)

// shopProto uses a nested message, a message of another package and the default namespace. Like
// most protos written for C#, it has no go_package.
const shopProto = `
name: "acme/shop.proto"
package: "acme.shop_v2"
dependency: "google/protobuf/empty.proto"
syntax: "proto3"
message_type { name: "Item" }
message_type {
  name: "Order"
  nested_type { name: "Line" nested_type { name: "Price" } }
}
service {
  name: "Shop"
  method { name: "GetItem" input_type: ".acme.shop_v2.Item" output_type: ".acme.shop_v2.Item" }
  method { name: "Price" input_type: ".acme.shop_v2.Order.Line.Price" output_type: ".google.protobuf.Empty" }
}
`

// importedProto declares a service, but is only a dependency of the file to generate.
const importedProto = `
name: "acme/imported.proto"
package: "acme.imported"
syntax: "proto3"
options { csharp_namespace: "Acme.Imported.Proto" }
message_type { name: "Thing" }
service {
  name: "Imported"
  method { name: "Get" input_type: ".acme.imported.Thing" output_type: ".acme.imported.Thing" }
}
`

func request(t *testing.T, parameter string, files ...string) *pluginpb.CodeGeneratorRequest {
	t.Helper()
	req := &pluginpb.CodeGeneratorRequest{
		Parameter: proto.String(parameter),
		ProtoFile: []*descriptorpb.FileDescriptorProto{protodesc.ToFileDescriptorProto(emptypb.File_google_protobuf_empty_proto)},
	}
	for _, text := range files {
		file := &descriptorpb.FileDescriptorProto{}
		if err := prototext.Unmarshal([]byte(text), file); err != nil {
			t.Fatalf("bad test proto: %v", err)
		}
		req.ProtoFile = append(req.ProtoFile, file)
	}
	req.FileToGenerate = []string{req.ProtoFile[len(req.ProtoFile)-1].GetName()}
	return req
}

// run generates the C# file for the last of files, with the others as its dependencies. It goes
// through Run, with the request and the response serialized as protoc sends and reads them.
func run(t *testing.T, parameter string, files ...string) *pluginpb.CodeGeneratorResponse {
	t.Helper()
	in, err := proto.Marshal(request(t, parameter, files...))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run(bytes.NewReader(in), &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	resp := &pluginpb.CodeGeneratorResponse{}
	if err := proto.Unmarshal(out.Bytes(), resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func generated(t *testing.T, parameter string, files ...string) string {
	t.Helper()
	resp := run(t, parameter, files...)
	if resp.Error != nil {
		t.Fatalf("generation failed: %s", resp.GetError())
	}
	if len(resp.File) != 1 {
		t.Fatalf("expected one file, got %d", len(resp.File))
	}
	return resp.File[0].GetContent()
}

func failure(t *testing.T, parameter string, files ...string) string {
	t.Helper()
	resp := run(t, parameter, files...)
	if resp.Error == nil {
		t.Fatalf("generation succeeded, expected an error")
	}
	return resp.GetError()
}

func assertContains(t *testing.T, content string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(content, want) {
			t.Errorf("generated code does not contain\n  %s\n\n%s", want, content)
		}
	}
}

func assertNotContains(t *testing.T, content string, unwanted ...string) {
	t.Helper()
	for _, s := range unwanted {
		if strings.Contains(content, s) {
			t.Errorf("generated code contains %q", s)
		}
	}
}

func TestTypesAreNamedAfterTheirOwnFile(t *testing.T) {
	content := generated(t, "", shopProto)
	assertContains(t, content,
		// The package in PascalCase, as protoc's C# generator names it without csharp_namespace
		"  public static global::System.Threading.Tasks.Task<global::Acme.ShopV2.Item> GetItem(\n"+
			"      global::System.Net.Http.HttpClient client,\n      global::Acme.ShopV2.Item req) {\n",
		// Nested messages live in the Types class of their parent
		"      global::Acme.ShopV2.Order.Types.Line.Types.Price req) {\n",
		// A message of another package keeps that package's csharp_namespace
		"Task<global::Google.Protobuf.WellKnownTypes.Empty> Price(",
		"global::Google.Protobuf.WellKnownTypes.Empty.Parser, configureRequest, cancellationToken);",
	)
}

func TestCsharpNamespaceOptionWins(t *testing.T) {
	file := strings.Replace(shopProto, `syntax: "proto3"`, `syntax: "proto3" options { csharp_namespace: "Acme.Shop.Proto" }`, 1)
	assertContains(t, generated(t, "", file), "Task<global::Acme.Shop.Proto.Item> GetItem(", "      global::Acme.Shop.Proto.Item req) {")
}

func TestUnderscoresToPascalCase(t *testing.T) {
	for in, want := range map[string]string{
		"":                "",
		"livekit":         "Livekit",
		"google.protobuf": "Google.Protobuf",
		"foo_bar.baz2qux": "FooBar.Baz2Qux",
		"already.Pascal":  "Already.Pascal",
	} {
		if got := underscoresToPascalCase(in); got != want {
			t.Errorf("underscoresToPascalCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGoPackageIsNotNeeded(t *testing.T) {
	// Two files that a Go generator would reject: the same go_package for different proto packages
	other := `
name: "acme/other.proto"
package: "acme.other"
syntax: "proto3"
options { go_package: "example.com/acme" }
message_type { name: "Thing" }
`
	file := strings.Replace(shopProto, `syntax: "proto3"`, `syntax: "proto3" options { go_package: "example.com/acme" }`, 1)
	file = strings.Replace(file, `dependency: "google/protobuf/empty.proto"`,
		`dependency: "google/protobuf/empty.proto" dependency: "acme/other.proto"`, 1)
	assertContains(t, generated(t, "", other, file), "GetItem(")
}

func TestEveryRpcHasBothOverloads(t *testing.T) {
	assertContains(t, generated(t, "", shopProto),
		"      global::Acme.ShopV2.Item req) {\n"+
			"    return GetItem(client, req, null, default(global::System.Threading.CancellationToken));\n  }\n",
		"      global::Acme.ShopV2.Item req,\n"+
			"      global::System.Action<global::System.Net.Http.HttpRequestMessage> configureRequest = null,\n"+
			"      global::System.Threading.CancellationToken cancellationToken = default(global::System.Threading.CancellationToken)) {\n"+
			"    return SendAsync(client, Routes.Shop.GetItem, req, global::Acme.ShopV2.Item.Parser, configureRequest, cancellationToken);\n",
		// The request goes through configureRequest, then is sent with the token
		"configureRequest(message);",
		"await client.SendAsync(message, cancellationToken).ConfigureAwait(false)",
	)
}

func TestRoutesAreConstants(t *testing.T) {
	content := generated(t, "pathPrefix=twirp", shopProto)
	assertContains(t, content,
		"  internal static class Routes {\n    internal static class Shop {\n"+
			"      internal const string GetItem = \"twirp/acme.shop_v2.Shop/GetItem\";\n"+
			"      internal const string Price = \"twirp/acme.shop_v2.Shop/Price\";\n    }\n  }\n}\n",
	)
	if strings.Count(content, `"twirp/`) != 2 {
		t.Errorf("every route should be written once, in Routes:\n%s", content)
	}
}

func TestDefaultPathPrefix(t *testing.T) {
	assertContains(t, generated(t, "", shopProto), `GetItem = "/twirp/acme.shop_v2.Shop/GetItem";`)
}

func TestAwaitsDoNotCaptureTheContext(t *testing.T) {
	content := generated(t, "", shopProto)
	if awaits, configured := strings.Count(content, "await "), strings.Count(content, ".ConfigureAwait(false)"); awaits != configured {
		t.Errorf("%d awaits but %d ConfigureAwait(false)", awaits, configured)
	}
}

func TestClassName(t *testing.T) {
	resp := run(t, "className=Twirp", shopProto)
	if got := resp.File[0].GetName(); got != "Twirp.cs" {
		t.Errorf("file name = %q, want Twirp.cs", got)
	}
	assertContains(t, resp.File[0].GetContent(), "public class Twirp {")
}

func TestDefaultClassName(t *testing.T) {
	assertContains(t, generated(t, "", shopProto), "public class TwirpApi {")
}

func TestNamespace(t *testing.T) {
	content := generated(t, "namespace=Acme.Clients", shopProto)
	assertContains(t, content,
		// The class is indented inside the namespace
		"// </auto-generated>\n\nnamespace Acme.Clients {\n  // Twirp client of these services,",
		"\n  public class TwirpApi {\n",
		"\n    public static global::System.Threading.Tasks.Task<global::Acme.ShopV2.Item> GetItem(\n        global::System.Net.Http.HttpClient client,\n",
	)
	if !strings.HasSuffix(content, "\n  }\n}\n") {
		t.Errorf("the namespace is not closed after the class:\n%s", content)
	}
	if strings.Contains(content, "  \n") {
		t.Error("blank lines inside the namespace got trailing spaces")
	}
}

func TestEveryTypeIsFullyQualified(t *testing.T) {
	// Without a using, and with every System. or Google.Protobuf. written as global::, no name of
	// the user's code (a namespace Acme.System next to the class, a message called HttpClient) can
	// change what a type of the generated code refers to
	for _, parameter := range []string{"", "namespace=Acme.Clients"} {
		content := generated(t, parameter, shopProto)
		assertNotContains(t, content, "\nusing ")
		for _, prefix := range []string{"System.", "Google.Protobuf."} {
			for i := strings.Index(content, prefix); i >= 0; i = next(content, prefix, i) {
				if !strings.HasSuffix(content[:i], "global::") {
					t.Errorf("%s is not written as global::%s at %q", prefix, prefix, content[max(0, i-30):i+len(prefix)+20])
				}
			}
		}
	}
}

func next(s, substr string, after int) int {
	i := strings.Index(s[after+1:], substr)
	if i < 0 {
		return -1
	}
	return after + 1 + i
}

func TestHeaderAndClassComment(t *testing.T) {
	file := shopProto + `service {
  name: "Warehouse"
  method { name: "Stock" input_type: ".acme.shop_v2.Item" output_type: ".acme.shop_v2.Item" }
}`
	assertContains(t, generated(t, "", file),
		"// <auto-generated>\n//   Generated by protoc-gen-twirp_csharp. Do not edit: regenerate it.\n"+
			"//   Source: acme/shop.proto\n// </auto-generated>\n\n",
		"// request with an HttpClient:\n//   acme.shop_v2.Shop\n//   acme.shop_v2.Warehouse\npublic class TwirpApi {\n",
	)
}

func TestRpcCommentsAreCopied(t *testing.T) {
	// The leading comments of the first method of the first service (path: service 0, method 0)
	file := shopProto + `source_code_info {
  location { path: [6, 0, 2, 0] span: [0, 0, 1] leading_comments: " Returns the item.\n With two lines.\n" }
}`
	assertContains(t, generated(t, "", file),
		"\n  // Returns the item.\n  // With two lines.\n  public static global::System.Threading.Tasks.Task<global::Acme.ShopV2.Item> GetItem(\n")
}

func TestGlobalNamespaceByDefault(t *testing.T) {
	assertNotContains(t, generated(t, "", shopProto), "namespace ")
}

func TestBuiltInErrors(t *testing.T) {
	content := generated(t, "", shopProto)
	assertContains(t, content,
		"throw CreateException(response, global::System.Text.Encoding.UTF8.GetString(body));",
		"public Exception(ErrorCode type, string message, int status, global::System.Collections.Generic.IReadOnlyDictionary<string, string> meta) : base(message) {",
		"public global::System.Collections.Generic.IReadOnlyDictionary<string, string> Meta { get; }",
		`case "dataloss": errorCode = ErrorCode.Data_Loss; return true;`,
		"case 429: code = ErrorCode.Resource_Exhausted; break;",
		"private sealed class JsonReader {",
	)
}

func TestErrorFactory(t *testing.T) {
	content := generated(t, "errorFactory=Acme.Errors.FromResponse", shopProto)
	assertContains(t, content,
		"throw global::Acme.Errors.FromResponse(global::System.Text.Encoding.UTF8.GetString(body), (int)response.StatusCode);",
		// The factory brings its own details: the generated exception keeps only the code
		"  public class Exception : global::System.Exception {\n    // The Twirp error code.\n    public readonly ErrorCode Type;\n\n"+
			"    public Exception(ErrorCode type, string message) : base(message) {\n      Type = type;\n    }\n  }\n",
	)
	assertNotContains(t, content, "CreateException", "JsonReader", "Meta {", "int Status")
}

func TestOnlyTheFilesToGenerateAreGenerated(t *testing.T) {
	file := strings.Replace(shopProto, `dependency: "google/protobuf/empty.proto"`,
		`dependency: "google/protobuf/empty.proto" dependency: "acme/imported.proto"`, 1)
	assertNotContains(t, generated(t, "", importedProto, file), "Imported")
}

func TestFilesWithoutServicesGenerateNothing(t *testing.T) {
	messagesOnly := `
name: "acme/messages.proto"
package: "acme.messages"
syntax: "proto3"
message_type { name: "Thing" }
`
	resp := run(t, "", messagesOnly)
	if resp.Error != nil || len(resp.File) != 0 {
		t.Errorf("expected no file and no error, got %d files, error %q", len(resp.File), resp.GetError())
	}
}

func TestEditionsAreSupported(t *testing.T) {
	file := strings.Replace(shopProto, `syntax: "proto3"`, `syntax: "editions" edition: EDITION_2023`, 1)
	resp := run(t, "", file)
	if resp.Error != nil {
		t.Fatalf("generation failed: %s", resp.GetError())
	}
	// protoc refuses to run a plugin on an editions file unless the response declares it
	if resp.GetSupportedFeatures()&uint64(pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS) == 0 {
		t.Error("FEATURE_SUPPORTS_EDITIONS is not declared")
	}
	if resp.GetMaximumEdition() != int32(descriptorpb.Edition_EDITION_2023) {
		t.Errorf("maximum edition = %d", resp.GetMaximumEdition())
	}
}

func TestStreamingRpcsAreRejected(t *testing.T) {
	file := strings.Replace(shopProto, `name: "GetItem" input_type`, `name: "GetItem" client_streaming: true input_type`, 1)
	if err := failure(t, "", file); !strings.Contains(err, "acme.shop_v2.Shop.GetItem: Twirp does not support streaming RPCs") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestClashingMethodsAreRejected(t *testing.T) {
	file := shopProto + `service {
  name: "Warehouse"
  method { name: "GetItem" input_type: ".acme.shop_v2.Item" output_type: ".acme.shop_v2.Order" }
}`
	if err := failure(t, "", file); !strings.Contains(err, "would both generate GetItem(global::Acme.ShopV2.Item)") {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestMethodsNamedLikeAMemberAreRejected(t *testing.T) {
	for _, name := range []string{"Routes", "Exception", "ErrorCode", "JsonReader", "TwirpApi", "Shop"} {
		file := strings.Replace(shopProto, `name: "GetItem"`, `name: "`+name+`"`, 1)
		if err := failure(t, "", file); !strings.Contains(err, "would clash with a member of the generated code") {
			t.Errorf("method %s: unexpected error: %s", name, err)
		}
	}
}

func TestInvalidOptionsAreRejected(t *testing.T) {
	for parameter, want := range map[string]string{
		"className=Not-An-Identifier": `className "Not-An-Identifier" is not a C# identifier`,
		"namespace=Acme..Clients":     `namespace "Acme..Clients" is not a C# namespace`,
		"errorFactory=global::F":      `errorFactory "global::F" is not a fully qualified C# method name`,
	} {
		if err := failure(t, parameter, shopProto); !strings.Contains(err, want) {
			t.Errorf("%s: unexpected error: %s", parameter, err)
		}
	}
}

func TestUnknownOptionsAreRejected(t *testing.T) {
	in, err := proto.Marshal(request(t, "nope=1", shopProto))
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(bytes.NewReader(in), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("unexpected error: %v", err)
	}
}
