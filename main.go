package main

import (
	"dummy-rom/server"
	"log"
	"time"
)

func main() {
	s, err := server.NewServer()
	if err != nil {
		log.Panic(err)
	}

	s.BroadcastHello()

	go s.StartBroadcastListener()
	go s.StartUniCastSender()
	go s.StartUniCastListener()

	time.Sleep(2 * time.Second)
	s.Shutdown()
}
