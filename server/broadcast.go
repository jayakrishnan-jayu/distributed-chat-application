package server

import (
	"github.com/google/uuid"
)

func (s *Server) StartBroadcastListener() {
	go s.startBroadcastMessageListener()
	s.broadcaster.StartListener()

}

// Broadcast is  used for leader discovery
func (s *Server) startBroadcastMessageListener() {
	prevLeaderID := ""
	prevLeaderBroadcastCount := 0
	s.mu.Lock()
	id := s.id
	s.mu.Unlock()

	for {
		select {
		case <-s.quit:
			s.logger.Println("Quitting Broadcast message listner")
			return
		case msg := <-s.bMsgChan:
			s.logger.Println("broadcast from", msg.IP)

			clientUUIDStr := msg.UUID
			ip := msg.IP

			clientUUID, err := uuid.Parse(clientUUIDStr)
			if err != nil {
				s.logger.Printf("Invalid UUID received from %s: %v\n", ip, err)
				break
			}
			clientUUIDStr = clientUUID.String()

			// if broadcast from self, skio
			if clientUUIDStr == id {
				break
			}

			s.mu.Lock()
			_, ok := s.peers[clientUUIDStr]
			if !ok {
				s.peers[clientUUIDStr] = ip
			}
			state := s.state
			if state == LEADER && clientUUIDStr > id {
				s.state = INIT
				s.leaderID = ""
				s.broadcaster.Stop()
			}
			s.mu.Unlock()

			if prevLeaderID != clientUUIDStr {
				prevLeaderID = clientUUIDStr
				prevLeaderBroadcastCount = 1
				break
			}
			if prevLeaderBroadcastCount < 10 {
				prevLeaderBroadcastCount += 1
				break
			}

			// if clientUUIDStr > id && leaderID != clientUUIDStr {
			// 	s.logger.Println("broadcast", state, clientUUIDStr, id, clientUUIDStr > id)
			// }
			// if broadcast is from the same node, skip
			s.logger.Println("got broadcast from", ip)

			switch state {
			case INIT:
				s.mu.Lock()
				discovered, ok := s.discoveredPeers[clientUUIDStr]
				s.mu.Unlock()
				if ok && discovered {
					break
				}
				s.logger.Println("connecting to broadcaster", ip)
				s.connectToLeader(ip, clientUUIDStr, id)
				break
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
			case ELECTION:
				s.logger.Println("broadcast in ", state)
				break
			case FOLLOWER:
				s.logger.Println("broadcast in ", state)
				break
			case LEADER:
				s.logger.Println(clientUUIDStr > id, clientUUIDStr)

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
	}
}
