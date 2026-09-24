package remote

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func clientCertificates(t *testing.T) (*x509.CertPool, []tls.Certificate) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	var clients []tls.Certificate
	for i := int64(2); i < 5; i++ {
		k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		template := &x509.Certificate{SerialNumber: big.NewInt(i), Subject: pkix.Name{CommonName: "test-client"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		raw, err := x509.CreateCertificate(rand.Reader, template, ca, &k.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		keyDER, _ := x509.MarshalECPrivateKey(k)
		certificate, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, certificate)
	}
	return pool, clients
}
func testRemote(t *testing.T, changes ...func(*Config)) (*httptest.Server, []*http.Client, Config) {
	t.Helper()
	pool, certificates := clientCertificates(t)
	config := Config{AllowedHosts: []string{"127.0.0.1"}}
	for i, role := range []string{"admin", "viewer"} {
		dir := t.TempDir()
		os.Chmod(dir, 0700)
		creds := filepath.Join(dir, "credentials.json")
		data, _ := json.Marshal(map[string]string{"OS_AUTH_URL": "https://identity.example.invalid/v3", "OS_APPLICATION_CREDENTIAL_ID": role, "OS_APPLICATION_CREDENTIAL_SECRET": "fixture-only"})
		os.WriteFile(creds, data, 0600)
		fp := sha256.Sum256(certificates[i].Certificate[0])
		config.Principals = append(config.Principals, Principal{Name: role, Role: role, StateDir: dir, CredentialsFile: creds, CertificateSHA256: hex.EncodeToString(fp[:])})
	}
	for _, change := range changes {
		change(&config)
	}
	handler, err := NewHandler(config)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool, MinVersion: tls.VersionTLS13}
	server.StartTLS()
	handler.hosts[strings.TrimPrefix(server.URL, "https://")] = true
	t.Cleanup(func() { handler.Close(); server.Close() })
	var clients []*http.Client
	for _, cert := range certificates {
		transport := server.Client().Transport.(*http.Transport).Clone()
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
		transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
		clients = append(clients, &http.Client{Transport: transport, Timeout: 10 * time.Second})
		t.Cleanup(transport.CloseIdleConnections)
	}
	return server, clients, config
}
func connect(t *testing.T, url string, client *http.Client, options ...*mcp.ClientOptions) *mcp.ClientSession {
	t.Helper()
	var opts *mcp.ClientOptions
	if len(options) > 0 {
		opts = options[0]
	}
	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, opts)
	session, err := c.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url + "/mcp", HTTPClient: client}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
func TestMTLSRolesAndPrincipalIsolation(t *testing.T) {
	server, clients, config := testRemote(t)
	admin := connect(t, server.URL, clients[0])
	viewer := connect(t, server.URL, clients[1])
	ctx := context.Background()
	r, err := admin.CallTool(ctx, &mcp.CallToolParams{Name: "record_agent_memory", Arguments: map[string]any{"note": "admin-only-note"}})
	if err != nil || r.IsError {
		t.Fatalf("admin: %+v %v", r, err)
	}
	r, err = viewer.CallTool(ctx, &mcp.CallToolParams{Name: "get_agent_memory", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "admin-only-note") {
		t.Fatal("cross-principal memory leak")
	}
	_, err = viewer.CallTool(ctx, &mcp.CallToolParams{Name: "record_agent_memory", Arguments: map[string]any{"note": "denied"}})
	if err == nil {
		t.Fatal("viewer mutation permitted")
	}
	audit, _ := os.ReadFile(filepath.Join(config.Principals[1].StateDir, "audit.jsonl"))
	if !strings.Contains(string(audit), `"principal":"viewer"`) || !strings.Contains(string(audit), `"status":"rejected"`) || strings.Contains(string(audit), "fixture-only") {
		t.Fatalf("incorrect audit %s", audit)
	}
	unknown, err := clients[2].Get(server.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	unknown.Body.Close()
	if unknown.StatusCode != 403 {
		t.Fatal("unknown certificate accepted")
	}
	// Missing client certificate must fail during TLS negotiation.
	if response, err := server.Client().Get(server.URL + "/mcp"); err == nil {
		response.Body.Close()
		t.Fatal("unauthenticated TLS accepted")
	}
	request, _ := http.NewRequest("POST", server.URL+"/mcp", strings.NewReader("{}"))
	request.Header.Set("Origin", "https://attacker.invalid")
	response, err := clients[0].Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("hostile origin accepted")
	}
}
func TestCredentialEnvironmentDoesNotInheritAmbient(t *testing.T) {
	_, _, config := testRemote(t)
	t.Setenv("OS_TOKEN", "ambient-secret")
	t.Setenv("OS_CLOUD", "ambient-cloud")
	sc, err := principalScope(config.Principals[0])
	if err != nil {
		t.Fatal(err)
	}
	env := strings.Join(sc.Environment, "\n")
	if strings.Contains(env, "ambient-") || !strings.Contains(env, "OS_APPLICATION_CREDENTIAL_ID=admin") {
		t.Fatal("credential isolation failed")
	}
}
func TestSessionCannotCrossPrincipals(t *testing.T) {
	server, clients, _ := testRemote(t)
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"test"}}}`
	req, _ := http.NewRequest("POST", server.URL+"/mcp", strings.NewReader(initialize))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	response, err := clients[0].Do(req)
	if err != nil {
		t.Fatal(err)
	}
	id := response.Header.Get("Mcp-Session-Id")
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if id == "" {
		t.Fatal("missing session ID")
	}
	req, _ = http.NewRequest("POST", server.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", id)
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")
	response, err = clients[1].Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatalf("cross principal session status %d", response.StatusCode)
	}
}
func TestPrivateConfigurationAndAliases(t *testing.T) {
	_, _, config := testRemote(t)
	config.Principals[1].StateDir = config.Principals[0].StateDir
	if _, err := NewHandler(config); err == nil {
		t.Fatal("shared state accepted")
	}
	file := filepath.Join(t.TempDir(), "config")
	os.WriteFile(file, []byte(`{}`), 0644)
	if _, err := Load(file); err == nil {
		t.Fatal("public configuration accepted")
	}
}

func TestScopedCLIAndProtectedID(t *testing.T) {
	server, clients, config := testRemote(t)
	// PATH was captured at handler construction; create the fixture first in a
	// separate setup and verify context environment directly through the CLI API.
	_ = server
	_ = clients
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(dir, "openstack"), []byte("#!/bin/sh\nif [ \"$1 $2\" = 'server show' ]; then echo '{\"id\":\"uuid\",\"name\":\"prod-db\",\"status\":\"ACTIVE\"}'; else echo '[]'; fi\n"), 0700)
	sc, err := principalScope(config.Principals[0])
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(sc)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	c := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "t"}, nil)
	cs, err := c.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "delete_instance", Arguments: map[string]any{"name": "uuid", "confirm_name": "uuid"}})
	if err != nil || !r.IsError {
		t.Fatalf("protected UUID accepted: %+v %v", r, err)
	}
}

func TestRemoteElicitationSamplingAndCloudCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OS_TOKEN", "AMBIENT_SECRET")
	os.WriteFile(filepath.Join(dir, "openstack"), []byte("#!/bin/sh\nif [ -n \"$OS_TOKEN\" ]; then exit 9; fi\nprintf '[{\"ID\":\"net\",\"Name\":\"%s\"}]' \"$OS_APPLICATION_CREDENTIAL_ID\"\n"), 0700)
	server, clients, config := testRemote(t, func(c *Config) { c.Principals[0].RequireElicitation = true; c.Principals[0].AllowSampling = true })
	elicited := false
	sampled := false
	admin := connect(t, server.URL, clients[0], &mcp.ClientOptions{
		ElicitationHandler: func(ctx context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			elicited = true
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}, nil
		},
		CreateMessageHandler: func(ctx context.Context, r *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			sampled = true
			data, _ := json.Marshal(r.Params)
			if strings.Contains(string(data), "fixture-only") || strings.Contains(string(data), "my-private-note") {
				t.Error("private data sampled")
			}
			return &mcp.CreateMessageResult{Role: "assistant", Content: &mcp.TextContent{Text: "Counts reviewed."}, Model: "test"}, nil
		},
	})
	result, err := admin.CallTool(context.Background(), &mcp.CallToolParams{Name: "record_agent_memory", Arguments: map[string]any{"note": "my-private-note"}})
	if err != nil || result.IsError || !elicited {
		t.Fatalf("remote elicitation: %+v %v", result, err)
	}
	result, err = admin.CallTool(context.Background(), &mcp.CallToolParams{Name: "analyze_agent_activity", Arguments: map[string]any{"allow_sampling": true}})
	if err != nil || result.IsError || !sampled {
		t.Fatalf("remote sampling: %+v %v", result, err)
	}
	viewer := connect(t, server.URL, clients[1])
	for i, session := range []*mcp.ClientSession{admin, viewer} {
		r, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "openstack://networks"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(r.Contents[0].Text, config.Principals[i].Name) {
			t.Fatal("wrong credential context")
		}
	}
}
func TestLimitsAndMalformedConfiguration(t *testing.T) {
	server, clients, config := testRemote(t)
	req, _ := http.NewRequest("POST", server.URL+"/mcp", strings.NewReader(strings.Repeat("x", (1<<20)+100)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	response, err := clients[0].Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode < 400 {
		t.Fatal("oversized body accepted")
	}
	limited := false
	for i := 0; i < 40; i++ {
		r, err := clients[1].Get(server.URL + "/mcp")
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if r.StatusCode == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("rate limit not enforced")
	}
	config.Principals[0].ProtectedPatterns = []string{"["}
	if _, err := NewHandler(config); err == nil {
		t.Fatal("invalid pattern accepted")
	}
}
