package telemetry

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// KeyInfo provides metadata for dashboard key inspection.
type KeyInfo struct {
	Key  string `json:"key"`
	Type string `json:"type"`
	TTL  int64  `json:"ttl"`
}

// ServerCallbacks interfaces telemetry with core Redis state.
type ServerCallbacks struct {
	GetClientsCount func() int
	GetKeysCount    func() int
	GetKeysInfo     func() []KeyInfo
	ExecuteCommand  func(cmd string) string
}

// HTTPServer hosts the Web Dashboard and Prometheus metrics endpoint.
type HTTPServer struct {
	addr      string
	metrics   *Metrics
	callbacks ServerCallbacks
	srv       *http.Server
	ln        net.Listener
	mu        sync.Mutex
}

// NewHTTPServer constructs the telemetry HTTP service.
func NewHTTPServer(addr string, metrics *Metrics, callbacks ServerCallbacks) *HTTPServer {
	return &HTTPServer{
		addr:      addr,
		metrics:   metrics,
		callbacks: callbacks,
	}
}

// Start begins listening and serving HTTP requests.
func (h *HTTPServer) Start() error {
	mux := http.NewServeMux()

	// 1. Prometheus /metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		clients := 0
		keys := 0
		if h.callbacks.GetClientsCount != nil {
			clients = h.callbacks.GetClientsCount()
		}
		if h.callbacks.GetKeysCount != nil {
			keys = h.callbacks.GetKeysCount()
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(w, h.metrics.PrometheusFormat(clients, keys))
	})

	// 2. Real-time Web Dashboard
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, DashboardHTML)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/dashboard", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	// 3. JSON API: /api/stats
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		snap := h.metrics.Snapshot()
		clients := 0
		keys := 0
		if h.callbacks.GetClientsCount != nil {
			clients = h.callbacks.GetClientsCount()
		}
		if h.callbacks.GetKeysCount != nil {
			keys = h.callbacks.GetKeysCount()
		}

		respObj := map[string]interface{}{
			"uptime_seconds":     snap.UptimeSeconds,
			"total_commands":     snap.TotalCommands,
			"ops_per_second":     snap.OpsPerSec,
			"cache_hits":         snap.CacheHits,
			"cache_misses":       snap.CacheMisses,
			"hit_rate":           snap.HitRate,
			"memory_bytes":       snap.MemoryAllocated,
			"goroutines":         snap.NumGoroutines,
			"connected_clients":  clients,
			"keys_count":         keys,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(respObj)
	})

	// 4. JSON API: /api/keys
	mux.HandleFunc("/api/keys", func(w http.ResponseWriter, r *http.Request) {
		var list []KeyInfo
		if h.callbacks.GetKeysInfo != nil {
			list = h.callbacks.GetKeysInfo()
		}
		if list == nil {
			list = []KeyInfo{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

	// 5. JSON API: /api/query
	mux.HandleFunc("/api/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Command string `json:"command"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		reply := "OK"
		if h.callbacks.ExecuteCommand != nil {
			reply = h.callbacks.ExecuteCommand(req.Command)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"reply": reply,
		})
	})

	ln, err := net.Listen("tcp", h.addr)
	if err != nil {
		return err
	}

	h.mu.Lock()
	h.ln = ln
	h.srv = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	srv := h.srv
	h.mu.Unlock()

	return srv.Serve(ln)
}

// Stop gracefully stops the HTTP telemetry service.
func (h *HTTPServer) Stop() error {
	h.mu.Lock()
	srv := h.srv
	ln := h.ln
	h.mu.Unlock()

	var err error
	if srv != nil {
		err = srv.Close()
	}
	if ln != nil {
		_ = ln.Close()
	}
	return err
}
