package main

import (
	"dummy-rom/server"
	"log"
	"math/rand/v2"
	"time"
)

func main() {
	randomDelay := time.Duration(rand.IntN(500)+2500) * time.Millisecond
	time.Sleep(randomDelay)
	s, err := server.NewServer()
	if err != nil {
		log.Panic(err)
	}

	go s.StartBroadcastListener()
	go s.StartUniCastSender()
	go s.StartUniCastListener()

	go s.StartInit()

	time.Sleep(9 * time.Second)
	s.Shutdown()
	s.Debug()
}
