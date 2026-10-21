package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/Jatin-dudhani/my-redis/aof"
	"github.com/Jatin-dudhani/my-redis/server"
)

func main() {
	port := flag.Int("port", 6379, "server port")
	dbPath := flag.String("db", "", "path to DB snapshot file for persistence")
	aofPath := flag.String("aof", "", "path to append-only file (AOF) for durability")
	fsyncPolicy := flag.String("fsync", "everysec", "AOF fsync policy: always, everysec, no")
	clusterEnabled := flag.Bool("cluster", false, "enable Redis cluster slot mode")
	clusterNodeID := flag.String("cluster-node-id", "node-main", "cluster node ID")
	clusterStartSlot := flag.Int("cluster-start-slot", 0, "cluster start slot (0-16383)")
	clusterEndSlot := flag.Int("cluster-end-slot", 16383, "cluster end slot (0-16383)")
	busPort := flag.Int("bus-port", 0, "cluster gossip bus port (default port+10000)")
	httpPort := flag.Int("http-port", 8080, "HTTP Web dashboard and Prometheus metrics port (0 to disable)")
	requirePass := flag.String("requirepass", "", "password for default user")
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)
	srv := server.New(addr, *dbPath)

	if *requirePass != "" {
		_ = srv.ACL().SetUser("default", []string{">" + *requirePass})
		log.Printf("Enterprise security enabled: default user protected with password")
	}

	if *aofPath != "" {
		policy := aof.FsyncEverySec
		switch *fsyncPolicy {
		case "always":
			policy = aof.FsyncAlways
		case "no":
			policy = aof.FsyncNo
		}
		if err := srv.EnableAOF(*aofPath, policy); err != nil {
			log.Fatalf("failed to enable AOF: %v", err)
		}
		log.Printf("AOF enabled: %s (fsync=%s)", *aofPath, *fsyncPolicy)
	}

	if *clusterEnabled {
		srv.EnableCluster(*clusterNodeID, uint16(*clusterStartSlot), uint16(*clusterEndSlot))
		bPort := *port + 10000
		if *busPort > 0 {
			bPort = *busPort
		}
		busAddr := fmt.Sprintf(":%d", bPort)
		if err := srv.EnableClusterBus(busAddr); err != nil {
			log.Printf("warning: failed to start gossip bus: %v", err)
		} else {
			log.Printf("Cluster Gossip Bus active on %s", busAddr)
		}
		log.Printf("Cluster mode enabled: slots %d-%d (node %s)", *clusterStartSlot, *clusterEndSlot, *clusterNodeID)
	}

	if *httpPort > 0 {
		httpAddr := fmt.Sprintf(":%d", *httpPort)
		if err := srv.EnableHTTPTelemetry(httpAddr); err != nil {
			log.Printf("warning: failed to start HTTP telemetry server: %v", err)
		} else {
			log.Printf("Web Dashboard & Prometheus Metrics active on http://localhost:%d/dashboard and /metrics", *httpPort)
		}
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		log.Println("shutting down...")
		srv.Stop()
	}()

	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
}
