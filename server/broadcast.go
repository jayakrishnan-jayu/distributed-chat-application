package server

import (
	"github.com/google/uuid"
)

func (s *Server) StartBroadcastListener() {
	s.Broadcaster.StartListener()
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
	// leaderID := s.leaderID
	s.mu.Unlock()

	// if clientUUIDStr > id && leaderID != clientUUIDStr {
	// 	s.logger.Println("broadcast", state, clientUUIDStr, id, clientUUIDStr > id)
	// }
	// if broadcast is from the same node, skip
	if clientUUIDStr == id {
		return
	}
	s.logger.Println("broadcast from", ip, state)

	switch state {
	// case INIT:
	// 	s.mu.Lock()
	// 	_, ok := s.peers[clientUUIDStr]
	// 	s.peers[clientUUIDStr] = ip
	// 	s.mu.Unlock()
	// 	if !ok {
	// 		log.Println("already connected")
	// 		break
	// 	}
	// 	s.logger.Println("connecting to broadcaster", ip)
	// 	s.connectToLeader(ip, clientUUIDStr, id)
	// 	break
	// case FOLLOWER:
	// 	if leaderID == "" {
	// 		log.Panic("Invalid state: leaderID is empty while in FOLLOWER state")
	// 		break
	// 	}
	// 	if clientUUIDStr == leaderID {
	// 		log.Println("broadcast from leader")
	// 		break
	// 	}
	// 	s.mu.Lock()
	// 	if _, ok := s.peers[clientUUIDStr]; !ok {
	// 		s.peers[clientUUIDStr] = ip
	// 	}
	// 	s.mu.Unlock()
	// 	if clientUUIDStr > leaderID {
	// 		log.Println("found new leader with higher id")
	// 		s.mu.Lock()
	// 		s.peers[clientUUIDStr] = ip
	// 		s.mu.Unlock()
	// 		s.connectToLeader(ip, clientUUIDStr, id)
	// 		break
	// 	}
	// 	log.Println("lower id node broadcasting", ip, clientUUIDStr, leaderID)
	// 	break
	// case ELECTION:
	// 	log.Println("in election, skipping broadcast")
	// 	break
	case INIT:
	case ELECTION:
	case FOLLOWER:
		s.logger.Println("broadcast in ", state)
	case LEADER:
		s.logger.Println(clientUUIDStr > id)
		if clientUUIDStr > id {
			s.mu.Lock()
			s.state = INIT
			s.leaderID = ""
			// s.StopBroadcasting()
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
