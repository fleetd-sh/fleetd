// Code generated by protoc-gen-connect-go. DO NOT EDIT.
//
// Source: auth/v1/auth.proto

package authv1connect

import (
	connect "connectrpc.com/connect"
	context "context"
	errors "errors"
	v1 "fleetd.sh/gen/auth/v1"
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
	// AuthServiceName is the fully-qualified name of the AuthService service.
	AuthServiceName = "auth.v1.AuthService"
)

// These constants are the fully-qualified names of the RPCs defined in this package. They're
// exposed at runtime as Spec.Procedure and as the final two segments of the HTTP route.
//
// Note that these are different from the fully-qualified method names used by
// google.golang.org/protobuf/reflect/protoreflect. To convert from these constants to
// reflection-formatted method names, remove the leading slash and convert the remaining slash to a
// period.
const (
	// AuthServiceAuthenticateProcedure is the fully-qualified name of the AuthService's Authenticate
	// RPC.
	AuthServiceAuthenticateProcedure = "/auth.v1.AuthService/Authenticate"
	// AuthServiceGenerateAPIKeyProcedure is the fully-qualified name of the AuthService's
	// GenerateAPIKey RPC.
	AuthServiceGenerateAPIKeyProcedure = "/auth.v1.AuthService/GenerateAPIKey"
	// AuthServiceRevokeAPIKeyProcedure is the fully-qualified name of the AuthService's RevokeAPIKey
	// RPC.
	AuthServiceRevokeAPIKeyProcedure = "/auth.v1.AuthService/RevokeAPIKey"
)

// These variables are the protoreflect.Descriptor objects for the RPCs defined in this package.
var (
	authServiceServiceDescriptor              = v1.File_auth_v1_auth_proto.Services().ByName("AuthService")
	authServiceAuthenticateMethodDescriptor   = authServiceServiceDescriptor.Methods().ByName("Authenticate")
	authServiceGenerateAPIKeyMethodDescriptor = authServiceServiceDescriptor.Methods().ByName("GenerateAPIKey")
	authServiceRevokeAPIKeyMethodDescriptor   = authServiceServiceDescriptor.Methods().ByName("RevokeAPIKey")
)

// AuthServiceClient is a client for the auth.v1.AuthService service.
type AuthServiceClient interface {
	Authenticate(context.Context, *connect.Request[v1.AuthenticateRequest]) (*connect.Response[v1.AuthenticateResponse], error)
	GenerateAPIKey(context.Context, *connect.Request[v1.GenerateAPIKeyRequest]) (*connect.Response[v1.GenerateAPIKeyResponse], error)
	RevokeAPIKey(context.Context, *connect.Request[v1.RevokeAPIKeyRequest]) (*connect.Response[v1.RevokeAPIKeyResponse], error)
}

// NewAuthServiceClient constructs a client for the auth.v1.AuthService service. By default, it uses
// the Connect protocol with the binary Protobuf Codec, asks for gzipped responses, and sends
// uncompressed requests. To use the gRPC or gRPC-Web protocols, supply the connect.WithGRPC() or
// connect.WithGRPCWeb() options.
//
// The URL supplied here should be the base URL for the Connect or gRPC server (for example,
// http://api.acme.com or https://acme.com/grpc).
func NewAuthServiceClient(httpClient connect.HTTPClient, baseURL string, opts ...connect.ClientOption) AuthServiceClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &authServiceClient{
		authenticate: connect.NewClient[v1.AuthenticateRequest, v1.AuthenticateResponse](
			httpClient,
			baseURL+AuthServiceAuthenticateProcedure,
			connect.WithSchema(authServiceAuthenticateMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		generateAPIKey: connect.NewClient[v1.GenerateAPIKeyRequest, v1.GenerateAPIKeyResponse](
			httpClient,
			baseURL+AuthServiceGenerateAPIKeyProcedure,
			connect.WithSchema(authServiceGenerateAPIKeyMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
		revokeAPIKey: connect.NewClient[v1.RevokeAPIKeyRequest, v1.RevokeAPIKeyResponse](
			httpClient,
			baseURL+AuthServiceRevokeAPIKeyProcedure,
			connect.WithSchema(authServiceRevokeAPIKeyMethodDescriptor),
			connect.WithClientOptions(opts...),
		),
	}
}

// authServiceClient implements AuthServiceClient.
type authServiceClient struct {
	authenticate   *connect.Client[v1.AuthenticateRequest, v1.AuthenticateResponse]
	generateAPIKey *connect.Client[v1.GenerateAPIKeyRequest, v1.GenerateAPIKeyResponse]
	revokeAPIKey   *connect.Client[v1.RevokeAPIKeyRequest, v1.RevokeAPIKeyResponse]
}

// Authenticate calls auth.v1.AuthService.Authenticate.
func (c *authServiceClient) Authenticate(ctx context.Context, req *connect.Request[v1.AuthenticateRequest]) (*connect.Response[v1.AuthenticateResponse], error) {
	return c.authenticate.CallUnary(ctx, req)
}

// GenerateAPIKey calls auth.v1.AuthService.GenerateAPIKey.
func (c *authServiceClient) GenerateAPIKey(ctx context.Context, req *connect.Request[v1.GenerateAPIKeyRequest]) (*connect.Response[v1.GenerateAPIKeyResponse], error) {
	return c.generateAPIKey.CallUnary(ctx, req)
}

// RevokeAPIKey calls auth.v1.AuthService.RevokeAPIKey.
func (c *authServiceClient) RevokeAPIKey(ctx context.Context, req *connect.Request[v1.RevokeAPIKeyRequest]) (*connect.Response[v1.RevokeAPIKeyResponse], error) {
	return c.revokeAPIKey.CallUnary(ctx, req)
}

// AuthServiceHandler is an implementation of the auth.v1.AuthService service.
type AuthServiceHandler interface {
	Authenticate(context.Context, *connect.Request[v1.AuthenticateRequest]) (*connect.Response[v1.AuthenticateResponse], error)
	GenerateAPIKey(context.Context, *connect.Request[v1.GenerateAPIKeyRequest]) (*connect.Response[v1.GenerateAPIKeyResponse], error)
	RevokeAPIKey(context.Context, *connect.Request[v1.RevokeAPIKeyRequest]) (*connect.Response[v1.RevokeAPIKeyResponse], error)
}

// NewAuthServiceHandler builds an HTTP handler from the service implementation. It returns the path
// on which to mount the handler and the handler itself.
//
// By default, handlers support the Connect, gRPC, and gRPC-Web protocols with the binary Protobuf
// and JSON codecs. They also support gzip compression.
func NewAuthServiceHandler(svc AuthServiceHandler, opts ...connect.HandlerOption) (string, http.Handler) {
	authServiceAuthenticateHandler := connect.NewUnaryHandler(
		AuthServiceAuthenticateProcedure,
		svc.Authenticate,
		connect.WithSchema(authServiceAuthenticateMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	authServiceGenerateAPIKeyHandler := connect.NewUnaryHandler(
		AuthServiceGenerateAPIKeyProcedure,
		svc.GenerateAPIKey,
		connect.WithSchema(authServiceGenerateAPIKeyMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	authServiceRevokeAPIKeyHandler := connect.NewUnaryHandler(
		AuthServiceRevokeAPIKeyProcedure,
		svc.RevokeAPIKey,
		connect.WithSchema(authServiceRevokeAPIKeyMethodDescriptor),
		connect.WithHandlerOptions(opts...),
	)
	return "/auth.v1.AuthService/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case AuthServiceAuthenticateProcedure:
			authServiceAuthenticateHandler.ServeHTTP(w, r)
		case AuthServiceGenerateAPIKeyProcedure:
			authServiceGenerateAPIKeyHandler.ServeHTTP(w, r)
		case AuthServiceRevokeAPIKeyProcedure:
			authServiceRevokeAPIKeyHandler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// UnimplementedAuthServiceHandler returns CodeUnimplemented from all methods.
type UnimplementedAuthServiceHandler struct{}

func (UnimplementedAuthServiceHandler) Authenticate(context.Context, *connect.Request[v1.AuthenticateRequest]) (*connect.Response[v1.AuthenticateResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("auth.v1.AuthService.Authenticate is not implemented"))
}

func (UnimplementedAuthServiceHandler) GenerateAPIKey(context.Context, *connect.Request[v1.GenerateAPIKeyRequest]) (*connect.Response[v1.GenerateAPIKeyResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("auth.v1.AuthService.GenerateAPIKey is not implemented"))
}

func (UnimplementedAuthServiceHandler) RevokeAPIKey(context.Context, *connect.Request[v1.RevokeAPIKeyRequest]) (*connect.Response[v1.RevokeAPIKeyResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("auth.v1.AuthService.RevokeAPIKey is not implemented"))
}