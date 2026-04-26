package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPMiddleware_recordsRequest(t *testing.T) {
	t.Parallel()
	m := New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := m.HTTPMiddleware(mux)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status: %d", rec.Code)
	}
	// scrape registry
	gather, err := m.reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, mf := range gather {
		if mf.GetName() == "http_requests_total" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected http_requests_total metric")
	}
}
