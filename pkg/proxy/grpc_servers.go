package proxy

import (
	"strings"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/golang/protobuf/protoc-gen-go/generator"
)

// generateRegistrator generates the embedded types for the registrator.
func (g *proxy) generateRegistrator(service *descriptor.ServiceDescriptorProto, pkgName string) {
	serviceName := generator.CamelCase(service.GetName())
	g.P(g.Registrator, pkgName+"."+serviceName+"Client")
	//g.P(g.Registrator, " *Local"+serviceName+"Client")
}

// generateRegistratorRegister generates the grpc server registration calls.
func (g *proxy) generateRegistratorRegister(service *descriptor.ServiceDescriptorProto, pkgName string) {
	serviceName := generator.CamelCase(service.GetName())
	g.P(g.RegistratorRegister, pkgName+"."+"Register"+serviceName+"Server(s,r)")
}

// generateGRPCServers generates the methods to satisfy the XXServer interface.
// This differs ever so slightly from the XXClient interface.
func (g *proxy) generateServerMethods(serviceName, pkgName string, method *descriptor.MethodDescriptorProto) {

	var serverArgs strings.Builder
	serverArgs.WriteString("(")

	// Streaming methods don't have context as an arg
	if !method.GetServerStreaming() && !method.GetClientStreaming() {
		serverArgs.WriteString("ctx context.Context, ")
	}

	serverArgs.WriteString("in *" + g.typeName(method.GetInputType()))

	// Streaming methods have a stream interface
	if method.GetServerStreaming() || method.GetClientStreaming() {
		serverArgs.WriteString(", stream " + pkgName + "." + serviceName + "_" + generator.CamelCase(method.GetName()) + "Server, ")
	}
	serverArgs.WriteString(")")

	var serverReturns strings.Builder
	serverReturns.WriteString("(")
	if !method.GetServerStreaming() && !method.GetClientStreaming() {
		serverReturns.WriteString("*" + g.typeName(method.GetOutputType()) + ", ")
	}
	serverReturns.WriteString("error")
	serverReturns.WriteString(")")

	g.P(g.GrpcServer, "func (r *Registrator) "+method.GetName()+
		serverArgs.String()+serverReturns.String()+"{")

	if method.GetServerStreaming() || method.GetClientStreaming() {
		//g.P(g.GrpcServer, "return r.Local"+serviceName+"Client."+method.GetName()+"(in, stream)")
		g.P(g.GrpcServer, "return r."+method.GetName()+"(in, stream)")
	} else {
		//g.P(g.GrpcServer, "return r.Local"+serviceName+"Client."+method.GetName()+"(ctx, in)")
		g.P(g.GrpcServer, "return r."+method.GetName()+"(ctx, in)")
	}

	g.P(g.GrpcServer, "}")

}
