package api

import (
	"net/http"
	"strings"
)

type AuthConfig struct {
	OperatorToken    string
	BootstrapToken   string
	NodeToken        string
	AllowAnonymousUI bool
}

type authScope string

const (
	scopeOperator  authScope = "operator"
	scopeBootstrap authScope = "bootstrap"
	scopeNode      authScope = "node"
)

func (s *Server) requireAuth(scope authScope, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authBypassed(scope, r) {
			next(w, r)
			return
		}

		if bearerToken(r) != s.tokenForScope(scope) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		next(w, r)
	}
}

func (s *Server) requireAuthAny(scopes []authScope, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)

		for _, scope := range scopes {
			if s.authBypassed(scope, r) {
				next(w, r)
				return
			}
			if token == s.tokenForScope(scope) {
				next(w, r)
				return
			}
		}

		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
}

func (s *Server) authBypassed(scope authScope, r *http.Request) bool {
	if scope == scopeOperator && s.auth.AllowAnonymousUI && r.Method == http.MethodGet {
		return true
	}

	return s.tokenForScope(scope) == ""
}

func (s *Server) tokenForScope(scope authScope) string {
	switch scope {
	case scopeOperator:
		return s.auth.OperatorToken
	case scopeBootstrap:
		return s.auth.BootstrapToken
	case scopeNode:
		return s.auth.NodeToken
	default:
		return ""
	}
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}

	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return ""
	}

	return strings.TrimSpace(token)
}
