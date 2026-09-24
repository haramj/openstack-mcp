package tools

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/haramj/openstack-mcp-server/internal/scope"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
	"time"
)

var interactionKey = func() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	return key
}()

type interactionState struct {
	Expires int64           `json:"expires"`
	Binding string          `json:"binding"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func binding(ctx context.Context, request *mcp.CallToolRequest) string {
	principal := "local"
	if c, ok := scope.From(ctx); ok {
		principal = c.Principal
	}
	digest := sha256.Sum256(append([]byte(principal+"\x00"+request.Params.Name+"\x00"), request.Params.Arguments...))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}
func issueState(ctx context.Context, r *mcp.CallToolRequest, data []byte) string {
	body, _ := json.Marshal(interactionState{Expires: time.Now().Add(time.Minute).Unix(), Binding: binding(ctx, r), Data: data})
	mac := hmac.New(sha256.New, interactionKey)
	mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func readState(ctx context.Context, r *mcp.CallToolRequest) (json.RawMessage, error) {
	bad := errors.New("interactive response expired or does not match this operation")
	if len(r.Params.RequestState) > 8192 {
		return nil, bad
	}
	body, sig, ok := strings.Cut(r.Params.RequestState, ".")
	if !ok {
		return nil, bad
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, bad
	}
	signature, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return nil, bad
	}
	mac := hmac.New(sha256.New, interactionKey)
	mac.Write(raw)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, bad
	}
	var state interactionState
	if json.Unmarshal(raw, &state) != nil || state.Expires < time.Now().Unix() || state.Binding != binding(ctx, r) {
		return nil, bad
	}
	return state.Data, nil
}
