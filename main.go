package main

import (
	"dummy-rom/server"
	"log"
	"time"
)

func main() {
	// randomDelay := time.Duration(rand.IntN(500)+2500) * time.Millisecond
	// time.Sleep(randomDelay)
	s, err := server.NewServer()
	if err != nil {
		log.Panic(err)
	}

	go s.StartBroadcastListener()
	go s.StartUniCastSender()
	go s.StartUniCastListener()
	go s.StartMulticastListener()

	go s.StartInit()

	// s.KillLeaderAfter(3 * time.Second)
	// s.KillFollowerAfter(5 * time.Second)

	time.Sleep(15 * time.Second)
	s.Shutdown()
	s.Debug()
}
