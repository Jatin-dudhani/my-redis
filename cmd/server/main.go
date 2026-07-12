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
	flag.Parse()
	addr := fmt.Sprintf(":%d", *port)

	srv := server.New(addr)

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
