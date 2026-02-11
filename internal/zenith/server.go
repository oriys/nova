package zenith

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/oriys/nova/api/proto/novapb"
	"github.com/oriys/nova/internal/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const defaultBodyLimit = 10 << 20 // 10MB

type Config struct {
	NovaURL       string
	CometGRPCAddr string
	Timeout       time.Duration
}

type Server struct {
	cfg         Config
	novaProxy   *httputil.ReverseProxy
	cometConn   *grpc.ClientConn
	cometClient novapb.NovaServiceClient
	httpClient  *http.Client
}

type invokeHTTPResponse struct {
	RequestID  string          `json:"request_id"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	DurationMs int64           `json:"duration_ms"`
	ColdStart  bool            `json:"cold_start"`
}

func New(cfg Config) (*Server, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if strings.TrimSpace(cfg.NovaURL) == "" {
		return nil, fmt.Errorf("nova URL is required")
	}
	if strings.TrimSpace(cfg.CometGRPCAddr) == "" {
		return nil, fmt.Errorf("comet gRPC address is required")
	}

	novaTarget, err := parseTargetURL(cfg.NovaURL)
	if err != nil {
		return nil, fmt.Errorf("parse nova URL: %w", err)
	}

	conn, err := grpc.Dial(cfg.CometGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial comet gRPC: %w", err)
	}

	s := &Server{
		cfg:         cfg,
		novaProxy:   newReverseProxy(novaTarget),
		cometConn:   conn,
		cometClient: novapb.NewNovaServiceClient(conn),
		httpClient:  &http.Client{Timeout: cfg.Timeout},
	}

	return s, nil
}

func (s *Server) Close() error {
	if s.cometConn != nil {
		return s.cometConn.Close()
	}
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	if isCometOnlyHTTPPath(path) {
		s.handleProxyHTTPViaGRPC(w, r)
		return
	}

	s.novaProxy.ServeHTTP(w, r)
}

func (s *Server) handleInvokeViaGRPC(w http.ResponseWriter, r *http.Request, function string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, defaultBodyLimit))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	payload := body
	if len(payload) == 0 {
		payload = []byte("{}")
	} else if !json.Valid(payload) {
		writeJSONError(w, http.StatusBadRequest, "payload must be valid JSON")
		return
	}

	ctx, cancel, timeoutSeconds := cometContextFromRequest(r)
	defer cancel()

	resp, err := s.cometClient.Invoke(ctx, &novapb.InvokeRequest{
		Function: function,
		Payload:  payload,
		TimeoutS: timeoutSeconds,
	})
	if err != nil {
		statusCode, message := mapGRPCError(err)
		writeJSONError(w, statusCode, message)
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
			writeJSONError(w, http.StatusBadRequest, "failed to read request body")
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

	ctx, cancel, _ := cometContextFromRequest(r)
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
		writeJSONError(w, statusCode, message)
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

	components := map[string]any{
		"zenith": "healthy",
	}
	allHealthy := true
	postgresKnown := false
	postgresHealthy := true

	novaHealth, err := s.checkNova(ctx)
	if err != nil {
		components["nova"] = "unhealthy: " + err.Error()
		allHealthy = false
	} else {
		components["nova"] = "healthy"
		if ok, known := extractPostgresHealth(novaHealth); known {
			postgresKnown = true
			postgresHealthy = postgresHealthy && ok
		}
	}

	cometHealth, err := s.checkComet(ctx)
	if err != nil {
		components["comet"] = "unhealthy: " + err.Error()
		allHealthy = false
	} else {
		components["comet"] = "healthy"
		if status, ok := cometHealth.Components["postgres"]; ok {
			postgresKnown = true
			postgresHealthy = postgresHealthy && isHealthyStatus(status)
		}
	}

	if activeVMs, totalPools, err := s.fetchCometPoolStats(ctx); err == nil {
		components["pool"] = map[string]any{
			"active_vms":  activeVMs,
			"total_pools": totalPools,
		}
	}

	if !postgresKnown {
		postgresHealthy = allHealthy
	}
	components["postgres"] = postgresHealthy
	if !postgresHealthy {
		allHealthy = false
	}

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

func (s *Server) checkNova(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.cfg.NovaURL, "/")+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, defaultBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if len(body) == 0 {
		return map[string]any{}, nil
	}

	payload := map[string]any{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode response body: %w", err)
	}
	return payload, nil
}

func (s *Server) checkComet(ctx context.Context) (*novapb.HealthCheckResponse, error) {
	resp, err := s.cometClient.HealthCheck(ctx, &novapb.HealthCheckRequest{})
	if err != nil {
		return nil, err
	}
	if strings.ToLower(strings.TrimSpace(resp.Status)) != "ok" {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	return resp, nil
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

func extractPostgresHealth(payload map[string]any) (bool, bool) {
	rawComponents, ok := payload["components"]
	if !ok {
		return false, false
	}
	components, ok := rawComponents.(map[string]any)
	if !ok {
		return false, false
	}

	rawPostgres, ok := components["postgres"]
	if !ok {
		return false, false
	}

	switch v := rawPostgres.(type) {
	case bool:
		return v, true
	case string:
		return isHealthyStatus(v), true
	default:
		return false, false
	}
}

func isHealthyStatus(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "healthy")
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
		case "invoke-stream", "invoke-async", "async-invocations", "logs", "metrics", "diagnostics", "heatmap":
			return true
		}
	}
	return false
}

func cometContextFromRequest(r *http.Request) (context.Context, context.CancelFunc, int32) {
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

	return ctx, cancel, timeoutSeconds
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

func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

func parseTargetURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty target")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid target %q", raw)
	}
	return u, nil
}

func newReverseProxy(target *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.Host = target.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		code := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			code = http.StatusGatewayTimeout
		}
		logging.Op().Error("reverse proxy error", "target", target.String(), "path", r.URL.Path, "error", err)
		writeJSONError(w, code, "upstream request failed")
	}
	return proxy
}
