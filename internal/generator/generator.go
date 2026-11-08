// Package generator implements protoc-gen-twirp_csharp, a protoc plugin that generates C# clients
// for Twirp services.
package generator

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"text/template"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

//go:embed template.tmpl
var templateText string

var templates = template.Must(template.New("").Parse(templateText))

// Config holds the plugin parameters, passed as --twirp_csharp_opt=key=value,key=value.
type Config struct {
	// PathPrefix starts every route: "/twirp", the Twirp default, unless set.
	PathPrefix string
	// ClassName names the generated class and its file.
	ClassName string
	// Namespace, when set, is the namespace of the class, which is global otherwise.
	Namespace string
	// ErrorFactory, when set, is the fully qualified C# method, without "global::", that builds the
	// exception thrown for a non-2xx response from the body and the HTTP status:
	// Exception Factory(string body, int status). Unset, the generated code builds it.
	ErrorFactory string
}

func options(cfg *Config) protogen.Options {
	var flags flag.FlagSet
	flags.StringVar(&cfg.PathPrefix, "pathPrefix", "/twirp", "the start of every route")
	flags.StringVar(&cfg.ClassName, "className", "TwirpApi", "the name of the generated class and file")
	flags.StringVar(&cfg.Namespace, "namespace", "", "the namespace of the generated class")
	flags.StringVar(&cfg.ErrorFactory, "errorFactory", "", "the C# method that builds the exception of a failed call from (body, status)")
	return protogen.Options{ParamFunc: flags.Set}
}

// Main runs the plugin named name, reading the request from stdin and writing the response to
// stdout, or prints its version when called with --version.
func Main(name, version string) {
	if len(os.Args) > 1 {
		if os.Args[1] == "--version" {
			fmt.Println(name, Version(version))
			return
		}
		fmt.Fprintf(os.Stderr, "%s is a protoc plugin: run it with protoc --%s_out=DIR (or --version)\n",
			name, strings.TrimPrefix(name, "protoc-gen-"))
		os.Exit(1)
	}
	if err := Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		os.Exit(1)
	}
}

// Version is the version stamped at build time or, for go install, the version of the module.
func Version(stamped string) string {
	if stamped != "" {
		return stamped
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// Run reads a CodeGeneratorRequest from in and writes the CodeGeneratorResponse to out.
func Run(in io.Reader, out io.Writer) error {
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	req := &pluginpb.CodeGeneratorRequest{}
	if err := proto.Unmarshal(data, req); err != nil {
		return err
	}
	resp, err := Generate(req)
	if err != nil {
		return err
	}
	if data, err = proto.Marshal(resp); err != nil {
		return err
	}
	_, err = out.Write(data)
	return err
}

// Generate answers a request with the generated file, or with the error that stopped the
// generation. It returns an error itself only for a request it cannot read, such as one with an
// unknown parameter.
func Generate(req *pluginpb.CodeGeneratorRequest) (*pluginpb.CodeGeneratorResponse, error) {
	// protogen, written for Go generators, rejects files without a Go import path and files whose
	// go_package values conflict. No Go is generated here, so every file gets a placeholder.
	for _, file := range req.ProtoFile {
		if file.Options == nil {
			file.Options = &descriptorpb.FileOptions{}
		}
		file.Options.GoPackage = proto.String("twirp-csharp.invalid/" + strings.TrimSuffix(file.GetName(), ".proto"))
	}
	var cfg Config
	plugin, err := options(&cfg).New(req)
	if err != nil {
		return nil, err
	}
	if err := generate(plugin, cfg); err != nil {
		plugin.Error(err)
	}
	return plugin.Response(), nil
}

type fileData struct {
	Config
	// Sources are the proto files that declare the services.
	Sources []string
	// ServiceNames are the full proto names of the services.
	ServiceNames []string
	Services     []service
}

type service struct {
	// Name is the C# name of the service's class in Routes.
	Name    string
	Methods []method
}

type method struct {
	// Name is the C# name of the methods and of the route constant.
	Name     string
	Service  string
	Route    string
	Request  string
	Response string
	// Comments are the leading comments of the RPC, as comment lines of the class.
	Comments string
}

var (
	identifier  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	dottedNames = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
)

// Members of the generated class that an RPC method cannot share a name with.
var reservedNames = map[string]bool{
	"CONTENT_TYPE_PROTOBUF": true, "ErrorCode": true, "Exception": true, "JsonReader": true,
	"Routes": true, "doParsing": true,
}

func generate(plugin *protogen.Plugin, cfg Config) error {
	plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL |
		pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS)
	// The generator reads names, comments and csharp_namespace, which no edition changes
	plugin.SupportedEditionsMinimum = descriptorpb.Edition_EDITION_PROTO2
	plugin.SupportedEditionsMaximum = descriptorpb.Edition_EDITION_2023

	if !identifier.MatchString(cfg.ClassName) {
		return fmt.Errorf("className %q is not a C# identifier", cfg.ClassName)
	}
	if cfg.Namespace != "" && !dottedNames.MatchString(cfg.Namespace) {
		return fmt.Errorf("namespace %q is not a C# namespace", cfg.Namespace)
	}
	if cfg.ErrorFactory != "" && !dottedNames.MatchString(cfg.ErrorFactory) {
		return fmt.Errorf("errorFactory %q is not a fully qualified C# method name", cfg.ErrorFactory)
	}

	data := fileData{Config: cfg}
	services := map[string]protoreflect.FullName{}
	signatures := map[string]protoreflect.FullName{}
	for _, file := range plugin.Files {
		if !file.Generate {
			continue
		}
		if len(file.Services) > 0 {
			data.Sources = append(data.Sources, file.Desc.Path())
		}
		for _, svc := range file.Services {
			s := service{Name: svc.GoName}
			if prev, ok := services[s.Name]; ok {
				return fmt.Errorf("services %s and %s would both generate Routes.%s", prev, svc.Desc.FullName(), s.Name)
			}
			services[s.Name] = svc.Desc.FullName()
			data.ServiceNames = append(data.ServiceNames, string(svc.Desc.FullName()))
			if s.Name == "Routes" {
				return fmt.Errorf("service %s would generate the class Routes.Routes", svc.Desc.FullName())
			}
			for _, m := range svc.Methods {
				if m.Desc.IsStreamingClient() || m.Desc.IsStreamingServer() {
					return fmt.Errorf("%s: Twirp does not support streaming RPCs", m.Desc.FullName())
				}
				name := m.GoName
				if reservedNames[name] || name == cfg.ClassName || name == s.Name {
					return fmt.Errorf("%s: a method named %s would clash with a member of the generated code", m.Desc.FullName(), name)
				}
				request := csharpType(m.Input.Desc)
				signature := name + "(" + request + ")"
				if prev, ok := signatures[signature]; ok {
					return fmt.Errorf("%s and %s would both generate %s", prev, m.Desc.FullName(), signature)
				}
				signatures[signature] = m.Desc.FullName()
				s.Methods = append(s.Methods, method{
					Name:    name,
					Service: s.Name,
					// The route names the service and the method as written in the proto
					Route:    fmt.Sprintf("%s/%s/%s", cfg.PathPrefix, svc.Desc.FullName(), m.Desc.Name()),
					Request:  request,
					Response: csharpType(m.Output.Desc),
					Comments: classComment(m.Comments.Leading),
				})
			}
			if len(s.Methods) > 0 {
				data.Services = append(data.Services, s)
			}
		}
	}

	// Without a service there is nothing to call, as gRPC's generators decide too
	if len(data.Services) == 0 {
		return nil
	}

	var class bytes.Buffer
	if err := templates.ExecuteTemplate(&class, "class", data); err != nil {
		return err
	}
	out := plugin.NewGeneratedFile(cfg.ClassName+".cs", "")
	if err := templates.ExecuteTemplate(out, "header", data); err != nil {
		return err
	}
	if cfg.Namespace == "" {
		_, err := out.Write(class.Bytes())
		return err
	}
	fmt.Fprintf(out, "namespace %s {\n", cfg.Namespace)
	for _, line := range strings.SplitAfter(class.String(), "\n") {
		if strings.TrimSpace(line) != "" {
			line = "  " + line
		}
		out.Write([]byte(line))
	}
	_, err := out.Write([]byte("}\n"))
	return err
}

// csharpType is the name protoc's C# generator gives a message: the namespace of its own file, then
// its enclosing messages, each followed by ".Types".
func csharpType(msg protoreflect.MessageDescriptor) string {
	name := string(msg.Name())
	for parent, ok := msg.Parent().(protoreflect.MessageDescriptor); ok; parent, ok = parent.Parent().(protoreflect.MessageDescriptor) {
		name = string(parent.Name()) + ".Types." + name
	}
	if ns := csharpNamespace(msg.ParentFile()); ns != "" {
		name = ns + "." + name
	}
	return "global::" + name
}

// csharpNamespace is the csharp_namespace option of the file or, without one, its package in
// PascalCase, as protoc's C# generator does.
func csharpNamespace(file protoreflect.FileDescriptor) string {
	if opts, ok := file.Options().(*descriptorpb.FileOptions); ok && opts.CsharpNamespace != nil {
		return opts.GetCsharpNamespace()
	}
	return underscoresToPascalCase(string(file.Package()))
}

// underscoresToPascalCase mirrors UnderscoresToCamelCase(name, true, true) of protoc's C# generator:
// letters after a separator or a digit are capitalized, and separators other than '.' are dropped.
func underscoresToPascalCase(name string) string {
	var b strings.Builder
	capNext := true
	for _, c := range name {
		switch {
		case 'a' <= c && c <= 'z':
			if capNext {
				c -= 'a' - 'A'
			}
			b.WriteRune(c)
			capNext = false
		case 'A' <= c && c <= 'Z':
			b.WriteRune(c)
			capNext = false
		case '0' <= c && c <= '9':
			b.WriteRune(c)
			capNext = true
		default:
			if c == '.' {
				b.WriteRune(c)
			}
			capNext = true
		}
	}
	return b.String()
}

// classComment writes comments as "//" lines indented to the members of the class, or returns an
// empty string when there are none.
func classComment(comments protogen.Comments) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(comments.String(), "\n") {
		if line != "" {
			b.WriteString("  " + line)
		}
	}
	return b.String()
}
