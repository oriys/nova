package grpc

import (
"context"
"time"

"github.com/oriys/nova/internal/logging"
"google.golang.org/grpc"
"google.golang.org/grpc/codes"
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
