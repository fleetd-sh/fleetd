// Code generated by protoc-gen-connect-go. DO NOT EDIT.
//
// Source: device/v1/device.proto

package devicev1connect

import (
	connect "connectrpc.com/connect"
	context "context"
	errors "errors"
	v1 "fleetd.sh/gen/device/v1"
	http "net/http"
	strings "strings"
)

// This is a compile-time assertion to ensure that this generated file and the connect package are
// compatible. If you get a compiler error that this constant is not defined, this code was
// generated with a version of connect newer than the one compiled into your binary. You can fix the
// problem by either regenerating this code with an older version of connect or updating the connect
// version compiled into your binary.
const _ = connect.IsAtLeastVersion1_13_0

const (
	// DeviceServiceName is the fully-qualified name of the DeviceService service.
	DeviceServiceName = "device.v1.DeviceService"
)

// These constants are the fully-qualified names of the RPCs defined in this package. They're
// exposed at runtime as Spec.Procedure and as the final two segments of the HTTP route.
//
// Note that these are different from the fully-qualified method names used by
// google.golang.org/protobuf/reflect/protoreflect. To convert from these constants to
// reflection-formatted method names, remove the leading slash and convert the remaining slash to a
// period.
const (
	// DeviceServiceRegisterDeviceProcedure is the fully-qualified name of the DeviceService's
	// RegisterDevice RPC.
	DeviceServiceRegisterDeviceProcedure = "/device.v1.DeviceService/RegisterDevice"
	// DeviceServiceUnregisterDeviceProcedure is the fully-qualified name of the DeviceService's
	// UnregisterDevice RPC.
	DeviceServiceUnregisterDeviceProcedure = "/device.v1.DeviceService/UnregisterDevice"
	// DeviceServiceGetDeviceProcedure is the fully-qualified name of the DeviceService's GetDevice RPC.
	DeviceServiceGetDeviceProcedure = "/device.v1.DeviceService/GetDevice"
	// DeviceServiceListDevicesProcedure is the fully-qualified name of the DeviceService's ListDevices
	// RPC.
	DeviceServiceListDevicesProcedure = "/device.v1.DeviceService/ListDevices"
	// DeviceServiceUpdateDeviceStatusProcedure is the fully-qualified name of the DeviceService's
	// UpdateDeviceStatus RPC.
	DeviceServiceUpdateDeviceStatusProcedure = "/device.v1.DeviceService/UpdateDeviceStatus"
	// DeviceServiceUpdateDeviceProcedure is the fully-qualified name of the DeviceService's
	// UpdateDevice RPC.
	DeviceServiceUpdateDeviceProcedure = "/device.v1.DeviceService/UpdateDevice"
)

// These variables are the protoreflect.Descriptor objects for the RPCs defined in this package.
var (
	deviceServiceServiceDescriptor                  = v1.File_device_v1_device_proto.Services().ByName("DeviceService")
	deviceServiceRegisterDeviceMethodDescriptor     = deviceServiceServiceDescriptor.Methods().ByName("RegisterDevice")
	deviceServiceUnregisterDeviceMethodDescriptor   = deviceServiceServiceDescriptor.Methods().ByName("UnregisterDevice")
	deviceServiceGetDeviceMethodDescriptor          = deviceServiceServiceDescriptor.Methods().ByName("GetDevice")
	deviceServiceListDevicesMethodDescriptor        = deviceServiceServiceDescriptor.Methods().ByName("ListDevices")
	deviceServiceUpdateDeviceStatusMethodDescriptor = deviceServiceServiceDescriptor.Methods().ByName("UpdateDeviceStatus")
	deviceServiceUpdateDeviceMethodDescriptor       = deviceServiceServiceDescriptor.Methods().ByName("UpdateDevice")
)

// DeviceServiceClient is a client for the device.v1.DeviceService service.
type DeviceServiceClient interface {
	RegisterDevice(context.Context, *connect.Request[v1.RegisterDeviceRequest]) (*connect.Response[v1.RegisterDeviceResponse], error)
	UnregisterDevice(context.Context, *connect.Request[v1.UnregisterDeviceRequest]) (*connect.Response[v1.UnregisterDeviceResponse], error)
	GetDevice(context.Context, *connect.Request[v1.GetDeviceRequest]) (*connect.Response[v1.GetDeviceResponse], error)
	ListDevices(context.Context, *connect.Request[v1.ListDevicesRequest]) (*connect.ServerStreamForClient[v1.ListDevicesResponse], error)
	UpdateDeviceStatus(context.Context, *connect.Request[v1.UpdateDeviceStatusRequest]) (*connect.Response[v1.UpdateDeviceStatusResponse], error)
	UpdateDevice(context.Context, *connect.Request[v1.UpdateDeviceRequest]) (*connect.Response[v1.UpdateDeviceResponse], error)
}

// NewDeviceServiceClient constructs a client for the device.v1.DeviceService service. By default,
// it uses the Connect protocol with the binary Protobuf Codec, asks for gzipped responses, and
// sends uncompressed requests. To use the gRPC or gRPC-Web protocols, supply the connect.WithGRPC()
// or connect.WithGRPCWeb() options.
//
// The URL supplied here should be the base URL for the Connect or gRPC server (for example,
// http://api.acme.com or https://acme.com/grpc).
func NewDeviceServiceClient(httpClient connect.HTTPClient, baseURL string, opts ...connect.ClientOption) DeviceServiceClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &deviceServiceClient{
		registerDevice: connect.NewClient[v1.RegisterDeviceRequest, v1.RegisterDeviceResponse](
			httpClient,
			baseURL+DeviceServiceRegisterDeviceProcedure,
			connect.WithSchema(deviceServiceRegisterDeviceMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		unregisterDevice: connect.NewClient[v1.UnregisterDeviceRequest, v1.UnregisterDeviceResponse](
			httpClient,
			baseURL+DeviceServiceUnregisterDeviceProcedure,
			connect.WithSchema(deviceServiceUnregisterDeviceMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		getDevice: connect.NewClient[v1.GetDeviceRequest, v1.GetDeviceResponse](
			httpClient,
			baseURL+DeviceServiceGetDeviceProcedure,
			connect.WithSchema(deviceServiceGetDeviceMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		listDevices: connect.NewClient[v1.ListDevicesRequest, v1.ListDevicesResponse](
			httpClient,
			baseURL+DeviceServiceListDevicesProcedure,
			connect.WithSchema(deviceServiceListDevicesMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		updateDeviceStatus: connect.NewClient[v1.UpdateDeviceStatusRequest, v1.UpdateDeviceStatusResponse](
			httpClient,
			baseURL+DeviceServiceUpdateDeviceStatusProcedure,
			connect.WithSchema(deviceServiceUpdateDeviceStatusMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		updateDevice: connect.NewClient[v1.UpdateDeviceRequest, v1.UpdateDeviceResponse](
			httpClient,
			baseURL+DeviceServiceUpdateDeviceProcedure,
			connect.WithSchema(deviceServiceUpdateDeviceMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
	}
}

// deviceServiceClient implements DeviceServiceClient.
type deviceServiceClient struct {
	registerDevice     *connect.Client[v1.RegisterDeviceRequest, v1.RegisterDeviceResponse]
	unregisterDevice   *connect.Client[v1.UnregisterDeviceRequest, v1.UnregisterDeviceResponse]
	getDevice          *connect.Client[v1.GetDeviceRequest, v1.GetDeviceResponse]
	listDevices        *connect.Client[v1.ListDevicesRequest, v1.ListDevicesResponse]
	updateDeviceStatus *connect.Client[v1.UpdateDeviceStatusRequest, v1.UpdateDeviceStatusResponse]
	updateDevice       *connect.Client[v1.UpdateDeviceRequest, v1.UpdateDeviceResponse]
}

// RegisterDevice calls device.v1.DeviceService.RegisterDevice.
func (c *deviceServiceClient) RegisterDevice(ctx context.Context, req *connect.Request[v1.RegisterDeviceRequest]) (*connect.Response[v1.RegisterDeviceResponse], error) {
	return c.registerDevice.CallUnary(ctx, req)
}

// UnregisterDevice calls device.v1.DeviceService.UnregisterDevice.
func (c *deviceServiceClient) UnregisterDevice(ctx context.Context, req *connect.Request[v1.UnregisterDeviceRequest]) (*connect.Response[v1.UnregisterDeviceResponse], error) {
	return c.unregisterDevice.CallUnary(ctx, req)
}

// GetDevice calls device.v1.DeviceService.GetDevice.
func (c *deviceServiceClient) GetDevice(ctx context.Context, req *connect.Request[v1.GetDeviceRequest]) (*connect.Response[v1.GetDeviceResponse], error) {
	return c.getDevice.CallUnary(ctx, req)
}

// ListDevices calls device.v1.DeviceService.ListDevices.
func (c *deviceServiceClient) ListDevices(ctx context.Context, req *connect.Request[v1.ListDevicesRequest]) (*connect.ServerStreamForClient[v1.ListDevicesResponse], error) {
	return c.listDevices.CallServerStream(ctx, req)
}

// UpdateDeviceStatus calls device.v1.DeviceService.UpdateDeviceStatus.
func (c *deviceServiceClient) UpdateDeviceStatus(ctx context.Context, req *connect.Request[v1.UpdateDeviceStatusRequest]) (*connect.Response[v1.UpdateDeviceStatusResponse], error) {
	return c.updateDeviceStatus.CallUnary(ctx, req)
}

// UpdateDevice calls device.v1.DeviceService.UpdateDevice.
func (c *deviceServiceClient) UpdateDevice(ctx context.Context, req *connect.Request[v1.UpdateDeviceRequest]) (*connect.Response[v1.UpdateDeviceResponse], error) {
	return c.updateDevice.CallUnary(ctx, req)
}

// DeviceServiceHandler is an implementation of the device.v1.DeviceService service.
type DeviceServiceHandler interface {
	RegisterDevice(context.Context, *connect.Request[v1.RegisterDeviceRequest]) (*connect.Response[v1.RegisterDeviceResponse], error)
	UnregisterDevice(context.Context, *connect.Request[v1.UnregisterDeviceRequest]) (*connect.Response[v1.UnregisterDeviceResponse], error)
	GetDevice(context.Context, *connect.Request[v1.GetDeviceRequest]) (*connect.Response[v1.GetDeviceResponse], error)
	ListDevices(context.Context, *connect.Request[v1.ListDevicesRequest], *connect.ServerStream[v1.ListDevicesResponse]) error
	UpdateDeviceStatus(context.Context, *connect.Request[v1.UpdateDeviceStatusRequest]) (*connect.Response[v1.UpdateDeviceStatusResponse], error)
	UpdateDevice(context.Context, *connect.Request[v1.UpdateDeviceRequest]) (*connect.Response[v1.UpdateDeviceResponse], error)
}

// NewDeviceServiceHandler builds an HTTP handler from the service implementation. It returns the
// path on which to mount the handler and the handler itself.
//
// By default, handlers support the Connect, gRPC, and gRPC-Web protocols with the binary Protobuf
// and JSON codecs. They also support gzip compression.
func NewDeviceServiceHandler(svc DeviceServiceHandler, opts ...connect.HandlerOption) (string, http.Handler) {
	deviceServiceRegisterDeviceHandler := connect.NewUnaryHandler(
		DeviceServiceRegisterDeviceProcedure,
		svc.RegisterDevice,
		connect.WithSchema(deviceServiceRegisterDeviceMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	deviceServiceUnregisterDeviceHandler := connect.NewUnaryHandler(
		DeviceServiceUnregisterDeviceProcedure,
		svc.UnregisterDevice,
		connect.WithSchema(deviceServiceUnregisterDeviceMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	deviceServiceGetDeviceHandler := connect.NewUnaryHandler(
		DeviceServiceGetDeviceProcedure,
		svc.GetDevice,
		connect.WithSchema(deviceServiceGetDeviceMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	deviceServiceListDevicesHandler := connect.NewServerStreamHandler(
		DeviceServiceListDevicesProcedure,
		svc.ListDevices,
		connect.WithSchema(deviceServiceListDevicesMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	deviceServiceUpdateDeviceStatusHandler := connect.NewUnaryHandler(
		DeviceServiceUpdateDeviceStatusProcedure,
		svc.UpdateDeviceStatus,
		connect.WithSchema(deviceServiceUpdateDeviceStatusMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	deviceServiceUpdateDeviceHandler := connect.NewUnaryHandler(
		DeviceServiceUpdateDeviceProcedure,
		svc.UpdateDevice,
		connect.WithSchema(deviceServiceUpdateDeviceMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	return "/device.v1.DeviceService/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case DeviceServiceRegisterDeviceProcedure:
			deviceServiceRegisterDeviceHandler.ServeHTTP(w, r)
		case DeviceServiceUnregisterDeviceProcedure:
			deviceServiceUnregisterDeviceHandler.ServeHTTP(w, r)
		case DeviceServiceGetDeviceProcedure:
			deviceServiceGetDeviceHandler.ServeHTTP(w, r)
		case DeviceServiceListDevicesProcedure:
			deviceServiceListDevicesHandler.ServeHTTP(w, r)
		case DeviceServiceUpdateDeviceStatusProcedure:
			deviceServiceUpdateDeviceStatusHandler.ServeHTTP(w, r)
		case DeviceServiceUpdateDeviceProcedure:
			deviceServiceUpdateDeviceHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// UnimplementedDeviceServiceHandler returns CodeUnimplemented from all methods.
type UnimplementedDeviceServiceHandler struct{}

func (UnimplementedDeviceServiceHandler) RegisterDevice(context.Context, *connect.Request[v1.RegisterDeviceRequest]) (*connect.Response[v1.RegisterDeviceResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("device.v1.DeviceService.RegisterDevice is not implemented"))
}

func (UnimplementedDeviceServiceHandler) UnregisterDevice(context.Context, *connect.Request[v1.UnregisterDeviceRequest]) (*connect.Response[v1.UnregisterDeviceResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("device.v1.DeviceService.UnregisterDevice is not implemented"))
}

func (UnimplementedDeviceServiceHandler) GetDevice(context.Context, *connect.Request[v1.GetDeviceRequest]) (*connect.Response[v1.GetDeviceResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("device.v1.DeviceService.GetDevice is not implemented"))
}

func (UnimplementedDeviceServiceHandler) ListDevices(context.Context, *connect.Request[v1.ListDevicesRequest], *connect.ServerStream[v1.ListDevicesResponse]) error {
	return connect.NewError(connect.CodeUnimplemented, errors.New("device.v1.DeviceService.ListDevices is not implemented"))
}

func (UnimplementedDeviceServiceHandler) UpdateDeviceStatus(context.Context, *connect.Request[v1.UpdateDeviceStatusRequest]) (*connect.Response[v1.UpdateDeviceStatusResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("device.v1.DeviceService.UpdateDeviceStatus is not implemented"))
}

func (UnimplementedDeviceServiceHandler) UpdateDevice(context.Context, *connect.Request[v1.UpdateDeviceRequest]) (*connect.Response[v1.UpdateDeviceResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("device.v1.DeviceService.UpdateDevice is not implemented"))
}
