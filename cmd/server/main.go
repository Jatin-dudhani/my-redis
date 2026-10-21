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
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)
	srv := server.New(addr, *dbPath)

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
		log.Printf("Cluster mode enabled: slots %d-%d (node %s)", *clusterStartSlot, *clusterEndSlot, *clusterNodeID)
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
