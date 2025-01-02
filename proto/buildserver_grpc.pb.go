// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.12.4
// source: buildserver.proto

package proto

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// BuildServiceClient is the client API for BuildService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type BuildServiceClient interface {
	Build(ctx context.Context, in *BuildRequest, opts ...grpc.CallOption) (*BuildResponse, error)
	GetVersions(ctx context.Context, in *VersionRequest, opts ...grpc.CallOption) (*VersionResponse, error)
	ReportCrash(ctx context.Context, in *CrashRequest, opts ...grpc.CallOption) (*CrashResponse, error)
}

type buildServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewBuildServiceClient(cc grpc.ClientConnInterface) BuildServiceClient {
	return &buildServiceClient{cc}
}

func (c *buildServiceClient) Build(ctx context.Context, in *BuildRequest, opts ...grpc.CallOption) (*BuildResponse, error) {
	out := new(BuildResponse)
	err := c.cc.Invoke(ctx, "/buildserver.BuildService/Build", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *buildServiceClient) GetVersions(ctx context.Context, in *VersionRequest, opts ...grpc.CallOption) (*VersionResponse, error) {
	out := new(VersionResponse)
	err := c.cc.Invoke(ctx, "/buildserver.BuildService/GetVersions", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *buildServiceClient) ReportCrash(ctx context.Context, in *CrashRequest, opts ...grpc.CallOption) (*CrashResponse, error) {
	out := new(CrashResponse)
	err := c.cc.Invoke(ctx, "/buildserver.BuildService/ReportCrash", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BuildServiceServer is the server API for BuildService service.
// All implementations should embed UnimplementedBuildServiceServer
// for forward compatibility
type BuildServiceServer interface {
	Build(context.Context, *BuildRequest) (*BuildResponse, error)
	GetVersions(context.Context, *VersionRequest) (*VersionResponse, error)
	ReportCrash(context.Context, *CrashRequest) (*CrashResponse, error)
}

// UnimplementedBuildServiceServer should be embedded to have forward compatible implementations.
type UnimplementedBuildServiceServer struct {
}

func (UnimplementedBuildServiceServer) Build(context.Context, *BuildRequest) (*BuildResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Build not implemented")
}
func (UnimplementedBuildServiceServer) GetVersions(context.Context, *VersionRequest) (*VersionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetVersions not implemented")
}
func (UnimplementedBuildServiceServer) ReportCrash(context.Context, *CrashRequest) (*CrashResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReportCrash not implemented")
}

// UnsafeBuildServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to BuildServiceServer will
// result in compilation errors.
type UnsafeBuildServiceServer interface {
	mustEmbedUnimplementedBuildServiceServer()
}

func RegisterBuildServiceServer(s grpc.ServiceRegistrar, srv BuildServiceServer) {
	s.RegisterService(&BuildService_ServiceDesc, srv)
}

func _BuildService_Build_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(BuildRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BuildServiceServer).Build(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/buildserver.BuildService/Build",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BuildServiceServer).Build(ctx, req.(*BuildRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BuildService_GetVersions_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(VersionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BuildServiceServer).GetVersions(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/buildserver.BuildService/GetVersions",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BuildServiceServer).GetVersions(ctx, req.(*VersionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _BuildService_ReportCrash_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CrashRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BuildServiceServer).ReportCrash(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/buildserver.BuildService/ReportCrash",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BuildServiceServer).ReportCrash(ctx, req.(*CrashRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// BuildService_ServiceDesc is the grpc.ServiceDesc for BuildService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var BuildService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "buildserver.BuildService",
	HandlerType: (*BuildServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Build",
			Handler:    _BuildService_Build_Handler,
		},
		{
			MethodName: "GetVersions",
			Handler:    _BuildService_GetVersions_Handler,
		},
		{
			MethodName: "ReportCrash",
			Handler:    _BuildService_ReportCrash_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "buildserver.proto",
}
