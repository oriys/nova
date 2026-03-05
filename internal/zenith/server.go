package zenith

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oriys/nova/api/proto/novapb"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/pkg/httpjson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const defaultBodyLimit = 10 << 20 // 10MB

type Config struct {
	NovaGRPCAddr      string
	CometGRPCAddr     string
	CoronaGRPCAddr    string
	NebulaGRPCAddr    string
	AuroraGRPCAddr    string
	Timeout           time.Duration
	MaxTimeout        time.Duration // Global maximum timeout for gRPC calls (default: 300s)
	CometServiceToken string        // Shared secret for gRPC authentication
	GRPCTLSCertFile   string        // TLS certificate file for gRPC client connections
	GRPCTLSKeyFile    string        // TLS key file for gRPC client connections (mTLS)
	GRPCTLSCAFile     string        // CA certificate file for verifying gRPC server certs
}

type Server struct {
	cfg         Config
	novaConn    *grpc.ClientConn
	novaClient  novapb.NovaServiceClient
	cometConn   *grpc.ClientConn
	cometClient novapb.NovaServiceClient
	coronaConn  *grpc.ClientConn
	nebulaConn  *grpc.ClientConn
	auroraConn  *grpc.ClientConn
}

type invokeHTTPResponse struct {
	RequestID  string          `json:"request_id"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	ColdStart  bool            `json:"cold_start"`
}

const defaultMaxTimeout = 300 * time.Second // 5 minutes global max

func New(cfg Config) (*Server, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxTimeout <= 0 {
		cfg.MaxTimeout = defaultMaxTimeout
	}
	if strings.TrimSpace(cfg.NovaGRPCAddr) == "" {
		return nil, fmt.Errorf("nova gRPC address is required")
	}
	if strings.TrimSpace(cfg.CometGRPCAddr) == "" {
		return nil, fmt.Errorf("comet gRPC address is required")
	}

	dialOpts, err := buildGRPCDialOpts(cfg)
	if err != nil {
		return nil, fmt.Errorf("build gRPC dial options: %w", err)
	}

	novaConn, err := dialWithLB(cfg.NovaGRPCAddr, dialOpts)
	if err != nil {
		return nil, fmt.Errorf("dial nova gRPC: %w", err)
	}

	cometConn, err := dialWithLB(cfg.CometGRPCAddr, dialOpts)
	if err != nil {
		novaConn.Close()
		return nil, fmt.Errorf("dial comet gRPC: %w", err)
	}

	s := &Server{
		cfg:         cfg,
		novaConn:    novaConn,
		novaClient:  novapb.NewNovaServiceClient(novaConn),
		cometConn:   cometConn,
		cometClient: novapb.NewNovaServiceClient(cometConn),
	}

	// Connect to optional services for health checking
	if addr := strings.TrimSpace(cfg.CoronaGRPCAddr); addr != "" {
		conn, err := dialWithLB(addr, dialOpts)
		if err != nil {
			logging.Op().Warn("failed to dial corona gRPC", "addr", addr, "error", err)
		} else {
			s.coronaConn = conn
		}
	}
	if addr := strings.TrimSpace(cfg.NebulaGRPCAddr); addr != "" {
		conn, err := dialWithLB(addr, dialOpts)
		if err != nil {
			logging.Op().Warn("failed to dial nebula gRPC", "addr", addr, "error", err)
		} else {
			s.nebulaConn = conn
		}
	}
	if addr := strings.TrimSpace(cfg.AuroraGRPCAddr); addr != "" {
		conn, err := dialWithLB(addr, dialOpts)
		if err != nil {
			logging.Op().Warn("failed to dial aurora gRPC", "addr", addr, "error", err)
		} else {
			s.auroraConn = conn
		}
	}

	return s, nil
}

// buildGRPCDialOpts builds gRPC dial options with TLS if configured, otherwise insecure.
func buildGRPCDialOpts(cfg Config) ([]grpc.DialOption, error) {
	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	if cfg.GRPCTLSCertFile != "" && cfg.GRPCTLSKeyFile != "" {
		// mTLS: client certificate + optional CA
		cert, err := tls.LoadX509KeyPair(cfg.GRPCTLSCertFile, cfg.GRPCTLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client TLS key pair: %w", err)
		}
		tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}}
		if cfg.GRPCTLSCAFile != "" {
			caCert, err := os.ReadFile(cfg.GRPCTLSCAFile)
			if err != nil {
				return nil, fmt.Errorf("read CA file: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("failed to parse CA certificate")
			}
			tlsCfg.RootCAs = pool
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
		logging.Op().Info("gRPC client TLS enabled (mTLS)")
	} else if cfg.GRPCTLSCAFile != "" {
		// Server-only TLS verification
		caCert, err := os.ReadFile(cfg.GRPCTLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsCfg := &tls.Config{RootCAs: pool}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
		logging.Op().Info("gRPC client TLS enabled (server verification)")
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		logging.Op().Warn("gRPC client running without TLS")
	}

	return opts, nil
}

// dialWithLB creates a gRPC client connection supporting multiple backends.
// If addr contains commas (e.g. "host1:9090,host2:9090"), it uses a static
// resolver with round-robin load balancing.  Otherwise it dials a single
// address.  A unary retry interceptor is added for transient failures.
func dialWithLB(addr string, baseOpts []grpc.DialOption) (*grpc.ClientConn, error) {
	opts := make([]grpc.DialOption, len(baseOpts))
	copy(opts, baseOpts)

	// Add retry interceptor for Unavailable errors (backend restart, connection drop).
	opts = append(opts, grpc.WithUnaryInterceptor(retryUnary(3, 200*time.Millisecond)))

	addrs := strings.Split(addr, ",")
	if len(addrs) > 1 {
		// Use round-robin with a static resolver.
		// Build a service config enabling round_robin and increase
		// the default connection pool to cover all endpoints.
		opts = append(opts,
			grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}]}`),
		)
		// dns:/// doesn't support comma-separated.  Use manual resolver.
		resolver := newStaticResolver(addrs)
		opts = append(opts, grpc.WithResolvers(resolver))
		return grpc.Dial("static:///lb", opts...)
	}

	return grpc.Dial(addr, opts...)
}

// retryUnary returns a gRPC unary client interceptor that retries on
// Unavailable errors with exponential backoff.
func retryUnary(maxRetries int, baseDelay time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		var err error
		for attempt := 0; attempt <= maxRetries; attempt++ {
			err = invoker(ctx, method, req, reply, cc, callOpts...)
			if err == nil {
				return nil
			}
			st, ok := status.FromError(err)
			if !ok || st.Code() != codes.Unavailable {
				return err
			}
			if attempt == maxRetries {
				break
			}
			delay := baseDelay * time.Duration(1<<uint(attempt))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		return err
	}
}

func (s *Server) Close() error {
	var firstErr error
	for _, conn := range []*grpc.ClientConn{s.novaConn, s.cometConn, s.coronaConn, s.nebulaConn, s.auroraConn} {
		if conn != nil {
			if err := conn.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

type serviceTokenKey struct{}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Inject service token into request context for gRPC calls
	if s.cfg.CometServiceToken != "" {
		ctx := context.WithValue(r.Context(), serviceTokenKey{}, s.cfg.CometServiceToken)
		r = r.WithContext(ctx)
	}

	path := r.URL.Path

	switch path {
	case "/health/live", "/health/startup":
		s.writeHealthProbe(w, map[string]any{
			"status": "ok",
		})
		return
	case "/health", "/health/ready":
		s.handleCompositeHealth(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if function, ok := matchFunctionPath(path, "invoke"); ok {
			s.handleInvokeViaGRPC(w, r, function)
			return
		}
	}

	// Function URLs: /fn/{name} — invoke function with HTTP context
	if function, ok := matchFunctionURL(path); ok {
		s.handleFunctionURL(w, r, function)
		return
	}

	if isCometOnlyHTTPPath(path) {
		s.handleProxyHTTPViaGRPC(w, r)
		return
	}

	// All other control-plane paths: proxy to Nova via gRPC
	s.handleProxyHTTPViaNova(w, r)
}

func (s *Server) handleInvokeViaGRPC(w http.ResponseWriter, r *http.Request, function string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, defaultBodyLimit))
	if err != nil {
		httpjson.Error(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	payload := body
	if len(payload) == 0 {
		payload = []byte("{}")
	} else if !json.Valid(payload) {
		httpjson.Error(w, http.StatusBadRequest, "payload must be valid JSON")
		return
	}

	ctx, cancel, timeoutSeconds := s.cometContextFromRequest(r)
	defer cancel()

	resp, err := s.cometClient.Invoke(ctx, &novapb.InvokeRequest{
		Function: function,
		Payload:  payload,
		TimeoutS: timeoutSeconds,
	})
	if err != nil {
		statusCode, message := mapGRPCError(err)
		httpjson.Error(w, statusCode, message)
		return
	}

	output := resp.Output
	if len(output) == 0 {
		output = []byte("null")
	}
	if !json.Valid(output) {
		output = []byte(strconv.Quote(base64.StdEncoding.EncodeToString(output)))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(&invokeHTTPResponse{
		RequestID:  resp.RequestId,
		Output:     output,
		Error:      resp.Error,
		DurationMs: resp.DurationMs,
		ColdStart:  resp.ColdStart,
	})
}

func (s *Server) handleProxyHTTPViaGRPC(w http.ResponseWriter, r *http.Request) {
	body := []byte{}
	if r.Body != nil {
		b, err := io.ReadAll(io.LimitReader(r.Body, defaultBodyLimit))
		if err != nil {
			httpjson.Error(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		body = b
	}

	headers := make(map[string]string)
	for key, values := range r.Header {
		if strings.EqualFold(key, "Host") || strings.EqualFold(key, "Content-Length") {
			continue
		}
		if len(values) == 0 {
			continue
		}
		headers[key] = strings.Join(values, ", ")
	}
	source := requestSourceFromRequest(r)
	if source.Function != "" {
		headers["X-Nova-Source-Function"] = source.Function
	}
	if source.IP != "" {
		headers["X-Nova-Source-IP"] = source.IP
	}
	if source.Protocol != "" {
		headers["X-Nova-Source-Protocol"] = source.Protocol
	}
	if source.Port > 0 {
		headers["X-Nova-Source-Port"] = strconv.Itoa(source.Port)
	}

	ctx, cancel, _ := s.cometContextFromRequest(r)
	defer cancel()

	resp, err := s.cometClient.ProxyHTTP(ctx, &novapb.ProxyHTTPRequest{
		Method:  r.Method,
		Path:    r.URL.Path,
		Query:   r.URL.RawQuery,
		Body:    body,
		Headers: headers,
	})
	if err != nil {
		statusCode, message := mapGRPCError(err)
		httpjson.Error(w, statusCode, message)
		return
	}

	for key, value := range resp.Headers {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") {
			continue
		}
		// Sanitize header values: reject CRLF sequences to prevent header injection
		if strings.ContainsAny(value, "\r\n") {
			continue
		}
		w.Header().Set(k, value)
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}

	statusCode := int(resp.StatusCode)
	if statusCode < 100 || statusCode > 999 {
		statusCode = http.StatusInternalServerError
	}
	w.WriteHeader(statusCode)
	_, _ = w.Write(resp.Body)
}

func (s *Server) handleCompositeHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Timeout)
	defer cancel()

	var mu sync.Mutex
	components := map[string]any{
		"zenith": "healthy",
	}
	allHealthy := true

	var wg sync.WaitGroup

	// Check all gRPC services concurrently via standard grpc_health_v1
	for _, dep := range []struct {
		name string
		conn *grpc.ClientConn
	}{
		{name: "nova", conn: s.novaConn},
		{name: "comet", conn: s.cometConn},
		{name: "corona", conn: s.coronaConn},
		{name: "nebula", conn: s.nebulaConn},
		{name: "aurora", conn: s.auroraConn},
	} {
		if dep.conn == nil {
			continue
		}
		wg.Add(1)
		go func(name string, conn *grpc.ClientConn) {
			defer wg.Done()
			st, err := s.checkGRPCHealth(ctx, conn)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				components[name] = "unhealthy: " + err.Error()
				allHealthy = false
			} else {
				components[name] = st
				if st != "healthy" {
					allHealthy = false
				}
			}
		}(dep.name, dep.conn)
	}

	// Fetch pool stats concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		activeVMs, totalPools, err := s.fetchCometPoolStats(ctx)
		if err == nil {
			mu.Lock()
			components["pool"] = map[string]any{
				"active_vms":  activeVMs,
				"total_pools": totalPools,
			}
			mu.Unlock()
		}
	}()

	wg.Wait()

	statusText := "ok"
	httpStatus := http.StatusOK
	if !allHealthy {
		statusText = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	s.writeHealthProbeWithCode(w, httpStatus, map[string]any{
		"status":     statusText,
		"components": components,
	})
}

// handleProxyHTTPViaNova forwards control-plane HTTP requests to Nova via gRPC ProxyHTTP.
func (s *Server) handleProxyHTTPViaNova(w http.ResponseWriter, r *http.Request) {
	body := []byte{}
	if r.Body != nil {
		b, err := io.ReadAll(io.LimitReader(r.Body, defaultBodyLimit))
		if err != nil {
			httpjson.Error(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		body = b
	}

	headers := make(map[string]string)
	for key, values := range r.Header {
		if strings.EqualFold(key, "Host") || strings.EqualFold(key, "Content-Length") {
			continue
		}
		if len(values) == 0 {
			continue
		}
		headers[key] = strings.Join(values, ", ")
	}
	source := requestSourceFromRequest(r)
	if source.Function != "" {
		headers["X-Nova-Source-Function"] = source.Function
	}
	if source.IP != "" {
		headers["X-Nova-Source-IP"] = source.IP
	}
	if source.Protocol != "" {
		headers["X-Nova-Source-Protocol"] = source.Protocol
	}
	if source.Port > 0 {
		headers["X-Nova-Source-Port"] = strconv.Itoa(source.Port)
	}

	ctx, cancel, _ := novaContextFromRequest(r, s.cfg.CometServiceToken)
	defer cancel()

	resp, err := s.novaClient.ProxyHTTP(ctx, &novapb.ProxyHTTPRequest{
		Method:  r.Method,
		Path:    r.URL.Path,
		Query:   r.URL.RawQuery,
		Body:    body,
		Headers: headers,
	})
	if err != nil {
		statusCode, message := mapGRPCError(err)
		httpjson.Error(w, statusCode, message)
		return
	}

	for key, value := range resp.Headers {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") {
			continue
		}
		if strings.ContainsAny(value, "\r\n") {
			continue
		}
		w.Header().Set(k, value)
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}

	statusCode := int(resp.StatusCode)
	if statusCode < 100 || statusCode > 999 {
		statusCode = http.StatusInternalServerError
	}
	w.WriteHeader(statusCode)
	_, _ = w.Write(resp.Body)
}

func (s *Server) fetchCometPoolStats(ctx context.Context) (int64, *int64, error) {
	resp, err := s.cometClient.ProxyHTTP(ctx, &novapb.ProxyHTTPRequest{
		Method: http.MethodGet,
		Path:   "/stats",
	})
	if err != nil {
		return 0, nil, err
	}
	if resp.StatusCode >= 400 {
		return 0, nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	payload := map[string]any{}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return 0, nil, fmt.Errorf("decode response body: %w", err)
	}

	activeVMs, _ := toInt64(payload["active_vms"])

	var totalPoolsPtr *int64
	if raw, ok := payload["total_pools"]; ok {
		if totalPools, ok := toInt64(raw); ok {
			totalPoolsPtr = &totalPools
		}
	}
	return activeVMs, totalPoolsPtr, nil
}

func (s *Server) checkGRPCHealth(ctx context.Context, conn *grpc.ClientConn) (string, error) {
	client := grpc_health_v1.NewHealthClient(conn)
	resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return "", err
	}
	switch resp.Status {
	case grpc_health_v1.HealthCheckResponse_SERVING:
		return "healthy", nil
	case grpc_health_v1.HealthCheckResponse_NOT_SERVING:
		return "unhealthy", nil
	default:
		return "unknown", nil
	}
}

func toInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case int:
		return int64(v), true
	case int64:
		return v, true
	case int32:
		return int64(v), true
	case uint:
		return int64(v), true
	case uint64:
		return int64(v), true
	case uint32:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func (s *Server) writeHealthProbe(w http.ResponseWriter, payload map[string]any) {
	s.writeHealthProbeWithCode(w, http.StatusOK, payload)
}

func (s *Server) writeHealthProbeWithCode(w http.ResponseWriter, statusCode int, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func isCometOnlyHTTPPath(path string) bool {
	if strings.HasPrefix(path, "/async-invocations") {
		return true
	}
	if strings.HasPrefix(path, "/durable-executions") {
		return true
	}
	if strings.HasPrefix(path, "/metrics") {
		return true
	}
	if strings.HasPrefix(path, "/functions/") && strings.HasSuffix(path, "/slo/status") {
		return true
	}
	switch path {
	case "/stats", "/invocations":
		return true
	}

	if function, action, ok := splitFunctionPath(path); ok {
		_ = function
		switch action {
		case "invoke-stream", "invoke-async", "invoke-durable", "async-invocations", "durable-executions", "logs", "metrics", "diagnostics", "heatmap", "state", "prewarm":
			return true
		}
	}
	return false
}

func (s *Server) cometContextFromRequest(r *http.Request) (context.Context, context.CancelFunc, int32) {
	timeoutSeconds := int32(0)
	if rawTimeout := strings.TrimSpace(r.Header.Get("X-Nova-Timeout-S")); rawTimeout != "" {
		if n, err := strconv.Atoi(rawTimeout); err == nil && n > 0 {
			timeoutSeconds = int32(n)
		}
	}

	// Enforce global max timeout cap
	maxTimeoutS := int32(s.cfg.MaxTimeout / time.Second)
	if maxTimeoutS <= 0 {
		maxTimeoutS = int32(defaultMaxTimeout / time.Second)
	}
	if timeoutSeconds <= 0 || timeoutSeconds > maxTimeoutS {
		timeoutSeconds = maxTimeoutS
	}

	ctx := r.Context()
	var c context.CancelFunc
	ctx, c = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)

	md := metadata.MD{}
	// Attach service token for inter-service auth
	if sv, ok := ctx.Value(serviceTokenKey{}).(string); ok && sv != "" {
		md.Set("authorization", "Bearer "+sv)
	}
	for _, key := range []string{"X-Nova-Tenant", "X-Nova-Namespace", "X-Request-ID"} {
		if val := strings.TrimSpace(r.Header.Get(key)); val != "" {
			md.Set(strings.ToLower(key), val)
		}
	}
	source := requestSourceFromRequest(r)
	if source.Function != "" {
		md.Set("x-nova-source-function", source.Function)
	}
	if source.IP != "" {
		md.Set("x-nova-source-ip", source.IP)
	}
	if source.Protocol != "" {
		md.Set("x-nova-source-protocol", source.Protocol)
	}
	if source.Port > 0 {
		md.Set("x-nova-source-port", strconv.Itoa(source.Port))
	}
	if len(md) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	return ctx, c, timeoutSeconds
}

type requestSource struct {
	Function string
	IP       string
	Protocol string
	Port     int
}

func requestSourceFromRequest(r *http.Request) requestSource {
	return requestSource{
		Function: strings.TrimSpace(r.Header.Get("X-Nova-Source-Function")),
		IP:       requestSourceIP(r),
		Protocol: normalizeTransport(strings.TrimSpace(r.Header.Get("X-Nova-Source-Protocol"))),
		Port:     requestDestinationPort(r),
	}
}

func requestSourceIP(r *http.Request) string {
	if ip := firstIP(strings.TrimSpace(r.Header.Get("X-Nova-Source-IP"))); ip != "" {
		return ip
	}
	if ip := firstIP(strings.TrimSpace(r.Header.Get("X-Forwarded-For"))); ip != "" {
		return ip
	}
	remoteAddr := strings.TrimSpace(r.RemoteAddr)
	if remoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(remoteAddr); ip != nil {
		return ip.String()
	}
	return ""
}

func requestDestinationPort(r *http.Request) int {
	if n := parsePort(strings.TrimSpace(r.Header.Get("X-Nova-Source-Port"))); n > 0 {
		return n
	}
	if n := parsePort(strings.TrimSpace(r.Header.Get("X-Forwarded-Port"))); n > 0 {
		return n
	}
	host := strings.TrimSpace(r.Host)
	if host != "" {
		if _, rawPort, err := net.SplitHostPort(host); err == nil {
			if n := parsePort(rawPort); n > 0 {
				return n
			}
		}
	}
	if r.TLS != nil {
		return 443
	}
	return 80
}

func firstIP(raw string) string {
	for _, part := range strings.Split(raw, ",") {
		candidate := strings.TrimSpace(part)
		if candidate == "" {
			continue
		}
		host := candidate
		if strings.Contains(candidate, ":") {
			if parsedHost, _, err := net.SplitHostPort(candidate); err == nil {
				host = parsedHost
			}
		}
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	return ""
}

func parsePort(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 || n > 65535 {
		return 0
	}
	return n
}

func normalizeTransport(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "http", "https", "grpc", "h2c", "tcp":
		return "tcp"
	case "udp", "quic":
		return "udp"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func splitFunctionPath(path string) (function string, action string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "functions" {
		return "", "", false
	}
	if parts[1] == "" {
		return "", "", false
	}
	name, err := url.PathUnescape(parts[1])
	if err != nil || name == "" {
		return "", "", false
	}
	return name, parts[2], true
}

func matchFunctionPath(path, action string) (string, bool) {
	function, actualAction, ok := splitFunctionPath(path)
	if !ok || actualAction != action {
		return "", false
	}
	return function, true
}

// matchFunctionURL matches /fn/{name} paths for function URLs.
func matchFunctionURL(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] != "fn" || parts[1] == "" {
		return "", false
	}
	name, err := url.PathUnescape(parts[1])
	if err != nil || name == "" {
		return "", false
	}
	return name, true
}

// handleFunctionURL invokes a function via its dedicated URL, passing full HTTP context.
func (s *Server) handleFunctionURL(w http.ResponseWriter, r *http.Request, function string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, defaultBodyLimit))
	if err != nil {
		httpjson.Error(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	// Build HTTP context event
	headers := make(map[string]string, len(r.Header))
	for k := range r.Header {
		headers[k] = r.Header.Get(k)
	}
	queryParams := make(map[string]string, len(r.URL.Query()))
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			queryParams[k] = v[0]
		}
	}

	httpEvent := map[string]any{
		"method":       r.Method,
		"path":         r.URL.Path,
		"headers":      headers,
		"query_params": queryParams,
	}
	if len(body) > 0 {
		if json.Valid(body) {
			httpEvent["body"] = json.RawMessage(body)
		} else {
			httpEvent["body"] = base64.StdEncoding.EncodeToString(body)
			httpEvent["body_encoding"] = "base64"
		}
	}

	payload, _ := json.Marshal(httpEvent)

	ctx, cancel, timeoutSeconds := s.cometContextFromRequest(r)
	defer cancel()

	resp, err := s.cometClient.Invoke(ctx, &novapb.InvokeRequest{
		Function: function,
		Payload:  payload,
		TimeoutS: timeoutSeconds,
	})
	if err != nil {
		statusCode, message := mapGRPCError(err)
		httpjson.Error(w, statusCode, message)
		return
	}

	// If the function returns a structured HTTP response, respect it
	var httpResp struct {
		StatusCode int               `json:"statusCode"`
		Headers    map[string]string `json:"headers"`
		Body       json.RawMessage   `json:"body"`
	}
	if err := json.Unmarshal(resp.Output, &httpResp); err == nil && httpResp.StatusCode > 0 {
		for k, v := range httpResp.Headers {
			w.Header().Set(k, v)
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(httpResp.StatusCode)
		if len(httpResp.Body) > 0 {
			w.Write(httpResp.Body)
		}
		return
	}

	// Default: return raw output as JSON
	output := resp.Output
	if len(output) == 0 {
		output = []byte("null")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(output)
}

func mapGRPCError(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}

	st, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError, err.Error()
	}

	switch st.Code() {
	case codes.InvalidArgument:
		return http.StatusBadRequest, st.Message()
	case codes.NotFound:
		return http.StatusNotFound, st.Message()
	case codes.Unauthenticated:
		return http.StatusUnauthorized, st.Message()
	case codes.PermissionDenied:
		return http.StatusForbidden, st.Message()
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests, st.Message()
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout, st.Message()
	case codes.Unavailable:
		return http.StatusServiceUnavailable, st.Message()
	case codes.Unimplemented:
		return http.StatusNotImplemented, st.Message()
	default:
		return http.StatusInternalServerError, st.Message()
	}
}

// novaContextFromRequest creates a gRPC context for Nova ProxyHTTP calls, similar to cometContextFromRequest.
func novaContextFromRequest(r *http.Request, serviceToken string) (context.Context, context.CancelFunc, int32) {
	timeoutSeconds := int32(0)
	if rawTimeout := strings.TrimSpace(r.Header.Get("X-Nova-Timeout-S")); rawTimeout != "" {
		if n, err := strconv.Atoi(rawTimeout); err == nil && n > 0 {
			timeoutSeconds = int32(n)
		}
	}

	ctx := r.Context()
	cancel := func() {}
	if timeoutSeconds > 0 {
		var c context.CancelFunc
		ctx, c = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		cancel = c
	}

	md := metadata.MD{}
	if serviceToken != "" {
		md.Set("authorization", "Bearer "+serviceToken)
	}
	for _, key := range []string{"X-Nova-Tenant", "X-Nova-Namespace", "X-Request-ID"} {
		if val := strings.TrimSpace(r.Header.Get(key)); val != "" {
			md.Set(strings.ToLower(key), val)
		}
	}
	if len(md) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	return ctx, cancel, timeoutSeconds
}
