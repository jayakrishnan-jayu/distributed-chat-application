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
	stopCh := make(chan struct{})

	// Start the broadcast listener in a goroutine
	go s.StartBroadcastListener(stopCh)

	// Simulate some other work
	time.Sleep(5 * time.Second)

	// Signal the listener to stop and wait for cleanup
	close(stopCh)
	time.Sleep(1 * time.Second)

}
