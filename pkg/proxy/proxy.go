/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package proxy

import (
	"fmt"
	"log"
	"strings"

	pb "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/golang/protobuf/protoc-gen-go/generator"
)

// Generate is the main entrypoint to the plugin. This is where all
// the magic happens.
func (g *proxy) Generate(file *generator.FileDescriptor) {
	// Try to filter out non-builtins
	// ex, we don't want to do this for  google/protobuf/empty.proto
	if strings.Contains(*file.Package, "google") {
		return
	}

	// If we're dealing with the actual file to generate,
	// we'll print out everything we've generated so far ( all bytes.Buffers )
	// and we'll generate the high level wrappers like `Proxy()`,
	// `UnaryInterceptor()`, `Runner()`
	for _, f := range g.gen.Request.FileToGenerate {
		if file.GetName() == f {
			g.generateProxyStuff(file)
			// TODO maybe call buffer.Reset() on all local buffers
			return
		}
	}

	// Otherwise, we'll generate all the fun per package/proto
	// imports and switch statements so we can satisfy the
	// - switch statement cases
	// - various function definitions
	// - client creation functions
	for _, service := range file.FileDescriptorProto.Service {
		log.Println(service.GetName())
		serviceName := generator.CamelCase(service.GetName())
		g.generateSwitchStatement(serviceName, file.GetPackage(), service.Method)
		g.generateServiceFuncType(serviceName)

		g.P(g.ProxyFns, "")
		g.generateServiceRunner(serviceName)

		g.P(g.ProxyFns, "")
		g.generateProxyClientStruct(serviceName)

		g.gen.P("")
		g.generateClientFns(serviceName, file.GetPackage())

		g.generateWrapperTypes(serviceName, file.GetPackage())

		for _, method := range service.Method {
			// No support for streaming stuff yet
			if method.GetServerStreaming() || method.GetClientStreaming() {
				continue
			}
			g.generateServiceFunc(serviceName, method)
			g.P(g.ProxyFns, "")
			g.generateWrapperFns(serviceName, file.GetPackage(), method)
		}
	}
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
}

func (g *proxy) generateProxyStuff(file *generator.FileDescriptor) {
	g.gen.AddImport("strings")
	g.gen.AddImport("sync")
	g.gen.AddImport("google.golang.org/grpc/metadata")
	g.gen.AddImport("google.golang.org/grpc/credentials")
	g.gen.AddImport("github.com/hashicorp/go-multierror")
	g.gen.AddImport("github.com/talos-systems/talos/pkg/grpc/tls")
	// TODO: Add in additional imports for other protos
	//g.gen.AddImport("")

	g.generateProxyStruct(file.GetPackage())
	g.gen.P("")
	g.generateProxyInterceptor(file.GetPackage())
	g.gen.P("")
	g.generateProxyRouter(file.GetPackage())
	g.gen.P("")
	g.gen.P(g.ProxyFns.String())
	g.gen.P("")
	g.gen.P(g.Clients.String())
	g.gen.P("")
	g.gen.P(g.WrapperFns.String())
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
	g.gen.P("type " + tName + " struct {")
	g.gen.P("Provider tls.CertificateProvider")
	g.gen.P("}")

	g.gen.P("")

	var args strings.Builder
	args.WriteString("(")
	args.WriteString("provider tls.CertificateProvider")
	args.WriteString(")")
	g.gen.P("func New" + tName + args.String() + " *" + tName + "{")
	g.gen.P("return &" + tName + "{")
	g.gen.P("Provider: provider,")
	//g.gen.P(g.ProxyFns, g.InitClients.String())
	g.gen.P("}")
	g.gen.P("}")
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
