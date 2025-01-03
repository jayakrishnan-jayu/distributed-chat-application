package main

import (
	"dummy-rom/server"
	"fmt"
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

	// go s.StartInit()
	// s.StartBroadcasting()
	// time.Sleep(1 * time.Second)
	// s.StopBroadcasting()
	s.Broadcaster.Start()
	// s.StopBroadcast()
	// s.StartBroadcast()
	fmt.Println("Broadcast started. Broadcasting every second.")

	// Let it s.oadcast for 5 seconds
	time.Sleep(3 * time.Second)

	// Stop s.oadcasting
	s.Broadcaster.Stop()
	time.Sleep(3 * time.Second)
	s.Broadcaster.Start()
	time.Sleep(3 * time.Second)
	// fmt.Println("Broadcast stopped.")
	//
	// // Stop s.oadcast called again to demonstrate safety
	// s.StopBroadcast()
	// fmt.Println("Stop called multiple times without error.")
	// time.Sleep(5 * time.Second)
	// s.StartBroadcast()
	// time.Sleep(2 * time.Second)
	// s.StopBroadcast()
	// time.Sleep(5 * time.Second)
	// s.StartBroadcast()
	s.Shutdown()
	s.Debug()
}
