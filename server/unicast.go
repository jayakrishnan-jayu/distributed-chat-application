package server

import (
	"dummy-rom/server/message"
	"dummy-rom/server/multicast"
	"dummy-rom/server/unicast"
	"time"
)

// func (s *Server) StartUniCastListener() {
// 	go s.startUnicastMessageListener()
// 	// s.ru.StartListener()
// }

func (s *Server) peerInfoFromLeader(leaderUUID string, duration time.Duration) (bool, *unicast.Message) {
	t := time.NewTimer(duration)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			return false, nil
		case msg := <-s.uMsgChan:
			unicastMessage, err := message.Decode(msg.Message)
			if err != nil {
				s.logger.Panic(err)
			}
			if unicastMessage.Type != message.PeerInfo || msg.FromUUID != leaderUUID {
				continue
			}
			return true, msg
		}
	}
}

func (s *Server) startUnicastMessageListener() {
	for {
		select {
		case <-s.quit:
			s.logger.Println("Quiting Unicast message listener")
			return
		case msg := <-s.ruMsgChan:
			uMsg, err := message.Decode(msg.Message)
			if err != nil {
				s.logger.Panic(err)
			}
			s.mu.Lock()
			state := s.state
			s.mu.Unlock()
			if uMsg.Type == message.MulticastSessionChange {
				s.logger.Println("connecting to new multicast", uMsg.MulticastPort, "in state", state)
				s.mu.Lock()
				s.logger.Println("connecting to new multicast", uMsg.MulticastPort)
				s.peers = make(map[string]string, len(uMsg.PeerIds))
				for index, id := range uMsg.PeerIds {
					s.peers[id] = uMsg.PeerIps[index]
				}
				newMulticastSession := multicast.NewReliableMulticast(s.id, uMsg.MulticastPort, uMsg.PeerIds, s.rmMsgChan)
				s.rm.Shutdown()
				s.rm = newMulticastSession
				s.rmPort = uMsg.MulticastPort
				s.mu.Unlock()
				go s.rm.StartListener()
			}

			switch state {
			case BECOME_FOLLOWER:
				if uMsg.Type == message.PeerInfo {
					s.uMsgChan <- msg
					break
				}
				break
			case FOLLOWER:
				if uMsg.Type == message.Election || uMsg.Type == message.ElectionVictory {
					s.uMsgChan <- msg
					break
				}

			case LEADER:
				if uMsg.Type == message.ConnectToLeader || uMsg.Type == message.ElectionVictory {
					s.uMsgChan <- msg
					break
				}
				break
			}

			// switch unicastMessage.Type {
			// case message.ConnectToLeader:
			// 	randomPort, err := getRandomUDPPort()
			// 	if err != nil {
			// 		s.logger.Fatal(err)
			// 	}
			// 	s.mu.Lock()
			//
			// 	state := s.state
			// 	s.logger.Println("got connectToLeader is leader: ", state == LEADER, msg.IP)
			// 	if state != LEADER {
			// 		s.mu.Unlock()
			// 		s.logger.Println("got connect to leader while not being leader")
			// 		return
			// 	}
			//
			// 	s.peers[msg.FromUUID] = msg.IP
			// 	peerIds := make([]string, 0, len(s.peers))
			// 	peerIps := make([]string, 0, len(s.peers))
			// 	for uuid, ip := range s.peers {
			// 		s.logger.Println("peers", uuid, ip)
			// 		peerIds = append(peerIds, uuid)
			// 		peerIps = append(peerIps, ip)
			// 	}
			// 	multicastServer := multicast.NewReliableMulticast(s.id, randomPort, peerIds, s.rmMsgChan)
			// 	if s.rm != nil {
			// 		s.logger.Println("shutting down old multicast")
			// 		s.rm.Shutdown()
			// 	}
			// 	s.rm = multicastServer
			// 	s.rmPort = randomPort
			// 	go s.rm.StartListener()
			// 	ownIP := s.ip
			//
			// 	// encodedMessage = s.getMulticastSessionChangeInfo()
			// 	s.mu.Unlock()
			//
			// 	encodedMessage := message.NewPeerInfoMessage(peerIds, peerIps, randomPort)
			// 	send := s.ru.SendMessage(msg.IP, msg.FromUUID, encodedMessage)
			// 	if !send {
			// 		s.logger.Println("failed to send peer info to new node", msg.IP)
			// 		s.handleDeadServer(msg.FromUUID, msg.IP)
			// 		return
			// 	}
			//
			// 	// send = s.rm.SendMessage(encodedMessage)
			// 	// if !send {
			// 	// 	s.logger.Println("failed to multicast peerinfo", msg.IP)
			// 	// 	s.handleDeadServer(msg.FromUUID, msg.IP)
			// 	// 	return
			// 	// }
			//
			// 	s.logger.Println("send peerinfo")
			// 	encodedMessage = message.NewMulticastSessionChangeMessage(peerIds, peerIps, randomPort)
			//
			// 	for index, peerIp := range peerIps {
			// 		s.logger.Println("should send to ", peerIp, peerIds[index])
			// 		if peerIp == ownIP || peerIp == msg.IP {
			// 			s.logger.Println("skipping", peerIp)
			// 			continue
			// 		}
			// 		go func(ip string, id string) {
			// 			s.logger.Println("sending multicast session change info to", ip, id, randomPort)
			// 			send := s.ru.SendMessage(ip, id, encodedMessage)
			// 			if !send {
			// 				s.logger.Println("faild to send peer info to", ip)
			// 				s.handleDeadServer(id, ip)
			// 				return
			// 			}
			// 		}(peerIp, peerIds[index])
			// 	}
			// 	break
			//
			// case message.PeerInfo:
			// 	// message from leader
			// 	s.logger.Println("got peer info from", msg.IP, unicastMessage.MulticastPort)
			// 	if len(unicastMessage.PeerIds) != len(unicastMessage.PeerIps) {
			// 		s.logger.Fatal("message.peerinfo message invalid length")
			// 	}
			// 	s.mu.Lock()
			//
			// 	id := s.id
			// 	s.peers = make(map[string]string)
			// 	higherNodeExists := false
			// 	for index, uuid := range unicastMessage.PeerIds {
			// 		s.peers[uuid] = unicastMessage.PeerIps[index]
			// 		// s.rm.AddPeer(uuid)
			// 		if uuid != msg.FromUUID && uuid > msg.FromUUID {
			// 			higherNodeExists = true
			// 		}
			// 	}
			// 	if id > msg.FromUUID || higherNodeExists {
			// 		if s.state == LEADER {
			// 			s.broadcaster.Stop()
			// 		}
			// 		s.logger.Println("election 3")
			// 		go s.StartElection()
			// 		break
			// 	}
			//
			// 	if s.state == FOLLOWER {
			// 		if s.leaderID > msg.FromUUID {
			// 			s.mu.Unlock()
			// 			break
			// 		}
			// 		if s.leaderID == msg.FromUUID {
			// 			s.mu.Unlock()
			// 			s.becomeFollower(msg.FromUUID, unicastMessage.MulticastPort, unicastMessage.PeerIds)
			// 			break
			// 		}
			// 	}
			// 	if s.state != FOLLOWER {
			// 		s.mu.Unlock()
			// 		// s.rm.AddPeers(unicastMessage.PeerIds, unicastMessage.Clock)
			// 		s.becomeFollower(msg.FromUUID, unicastMessage.MulticastPort, unicastMessage.PeerIds)
			// 		break
			// 	}
			// 	s.logger.Println("not sure if this should happen")
			// 	s.mu.Unlock()
			// 	break
			//
			// case message.MulticastSessionChange:
			// 	s.mu.Lock()
			// 	state := s.state
			// 	if state == LEADER || state == INIT {
			// 		s.mu.Unlock()
			// 		break
			// 	}
			// 	if state != FOLLOWER || msg.FromUUID != s.leaderID {
			// 		s.mu.Unlock()
			// 		break
			// 	}
			// 	if s.rmPort != unicastMessage.MulticastPort {
			// 		s.rmPort = unicastMessage.MulticastPort
			// 		if s.rm != nil {
			// 			s.rm.Shutdown()
			// 		}
			// 		s.rm = multicast.NewReliableMulticast(s.id, s.rmPort, unicastMessage.PeerIds, s.rmMsgChan)
			// 		go s.rm.StartListener()
			// 		s.logger.Println("multicast session changed to ", s.rmPort)
			// 	}
			// 	s.mu.Unlock()
			// 	break
			//
			// case message.Election:
			// 	s.logger.Println("Got election message from", msg.IP)
			//
			// 	if s.id > msg.FromUUID {
			// 		// send alive
			// 		s.logger.Println("sending alive")
			// 		encodedData := message.NewElectionAliveMessage()
			// 		send := s.ru.SendMessage(msg.IP, msg.FromUUID, encodedData)
			// 		if !send {
			// 			s.handleDeadServer(msg.FromUUID, msg.IP)
			// 			s.logger.Println("failed to send alive message to", msg.IP)
			// 		}
			// 		s.logger.Println("election 2")
			// 		go s.StartElection()
			// 		break
			// 	}
			// 	// if node is higher, then stop broadcasting
			// 	// s.StopBroadcasting()
			// 	s.broadcaster.Stop()
			// 	s.mu.Lock()
			// 	s.state = ELECTION
			// 	s.mu.Unlock()
			// 	// wait 2 seconds, then check if we are a follower, if we are not,
			// 	// start the election again
			// 	go func() {
			// 		time.Sleep(2 * time.Second)
			// 		s.mu.Lock()
			// 		if s.state != FOLLOWER {
			// 			s.logger.Println("election 1")
			// 			go s.StartElection()
			// 		}
			// 		s.mu.Unlock()
			// 	}()
			// 	break
			// case message.ElectionAlive:
			// 	s.logger.Printf("got election alive message from %s, waitng for victory message", msg.IP)
			// 	go func() {
			// 		time.Sleep(2)
			// 		s.mu.Lock()
			// 		if s.state == ELECTION {
			// 			s.discoveredPeers[msg.FromUUID] = true
			// 		}
			// 		s.mu.Unlock()
			// 	}()
			// 	break
			//
			// case message.ElectionVictory:
			// 	s.logger.Println("got election victory from", msg.IP, msg.FromUUID)
			// 	s.logger.Println("new leader", msg.FromUUID)
			// 	s.logger.Println("is new leader greater", msg.FromUUID > s.id)
			// 	s.becomeFollower(msg.FromUUID, unicastMessage.MulticastPort, unicastMessage.PeerIds)
			// 	// s.StopBroadcasting()
			// 	s.broadcaster.Stop()
			// 	break
			// case message.Heartbeat:
			// 	s.mu.Lock()
			// 	s.leaderHeartbeatTimestamp = time.Now()
			// 	s.mu.Unlock()
			// 	break
			// }
		}
	}
}
