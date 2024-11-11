// Code generated by protoc-gen-connect-go. DO NOT EDIT.
//
// Source: fleetd/v1/analytics.proto

package fleetpbconnect

import (
	connect "connectrpc.com/connect"
	context "context"
	errors "errors"
	v1 "fleetd.sh/gen/fleetd/v1"
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
	// AnalyticsServiceName is the fully-qualified name of the AnalyticsService service.
	AnalyticsServiceName = "fleetd.v1.AnalyticsService"
)

// These constants are the fully-qualified names of the RPCs defined in this package. They're
// exposed at runtime as Spec.Procedure and as the final two segments of the HTTP route.
//
// Note that these are different from the fully-qualified method names used by
// google.golang.org/protobuf/reflect/protoreflect. To convert from these constants to
// reflection-formatted method names, remove the leading slash and convert the remaining slash to a
// period.
const (
	// AnalyticsServiceGetDeviceMetricsProcedure is the fully-qualified name of the AnalyticsService's
	// GetDeviceMetrics RPC.
	AnalyticsServiceGetDeviceMetricsProcedure = "/fleetd.v1.AnalyticsService/GetDeviceMetrics"
	// AnalyticsServiceGetUpdateAnalyticsProcedure is the fully-qualified name of the AnalyticsService's
	// GetUpdateAnalytics RPC.
	AnalyticsServiceGetUpdateAnalyticsProcedure = "/fleetd.v1.AnalyticsService/GetUpdateAnalytics"
	// AnalyticsServiceGetDeviceHealthProcedure is the fully-qualified name of the AnalyticsService's
	// GetDeviceHealth RPC.
	AnalyticsServiceGetDeviceHealthProcedure = "/fleetd.v1.AnalyticsService/GetDeviceHealth"
	// AnalyticsServiceGetPerformanceMetricsProcedure is the fully-qualified name of the
	// AnalyticsService's GetPerformanceMetrics RPC.
	AnalyticsServiceGetPerformanceMetricsProcedure = "/fleetd.v1.AnalyticsService/GetPerformanceMetrics"
)

// These variables are the protoreflect.Descriptor objects for the RPCs defined in this package.
var (
	analyticsServiceServiceDescriptor                     = v1.File_fleetd_v1_analytics_proto.Services().ByName("AnalyticsService")
	analyticsServiceGetDeviceMetricsMethodDescriptor      = analyticsServiceServiceDescriptor.Methods().ByName("GetDeviceMetrics")
	analyticsServiceGetUpdateAnalyticsMethodDescriptor    = analyticsServiceServiceDescriptor.Methods().ByName("GetUpdateAnalytics")
	analyticsServiceGetDeviceHealthMethodDescriptor       = analyticsServiceServiceDescriptor.Methods().ByName("GetDeviceHealth")
	analyticsServiceGetPerformanceMetricsMethodDescriptor = analyticsServiceServiceDescriptor.Methods().ByName("GetPerformanceMetrics")
)

// AnalyticsServiceClient is a client for the fleetd.v1.AnalyticsService service.
type AnalyticsServiceClient interface {
	// Get device metrics aggregation
	GetDeviceMetrics(context.Context, *connect.Request[v1.GetDeviceMetricsRequest]) (*connect.Response[v1.GetDeviceMetricsResponse], error)
	// Get update campaign analytics
	GetUpdateAnalytics(context.Context, *connect.Request[v1.GetUpdateAnalyticsRequest]) (*connect.Response[v1.GetUpdateAnalyticsResponse], error)
	// Get device health metrics
	GetDeviceHealth(context.Context, *connect.Request[v1.GetDeviceHealthRequest]) (*connect.Response[v1.GetDeviceHealthResponse], error)
	// Get performance metrics
	GetPerformanceMetrics(context.Context, *connect.Request[v1.GetPerformanceMetricsRequest]) (*connect.Response[v1.GetPerformanceMetricsResponse], error)
}

// NewAnalyticsServiceClient constructs a client for the fleetd.v1.AnalyticsService service. By
// default, it uses the Connect protocol with the binary Protobuf Codec, asks for gzipped responses,
// and sends uncompressed requests. To use the gRPC or gRPC-Web protocols, supply the
// connect.WithGRPC() or connect.WithGRPCWeb() options.
//
// The URL supplied here should be the base URL for the Connect or gRPC server (for example,
// http://api.acme.com or https://acme.com/grpc).
func NewAnalyticsServiceClient(httpClient connect.HTTPClient, baseURL string, opts ...connect.ClientOption) AnalyticsServiceClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &analyticsServiceClient{
		getDeviceMetrics: connect.NewClient[v1.GetDeviceMetricsRequest, v1.GetDeviceMetricsResponse](
			httpClient,
			baseURL+AnalyticsServiceGetDeviceMetricsProcedure,
			connect.WithSchema(analyticsServiceGetDeviceMetricsMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		getUpdateAnalytics: connect.NewClient[v1.GetUpdateAnalyticsRequest, v1.GetUpdateAnalyticsResponse](
			httpClient,
			baseURL+AnalyticsServiceGetUpdateAnalyticsProcedure,
			connect.WithSchema(analyticsServiceGetUpdateAnalyticsMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		getDeviceHealth: connect.NewClient[v1.GetDeviceHealthRequest, v1.GetDeviceHealthResponse](
			httpClient,
			baseURL+AnalyticsServiceGetDeviceHealthProcedure,
			connect.WithSchema(analyticsServiceGetDeviceHealthMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		getPerformanceMetrics: connect.NewClient[v1.GetPerformanceMetricsRequest, v1.GetPerformanceMetricsResponse](
			httpClient,
			baseURL+AnalyticsServiceGetPerformanceMetricsProcedure,
			connect.WithSchema(analyticsServiceGetPerformanceMetricsMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
	}
}

// analyticsServiceClient implements AnalyticsServiceClient.
type analyticsServiceClient struct {
	getDeviceMetrics      *connect.Client[v1.GetDeviceMetricsRequest, v1.GetDeviceMetricsResponse]
	getUpdateAnalytics    *connect.Client[v1.GetUpdateAnalyticsRequest, v1.GetUpdateAnalyticsResponse]
	getDeviceHealth       *connect.Client[v1.GetDeviceHealthRequest, v1.GetDeviceHealthResponse]
	getPerformanceMetrics *connect.Client[v1.GetPerformanceMetricsRequest, v1.GetPerformanceMetricsResponse]
}

// GetDeviceMetrics calls fleetd.v1.AnalyticsService.GetDeviceMetrics.
func (c *analyticsServiceClient) GetDeviceMetrics(ctx context.Context, req *connect.Request[v1.GetDeviceMetricsRequest]) (*connect.Response[v1.GetDeviceMetricsResponse], error) {
	return c.getDeviceMetrics.CallUnary(ctx, req)
}

// GetUpdateAnalytics calls fleetd.v1.AnalyticsService.GetUpdateAnalytics.
func (c *analyticsServiceClient) GetUpdateAnalytics(ctx context.Context, req *connect.Request[v1.GetUpdateAnalyticsRequest]) (*connect.Response[v1.GetUpdateAnalyticsResponse], error) {
	return c.getUpdateAnalytics.CallUnary(ctx, req)
}

// GetDeviceHealth calls fleetd.v1.AnalyticsService.GetDeviceHealth.
func (c *analyticsServiceClient) GetDeviceHealth(ctx context.Context, req *connect.Request[v1.GetDeviceHealthRequest]) (*connect.Response[v1.GetDeviceHealthResponse], error) {
	return c.getDeviceHealth.CallUnary(ctx, req)
}

// GetPerformanceMetrics calls fleetd.v1.AnalyticsService.GetPerformanceMetrics.
func (c *analyticsServiceClient) GetPerformanceMetrics(ctx context.Context, req *connect.Request[v1.GetPerformanceMetricsRequest]) (*connect.Response[v1.GetPerformanceMetricsResponse], error) {
	return c.getPerformanceMetrics.CallUnary(ctx, req)
}

// AnalyticsServiceHandler is an implementation of the fleetd.v1.AnalyticsService service.
type AnalyticsServiceHandler interface {
	// Get device metrics aggregation
	GetDeviceMetrics(context.Context, *connect.Request[v1.GetDeviceMetricsRequest]) (*connect.Response[v1.GetDeviceMetricsResponse], error)
	// Get update campaign analytics
	GetUpdateAnalytics(context.Context, *connect.Request[v1.GetUpdateAnalyticsRequest]) (*connect.Response[v1.GetUpdateAnalyticsResponse], error)
	// Get device health metrics
	GetDeviceHealth(context.Context, *connect.Request[v1.GetDeviceHealthRequest]) (*connect.Response[v1.GetDeviceHealthResponse], error)
	// Get performance metrics
	GetPerformanceMetrics(context.Context, *connect.Request[v1.GetPerformanceMetricsRequest]) (*connect.Response[v1.GetPerformanceMetricsResponse], error)
}

// NewAnalyticsServiceHandler builds an HTTP handler from the service implementation. It returns the
// path on which to mount the handler and the handler itself.
//
// By default, handlers support the Connect, gRPC, and gRPC-Web protocols with the binary Protobuf
// and JSON codecs. They also support gzip compression.
func NewAnalyticsServiceHandler(svc AnalyticsServiceHandler, opts ...connect.HandlerOption) (string, http.Handler) {
	analyticsServiceGetDeviceMetricsHandler := connect.NewUnaryHandler(
		AnalyticsServiceGetDeviceMetricsProcedure,
		svc.GetDeviceMetrics,
		connect.WithSchema(analyticsServiceGetDeviceMetricsMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	analyticsServiceGetUpdateAnalyticsHandler := connect.NewUnaryHandler(
		AnalyticsServiceGetUpdateAnalyticsProcedure,
		svc.GetUpdateAnalytics,
		connect.WithSchema(analyticsServiceGetUpdateAnalyticsMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	analyticsServiceGetDeviceHealthHandler := connect.NewUnaryHandler(
		AnalyticsServiceGetDeviceHealthProcedure,
		svc.GetDeviceHealth,
		connect.WithSchema(analyticsServiceGetDeviceHealthMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	analyticsServiceGetPerformanceMetricsHandler := connect.NewUnaryHandler(
		AnalyticsServiceGetPerformanceMetricsProcedure,
		svc.GetPerformanceMetrics,
		connect.WithSchema(analyticsServiceGetPerformanceMetricsMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	return "/fleetd.v1.AnalyticsService/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case AnalyticsServiceGetDeviceMetricsProcedure:
			analyticsServiceGetDeviceMetricsHandler.ServeHTTP(w, r)
		case AnalyticsServiceGetUpdateAnalyticsProcedure:
			analyticsServiceGetUpdateAnalyticsHandler.ServeHTTP(w, r)
		case AnalyticsServiceGetDeviceHealthProcedure:
			analyticsServiceGetDeviceHealthHandler.ServeHTTP(w, r)
		case AnalyticsServiceGetPerformanceMetricsProcedure:
			analyticsServiceGetPerformanceMetricsHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// UnimplementedAnalyticsServiceHandler returns CodeUnimplemented from all methods.
type UnimplementedAnalyticsServiceHandler struct{}

func (UnimplementedAnalyticsServiceHandler) GetDeviceMetrics(context.Context, *connect.Request[v1.GetDeviceMetricsRequest]) (*connect.Response[v1.GetDeviceMetricsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("fleetd.v1.AnalyticsService.GetDeviceMetrics is not implemented"))
}

func (UnimplementedAnalyticsServiceHandler) GetUpdateAnalytics(context.Context, *connect.Request[v1.GetUpdateAnalyticsRequest]) (*connect.Response[v1.GetUpdateAnalyticsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("fleetd.v1.AnalyticsService.GetUpdateAnalytics is not implemented"))
}

func (UnimplementedAnalyticsServiceHandler) GetDeviceHealth(context.Context, *connect.Request[v1.GetDeviceHealthRequest]) (*connect.Response[v1.GetDeviceHealthResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("fleetd.v1.AnalyticsService.GetDeviceHealth is not implemented"))
}

func (UnimplementedAnalyticsServiceHandler) GetPerformanceMetrics(context.Context, *connect.Request[v1.GetPerformanceMetricsRequest]) (*connect.Response[v1.GetPerformanceMetricsResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("fleetd.v1.AnalyticsService.GetPerformanceMetrics is not implemented"))
}
