package telemetry

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics records server activity, performance, and Prometheus metrics.
type Metrics struct {
	mu             sync.RWMutex
	startTime      time.Time
	totalCommands  int64
	cacheHits      int64
	cacheMisses    int64
	cmdCalls       map[string]int64
	prevCmds       int64
	prevTime       time.Time
	opsPerSec      float64
	stopCh         chan struct{}
}

// NewMetrics initializes metrics collection and launches background rate calculation.
func NewMetrics() *Metrics {
	m := &Metrics{
		startTime: time.Now(),
		cmdCalls:  make(map[string]int64),
		prevTime:  time.Now(),
		stopCh:    make(chan struct{}),
	}
	m.startRateWorker()
	return m
}

// Close stops background workers.
func (m *Metrics) Close() {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
}

// RecordCommand records execution of a command.
func (m *Metrics) RecordCommand(cmd string) {
	atomic.AddInt64(&m.totalCommands, 1)
	cmd = strings.ToUpper(cmd)

	m.mu.Lock()
	m.cmdCalls[cmd]++
	m.mu.Unlock()
}

// RecordHit increments cache hit counter.
func (m *Metrics) RecordHit() {
	atomic.AddInt64(&m.cacheHits, 1)
}

// RecordMiss increments cache miss counter.
func (m *Metrics) RecordMiss() {
	atomic.AddInt64(&m.cacheMisses, 1)
}

func (m *Metrics) startRateWorker() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				now := time.Now()
				curr := atomic.LoadInt64(&m.totalCommands)
				m.mu.Lock()
				dur := now.Sub(m.prevTime).Seconds()
				if dur > 0 {
					m.opsPerSec = float64(curr-m.prevCmds) / dur
				}
				m.prevCmds = curr
				m.prevTime = now
				m.mu.Unlock()
			case <-m.stopCh:
				return
			}
		}
	}()
}

// GetOpsPerSec returns current throughput.
func (m *Metrics) GetOpsPerSec() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.opsPerSec
}

// Snapshot returns summary stats.
type StatsSnapshot struct {
	UptimeSeconds   int64              `json:"uptime_seconds"`
	TotalCommands   int64              `json:"total_commands"`
	OpsPerSec       float64            `json:"ops_per_second"`
	CacheHits       int64              `json:"cache_hits"`
	CacheMisses     int64              `json:"cache_misses"`
	HitRate         float64            `json:"hit_rate"`
	MemoryAllocated uint64             `json:"memory_bytes"`
	NumGoroutines   int                `json:"goroutines"`
	CommandCalls    map[string]int64   `json:"command_calls"`
}

// Snapshot gathers a point-in-time statistics snapshot.
func (m *Metrics) Snapshot() StatsSnapshot {
	m.mu.RLock()
	callsCopy := make(map[string]int64, len(m.cmdCalls))
	for k, v := range m.cmdCalls {
		callsCopy[k] = v
	}
	ops := m.opsPerSec
	m.mu.RUnlock()

	totalCmds := atomic.LoadInt64(&m.totalCommands)
	hits := atomic.LoadInt64(&m.cacheHits)
	misses := atomic.LoadInt64(&m.cacheMisses)

	var hitRate float64
	if hits+misses > 0 {
		hitRate = float64(hits) / float64(hits+misses) * 100.0
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return StatsSnapshot{
		UptimeSeconds:   int64(time.Since(m.startTime).Seconds()),
		TotalCommands:   totalCmds,
		OpsPerSec:       ops,
		CacheHits:       hits,
		CacheMisses:     misses,
		HitRate:         hitRate,
		MemoryAllocated: memStats.Alloc,
		NumGoroutines:   runtime.NumGoroutine(),
		CommandCalls:    callsCopy,
	}
}

// PrometheusFormat exports metrics in Prometheus text exposition format.
func (m *Metrics) PrometheusFormat(connectedClients int, keysTotal int) string {
	snap := m.Snapshot()
	var b strings.Builder

	b.WriteString("# HELP redis_uptime_seconds Total server uptime in seconds\n")
	b.WriteString("# TYPE redis_uptime_seconds gauge\n")
	b.WriteString(fmt.Sprintf("redis_uptime_seconds %d\n\n", snap.UptimeSeconds))

	b.WriteString("# HELP redis_connected_clients Total active client connections\n")
	b.WriteString("# TYPE redis_connected_clients gauge\n")
	b.WriteString(fmt.Sprintf("redis_connected_clients %d\n\n", connectedClients))

	b.WriteString("# HELP redis_total_commands_processed Total commands processed\n")
	b.WriteString("# TYPE redis_total_commands_processed counter\n")
	b.WriteString(fmt.Sprintf("redis_total_commands_processed %d\n\n", snap.TotalCommands))

	b.WriteString("# HELP redis_ops_per_second Instantaneous operations per second\n")
	b.WriteString("# TYPE redis_ops_per_second gauge\n")
	b.WriteString(fmt.Sprintf("redis_ops_per_second %.2f\n\n", snap.OpsPerSec))

	b.WriteString("# HELP redis_keys_total Total keys stored in the database\n")
	b.WriteString("# TYPE redis_keys_total gauge\n")
	b.WriteString(fmt.Sprintf("redis_keys_total %d\n\n", keysTotal))

	b.WriteString("# HELP redis_memory_used_bytes Current heap allocation in bytes\n")
	b.WriteString("# TYPE redis_memory_used_bytes gauge\n")
	b.WriteString(fmt.Sprintf("redis_memory_used_bytes %d\n\n", snap.MemoryAllocated))

	b.WriteString("# HELP redis_cache_hits_total Total cache hits\n")
	b.WriteString("# TYPE redis_cache_hits_total counter\n")
	b.WriteString(fmt.Sprintf("redis_cache_hits_total %d\n\n", snap.CacheHits))

	b.WriteString("# HELP redis_cache_misses_total Total cache misses\n")
	b.WriteString("# TYPE redis_cache_misses_total counter\n")
	b.WriteString(fmt.Sprintf("redis_cache_misses_total %d\n\n", snap.CacheMisses))

	b.WriteString("# HELP redis_command_calls_total Number of calls per command\n")
	b.WriteString("# TYPE redis_command_calls_total counter\n")
	for cmd, count := range snap.CommandCalls {
		b.WriteString(fmt.Sprintf("redis_command_calls_total{cmd=\"%s\"} %d\n", cmd, count))
	}

	return b.String()
}
