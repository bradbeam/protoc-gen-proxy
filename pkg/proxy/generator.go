package proxy

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/golang/protobuf/protoc-gen-go/generator"
)

// generatedCodeVersion indicates a version of the generated code.
// It is incremented whenever an incompatibility between the generated code and
// the grpc package is introduced; the generated code references
// a constant, grpc.SupportPackageIsVersionN (where N is generatedCodeVersion).
const generatedCodeVersion = 4

// Paths for packages used by code generated in this file,
// relative to the import_prefix of the generator.Generator.
const (
	contextPkgPath = "context"
	grpcPkgPath    = "google.golang.org/grpc"
	codePkgPath    = "google.golang.org/grpc/codes"
	statusPkgPath  = "google.golang.org/grpc/status"
)

func init() {
	generator.RegisterPlugin(new(proxy))
}

// grpc is an implementation of the Go protocol buffer compiler's
// plugin architecture.  It generates bindings for gRPC support.
type proxy struct {
	ProxySwitch *bytes.Buffer
	ProxyFns    *bytes.Buffer
	InitClients *bytes.Buffer
	Clients     *bytes.Buffer

	WrapperFns *bytes.Buffer

	gen *generator.Generator
}

// Name returns the name of this plugin, "grpc".
func (g *proxy) Name() string {
	return "proxy"
}

// The names for packages imported in the generated code.
// They may vary from the final path component of the import path
// if the name is used by other packages.
var (
	contextPkg string
	grpcPkg    string
)

// Init initializes the plugin.
func (g *proxy) Init(gen *generator.Generator) {
	g.gen = gen
	g.ProxySwitch = new(bytes.Buffer)
	g.ProxyFns = new(bytes.Buffer)
	g.InitClients = new(bytes.Buffer)
	g.Clients = new(bytes.Buffer)
	g.WrapperFns = new(bytes.Buffer)
}

// Given a type name defined in a .proto, return its object.
// Also record that we're using it, to guarantee the associated import.
func (g *proxy) objectNamed(name string) generator.Object {
	g.gen.RecordTypeUse(name)
	return g.gen.ObjectNamed(name)
}

// Given a type name defined in a .proto, return its name as we will print it.
func (g *proxy) typeName(str string) string {
	return g.gen.TypeName(g.objectNamed(str))
}

// P forwards to g.gen.P.
//func (g *proxy) P(args ...interface{}) { g.gen.P(args...) }
func (g *proxy) P(w *bytes.Buffer, str ...interface{}) {
	//g.gen.WriteString(g.gen.indent)
	for _, v := range str {
		g.printAtom(w, v)
	}
	w.WriteByte('\n')
}

// printAtom prints the (atomic, non-annotation) argument to the generated output.
func (g *proxy) printAtom(w *bytes.Buffer, v interface{}) {
	switch v := v.(type) {
	case string:
		w.WriteString(v)
	case *string:
		w.WriteString(*v)
	case bool:
		fmt.Fprint(w, v)
	case *bool:
		fmt.Fprint(w, *v)
	case int:
		fmt.Fprint(w, v)
	case *int32:
		fmt.Fprint(w, *v)
	case *int64:
		fmt.Fprint(w, *v)
	case float64:
		fmt.Fprint(w, v)
	case *float64:
		fmt.Fprint(w, *v)
	case generator.GoPackageName:
		w.WriteString(string(v))
	case generator.GoImportPath:
		w.WriteString(strconv.Quote(string(v)))
	default:
		g.gen.Fail(fmt.Sprintf("unknown type in printer: %T", v))
	}
}

// GenerateImports generates the import declaration for this file.
func (g *proxy) GenerateImports(file *generator.FileDescriptor) {
}
