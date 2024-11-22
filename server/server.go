package server

import (
	"fmt"
	"log"
	"net"
	"time"
)

const BROADCAST_PORT = ":5000"

type Server struct {
	ip          string
	broadcastIP string
}

func NewServer(ip string, broadcastIP string) *Server {
	return &Server{ip, broadcastIP}
}
func (s *Server) StartBroadcastListener(stopCh <-chan struct{}) {
	pc, err := net.ListenPacket("udp4", BROADCAST_PORT)
	if err != nil {
		log.Panic(err)
	}
	defer pc.Close()

	buf := make([]byte, 1024)

	for {
		select {
		case <-stopCh:
			log.Println("Broadcast listener shutting down.")
			return
		default:
			// Set a read deadline to allow periodic checks of the stop channel
			if err := pc.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
				log.Printf("Failed to set read deadline: %v\n", err)
			}

			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					// Timeout is expected; check the stop channel and continue
					continue
				}
				log.Printf("Error reading from connection: %v\n", err)
				continue
			}

			log.Printf("Received from %s: %s\n", addr, buf[:n])
		}
	}
}

// func (*Server) StartBroadcastListener() {
// 	pc, err := net.ListenPacket("udp4", BROADCAST_PORT)
// 	if err != nil {
// 		log.Panic(err)
// 	}
// 	defer pc.Close()
//
// 	buf := make([]byte, 1024)
// 	n, addr, err := pc.ReadFrom(buf)
// 	if err != nil {
// 		log.Panic(err)
// 	}
//
// 	log.Printf("%s sent this: %s\n", addr, buf[:n])
// }

func (s *Server) BroadcastHello() {
	pc, err := net.ListenPacket("udp4", BROADCAST_PORT)
	if err != nil {
		log.Panic(err)
	}
	defer pc.Close()

	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s%s", s.broadcastIP, BROADCAST_PORT))
	if err != nil {
		log.Panic(err)
	}

	_, err = pc.WriteTo([]byte(fmt.Sprintf("hello from %s", s.ip)), addr)
	if err != nil {
		log.Panic(err)
	}
	log.Println("Sent hello")
}
