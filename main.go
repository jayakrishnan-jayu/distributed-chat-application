package main

import (
	"dummy-rom/server"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// randomDelay := time.Duration(rand.IntN(500)+2500) * time.Millisecond
	// time.Sleep(randomDelay)
	s, err := server.NewServer()
	if err != nil {
		log.Panic(err)
	}

	// go s.StartBroadcastListener()
	// go s.StartUniCastListener()
	// go s.StartMulticastListener()

	s.Run()

	// s.KillLeaderAfter(5 * time.Second)
	// s.KillFollowerAfter(4 * time.Second)

	// time.Sleep(10 * time.Second)
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		s.Shutdown()
		s.Debug()
		os.Exit(0)
	}()
	for {

	}
	// s.Shutdown()
	// s.Debug()
}
