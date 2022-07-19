// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.21.2
// source: consensus_engine/consensus.proto

package consensus

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// ConsensusEngineClient is the client API for ConsensusEngine service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ConsensusEngineClient interface {
	GetAuthor(ctx context.Context, in *GetAuthorRequest, opts ...grpc.CallOption) (*GetAuthorResponse, error)
	ChainSpec(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ChainSpecMessage, error)
	// Core requests verifications from the Consensus Engine via this function
	VerifyHeaders(ctx context.Context, opts ...grpc.CallOption) (ConsensusEngine_VerifyHeadersClient, error)
	// Consensis Engine may ask for extra informaton (more headers) from the core, and these requests are coming through the stream
	// returned by the ProvideHeaders function
	ProvideHeaders(ctx context.Context, opts ...grpc.CallOption) (ConsensusEngine_ProvideHeadersClient, error)
	VerifyUncles(ctx context.Context, opts ...grpc.CallOption) (ConsensusEngine_VerifyUnclesClient, error)
	Prepare(ctx context.Context, opts ...grpc.CallOption) (ConsensusEngine_PrepareClient, error)
	Finalize(ctx context.Context, opts ...grpc.CallOption) (ConsensusEngine_FinalizeClient, error)
	Seal(ctx context.Context, in *SealBlockRequest, opts ...grpc.CallOption) (ConsensusEngine_SealClient, error)
}

type consensusEngineClient struct {
	cc grpc.ClientConnInterface
}

func NewConsensusEngineClient(cc grpc.ClientConnInterface) ConsensusEngineClient {
	return &consensusEngineClient{cc}
}

func (c *consensusEngineClient) GetAuthor(ctx context.Context, in *GetAuthorRequest, opts ...grpc.CallOption) (*GetAuthorResponse, error) {
	out := new(GetAuthorResponse)
	err := c.cc.Invoke(ctx, "/consensus.ConsensusEngine/GetAuthor", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *consensusEngineClient) ChainSpec(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ChainSpecMessage, error) {
	out := new(ChainSpecMessage)
	err := c.cc.Invoke(ctx, "/consensus.ConsensusEngine/ChainSpec", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *consensusEngineClient) VerifyHeaders(ctx context.Context, opts ...grpc.CallOption) (ConsensusEngine_VerifyHeadersClient, error) {
	stream, err := c.cc.NewStream(ctx, &ConsensusEngine_ServiceDesc.Streams[0], "/consensus.ConsensusEngine/VerifyHeaders", opts...)
	if err != nil {
		return nil, err
	}
	x := &consensusEngineVerifyHeadersClient{stream}
	return x, nil
}

type ConsensusEngine_VerifyHeadersClient interface {
	Send(*VerifyHeaderRequest) error
	Recv() (*VerifyHeaderResponse, error)
	grpc.ClientStream
}

type consensusEngineVerifyHeadersClient struct {
	grpc.ClientStream
}

func (x *consensusEngineVerifyHeadersClient) Send(m *VerifyHeaderRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *consensusEngineVerifyHeadersClient) Recv() (*VerifyHeaderResponse, error) {
	m := new(VerifyHeaderResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *consensusEngineClient) ProvideHeaders(ctx context.Context, opts ...grpc.CallOption) (ConsensusEngine_ProvideHeadersClient, error) {
	stream, err := c.cc.NewStream(ctx, &ConsensusEngine_ServiceDesc.Streams[1], "/consensus.ConsensusEngine/ProvideHeaders", opts...)
	if err != nil {
		return nil, err
	}
	x := &consensusEngineProvideHeadersClient{stream}
	return x, nil
}

type ConsensusEngine_ProvideHeadersClient interface {
	Send(*HeadersResponse) error
	Recv() (*HeadersRequest, error)
	grpc.ClientStream
}

type consensusEngineProvideHeadersClient struct {
	grpc.ClientStream
}

func (x *consensusEngineProvideHeadersClient) Send(m *HeadersResponse) error {
	return x.ClientStream.SendMsg(m)
}

func (x *consensusEngineProvideHeadersClient) Recv() (*HeadersRequest, error) {
	m := new(HeadersRequest)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *consensusEngineClient) VerifyUncles(ctx context.Context, opts ...grpc.CallOption) (ConsensusEngine_VerifyUnclesClient, error) {
	stream, err := c.cc.NewStream(ctx, &ConsensusEngine_ServiceDesc.Streams[2], "/consensus.ConsensusEngine/VerifyUncles", opts...)
	if err != nil {
		return nil, err
	}
	x := &consensusEngineVerifyUnclesClient{stream}
	return x, nil
}

type ConsensusEngine_VerifyUnclesClient interface {
	Send(*VerifyUnclesRequest) error
	Recv() (*VerifyUnclesResponse, error)
	grpc.ClientStream
}

type consensusEngineVerifyUnclesClient struct {
	grpc.ClientStream
}

func (x *consensusEngineVerifyUnclesClient) Send(m *VerifyUnclesRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *consensusEngineVerifyUnclesClient) Recv() (*VerifyUnclesResponse, error) {
	m := new(VerifyUnclesResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *consensusEngineClient) Prepare(ctx context.Context, opts ...grpc.CallOption) (ConsensusEngine_PrepareClient, error) {
	stream, err := c.cc.NewStream(ctx, &ConsensusEngine_ServiceDesc.Streams[3], "/consensus.ConsensusEngine/Prepare", opts...)
	if err != nil {
		return nil, err
	}
	x := &consensusEnginePrepareClient{stream}
	return x, nil
}

type ConsensusEngine_PrepareClient interface {
	Send(*PrepareRequest) error
	Recv() (*PrepareResponse, error)
	grpc.ClientStream
}

type consensusEnginePrepareClient struct {
	grpc.ClientStream
}

func (x *consensusEnginePrepareClient) Send(m *PrepareRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *consensusEnginePrepareClient) Recv() (*PrepareResponse, error) {
	m := new(PrepareResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *consensusEngineClient) Finalize(ctx context.Context, opts ...grpc.CallOption) (ConsensusEngine_FinalizeClient, error) {
	stream, err := c.cc.NewStream(ctx, &ConsensusEngine_ServiceDesc.Streams[4], "/consensus.ConsensusEngine/Finalize", opts...)
	if err != nil {
		return nil, err
	}
	x := &consensusEngineFinalizeClient{stream}
	return x, nil
}

type ConsensusEngine_FinalizeClient interface {
	Send(*FinalizeRequest) error
	Recv() (*FinalizeResponse, error)
	grpc.ClientStream
}

type consensusEngineFinalizeClient struct {
	grpc.ClientStream
}

func (x *consensusEngineFinalizeClient) Send(m *FinalizeRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *consensusEngineFinalizeClient) Recv() (*FinalizeResponse, error) {
	m := new(FinalizeResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *consensusEngineClient) Seal(ctx context.Context, in *SealBlockRequest, opts ...grpc.CallOption) (ConsensusEngine_SealClient, error) {
	stream, err := c.cc.NewStream(ctx, &ConsensusEngine_ServiceDesc.Streams[5], "/consensus.ConsensusEngine/Seal", opts...)
	if err != nil {
		return nil, err
	}
	x := &consensusEngineSealClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type ConsensusEngine_SealClient interface {
	Recv() (*SealBlockResponse, error)
	grpc.ClientStream
}

type consensusEngineSealClient struct {
	grpc.ClientStream
}

func (x *consensusEngineSealClient) Recv() (*SealBlockResponse, error) {
	m := new(SealBlockResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// ConsensusEngineServer is the server API for ConsensusEngine service.
// All implementations must embed UnimplementedConsensusEngineServer
// for forward compatibility
type ConsensusEngineServer interface {
	GetAuthor(context.Context, *GetAuthorRequest) (*GetAuthorResponse, error)
	ChainSpec(context.Context, *emptypb.Empty) (*ChainSpecMessage, error)
	// Core requests verifications from the Consensus Engine via this function
	VerifyHeaders(ConsensusEngine_VerifyHeadersServer) error
	// Consensis Engine may ask for extra informaton (more headers) from the core, and these requests are coming through the stream
	// returned by the ProvideHeaders function
	ProvideHeaders(ConsensusEngine_ProvideHeadersServer) error
	VerifyUncles(ConsensusEngine_VerifyUnclesServer) error
	Prepare(ConsensusEngine_PrepareServer) error
	Finalize(ConsensusEngine_FinalizeServer) error
	Seal(*SealBlockRequest, ConsensusEngine_SealServer) error
	mustEmbedUnimplementedConsensusEngineServer()
}

// UnimplementedConsensusEngineServer must be embedded to have forward compatible implementations.
type UnimplementedConsensusEngineServer struct {
}

func (UnimplementedConsensusEngineServer) GetAuthor(context.Context, *GetAuthorRequest) (*GetAuthorResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetAuthor not implemented")
}
func (UnimplementedConsensusEngineServer) ChainSpec(context.Context, *emptypb.Empty) (*ChainSpecMessage, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ChainSpec not implemented")
}
func (UnimplementedConsensusEngineServer) VerifyHeaders(ConsensusEngine_VerifyHeadersServer) error {
	return status.Errorf(codes.Unimplemented, "method VerifyHeaders not implemented")
}
func (UnimplementedConsensusEngineServer) ProvideHeaders(ConsensusEngine_ProvideHeadersServer) error {
	return status.Errorf(codes.Unimplemented, "method ProvideHeaders not implemented")
}
func (UnimplementedConsensusEngineServer) VerifyUncles(ConsensusEngine_VerifyUnclesServer) error {
	return status.Errorf(codes.Unimplemented, "method VerifyUncles not implemented")
}
func (UnimplementedConsensusEngineServer) Prepare(ConsensusEngine_PrepareServer) error {
	return status.Errorf(codes.Unimplemented, "method Prepare not implemented")
}
func (UnimplementedConsensusEngineServer) Finalize(ConsensusEngine_FinalizeServer) error {
	return status.Errorf(codes.Unimplemented, "method Finalize not implemented")
}
func (UnimplementedConsensusEngineServer) Seal(*SealBlockRequest, ConsensusEngine_SealServer) error {
	return status.Errorf(codes.Unimplemented, "method Seal not implemented")
}
func (UnimplementedConsensusEngineServer) mustEmbedUnimplementedConsensusEngineServer() {}

// UnsafeConsensusEngineServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ConsensusEngineServer will
// result in compilation errors.
type UnsafeConsensusEngineServer interface {
	mustEmbedUnimplementedConsensusEngineServer()
}

func RegisterConsensusEngineServer(s grpc.ServiceRegistrar, srv ConsensusEngineServer) {
	s.RegisterService(&ConsensusEngine_ServiceDesc, srv)
}

func _ConsensusEngine_GetAuthor_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetAuthorRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ConsensusEngineServer).GetAuthor(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/consensus.ConsensusEngine/GetAuthor",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ConsensusEngineServer).GetAuthor(ctx, req.(*GetAuthorRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ConsensusEngine_ChainSpec_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ConsensusEngineServer).ChainSpec(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/consensus.ConsensusEngine/ChainSpec",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ConsensusEngineServer).ChainSpec(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _ConsensusEngine_VerifyHeaders_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(ConsensusEngineServer).VerifyHeaders(&consensusEngineVerifyHeadersServer{stream})
}

type ConsensusEngine_VerifyHeadersServer interface {
	Send(*VerifyHeaderResponse) error
	Recv() (*VerifyHeaderRequest, error)
	grpc.ServerStream
}

type consensusEngineVerifyHeadersServer struct {
	grpc.ServerStream
}

func (x *consensusEngineVerifyHeadersServer) Send(m *VerifyHeaderResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *consensusEngineVerifyHeadersServer) Recv() (*VerifyHeaderRequest, error) {
	m := new(VerifyHeaderRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _ConsensusEngine_ProvideHeaders_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(ConsensusEngineServer).ProvideHeaders(&consensusEngineProvideHeadersServer{stream})
}

type ConsensusEngine_ProvideHeadersServer interface {
	Send(*HeadersRequest) error
	Recv() (*HeadersResponse, error)
	grpc.ServerStream
}

type consensusEngineProvideHeadersServer struct {
	grpc.ServerStream
}

func (x *consensusEngineProvideHeadersServer) Send(m *HeadersRequest) error {
	return x.ServerStream.SendMsg(m)
}

func (x *consensusEngineProvideHeadersServer) Recv() (*HeadersResponse, error) {
	m := new(HeadersResponse)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _ConsensusEngine_VerifyUncles_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(ConsensusEngineServer).VerifyUncles(&consensusEngineVerifyUnclesServer{stream})
}

type ConsensusEngine_VerifyUnclesServer interface {
	Send(*VerifyUnclesResponse) error
	Recv() (*VerifyUnclesRequest, error)
	grpc.ServerStream
}

type consensusEngineVerifyUnclesServer struct {
	grpc.ServerStream
}

func (x *consensusEngineVerifyUnclesServer) Send(m *VerifyUnclesResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *consensusEngineVerifyUnclesServer) Recv() (*VerifyUnclesRequest, error) {
	m := new(VerifyUnclesRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _ConsensusEngine_Prepare_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(ConsensusEngineServer).Prepare(&consensusEnginePrepareServer{stream})
}

type ConsensusEngine_PrepareServer interface {
	Send(*PrepareResponse) error
	Recv() (*PrepareRequest, error)
	grpc.ServerStream
}

type consensusEnginePrepareServer struct {
	grpc.ServerStream
}

func (x *consensusEnginePrepareServer) Send(m *PrepareResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *consensusEnginePrepareServer) Recv() (*PrepareRequest, error) {
	m := new(PrepareRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _ConsensusEngine_Finalize_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(ConsensusEngineServer).Finalize(&consensusEngineFinalizeServer{stream})
}

type ConsensusEngine_FinalizeServer interface {
	Send(*FinalizeResponse) error
	Recv() (*FinalizeRequest, error)
	grpc.ServerStream
}

type consensusEngineFinalizeServer struct {
	grpc.ServerStream
}

func (x *consensusEngineFinalizeServer) Send(m *FinalizeResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *consensusEngineFinalizeServer) Recv() (*FinalizeRequest, error) {
	m := new(FinalizeRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _ConsensusEngine_Seal_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(SealBlockRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(ConsensusEngineServer).Seal(m, &consensusEngineSealServer{stream})
}

type ConsensusEngine_SealServer interface {
	Send(*SealBlockResponse) error
	grpc.ServerStream
}

type consensusEngineSealServer struct {
	grpc.ServerStream
}

func (x *consensusEngineSealServer) Send(m *SealBlockResponse) error {
	return x.ServerStream.SendMsg(m)
}

// ConsensusEngine_ServiceDesc is the grpc.ServiceDesc for ConsensusEngine service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var ConsensusEngine_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "consensus.ConsensusEngine",
	HandlerType: (*ConsensusEngineServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetAuthor",
			Handler:    _ConsensusEngine_GetAuthor_Handler,
		},
		{
			MethodName: "ChainSpec",
			Handler:    _ConsensusEngine_ChainSpec_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "VerifyHeaders",
			Handler:       _ConsensusEngine_VerifyHeaders_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "ProvideHeaders",
			Handler:       _ConsensusEngine_ProvideHeaders_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "VerifyUncles",
			Handler:       _ConsensusEngine_VerifyUncles_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "Prepare",
			Handler:       _ConsensusEngine_Prepare_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "Finalize",
			Handler:       _ConsensusEngine_Finalize_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "Seal",
			Handler:       _ConsensusEngine_Seal_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "consensus_engine/consensus.proto",
}

// TestClient is the client API for Test service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type TestClient interface {
	StartTestCase(ctx context.Context, in *StartTestCaseMessage, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type testClient struct {
	cc grpc.ClientConnInterface
}

func NewTestClient(cc grpc.ClientConnInterface) TestClient {
	return &testClient{cc}
}

func (c *testClient) StartTestCase(ctx context.Context, in *StartTestCaseMessage, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, "/consensus.Test/StartTestCase", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TestServer is the server API for Test service.
// All implementations must embed UnimplementedTestServer
// for forward compatibility
type TestServer interface {
	StartTestCase(context.Context, *StartTestCaseMessage) (*emptypb.Empty, error)
	mustEmbedUnimplementedTestServer()
}

// UnimplementedTestServer must be embedded to have forward compatible implementations.
type UnimplementedTestServer struct {
}

func (UnimplementedTestServer) StartTestCase(context.Context, *StartTestCaseMessage) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method StartTestCase not implemented")
}
func (UnimplementedTestServer) mustEmbedUnimplementedTestServer() {}

// UnsafeTestServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to TestServer will
// result in compilation errors.
type UnsafeTestServer interface {
	mustEmbedUnimplementedTestServer()
}

func RegisterTestServer(s grpc.ServiceRegistrar, srv TestServer) {
	s.RegisterService(&Test_ServiceDesc, srv)
}

func _Test_StartTestCase_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StartTestCaseMessage)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TestServer).StartTestCase(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/consensus.Test/StartTestCase",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TestServer).StartTestCase(ctx, req.(*StartTestCaseMessage))
	}
	return interceptor(ctx, in, info, handler)
}

// Test_ServiceDesc is the grpc.ServiceDesc for Test service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Test_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "consensus.Test",
	HandlerType: (*TestServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "StartTestCase",
			Handler:    _Test_StartTestCase_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "consensus_engine/consensus.proto",
}
