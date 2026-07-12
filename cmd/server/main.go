package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/macbook/my-redis/server"
)

func main() {
	port := flag.Int("port", 6379, "server port")
	dbPath := flag.String("db", "", "path to DB file for persistence")
	flag.Parse()
	addr := fmt.Sprintf(":%d", *port)

	srv := server.New(addr, *dbPath)

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
