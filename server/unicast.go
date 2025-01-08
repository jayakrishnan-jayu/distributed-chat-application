package server

import (
	"dummy-rom/server/message"
	"time"
)

func (s *Server) StartUniCastListener() {
	go s.startUnicastMessageListener()
	s.ru.StartListener()
}

func (s *Server) startUnicastMessageListener() {
	for {
		select {
		case <-s.quit:
			s.logger.Println("Quiting Unicast message listener")
			return
		case msg := <-s.ruMsgChan:
			unicastMessage, err := message.Decode(msg.Message)
			if err != nil {
				s.logger.Panic(err)
			}
			switch unicastMessage.Type {
			case message.ConnectToLeader:
				// when a new node listens to broadcast and sends a messsage
				// to connect
				// add node to peers

				s.mu.Lock()
				s.logger.Println("got connectToLeader is leader: ", s.state == LEADER)
				s.peers[msg.UUID] = msg.IP
				peerIds := make([]string, 0, len(s.peers))
				peerIps := make([]string, 0, len(s.peers))
				id := s.id
				// ownIP := s.ip
				for uuid, ip := range s.peers {
					peerIds = append(peerIds, uuid)
					peerIps = append(peerIps, ip)
				}
				s.mu.Unlock()

				encodedMessage := message.NewPeerInfoMessage(peerIds, peerIps)
				// send active peers
				go func() {
					// send active members to new node so that new node becomes a follower,
					// or starts an election
					send := s.ru.SendMessage(msg.IP, id, encodedMessage)
					if !send {
						s.logger.Println("failed to send peer info to", msg.IP)
						s.handleDeadServer(msg.UUID, msg.IP)
						return
					}
					// send existing nodes, the new node list
					send = s.rm.SendMessage(id, encodedMessage)
					if !send {
						s.logger.Fatal("Failed to send multicast")
					}

				}()
				break
				// for _, peerIp := range peerIps {
				// 	if peerIp == ownIP {
				// 		continue
				// 	}
				// 	go func(ip string) {
				// 		send := s.ru.SendMessage(ip, id, encodedMessage)
				// 		if !send {
				// 			s.logger.Println("faild to send peer info to", ip)
				// 			s.handleDeadServer(msg.uuid, ip)
				// 			return
				// 		}
				// 	}(peerIp)
				// 	break
				// }

			case message.PeerInfo:
				// message from leader
				s.logger.Println("got peer info from", msg.IP)
				if len(unicastMessage.PeerIds) != len(unicastMessage.PeerIps) {
					s.logger.Fatal("message.peerinfo message invalid length")
				}

				s.mu.Lock()
				id := s.id
				higherNodeExists := false
				for index, uuid := range unicastMessage.PeerIds {
					s.peers[uuid] = unicastMessage.PeerIps[index]
					if uuid != msg.UUID && uuid > msg.UUID {
						higherNodeExists = true
					}
				}
				s.mu.Unlock()

				if id > msg.UUID || higherNodeExists {
					// s.StopBroadcasting()
					s.broadcaster.Stop()
					s.logger.Println("election 3")
					go s.StartElection()
					break
				}
				s.becomeFollower(msg.UUID)
				break

			case message.Election:
				s.logger.Println("Got election message from", msg.IP)
				s.mu.Lock()
				id := s.id
				s.mu.Unlock()

				if s.id > msg.UUID {
					// send alive
					s.logger.Println("sending alive")
					encodedData := message.NewElectionAliveMessage()
					send := s.ru.SendMessage(msg.IP, id, encodedData)
					if !send {
						s.handleDeadServer(msg.UUID, msg.IP)
						s.logger.Println("failed to send alive message to", msg.IP)
					}
					s.logger.Println("election 2")
					go s.StartElection()
					break
				}
				// if node is higher, then stop broadcasting
				// s.StopBroadcasting()
				s.broadcaster.Stop()
				s.mu.Lock()
				s.state = ELECTION
				s.mu.Unlock()
				// wait 2 seconds, then check if we are a follower, if we are not,
				// start the election again
				go func() {
					time.Sleep(2 * time.Second)
					s.mu.Lock()
					if s.state != FOLLOWER {
						s.logger.Println("election 1")
						go s.StartElection()
					}
					s.mu.Unlock()
				}()
				break
			case message.ElectionAlive:
				s.logger.Printf("got election alive message from %s, waitng for victory message", msg.IP)
				go func() {
					time.Sleep(2)
					s.mu.Lock()
					if s.state == ELECTION {
						s.discoveredPeers[msg.UUID] = true
					}
					s.mu.Unlock()
				}()
				break

			case message.ElectionVictory:
				s.logger.Println("got election victory from", msg.IP, msg.UUID)
				s.logger.Println("new leader", msg.UUID)
				s.becomeFollower(msg.UUID)
				// s.StopBroadcasting()
				s.broadcaster.Stop()
				break
			case message.Heartbeat:
				s.mu.Lock()
				s.leaderHeartbeatTimestamp = time.Now()
				s.mu.Unlock()
				break
			}
		}
	}
}
