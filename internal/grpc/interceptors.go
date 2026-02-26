package grpc

import (
"context"
"crypto/subtle"
"strings"
"time"

"github.com/oriys/nova/internal/logging"
"github.com/oriys/nova/internal/store"
"google.golang.org/grpc"
"google.golang.org/grpc/codes"
"google.golang.org/grpc/metadata"
"google.golang.org/grpc/status"
)

// loggingInterceptor logs all gRPC requests
func loggingInterceptor(
ctx context.Context,
req interface{},
info *grpc.UnaryServerInfo,
handler grpc.UnaryHandler,
) (interface{}, error) {
start := time.Now()

logging.Op().Info("gRPC request started", 
"method", info.FullMethod,
)

resp, err := handler(ctx, req)

duration := time.Since(start)

if err != nil {
logging.Op().Error("gRPC request failed",
"method", info.FullMethod,
"duration", duration,
"error", err,
)
} else {
logging.Op().Info("gRPC request completed",
"method", info.FullMethod,
"duration", duration,
)
}

return resp, err
}

// errorHandlingInterceptor converts errors to gRPC status codes
func errorHandlingInterceptor(
ctx context.Context,
req interface{},
info *grpc.UnaryServerInfo,
handler grpc.UnaryHandler,
) (interface{}, error) {
resp, err := handler(ctx, req)

if err != nil {
// Convert error to appropriate gRPC status
// This is a simple implementation; expand as needed
return nil, status.Error(codes.Internal, err.Error())
}

return resp, nil
}

// serviceAuthInterceptor verifies that the caller provides a valid service
// token via the "authorization" gRPC metadata key. Health check endpoints
// are exempt. When serviceToken is empty, the interceptor is a no-op to
// preserve backward compatibility in development/testing environments.
func serviceAuthInterceptor(serviceToken string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if serviceToken == "" {
			return handler(ctx, req)
		}

		// Allow health checks without auth
		if strings.HasSuffix(info.FullMethod, "/HealthCheck") ||
			strings.HasPrefix(info.FullMethod, "/grpc.health.") {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			logging.Op().Warn("gRPC auth rejected: missing metadata", "method", info.FullMethod)
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		tokens := md.Get("authorization")
		if len(tokens) == 0 {
			logging.Op().Warn("gRPC auth rejected: missing authorization", "method", info.FullMethod)
			return nil, status.Error(codes.Unauthenticated, "missing authorization metadata")
		}

		token := strings.TrimPrefix(tokens[0], "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(serviceToken)) != 1 {
			logging.Op().Warn("gRPC auth rejected: invalid service token", "method", info.FullMethod)
			return nil, status.Error(codes.Unauthenticated, "invalid service token")
		}

		// Validate tenant scope from metadata
		tenantID := metadataValueFromMD(md, "x-nova-tenant")
		if tenantID != "" && !store.IsValidTenantScopePart(tenantID) {
			return nil, status.Error(codes.InvalidArgument, "invalid tenant ID in metadata")
		}
		namespace := metadataValueFromMD(md, "x-nova-namespace")
		if namespace != "" && !store.IsValidTenantScopePart(namespace) {
			return nil, status.Error(codes.InvalidArgument, "invalid namespace in metadata")
		}

		return handler(ctx, req)
	}
}

func metadataValueFromMD(md metadata.MD, key string) string {
	values := md.Get(strings.ToLower(strings.TrimSpace(key)))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
