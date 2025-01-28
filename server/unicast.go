package server

import (
	"dummy-rom/server/message"
	"dummy-rom/server/multicast"
	"dummy-rom/server/unicast"
	"time"
)

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
				newMulticastSession := multicast.NewReliableMulticast(s.id, uMsg.MulticastPort, uMsg.PeerIds, uMsg.PeerIps, s.rmMsgChan, s.ru)
				s.rm.Shutdown()
				s.rm = newMulticastSession
				s.rmPort = uMsg.MulticastPort
				s.mu.Unlock()
				go s.rm.StartListener()
			}

			if uMsg.Type == message.MulticastAck {
				s.rm.OnAck(msg.FromUUID, uMsg.Frame)
			}

			if uMsg.Type == message.Election {
				continue
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

		}
	}
}
