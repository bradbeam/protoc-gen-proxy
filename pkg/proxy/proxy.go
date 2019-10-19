/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package proxy

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"

	pb "github.com/golang/protobuf/protoc-gen-go/descriptor"
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
	ClientMap   *bytes.Buffer
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
	g.ClientMap = new(bytes.Buffer)
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

func (g *proxy) Generate(file *generator.FileDescriptor) {
	/*
		for _, pkg := range g.Request.ProtoFile {
			switch {
			case *pkg.Package == "osapi":
				for _, service := range pkg.Service {
					fmt.Println(service.GetName())
					for _, method := range service.Method {
						fmt.Println(method.GetName())
					}
				}
			case *pkg.Package == "machineapi":
				for _, service := range pkg.Service {
					fmt.Println(service.GetName())
					for _, method := range service.Method {
						fmt.Println(method.GetName())
					}
				}
			}
		}

		return
	*/

	// Try to filter out non-builtins
	// ex, we don't want to do this for  google/protobuf/empty.proto
	if strings.Contains(*file.Package, "google") {
		return
	}

	// If we're dealing with the actual file to generate,
	// we'll print out everything we've generated so far ( all bytes.Buffers )
	// and we'll generate the high level wrappers like `Proxy()`,
	// `UnaryInterceptor()`, `Runner()`
	if file.GetName() == g.gen.Request.FileToGenerate[0] {
		log.Println("generating proxy stuff")
		g.generateProxyStuff(file)
		return
	}

	// Otherwise, we'll generate all the fun per package/proto
	// imports and switch statements so we can satisfy the
	// - switch statement cases
	// - various function definitions
	for _, service := range file.FileDescriptorProto.Service {
		log.Println(service.GetName())
		serviceName := generator.CamelCase(service.GetName())
		g.generateSwitchStatement(serviceName, file.GetPackage(), service.Method)
		g.generateServiceFuncType(serviceName)
		g.P(g.ProxyFns, "")
		g.generateProxyRunner(serviceName)
		g.P(g.ProxyFns, "")
		g.generateClientMaps(serviceName, file.GetPackage())
		g.P(g.ProxyFns, "")
		g.generateInitializeClients(serviceName, file.GetPackage(), service.Method)
		g.generateClientFns(serviceName, file.GetPackage())
		for _, method := range service.Method {
			// No support for streaming stuff yet
			if method.GetServerStreaming() || method.GetClientStreaming() {
				continue
			}
			g.generateServiceFunc(serviceName, method)
			g.P(g.ProxyFns, "")
		}
	}
}

func (g *proxy) generateInitializeClients(serviceName, pkgName string, methods []*pb.MethodDescriptorProto) {
	fullServName := serviceName
	if pkgName != "" {
		fullServName = pkgName + "." + fullServName
	}
	g.P(g.InitClients, serviceName+"Clients: map[string]*"+fullServName+"Client{")
	for _, method := range methods {
		sname := fmt.Sprintf("/%s/%s", fullServName, method.GetName())
		g.P(g.InitClients, "\""+sname+"\": nil,")
	}
	g.P(g.InitClients, "},")
}

func (g *proxy) generateClientFns(serviceName, pkgName string) {
	fullServName := serviceName
	if pkgName != "" {
		fullServName = pkgName + "." + fullServName
	}
	g.P(g.Clients, "")
	g.P(g.Clients, "func create"+serviceName+"Client(targets []string, creds credentials.TransportCredentials, proxyMd metadata.MD) ([]*proxy"+serviceName+"Client ,error){")
	g.P(g.Clients, "var errors *go_multierror.Error")
	g.P(g.Clients, "clients := make([]*proxy"+serviceName+"Client{}, 0, len(targets))")
	g.P(g.Clients, "for _, target := range targets {")
	g.P(g.Clients, "c := &proxy"+serviceName+"Client{")
	g.P(g.Clients, "// TODO change the context to be more useful ( ex cancelable )")
	g.P(g.Clients, "Context: metadata.NewOutgoingContext(context.Background(), proxyMd),")
	g.P(g.Clients, "Target:  target,")
	g.P(g.Clients, "}")
	g.P(g.Clients, "// TODO: i think we potentially leak a client here,")
	g.P(g.Clients, "// we should close the request // cancel the context if it errors")
	g.P(g.Clients, "// Explicitly set OSD port")
	g.P(g.Clients, "conn, err := grpc.Dial(fmt.Sprintf(\"%s:%d\", target, 50000), grpc.WithTransportCredentials(creds))")
	g.P(g.Clients, "if err != nil {")
	g.P(g.Clients, "// TODO: probably worth wrapping err to add some context about the target")
	g.P(g.Clients, "errors = go_multierror.Append(errors, err)")
	g.P(g.Clients, "continue")
	g.P(g.Clients, "}")
	g.P(g.Clients, "c.Conn = New"+serviceName+"Client(conn)")
	g.P(g.Clients, "clients = append(clients, c)")
	g.P(g.Clients, "}")
	g.P(g.Clients, "return clients, errors.ErrorOrNil()")
	g.P(g.Clients, "}")
	/*
		g.gen.P("clients := []*proxy" + serviceName + "Client{}")
		g.gen.P("for _, target := range targets {")
	*/
}

func (g *proxy) generateSwitchStatement(serviceName, pkgName string, methods []*pb.MethodDescriptorProto) {
	for _, method := range methods {
		// No support for streaming stuff yet
		if method.GetServerStreaming() || method.GetClientStreaming() {
			continue
		}
		fullServName := serviceName
		if pkgName != "" {
			fullServName = pkgName + "." + fullServName
		}
		sname := fmt.Sprintf("/%s/%s", fullServName, method.GetName())
		g.P(g.ProxySwitch, "case \""+sname+"\":")
		g.P(g.ProxySwitch, "// Initialize target clients")
		g.P(g.ProxySwitch, "clients, err := create"+serviceName+"Client(targets, creds, proxyMd)")
		g.P(g.ProxySwitch, "if err != nil {")
		g.P(g.ProxySwitch, "break")
		g.P(g.ProxySwitch, "}")

		var fnArgs strings.Builder
		fnArgs.WriteString("(")
		fnArgs.WriteString("clients, ")
		fnArgs.WriteString("in, ")
		fnArgs.WriteString("proxy" + generator.CamelCase(method.GetName()))
		fnArgs.WriteString(")")

		g.P(g.ProxySwitch, "resp := &"+g.typeName(method.GetOutputType())+"{}")
		g.P(g.ProxySwitch, "msgs, err = proxy"+serviceName+"Runner"+fnArgs.String())
		g.P(g.ProxySwitch, "for _, msg := range msgs {")
		g.P(g.ProxySwitch, "resp.Response = append(resp.Response, msg.(*"+g.typeName(method.GetOutputType())+").Response[0])")
		g.P(g.ProxySwitch, "}")
		g.P(g.ProxySwitch, "response = resp")
	}
}

// generateServiceFunc is a function generated for each service defined in the
// proto file. The function signature satisfies the runnerfn type.
func (g *proxy) generateServiceFunc(serviceName string, method *pb.MethodDescriptorProto) {
	log.Println(method.GetName())
	var args strings.Builder
	args.WriteString("(")
	args.WriteString("client *proxy" + serviceName + "Client, ")
	args.WriteString("in interface{}, ")
	args.WriteString("wg *sync.WaitGroup, ")
	args.WriteString("respCh chan proto.Message, ")
	args.WriteString("errCh chan error")
	args.WriteString(")")

	g.P(g.ProxyFns, "func proxy"+generator.CamelCase(method.GetName())+args.String()+"{")
	g.P(g.ProxyFns, "defer wg.Done()")
	g.P(g.ProxyFns, "resp, err := client.Conn."+method.GetName()+"(client.Context, in.(*"+g.typeName(method.GetInputType())+"))")
	g.P(g.ProxyFns, "if err != nil {")
	g.P(g.ProxyFns, "errCh<-err")
	g.P(g.ProxyFns, "return")
	g.P(g.ProxyFns, "}")
	// TODO: See if we can better abstract this
	g.P(g.ProxyFns, "resp.Response[0].Metadata = &NodeMetadata{Hostname: client.Target}")
	g.P(g.ProxyFns, "respCh<-resp")
	g.P(g.ProxyFns, "}")
}

func (g *proxy) generateProxyStuff(file *generator.FileDescriptor) {
	//	g.P(g.gen.Buffer, g.ProxySwitch.String)
	/*
	   	log.Println(g.ProxySwitch.String())

	   	log.Println(g.ProxyFns.String())
	   }

	   	g.gen.P(g.String())
	   	log.Println(*file.Package)
	*/

	g.gen.AddImport("strings")
	g.gen.AddImport("sync")
	g.gen.AddImport("google.golang.org/grpc/metadata")
	g.gen.AddImport("google.golang.org/grpc/credentials")
	g.gen.AddImport("github.com/hashicorp/go-multierror")
	g.gen.AddImport("github.com/talos-systems/talos/pkg/grpc/tls")
	// Add in additional imports for other protos
	//g.gen.AddImport("")

	// Do additional generation
	g.generateProxyStruct(file.GetPackage())

	g.gen.P(g.ProxyFns.String())
	g.gen.P("")
	g.gen.P(g.Clients.String())
	g.gen.P("")
	g.generateProxyRouter(file.GetPackage())
	g.gen.P("")
	g.generateProxyClientStruct(file.GetPackage())
	g.gen.P("")

	/*
		for _, service := range file.FileDescriptorProto.Service {
			log.Println(service.GetName())
			serviceName := generator.CamelCase(service.GetName())
				//g.generateServiceFuncType(serviceName)
				//g.gen.P("")
				g.generateProxyClientStruct(serviceName)
				g.gen.P("")
				g.generateProxyStruct(serviceName)
				g.gen.P("")
				g.generateProxyInterceptor(serviceName, file.GetPackage())
				g.gen.P("")
			g.generateProxyRouter(serviceName, file.GetPackage(), service.Method)
				for _, method := range service.Method {
					log.Println(method.GetName())
					// No support for streaming stuff yet
					if method.GetServerStreaming() || method.GetClientStreaming() {
						continue
					}
					g.P("")
					g.generateServiceFunc(serviceName, method)
				}
		}
	*/
	log.Println(g.gen.Response)
}

// generateServiceFuncType is a function with a specific signature. This function
// gets passed through the 'runner' func to perform the actual client call.
func (g *proxy) generateServiceFuncType(serviceName string) {
	var args strings.Builder
	args.WriteString("(")
	args.WriteString("*proxy" + serviceName + "Client, ")
	args.WriteString("interface{}, ")
	args.WriteString("*sync.WaitGroup, ")
	args.WriteString("chan proto.Message, ")
	args.WriteString("chan error")
	args.WriteString(")")
	g.P(g.ProxyFns, "type runner"+generator.CamelCase(serviceName+"_fn")+" func"+args.String())
}

// generateProxyClientStruct holds the client connection and additional metadata
// associated with each grpc ( client ) connection that the proxy creates. This
// should only exist for the duration of the request.
func (g *proxy) generateProxyClientStruct(serviceName string) {
	g.P(g.ProxyFns, "type proxy"+serviceName+"Client struct {")
	g.P(g.ProxyFns, "Conn "+serviceName+"Client")
	g.P(g.ProxyFns, "Context context.Context")
	g.P(g.ProxyFns, "Target string")
	g.P(g.ProxyFns, "DialOpts []grpc.DialOption")
	g.P(g.ProxyFns, "}")
}

// generateProxyStruct is the public struct exposed for use by importers. It
// contains a tls provider to manage the TLS cert rotation/renewal. This also
// generates the constructor for the struct.
func (g *proxy) generateProxyStruct(serviceName string) {
	tName := generator.CamelCase(serviceName + "_proxy")
	g.P(g.ProxyFns, "type "+tName+" struct {")
	g.P(g.ProxyFns, "Provider tls.CertificateProvider")
	g.P(g.ProxyFns, g.ClientMap.String())
	g.P(g.ProxyFns, "}")

	g.P(g.ProxyFns, "")

	var args strings.Builder
	args.WriteString("(")
	args.WriteString("provider tls.CertificateProvider")
	args.WriteString(")")
	g.P(g.ProxyFns, "func New"+tName+args.String()+" *"+tName+"{")
	g.P(g.ProxyFns, "return &"+tName+"{")
	g.P(g.ProxyFns, "Provider: provider,")
	g.P(g.ProxyFns, g.InitClients.String())
	g.P(g.ProxyFns, "}")
	g.P(g.ProxyFns, "}")
}

func (g *proxy) generateClientMaps(serviceName, pkgName string) {
	fullServName := serviceName
	if pkgName != "" {
		fullServName = pkgName + "." + fullServName
	}
	g.P(g.ClientMap, serviceName+"Clients map[string]*"+fullServName+"Client")
}

/*
// generateProxyInterceptor is a method of the proxy struct that satisfies the
// grpc.UnaryInterceptor interface. This allows us to make use of the tls
// information from the provider to include it with each subsequent request
// from the proxy. This is also where we handle some of the routing decisions,
// namely being able to filter on the supported service and handling the
// 'proxyfrom' metadata field to prevent infinite loops.
func (g *proxy) generateProxyInterceptor(serviceName string, pkgName string) {
	tName := generator.CamelCase(serviceName + "_proxy")
	fullServName := serviceName
	if pkgName != "" {
		fullServName = pkgName + "." + fullServName
	}
	g.P("func (p *" + tName + ") UnaryInterceptor() grpc.UnaryServerInterceptor {")
	g.P("return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {")
	// Artifically limit scope to OS api
	g.P("pkg := strings.Split(info.FullMethod, \"/\")[1]")
	g.P("if pkg != \"" + fullServName + "\" {")
	g.P("return handler(ctx, req)")
	g.P("}")
	g.P("md, _ := metadata.FromIncomingContext(ctx)")
	g.P("if _, ok := md[\"proxyfrom\"]; ok {")
	g.P("return handler(ctx, req)")
	g.P("}")
	g.P("ca, err := p.Provider.GetCA()")
	g.P("if err != nil {")
	g.P("	return nil, err")
	g.P("}")
	g.P("certs, err := p.Provider.GetCertificate(nil)")
	g.P("if err != nil {")
	g.P("  return nil, err")
	g.P("}")
	g.P("tlsConfig, err := tls.New(")
	g.P("  tls.WithClientAuthType(tls.Mutual),")
	g.P("  tls.WithCACertPEM(ca),")
	g.P("  tls.WithKeypair(*certs),")
	g.P(")")
	g.P("return p.Proxy(ctx, info.FullMethod, credentials.NewTLS(tlsConfig), req)")
	g.P("}")
	g.P("}")
}
*/

// generateProxyRunner is the function that handles the client calls and response
// aggregation.
func (g *proxy) generateProxyRunner(serviceName string) {
	var args strings.Builder
	args.WriteString("(")
	args.WriteString("clients []*proxy" + serviceName + "Client, ")
	args.WriteString("in interface{}, ")
	args.WriteString("runner runner" + generator.CamelCase(serviceName+"_fn"))
	args.WriteString(")")

	var returns strings.Builder
	returns.WriteString("(")
	returns.WriteString("[]proto.Message, ")
	returns.WriteString("error")
	returns.WriteString(")")

	g.P(g.ProxyFns, "func proxy"+generator.CamelCase(serviceName+"_runner")+args.String()+returns.String()+"{")
	g.P(g.ProxyFns, "var (")
	g.P(g.ProxyFns, "errors *go_multierror.Error")
	g.P(g.ProxyFns, "wg sync.WaitGroup")
	g.P(g.ProxyFns, ")")

	g.P(g.ProxyFns, "respCh := make(chan proto.Message, len(clients))")
	g.P(g.ProxyFns, "errCh := make(chan error, len(clients))")
	g.P(g.ProxyFns, "wg.Add(len(clients))")

	g.P(g.ProxyFns, "for _, client := range clients {")
	g.P(g.ProxyFns, "go runner(client, in, &wg, respCh, errCh)")
	g.P(g.ProxyFns, "}")

	g.P(g.ProxyFns, "wg.Wait()")
	g.P(g.ProxyFns, "close(respCh)")
	g.P(g.ProxyFns, "close(errCh)")
	g.P(g.ProxyFns, "")

	g.P(g.ProxyFns, "var response []proto.Message")
	g.P(g.ProxyFns, "for resp := range respCh {")
	g.P(g.ProxyFns, "response = append(response, resp)")
	g.P(g.ProxyFns, "}")

	g.P(g.ProxyFns, "for err := range errCh {")
	g.P(g.ProxyFns, "errors = go_multierror.Append(errors, err)")
	g.P(g.ProxyFns, "}")

	g.P(g.ProxyFns, "return response, errors.ErrorOrNil()")
	g.P(g.ProxyFns, "}")
}

// generateProxyRouter creates the routing part of the proxy. That is it
// enables us to map the incoming grpc method to the function/client call
// so we can properly call the proper rpc endpoint.
func (g *proxy) generateProxyRouter(serviceName string) {
	// Leaving this in as left overs from grpc plugin, but not
	// sure it really makes sense to keep it like this versus
	// just calling the addimport call in Generate()
	contextPkg = string(g.gen.AddImport(contextPkgPath))
	grpcPkg = string(g.gen.AddImport(grpcPkgPath))

	var args strings.Builder
	args.WriteString("(")
	args.WriteString("ctx " + contextPkg + ".Context, ")
	args.WriteString("method string, ")
	args.WriteString("creds credentials.TransportCredentials, ")
	args.WriteString("in interface{}, ")
	args.WriteString("opts ..." + grpcPkg + ".CallOption")
	args.WriteString(")")

	var returns strings.Builder
	returns.WriteString("(")
	returns.WriteString("proto.Message, ")
	returns.WriteString("error")
	returns.WriteString(")")

	g.gen.P("func (p *" + generator.CamelCase(serviceName+"_proxy") + ") Proxy " + args.String() + returns.String() + "{")
	g.gen.P("var (")
	g.gen.P("err error")
	g.gen.P("errors *go_multierror.Error")
	g.gen.P("msgs []proto.Message")
	g.gen.P("ok bool")
	g.gen.P("response proto.Message")
	g.gen.P("targets []string")
	g.gen.P(")")

	// Parse targets from incoming metadata/context
	g.gen.P("md, _ := metadata.FromIncomingContext(ctx)")
	g.gen.P("// default to target node specified in config or on cli")
	g.gen.P("if targets, ok = md[\"targets\"]; !ok {")
	g.gen.P("targets = md[\":authority\"]")
	g.gen.P("}")

	// Set up client connections
	g.gen.P("proxyMd := metadata.New(make(map[string]string))")
	g.gen.P("proxyMd.Set(\"proxyfrom\", md[\":authority\"]...)")
	g.gen.P("")

	// TODO: removed client initialization from here,
	// want to add it back in to the proxyswitch generation

	// Handle routes
	g.gen.P("switch method {")
	g.gen.P(g.ProxySwitch.String())
	g.gen.P("}")
	g.gen.P("")
	g.gen.P("if err != nil {")
	g.gen.P("errors = go_multierror.Append(errors, err)")
	g.gen.P("}")
	g.gen.P("return response, errors.ErrorOrNil()")
	g.gen.P("}")
}
