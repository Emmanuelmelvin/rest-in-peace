package playground

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	srv := New(Config{Limits: DefaultLimits()})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ALIVE") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

func TestEnvDoesNotLeakHost(t *testing.T) {
	t.Setenv("HOST_SECRET", "do-not-leak")
	for _, kv := range Env(9000, "/tmp/p1", DefaultLimits()) {
		if strings.Contains(kv, "do-not-leak") {
			t.Fatalf("program env leaked a host variable: %s", kv)
		}
	}
}

func TestAllowlist(t *testing.T) {
	if !Allowed("github.com/google/uuid") || Allowed("os/exec") {
		t.Fatal("allowlist is wrong")
	}
}
