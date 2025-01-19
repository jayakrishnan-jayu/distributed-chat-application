package server

import (
	"dummy-rom/server/message"
)

func (s *Server) StartMulticastListener() {
	go s.StartMulticastMessageListener()
	// s.rm.StartListener()
}

func (s *Server) StartMulticastMessageListener() {
	s.logger.Println("started multicast message listner")
	for {
		select {
		case <-s.quit:
			s.logger.Println("Quitting Multicast message listner")
			return
		case msg := <-s.rmMsgChan:
			s.logger.Println("\n\nmulticast msgChan", msg.IP)
			s.mu.Lock()
			if msg.UUID == s.id {
				s.mu.Unlock()
				break
			}
			state := s.state
			s.mu.Unlock()
			decodedMsg, err := message.Decode(msg.Message)
			if err != nil {
				s.logger.Println("error decoding multicast message")
				break
			}

			switch decodedMsg.Type {
			case message.DeadNode:
				s.logger.Println("dead node from multicast", decodedMsg.UUID, decodedMsg.IP)
				s.mu.Lock()
				delete(s.peers, decodedMsg.UUID)
				delete(s.discoveredPeers, decodedMsg.UUID)
				// s.rm.HandleDeadNode(decodedMsg.UUID)
				s.mu.Unlock()
				break

			case message.ElectionVictory:
				s.mu.Lock()
				state := s.state
				s.logger.Println("Election victory from", msg.UUID, state)
				s.leaderID = msg.UUID
				s.logger.Println("new leaderID", s.leaderID, state)
				s.logger.Println("new leaderIP", s.peers[s.leaderID], state)
				if state == FOLLOWER || state == ELECTION {
					s.leaderID = msg.UUID
					// delete(s.peers, decodedMsg.OldLeaderID)
					s.mu.Unlock()
					s.sm.ChangeTo(FOLLOWER, nil)
					break
				}
				s.mu.Unlock()
				s.logger.Println("election victory in invalid state")
				break
			case message.Application:
				s.logger.Println("\tApplicationMSG: ", string(decodedMsg.Msg))
				break

			case message.PeerInfo:
				if len(decodedMsg.PeerIds) != len(decodedMsg.PeerIps) {
					s.logger.Fatal("message.peerinfo message invalid length")
				}
				s.mu.Lock()
				newPeers := make(map[string]string)
				for index, uuid := range decodedMsg.PeerIds {
					newPeers[uuid] = decodedMsg.PeerIps[index]
				}

				for uuid, ip := range s.peers {
					if _, ok := newPeers[uuid]; !ok {
						if uuid == s.id {
							s.logger.Println("\t\t own id not found", state, s.leaderID, ".")
						}
						s.logger.Println("removing node", ip, uuid)
					}
				}
				s.peers = newPeers
				s.mu.Unlock()

			}

			break
		}

	}
}
