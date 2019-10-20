package proxy

import pb "github.com/golang/protobuf/protoc-gen-go/descriptor"

func (g *proxy) generateWrapperTypes(serviceName, pkgName string) {
	g.P(g.WrapperFns, "type "+serviceName+"Client struct {")
	g.P(g.WrapperFns, serviceName+"Client")
	g.P(g.WrapperFns, "}")
	g.P(g.WrapperFns, "")

	g.P(g.WrapperFns, "func New"+serviceName+"Client() (*"+serviceName+", error) {")
	g.P(g.WrapperFns, "conn, err := grpc.Dial(\"unix:\"+constants."+serviceName+"SocketPath,")
	g.P(g.WrapperFns, "grpc.WithInsecure(),")
	g.P(g.WrapperFns, ")")
	g.P(g.WrapperFns, "if err != nil {")
	g.P(g.WrapperFns, "return nil, err")
	g.P(g.WrapperFns, "}")
	g.P(g.WrapperFns, "return &"+serviceName+"Client{")
	g.P(g.WrapperFns, serviceName+"Client: New"+serviceName+"Client(conn),")
	g.P(g.WrapperFns, "}, nil")
	g.P(g.WrapperFns, "}")

	/*
	   type MachineClient struct {
	     machineapi.MachineClient
	   }

	   // NewMachineClient initializes new client and connects to init
	   func NewMachineClient() (*MachineClient, error) {
	     conn, err := grpc.Dial("unix:"+constants.InitSocketPath,
	       grpc.WithInsecure(),
	     )
	     if err != nil {
	       return nil, err
	     }

	     return &MachineClient{
	       MachineClient: machineapi.NewMachineClient(conn),
	     }, nil
	   }

	*/

}
func (g *proxy) generateWrapperFns(serviceName, pkgName string, method *pb.MethodDescriptorProto) {

}
