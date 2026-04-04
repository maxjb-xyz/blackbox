package middleware

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
)

const agentNodeHeader = "X-Blackbox-Node-Name"

type agentContextKey string

const agentNodeKey agentContextKey = "agent-node"

type AgentAuthConfig struct {
	tokensByNode map[string]string
}

func NewAgentAuthConfig(raw string) (AgentAuthConfig, error) {
	tokensByNode := make(map[string]string)
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == ';'
	})
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		nodeName, token, found := strings.Cut(part, "=")
		if !found {
			return AgentAuthConfig{}, fmt.Errorf("invalid AGENT_TOKENS entry %q", part)
		}
		nodeName = strings.TrimSpace(nodeName)
		token = strings.TrimSpace(token)
		if nodeName == "" || token == "" {
			return AgentAuthConfig{}, fmt.Errorf("invalid AGENT_TOKENS entry %q", part)
		}
		if _, exists := tokensByNode[nodeName]; exists {
			return AgentAuthConfig{}, fmt.Errorf("duplicate AGENT_TOKENS entry for node %q", nodeName)
		}
		tokensByNode[nodeName] = token
	}
	if len(tokensByNode) == 0 {
		return AgentAuthConfig{}, fmt.Errorf("AGENT_TOKENS must define at least one node=token mapping")
	}
	return AgentAuthConfig{tokensByNode: tokensByNode}, nil
}

func AgentNodeFromContext(ctx context.Context) (string, bool) {
	nodeName, ok := ctx.Value(agentNodeKey).(string)
	return nodeName, ok
}

func AgentAuth(config AgentAuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nodeName := strings.TrimSpace(r.Header.Get(agentNodeHeader))
			if nodeName == "" {
				writeJSONError(w, http.StatusUnauthorized, "missing agent node header")
				return
			}

			expectedToken, ok := config.tokensByNode[nodeName]
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "unknown agent node")
				return
			}

			incoming := r.Header.Get("X-Blackbox-Agent-Key")
			if subtle.ConstantTimeCompare([]byte(incoming), []byte(expectedToken)) != 1 {
				writeJSONError(w, http.StatusUnauthorized, "invalid agent token")
				return
			}

			ctx := context.WithValue(r.Context(), agentNodeKey, nodeName)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
