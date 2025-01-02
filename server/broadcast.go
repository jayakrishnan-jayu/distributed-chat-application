package server

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/google/uuid"
)

func (s *Server) StartBroadcastListener() {
	var err error

	s.broadConn, err = net.ListenPacket("udp4", BROADCAST_L_PORT)
	if err != nil {
		s.logger.Panic(err)
	}
	defer s.broadConn.Close()

	buf := make([]byte, 1024)

	for {
		n, addr, err := s.broadConn.ReadFrom(buf)
		if err != nil {
			select {
			case <-s.quit:
				s.logger.Println("Broadcast Connection closed")
				return
			default:
				s.logger.Printf("Error reading from connection: %v\n", err)
			}
		}
		udpAddr, ok := addr.(*net.UDPAddr)
		if !ok {
			s.logger.Printf("Unexpected address type: %T", addr)
			continue
		}

		// if broadcast from self, discard
		if udpAddr.IP.String() == s.ip {
			continue
		}

		go s.handleBroadcastMessage(string(buf[:n]), udpAddr.IP.String())
	}
}

// Broadcast is  used for leader discovery
func (s *Server) handleBroadcastMessage(clientUUIDStr string, ip string) {
	clientUUID, err := uuid.Parse(clientUUIDStr)
	if err != nil {
		s.logger.Printf("Invalid UUID received from %s: %v\n", ip, err)
		return
	}
	clientUUIDStr = clientUUID.String()

	s.mu.Lock()
	id := s.id
	// _, ok := s.peers[clientUUIDStr]
	state := s.state
	leaderID := s.leaderID
	s.mu.Unlock()

	// if broadcast is from the same node, skip
	if clientUUIDStr > id && leaderID != clientUUIDStr {

		s.logger.Println("broadcast", state, clientUUIDStr, id, clientUUIDStr > id)
	}
	if clientUUIDStr == id {
		return
	}

	switch state {
	case INIT:
		s.mu.Lock()
		_, ok := s.peers[clientUUIDStr]
		s.peers[clientUUIDStr] = ip
		s.mu.Unlock()
		if !ok {
			log.Println("already connected")
			break
		}
		s.logger.Println("connecting to broadcaster", ip)
		s.connectToLeader(ip, clientUUIDStr, id)
		break
	case FOLLOWER:
		if leaderID == "" {
			log.Panic("Invalid state: leaderID is empty while in FOLLOWER state")
			break
		}
		if clientUUIDStr == leaderID {
			log.Println("broadcast from leader")
			break
		}
		s.mu.Lock()
		if _, ok := s.peers[clientUUIDStr]; !ok {
			s.peers[clientUUIDStr] = ip
		}
		s.mu.Unlock()
		if clientUUIDStr > leaderID {
			log.Println("found new leader with higher id")
			s.mu.Lock()
			s.peers[clientUUIDStr] = ip
			s.mu.Unlock()
			s.connectToLeader(ip, clientUUIDStr, id)
			break
		}
		log.Println("lower id node broadcasting", ip, clientUUIDStr, leaderID)
		break
	case ELECTION:
		log.Println("in election, skipping broadcast")
		break
	case LEADER:
		if clientUUIDStr > id {
			s.mu.Lock()
			s.StopBroadcasting()
			s.state = INIT
			s.mu.Unlock()
		}

		break

	}

	// if state == FOLLOWER {
	// 	if clientUUIDStr != leaderID {
	// 		s.logger.Println("broadcast rcvd from non leader")
	// 		return
	// 	}
	// 	s.logger.Println("broadcast from leader")
	// 	return
	// }
	//
	// if state == ELECTION {
	// 	s.logger.Println("In election mode, skipping broadcast")
	// 	return
	// }
	//
	// if state == INIT {
	// 	s.mu.Lock()
	// 	_, ok := s.peers[clientUUIDStr]
	// 	if !ok {
	// 		s.peers[clientUUIDStr] = ip
	// 	}
	// 	s.mu.Unlock()
	// 	// if the client was already connected, and waiting for response, then skip
	// 	if ok {
	// 		return
	// 	}
	// 	s.logger.Println("Broadcast rcvd from existing server, connecting to", leaderID, ip)
	// 	s.connectToLeader(ip, clientUUIDStr, id)
	// 	return
	// }
	//
	// if state == LEADER {
	// 	if clientUUIDStr > id {
	// 		s.mu.Lock()
	// 		s.state = INIT
	// 		s.leaderID = ""
	// 		s.StopBroadcasting()
	// 		s.mu.Unlock()
	// 		s.connectToLeader(ip, clientUUIDStr, id)
	// 	}
	// 	return
	// }

	// if !ok {
	// 	s.logger.Println("adding new server", ip)
	// 	s.mu.Lock()
	// 	s.peers[clientUUIDStr] = ip
	// 	s.discoveredPeers[clientUUIDStr] = false
	// 	s.mu.Unlock()
	// 	s.newServer <- clientUUIDStr
	// }

}

func (s *Server) BroadcastHello() {
	SendUDP(s.broadcast, BROADCAST_S_PORT, BROADCAST_L_PORT, []byte(s.id))
}

func (s *Server) startBroadcast() {
	if s.stopBroadcasting == nil {
		s.stopBroadcasting = make(chan struct{})
	}
	if s.broadcastDoneChan == nil {
		s.broadcastDoneChan = make(chan struct{})
	}

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				SendUDP(s.broadcast, BROADCAST_S_PORT, BROADCAST_L_PORT, []byte(s.id))
			case <-s.stopBroadcasting:
				// Signal that broadcasting has stopped
				close(s.broadcastDoneChan)
				return
			}
		}
	}()
}

func (s *Server) StopBroadcasting() {
	if s.stopBroadcasting != nil {
		close(s.stopBroadcasting)
		s.stopBroadcasting = nil

		// Wait for the broadcasting goroutine to finish
		<-s.broadcastDoneChan
		s.broadcastDoneChan = nil
	}
}

func (s *Server) StartBroadcasting() {
	s.logger.Println("starting broadcast")
	s.StopBroadcasting()
	s.startBroadcast()
}

func SendUDP(ip string, fromPort string, toPort string, data []byte) {
	pc, err := net.ListenPacket("udp4", fromPort)
	if err != nil {
		log.Panic(err)
	}
	defer pc.Close()

	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s%s", ip, toPort))
	if err != nil {
		log.Panic(err)
	}

	_, err = pc.WriteTo(data, addr)
	if err != nil {
		log.Panic(err)

	}
}
