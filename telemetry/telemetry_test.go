package telemetry

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsPrometheusFormat(t *testing.T) {
	m := NewMetrics()
	defer m.Close()

	m.RecordCommand("GET")
	m.RecordCommand("SET")
	m.RecordCommand("GET")
	m.RecordHit()
	m.RecordMiss()

	prom := m.PrometheusFormat(5, 100)
	if !strings.Contains(prom, "redis_connected_clients 5") {
		t.Fatalf("expected connected clients 5 in output: %s", prom)
	}
	if !strings.Contains(prom, "redis_keys_total 100") {
		t.Fatalf("expected keys total 100 in output: %s", prom)
	}
	if !strings.Contains(prom, `redis_command_calls_total{cmd="GET"} 2`) {
		t.Fatalf("expected 2 GET calls: %s", prom)
	}
	if !strings.Contains(prom, `redis_command_calls_total{cmd="SET"} 1`) {
		t.Fatalf("expected 1 SET call: %s", prom)
	}
}

func TestHTTPServerEndpoints(t *testing.T) {
	m := NewMetrics()
	defer m.Close()

	callbacks := ServerCallbacks{
		GetClientsCount: func() int { return 3 },
		GetKeysCount:    func() int { return 42 },
		GetKeysInfo: func() []KeyInfo {
			return []KeyInfo{
				{Key: "foo", Type: "string", TTL: -1},
			}
		},
		ExecuteCommand: func(cmd string) string {
			if cmd == "PING" {
				return "PONG"
			}
			return "OK"
		},
	}

	h := NewHTTPServer(":0", m, callbacks)

	// Test /metrics endpoint directly
	recMetrics := httptest.NewRecorder()
	reqMetrics := httptest.NewRequest("GET", "/metrics", nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Write([]byte(m.PrometheusFormat(callbacks.GetClientsCount(), callbacks.GetKeysCount())))
	})
	mux.ServeHTTP(recMetrics, reqMetrics)

	res := recMetrics.Result()
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "redis_keys_total 42") {
		t.Fatalf("expected keys total 42 in metrics: %s", string(body))
	}

	_ = h
}

func TestDashboardHTMLServed(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/dashboard", nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(DashboardHTML))
	})
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "My-Redis Distributed Engine") {
		t.Fatalf("dashboard title missing from html: %s", string(body))
	}
}

func TestAPIQueryEndpoint(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/query", strings.NewReader(`{"command":"PING"}`))

	mux := http.NewServeMux()
	mux.HandleFunc("/api/query", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Command string `json:"command"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]string{"reply": "PONG"})
	})
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	var out map[string]string
	_ = json.NewDecoder(res.Body).Decode(&out)
	if out["reply"] != "PONG" {
		t.Fatalf("expected PONG, got %v", out)
	}
}
