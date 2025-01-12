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
				state := s.state
				s.logger.Println("got connectToLeader is leader: ", state == LEADER)
				if state != LEADER {
					s.mu.Unlock()
					s.logger.Println("got connect to leader while not being leader")
					return
				}
				s.peers[msg.FromUUID] = msg.IP
				s.mu.Unlock()
				s.rm.AddPeer(msg.FromUUID)

				// ownIP := s.ip
				encodedMessage := s.getPeerInfo()

				send := s.ru.SendMessage(msg.IP, msg.FromUUID, encodedMessage)

				if !send {
					s.logger.Println("failed to send peer info to", msg.IP)
					s.handleDeadServer(msg.FromUUID, msg.IP)
					return
				}

				send = s.rm.SendMessage(encodedMessage)
				if !send {
					s.logger.Println("failed to multicast peerinfo", msg.IP)
					s.handleDeadServer(msg.FromUUID, msg.IP)
					return
				}
				s.logger.Println("send peerinfo")
				break
				// for index, peerIp := range peerIps {
				// 	if peerIp == ownIP {
				// 		continue
				// 	}
				// 	go func(ip string, id string) {
				// 		send := s.ru.SendMessage(ip, id, encodedMessage)
				// 		if !send {
				// 			s.logger.Println("faild to send peer info to", ip)
				// 			s.handleDeadServer(id, ip)
				// 			return
				// 		}
				// 	}(peerIp, peerIds[index])
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
				s.peers = make(map[string]string)
				higherNodeExists := false
				for index, uuid := range unicastMessage.PeerIds {
					s.peers[uuid] = unicastMessage.PeerIps[index]
					if uuid != msg.FromUUID && uuid > msg.FromUUID {
						higherNodeExists = true
					}
				}
				if id > msg.FromUUID || higherNodeExists {
					if s.state == LEADER {
						s.broadcaster.Stop()
					}
					s.logger.Println("election 3")
					go s.StartElection()
					break
				}

				if s.state == FOLLOWER && s.leaderID > msg.FromUUID {
					s.mu.Unlock()
					break
				}
				if s.state != FOLLOWER {
					s.mu.Unlock()
					s.rm.AddPeers(unicastMessage.PeerIds, unicastMessage.Clock)
					s.becomeFollower(msg.FromUUID)
					break
				}
				s.logger.Println("not sure if this should happen")
				s.mu.Unlock()
				break

			case message.Election:
				s.logger.Println("Got election message from", msg.IP)

				if s.id > msg.FromUUID {
					// send alive
					s.logger.Println("sending alive")
					encodedData := message.NewElectionAliveMessage()
					send := s.ru.SendMessage(msg.IP, msg.FromUUID, encodedData)
					if !send {
						s.handleDeadServer(msg.FromUUID, msg.IP)
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
						s.discoveredPeers[msg.FromUUID] = true
					}
					s.mu.Unlock()
				}()
				break

			case message.ElectionVictory:
				s.logger.Println("got election victory from", msg.IP, msg.FromUUID)
				s.logger.Println("new leader", msg.FromUUID)
				s.becomeFollower(msg.FromUUID)
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
