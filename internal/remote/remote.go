// Package remote provides an explicitly configured, mutually authenticated HTTP transport.
package remote

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/haramj/openstack-mcp-server/internal/openstack"
	"github.com/haramj/openstack-mcp-server/internal/scope"
	"github.com/haramj/openstack-mcp-server/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"
)

type Principal struct {
	Name               string   `json:"name"`
	CertificateSHA256  string   `json:"certificate_sha256"`
	Role               string   `json:"role"`
	StateDir           string   `json:"state_dir"`
	CredentialsFile    string   `json:"credentials_file"`
	RequireElicitation bool     `json:"require_elicitation"`
	ProtectedPatterns  []string `json:"protected_patterns,omitempty"`
	AllowSampling      bool     `json:"allow_sampling"`
}
type Config struct {
	Address        string      `json:"address"`
	Certificate    string      `json:"certificate"`
	PrivateKey     string      `json:"private_key"`
	ClientCA       string      `json:"client_ca"`
	AllowedHosts   []string    `json:"allowed_hosts"`
	AllowedOrigins []string    `json:"allowed_origins,omitempty"`
	Principals     []Principal `json:"principals"`
}

func readPrivateJSON(path string, out any) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("configuration files must be private regular files (0600)")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("configuration exceeds 1 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(out); err != nil {
		return fmt.Errorf("invalid configuration JSON")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("configuration contains trailing data")
	}
	return nil
}
func Load(path string) (Config, error) { var c Config; err := readPrivateJSON(path, &c); return c, err }
func principalScope(p Principal) (scope.Config, error) {
	if p.Name == "" || len(p.Name) > 128 || strings.ContainsAny(p.Name, "\r\n") {
		return scope.Config{}, fmt.Errorf("invalid principal name")
	}
	if p.Role != "viewer" && p.Role != "operator" && p.Role != "admin" {
		return scope.Config{}, fmt.Errorf("unknown role")
	}
	if !filepath.IsAbs(p.StateDir) || !filepath.IsAbs(p.CredentialsFile) {
		return scope.Config{}, fmt.Errorf("principal paths must be absolute")
	}
	if err := os.MkdirAll(p.StateDir, 0700); err != nil {
		return scope.Config{}, err
	}
	info, err := os.Stat(p.StateDir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return scope.Config{}, fmt.Errorf("state directory must be private (0700)")
	}
	for _, pattern := range p.ProtectedPatterns {
		if _, err := path.Match(pattern, ""); err != nil {
			return scope.Config{}, fmt.Errorf("invalid protected pattern")
		}
	}
	var credentials map[string]string
	if err := readPrivateJSON(p.CredentialsFile, &credentials); err != nil {
		return scope.Config{}, err
	}
	allowed := map[string]bool{"OS_AUTH_URL": true, "OS_APPLICATION_CREDENTIAL_ID": true, "OS_APPLICATION_CREDENTIAL_SECRET": true, "OS_REGION_NAME": true, "OS_INTERFACE": true, "OS_CACERT": true}
	// No ambient OS_* variables, cloud config, tokens, user profiles or plugin paths.
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + p.StateDir, "XDG_CONFIG_HOME=" + p.StateDir, "LANG=C.UTF-8", "OS_AUTH_TYPE=v3applicationcredential", "OS_IDENTITY_API_VERSION=3"}
	for key, value := range credentials {
		if !allowed[key] || strings.ContainsRune(value, 0) {
			return scope.Config{}, fmt.Errorf("unsupported credential setting")
		}
		env = append(env, key+"="+value)
	}
	authURL, urlErr := url.Parse(credentials["OS_AUTH_URL"])
	if urlErr != nil || authURL.Scheme != "https" || authURL.Hostname() == "" || authURL.User != nil || authURL.RawQuery != "" || authURL.Fragment != "" || credentials["OS_APPLICATION_CREDENTIAL_ID"] == "" || credentials["OS_APPLICATION_CREDENTIAL_SECRET"] == "" {
		return scope.Config{}, fmt.Errorf("HTTPS auth URL and application credential ID/secret are required")
	}
	return scope.Config{Principal: p.Name, Role: p.Role, MemoryFile: filepath.Join(p.StateDir, "memory.json"), AuditFile: filepath.Join(p.StateDir, "audit.jsonl"), Environment: env, RequireElicitation: p.RequireElicitation, AllowSampling: p.AllowSampling, ProtectedPatterns: p.ProtectedPatterns}, nil
}
func allowedTool(role, name string) bool {
	switch name {
	case "list_instances", "get_instance", "list_networks", "list_images", "list_flavors", "get_agent_memory", "plan_instance_operation", "summarize_agent_activity", "analyze_agent_activity", "render_instance_topology":
		return true
	case "create_instance":
		return role == "operator" || role == "admin"
	case "delete_instance", "admin_instance_action", "record_agent_memory":
		return role == "admin"
	default:
		return false
	}
}
func ScopedMiddleware(config scope.Config) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			ctx = scope.With(ctx, config)
			operation := method
			if params, ok := request.GetParams().(*mcp.CallToolParamsRaw); ok && method == "tools/call" {
				operation += "/" + params.Name
				if !allowedTool(config.Role, params.Name) {
					openstack.RecordMCPAudit(ctx, operation, "rejected")
					return nil, fmt.Errorf("tool not permitted for this principal")
				}
			}
			result, err := next(ctx, method, request)
			if method == "tools/call" || method == "resources/read" || method == "prompts/get" {
				status := "success"
				if r, ok := result.(*mcp.CallToolResult); ok && len(r.InputRequests) > 0 {
					status = "awaiting_input"
				}
				if err != nil {
					status = "failed"
				}
				if r, ok := result.(*mcp.CallToolResult); ok && r.IsError {
					status = "failed"
				}
				openstack.RecordMCPAudit(ctx, operation, status)
			}
			return result, err
		}
	}
}
func NewServer(config scope.Config) *mcp.Server {
	return newServer(context.Background(), config, false)
}
func newServer(ctx context.Context, config scope.Config, watch bool) *mcp.Server {
	feed := tools.NewEventFeed()
	server := mcp.NewServer(&mcp.Implementation{Name: "openstack-mcp", Version: "0.2.0"}, &mcp.ServerOptions{SubscribeHandler: feed.Subscribe, UnsubscribeHandler: feed.Unsubscribe})
	feed.Register(server)
	if watch {
		go feed.Run(scope.With(ctx, config), server)
	}
	tools.RegisterAgentTools(server)
	tools.RegisterInstanceTools(server)
	tools.RegisterNetworkTools(server)
	tools.RegisterImageTools(server)
	tools.RegisterFlavorTools(server)
	tools.RegisterResources(server)
	tools.RegisterPrompts(server)
	tools.RegisterAdvancedTools(server)
	server.AddReceivingMiddleware(ScopedMiddleware(config))
	return server
}

type endpoint struct {
	handler http.Handler
	limiter *rate.Limiter
	slots   chan struct{}
	server  *mcp.Server
	cancel  context.CancelFunc
	mu      sync.Mutex
	pending int
}
type Handler struct {
	endpoints      map[string]*endpoint
	hosts, origins map[string]bool
}

func NewHandler(config Config) (*Handler, error) {
	if len(config.Principals) == 0 || len(config.Principals) > 100 || len(config.AllowedHosts) == 0 {
		return nil, fmt.Errorf("1-100 principals and explicit allowed_hosts are required")
	}
	h := &Handler{endpoints: map[string]*endpoint{}, hosts: map[string]bool{}, origins: map[string]bool{}}
	complete := false
	defer func() {
		if !complete {
			h.Close()
		}
	}()
	for _, v := range config.AllowedHosts {
		if v == "" || v == "*" {
			return nil, fmt.Errorf("explicit hosts required")
		}
		h.hosts[v] = true
	}
	for _, v := range config.AllowedOrigins {
		if !strings.HasPrefix(v, "https://") || strings.Contains(v, "*") {
			return nil, fmt.Errorf("explicit HTTPS origins required")
		}
		h.origins[v] = true
	}
	names := map[string]bool{}
	paths := map[string]bool{}
	for _, p := range config.Principals {
		fingerprint := strings.ToLower(p.CertificateSHA256)
		decoded, err := hex.DecodeString(fingerprint)
		if err != nil || len(decoded) != 32 || h.endpoints[fingerprint] != nil {
			return nil, fmt.Errorf("unique SHA256 certificate fingerprints required")
		}
		resolved, err := filepath.Abs(p.StateDir)
		if err != nil {
			return nil, err
		}
		if names[p.Name] || paths[resolved] {
			return nil, fmt.Errorf("principal names and state directories must be unique")
		}
		sc, err := principalScope(p)
		if err != nil {
			return nil, err
		}
		canonical, err := filepath.EvalSymlinks(p.StateDir)
		if err != nil {
			return nil, err
		}
		if paths[canonical] {
			return nil, fmt.Errorf("state directories must not alias")
		}
		names[p.Name] = true
		paths[canonical] = true
		watchCtx, cancel := context.WithCancel(context.Background())
		server := newServer(watchCtx, sc, true)
		store := mcp.NewMemoryEventStore(nil)
		store.SetMaxBytes(1 << 20)
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{EventStore: store, SessionTimeout: 5 * time.Minute})
		h.endpoints[fingerprint] = &endpoint{handler: handler, server: server, cancel: cancel, limiter: rate.NewLimiter(10, 20), slots: make(chan struct{}, 16)}
	}
	complete = true
	return h, nil
}
func (h *Handler) Close() {
	for _, e := range h.endpoints {
		e.cancel()
		for session := range e.server.Sessions() {
			_ = session.Close()
		}
	}
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	if !h.hosts[r.Host] {
		http.Error(w, "host not allowed", 403)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && !h.origins[origin] {
		http.Error(w, "origin not allowed", 403)
		return
	}
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, "verified client certificate required", 401)
		return
	}
	cert := r.TLS.PeerCertificates[0]
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		http.Error(w, "expired client certificate", 401)
		return
	}
	fingerprint := sha256.Sum256(cert.Raw)
	e := h.endpoints[hex.EncodeToString(fingerprint[:])]
	if e == nil {
		http.Error(w, "principal not configured", 403)
		return
	}
	if !e.limiter.Allow() {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "rate limit exceeded", 429)
		return
	}
	select {
	case e.slots <- struct{}{}:
		defer func() { <-e.slots }()
	default:
		http.Error(w, "too many concurrent requests", 429)
		return
	}
	// Sessions are also isolated by separate StreamableHTTPHandler instances.
	if r.Method == http.MethodPost && r.Header.Get("Mcp-Session-Id") == "" {
		e.mu.Lock()
		count := e.pending
		for range e.server.Sessions() {
			count++
		}
		if count >= 16 {
			e.mu.Unlock()
			http.Error(w, "session limit exceeded", 429)
			return
		}
		e.pending++
		e.mu.Unlock()
		defer func() { e.mu.Lock(); e.pending--; e.mu.Unlock() }()
	}
	requestCtx, cancel := context.WithDeadline(r.Context(), cert.NotAfter)
	defer cancel()
	r = r.WithContext(requestCtx)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	e.handler.ServeHTTP(w, r)
}
func Serve(ctx context.Context, config Config) error {
	if _, _, err := net.SplitHostPort(config.Address); err != nil {
		return fmt.Errorf("explicit address with port is required")
	}
	keyInfo, err := os.Stat(config.PrivateKey)
	if err != nil {
		return err
	}
	if !keyInfo.Mode().IsRegular() || keyInfo.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("TLS private key must be private (0600)")
	}

	ca, err := os.ReadFile(config.ClientCA)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return fmt.Errorf("invalid client CA")
	}
	h, err := NewHandler(config)
	if err != nil {
		return err
	}
	defer h.Close()
	server := &http.Server{Addr: config.Address, Handler: h, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			h.Close()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server.Shutdown(shutdown)
		case <-done:
		}
	}()
	err = server.ListenAndServeTLS(config.Certificate, config.PrivateKey)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
