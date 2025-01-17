package server

// func (s *Server) isAnyOneBroadcasting(duration time.Duration) (bool, *broadcast.Message) {
// 	t := time.NewTimer(duration)
// 	defer t.Stop()
// 	select {
// 	case <-t.C:
// 		return false, nil
// 	case msg := <-s.bMsgChan:
// 		return true, msg
// 	}
// }

// func (s *Server) startLeaderHeartbeatListner(leaderID string) {
// 	s.broadcaster.StartListener()
//
// 	go func(interval time.Duration) {
// 		t := time.NewTimer(interval)
// 		defer t.Stop()
//
// 		for {
// 			select {
// 			case <-t.C:
// 				s.mu.Lock()
// 				if s.state != FOLLOWER {
// 					s.mu.Unlock()
// 					s.logger.Panic("node is not a follower after time out heartbeating")
// 				}
// 				s.mu.Unlock()
// 				s.logger.Println("leader hearbeat timeout")
// 				s.sm.ChangeTo(ELECTION, nil)
// 			case msg := <-s.bMsgChan:
// 				if msg.UUID != leaderID {
// 					s.logger.Println("should not happen")
// 				}
// 				t.Reset(interval)
// 			}
// 			<-t.C
// 			s.mu.Lock()
// 			diff := time.Now().Sub(s.leaderHeartbeatTimestamp)
// 			newLeader := s.leaderID != leaderID
// 			s.mu.Unlock()
// 			if newLeader {
// 				return
// 			}
// 			if diff > interval {
// 				s.logger.Println("election 4")
// 				go s.StartElection()
// 				return
// 			}
// 			t.Reset(interval)
// 		}
// 	}(time.Duration(rand.IntN(250)+250) * time.Millisecond)
// }

// Broadcast is  used for leader discovery
// func (s *Server) startBroadcastMessageListener() {
// 	prevLeaderID := ""
// 	prevLeaderBroadcastCount := 0
// 	s.mu.Lock()
// 	id := s.id
// 	s.mu.Unlock()
//
// 	for {
// 		select {
// 		case <-s.quit:
// 			s.logger.Println("Quitting Broadcast message listner")
// 			return
// 		case msg := <-s.bMsgChan:
// 			clientUUIDStr := msg.UUID
// 			ip := msg.IP
// 			s.mu.Lock()
// 			_, ok := s.peers[clientUUIDStr]
// 			if !ok {
// 				s.peers[clientUUIDStr] = ip
// 			}
// 			// if two or more nodes assume leader position and start position,
// 			// and if the current node is of lower id, restart again
// 			if s.state == LEADER && clientUUIDStr > id {
// 				// TODO: call init method
// 				s.state = INIT
// 				s.leaderID = ""
// 				s.broadcaster.Stop()
// 			}
// 			state := s.state
// 			leaderID := s.leaderID
// 			s.mu.Unlock()
//
// 			if prevLeaderID != clientUUIDStr {
// 				prevLeaderID = clientUUIDStr
// 				prevLeaderBroadcastCount = 1
// 				break
// 			}
// 			if prevLeaderBroadcastCount < 10 {
// 				prevLeaderBroadcastCount += 1
// 				break
// 			}
//
// 			switch state {
// 			case INIT:
// 				s.mu.Lock()
// 				discovered, ok := s.discoveredPeers[clientUUIDStr]
// 				s.mu.Unlock()
//
// 				if ok && discovered {
// 					break
// 				}
// 				s.logger.Println("connecting to broadcaster", ip, ok, discovered)
// 				go s.connectToLeader(ip, clientUUIDStr, id)
// 				time.Sleep(1 * time.Second)
// 				s.mu.Lock()
// 				s.logger.Println("connecting to broadcaster state", s.state)
// 				s.mu.Unlock()
// 				// s.mu.Lock()
// 				// s.state = FOLLOWER
// 				// s.leaderID = clientUUIDStr
// 				// s.mu.Unlock()
// 				break
// 			case FOLLOWER:
// 				if leaderID == "" {
// 					s.logger.Panic("Invalid state: leaderID is empty while in FOLLOWER state")
// 					break
// 				}
// 				if leaderID != msg.UUID {
// 					s.logger.Println("recvd broadcast from non leader node", msg.UUID)
// 					s.logger.Println("leader id ", leaderID)
// 				}
// 				if clientUUIDStr == leaderID {
// 					// s.logger.Println("broadcast from leader", leaderID, ip)
// 					break
// 				}
// 				s.logger.Println("dont' know", state, clientUUIDStr, leaderID)
// 				break
// 			case ELECTION:
// 				s.logger.Println("broadcast while in election ")
// 				break
// 			case LEADER:
// 				s.logger.Println("broadcast while being leader", clientUUIDStr > id, clientUUIDStr)
//
// 				break
//
// 			}
//
// 		}
// 	}
// }
