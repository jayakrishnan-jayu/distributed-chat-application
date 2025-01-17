package server

import (
	"dummy-rom/server/broadcast"
	"dummy-rom/server/message"
	"dummy-rom/server/multicast"
	"dummy-rom/server/unicast"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const UNI_S_PORT = ":5002"
const UNI_L_PORT = ":5003"

type ServerInterface interface {
	StartToInit(*StateMachineMessage)
	InitToBecomeFollower(*StateMachineMessage)
	InitToLeader(*StateMachineMessage)
	BecomeFollowerToInit(*StateMachineMessage)
	BecomeFollowerToFollower(*StateMachineMessage)
	LeaderToInit(*StateMachineMessage)
	LeaderToFollower(*StateMachineMessage)
	FollowerToBecomeLeader(*StateMachineMessage)
	FollowerToElection(*StateMachineMessage)
	FollowerToFollower(*StateMachineMessage)
	BecomeLeaderToLeader(*StateMachineMessage)
	BecomeLeaderToFollower(*StateMachineMessage)
	ElectionToFollower(*StateMachineMessage)
	ElectionToBecomeLeader(*StateMachineMessage)
	ElectionToElection(*StateMachineMessage)
}

type Server struct {
	id       string
	ip       string
	leaderID string

	leaderHeartbeatTimestamp time.Time
	peerHeartbeatTimestamp   map[string]time.Time

	state State

	sm              *StateMachine
	stateListener   StateListener
	stateChan       chan State
	stateChangeChan chan interface{}
	logger          *log.Logger

	hbSConn net.PacketConn
	hbLConn net.PacketConn

	leaderQuitChan chan interface{}

	ru        *unicast.ReliableUnicast
	ruMsgChan chan *unicast.Message
	uMsgChan  chan *unicast.Message

	rm        *multicast.ReliableMulticast
	rmMsgChan chan *multicast.Message
	rmPort    uint32

	broadcaster *broadcast.Broadcaster
	bMsgChan    chan *broadcast.Message

	peers           map[string]string
	discoveredPeers map[string]bool
	quit            chan interface{}
	newServer       chan string
	mu              sync.Mutex
}

func NewServer() (*Server, error) {
	id := uuid.New()
	ip, err := LocalIP()
	if err != nil {
		return &Server{}, err
	}
	broadcastIp, err := BroadcastIP(ip)
	if err != nil {
		return &Server{}, err
	}

	ruMsgChan := make(chan *unicast.Message, 5)
	uMsgChan := make(chan *unicast.Message)
	rmMsgChan := make(chan *multicast.Message, 5)
	bMsgChan := make(chan *broadcast.Message, 5)

	hbSConn, err := net.ListenPacket("udp4", HRT_S_PORT)
	if err != nil {
		log.Panic(err)
	}

	hbLConn, err := net.ListenPacket("udp4", HRT_L_PORT)
	if err != nil {
		log.Panic(err)
	}

	uniSenderConn, err := net.ListenPacket("udp4", UNI_S_PORT)
	if err != nil {
		log.Panic(err)
	}

	uniListnerConn, err := net.ListenPacket("udp4", UNI_L_PORT)
	if err != nil {
		log.Panic(err)
	}
	randomPort, err := getRandomUDPPort()
	if err != nil {
		log.Panic(err)
	}
	peers := make([]string, 1)
	peers[0] = id.String()

	s := new(Server)
	s.state = INIT
	s.sm = NewStateMachine(s)
	s.stateChan = make(chan State)
	s.stateChangeChan = make(chan interface{})

	s.hbSConn = hbSConn
	s.hbLConn = hbLConn

	s.id = id.String()
	s.ip = strings.Split(ip.String(), "/")[0]
	s.leaderID = ""
	s.logger = log.New(os.Stdout, fmt.Sprintf("[%s][%s] ", s.ip, s.id[:4]), log.Ltime)

	s.ruMsgChan = ruMsgChan
	s.uMsgChan = uMsgChan
	s.ru = unicast.NewReliableUnicast(ruMsgChan, s.id, uniSenderConn, uniListnerConn)

	s.rmMsgChan = rmMsgChan
	s.rm = multicast.NewReliableMulticast(s.id, randomPort, peers, rmMsgChan)
	s.rmPort = randomPort

	s.bMsgChan = bMsgChan
	s.broadcaster = broadcast.NewBroadcaster(s.id, s.ip, broadcastIp.String(), bMsgChan)

	s.peers = make(map[string]string)
	s.discoveredPeers = make(map[string]bool)
	s.newServer = make(chan string, 5)
	s.quit = make(chan interface{})

	s.peers[s.id] = s.ip

	s.stateListener = NewStateListener(START, s)
	s.logger.Println("Current ID: ", s.id)

	return s, nil
}

func (s *Server) Run() {
	go func() {
		for {
			select {
			case <-s.quit:
				s.logger.Println("Quitting main loop")
				return
			case newState := <-s.stateChan:
				s.stateListener.Stop()
				s.mu.Lock()
				s.state = newState
				s.stateListener = NewStateListener(newState, s)
				s.mu.Unlock()
				s.stateListener.Start()
			}
		}
	}()
	s.sm.ChangeTo(INIT, nil)
}

func (s *Server) StartToInit(_ *StateMachineMessage) {
	s.stateChan <- INIT
	go s.ru.StartListener()
	go s.broadcaster.StartListener()
	go s.startUnicastMessageListener()
	go s.rm.StartListener()
	go s.StartMulticastMessageListener()
	//
	// // randomDuration := time.Duration(rand.IntN(4)+1) * time.Second
	randomDuration := time.Duration(rand.IntN(4)+2) * time.Second
	// randomDuration := time.Duration(0 * time.Second)
	ok, msg := s.broadcaster.IsAnyOneBroadcasting(randomDuration)
	emptyChannel(s.bMsgChan)
	//
	// // if no on is broadcasting, assume leader role
	if !ok {
		s.logger.Println("no one is broadcasting, assuming leader role")

		s.sm.ChangeTo(LEADER, nil)
		return
	}
	// // else try to become a follower of the broadcasting node
	s.logger.Println("startToInit going to become follower")
	s.sm.ChangeTo(BECOME_FOLLOWER, &StateMachineMessage{Broadcast: msg})
}

func (s *Server) InitToBecomeFollower(msg *StateMachineMessage) {
	s.stateChan <- BECOME_FOLLOWER
	//
	data := message.NewConnectToLeaderMessage()
	send := s.ru.SendMessage(msg.Broadcast.IP, msg.Broadcast.UUID, data)
	if !send {
		s.logger.Println("failed to connect to leader")
		s.sm.ChangeTo(INIT, nil)
		return
	}
	ok, peerInfo := s.peerInfoFromLeader(msg.Broadcast.UUID, 2*time.Second)
	if !ok {
		s.logger.Println("no peer info from leader")
		s.sm.ChangeTo(INIT, nil)
		return
	}
	s.mu.Lock()
	s.leaderID = msg.Broadcast.UUID
	s.addPeersFromPeerInfo(peerInfo)
	s.mu.Unlock()
	s.sm.ChangeTo(FOLLOWER, &StateMachineMessage{Unicast: peerInfo})
}

func (s *Server) InitToLeader(_ *StateMachineMessage) {
	s.stateChan <- LEADER
	// go func() {
	// 	ownID := s.id
	// 	for {
	// 		msg := s.broadcaster.StartIsAnotherLeaderBroadcasting(ownID)
	// 		s.mu.Lock()
	// 		state := s.state
	// 		s.mu.Unlock()
	// 		if msg == nil || state != LEADER {
	// 			return
	// 		}
	// 		if msg.UUID > ownID {
	// 			s.broadcaster.StopIsAnotherLeaderBroadcasting()
	// 			s.logger.Println("another leader is broadcasting")
	// 			s.sm.ChangeTo(INIT, nil)
	// 			return
	// 		}
	//
	// 	}
	// }()

}
func (s *Server) BecomeFollowerToInit(_ *StateMachineMessage) {
	s.stateChan <- INIT
	randomDuration := time.Duration(rand.IntN(4)+2) * time.Second
	// // randomDuration := time.Duration(1 * time.Second)
	ok, msg := s.broadcaster.IsAnyOneBroadcasting(randomDuration)
	emptyChannel(s.bMsgChan)
	//
	// // if no on is broadcasting, assume leader role
	if !ok {
		s.logger.Println("no one is broadcasting, assuming leader role")

		s.sm.ChangeTo(LEADER, nil)
		return
	}
	// // else try to become a follower of the broadcasting node
	s.logger.Println("startToInit going to become follower")
	s.sm.ChangeTo(BECOME_FOLLOWER, &StateMachineMessage{Broadcast: msg})

}
func (s *Server) BecomeFollowerToFollower(msg *StateMachineMessage) {
	s.stateChan <- FOLLOWER
	s.mu.Lock()
	s.joinMulticastFromPeerInfo(msg.Unicast)
	// isHighestID := s.isHighestID()
	s.mu.Unlock()
	//
	// if isHighestID {
	// 	s.logger.Println("HIGHEST ID BECOMING LEADER")
	// 	s.sm.ChangeTo(BECOME_LEADER, nil)
	// 	return
	// }
	// s.logger.Println("not highest id")
	// s.startLeaderHeartbeatListner(msg.Unicast.FromUUID)
}

func (s *Server) LeaderToInit(msg *StateMachineMessage) {
	s.stateChan <- INIT
	s.sm.ChangeTo(BECOME_FOLLOWER, &StateMachineMessage{Broadcast: msg.Broadcast})
}

func (s *Server) LeaderToFollower(msg *StateMachineMessage) {
	// s.broadcaster.Stop()
	// s.stateChan <- FOLLOWER
	// s.mu.Lock()
	// s.leaderID = msg.Unicast.FromUUID
	// s.mu.Unlock()
}

func (s *Server) FollowerToBecomeLeader(msg *StateMachineMessage) {
	// s.stateChan <- BECOME_LEADER
}

func (s *Server) FollowerToElection(msg *StateMachineMessage) {
	// s.stateChan <- ELECTION
	// s.mu.Lock()
	// s.leaderID = ""
	// highestID := true
	// peers := make(map[string]string, len(s.peers))
	// for uuid, ip := range s.peers {
	// 	if uuid > s.id {
	// 		highestID = false
	// 	}
	// 	peers[uuid] = ip
	// }
	// s.mu.Unlock()
	//
	// if highestID {
	// 	s.sm.ChangeTo(BECOME_LEADER, nil)
	// 	return
	// }
	//
	// var wg sync.WaitGroup
	//
	// wg.Add(1)
	// go func(peers map[string]string) {
	// 	for uuid, ip := range peers {
	// 		if uuid > s.id {
	// 			highestID = false
	// 			wg.Add(1)
	// 			go func(uuid string, ip string) {
	// 				s.logger.Println("sending election message to", ip)
	// 				encodedData := message.NewElectionMessage()
	// 				send := s.ru.SendMessage(ip, uuid, encodedData)
	// 				if !send {
	// 					s.logger.Println("failed to send election message to", ip)
	// 				}
	// 				wg.Done()
	// 			}(uuid, ip)
	// 		}
	// 	}
	// 	wg.Done()
	// }(peers)
	//
	// wg.Wait()
	// // potential issue
	// time.Sleep(300 * time.Millisecond)
	// s.sm.ChangeTo(BECOME_LEADER, nil)
}
func (s *Server) FollowerToFollower(msg *StateMachineMessage) {

}
func (s *Server) BecomeLeaderToLeader(msg *StateMachineMessage) {
	// s.stateChan <- LEADER
}
func (s *Server) BecomeLeaderToFollower(msg *StateMachineMessage) {

}
func (s *Server) ElectionToFollower(msg *StateMachineMessage) {

}
func (s *Server) ElectionToBecomeLeader(msg *StateMachineMessage) {
	// s.stateChan <- BECOME_LEADER

}
func (s *Server) ElectionToElection(msg *StateMachineMessage) {

}

func (s *Server) Debug() {
	s.logger.Printf("\nUUID: %s\nIP: %s\nPeers: %v\nState: %v\nLeaderID: %s\nMulticast Port: %d", s.id, s.ip, s.peers, s.state, s.leaderID, s.rmPort)
	s.rm.Debug()
}

// requires s.mu to be locked
func (s *Server) addPeersFromPeerInfo(umsg *unicast.Message) {
	msg, _ := message.Decode(umsg.Message)
	s.peers = make(map[string]string)
	for index, uuid := range msg.PeerIds {
		s.peers[uuid] = msg.PeerIps[index]
	}
}

// requires s.mu to be locked
func (s *Server) isHighestID() bool {
	for uuid, _ := range s.peers {
		if uuid == s.id {
			continue
		}
		if uuid > s.id {
			return false
		}
	}
	return true
}

// requires s.mu to be locked
func (s *Server) joinMulticastFromPeerInfo(umsg *unicast.Message) {
	msg, _ := message.Decode(umsg.Message)
	if s.rmPort != msg.MulticastPort {
		if s.rm != nil {
			s.rm.Shutdown()
		}

		if s.rmPort != msg.MulticastPort {
			s.logger.Println("joinMulticastFromPeerInfo", msg.Clock)
			s.rmPort = msg.MulticastPort
			s.rm = multicast.NewReliableMulticast(s.id, s.rmPort, msg.PeerIds, s.rmMsgChan)
			go s.rm.StartListener()
			return
		}
		s.logger.Println("joinMulticastFromPeerInfo", false, s.rmPort, msg.MulticastPort)
	}
	//port is same, then update the vector clock
}

func (s *Server) connectToLeaderHandler(fromUUID string, fromIP string) {
	s.rm.AddNewNode(fromUUID)
	// vc := s.rm.VectorClock()
	s.mu.Lock()
	s.peers[fromUUID] = fromIP
	peerIds := make([]string, 0, len(s.peers))
	peerIps := make([]string, 0, len(s.peers))
	clocks := make([]uint32, 0, len(s.peers))
	for uuid, ip := range s.peers {
		// clock, ok := vc[uuid]
		// if !ok {
		// 	s.logger.Panicf("failed to find frame count for uuid in vc : %s", uuid, s.state)
		// }
		peerIds = append(peerIds, uuid)
		peerIps = append(peerIps, ip)
		clocks = append(clocks, 0)
	}
	rmPort := s.rmPort
	s.mu.Unlock()

	encodedMessage := message.NewPeerInfoMessage(peerIds, peerIps, rmPort, clocks)
	s.logger.Println("sending message to", fromUUID)
	send := s.ru.SendMessage(fromIP, fromUUID, encodedMessage)
	if !send {
		s.logger.Println("failed to send peer info to new node", fromIP)
		s.mu.Lock()
		delete(s.peers, fromUUID)
		s.mu.Unlock()
		return
	}
	go func(fromUUID string, fromIP string) {
		time.Sleep(1000 * time.Millisecond)
		s.logger.Println("sending multicast message", fromUUID)
		s.rm.SendMessage(message.NewNewNodeMessage(fromUUID, fromIP))
	}(fromUUID, fromIP)
}

// func (s *Server) StartElection() {
// 	// TODO
// 	// bully algorithm
// 	for {
//
// 		s.logger.Println("Starting election")
// 		s.mu.Lock()
// 		s.state = ELECTION
// 		peers := make(map[string]string, len(s.peers))
// 		for uuid, ip := range s.peers {
// 			peers[uuid] = ip
// 		}
// 		id := s.id
// 		s.discoveredPeers = make(map[string]bool)
// 		s.mu.Unlock()
// 		highestID := true
//
// 		potentialAliveLeader := false
// 		var mutex sync.Mutex
//
// 		for uuid, ip := range peers {
// 			if uuid > id || uuid == id {
// 				continue
// 			}
// 			highestID = false
// 			// send election message
// 			// wait unitl a specified time for
// 			go func(uuid string, ip string) {
// 				s.logger.Println("sending election message to", ip)
// 				encodedData := message.NewElectionMessage()
// 				send := s.ru.SendMessage(ip, uuid, encodedData)
// 				if !send {
// 					s.logger.Println("failed to send election message to", ip)
// 					s.handleDeadServer(uuid, ip)
// 				} else {
// 					mutex.Lock()
// 					potentialAliveLeader = true
// 					mutex.Unlock()
// 				}
//
// 			}(uuid, ip)
//
// 		}
//
// 		if highestID {
// 			// send out victory message
// 			s.logger.Println("sending victory messages")
// 			go s.SendElectionVictoryAndBecomeLeader()
// 			return
// 		}
//
// 		if potentialAliveLeader {
// 			// when election message was send to higher node, and a
// 			// higer node responds back, wait for 1 second for ElectionVictory
// 			// otherwise start the election again
// 			time.Sleep(1)
// 			s.mu.Lock()
// 			if s.state != FOLLOWER {
// 				s.mu.Unlock()
// 				// restart the election
// 				s.logger.Println("restarting election")
// 				continue
// 			}
// 			s.mu.Unlock()
// 			return
// 		}
//
// 		s.logger.Println("no potential leader, sending victory messages")
// 		go s.SendElectionVictoryAndBecomeLeader()
// 		return
// 	}
// 	// check if we got response from
// }

// func (s *Server) SendElectionVictoryAndBecomeLeader() {
// 	var wg sync.WaitGroup
// 	peers := make(map[string]string, len(s.peers))
// 	s.mu.Lock()
// 	for uuid, ip := range s.peers {
// 		peers[uuid] = ip
// 	}
// 	ownID := s.id
// 	s.mu.Unlock()
//
// 	wg.Add(len(peers))
//
// 	for uuid, ip := range peers {
// 		if uuid == ownID {
// 			continue
// 		}
// 		go func(uuid string, ip string) {
// 			defer wg.Done()
// 			victoryMessage := message.NewElectionVictoryMessage()
// 			send := s.ru.SendMessage(ip, uuid, victoryMessage)
// 			if !send {
// 				// TODO: handle node failure
// 				s.handleDeadServer(uuid, ip)
// 				log.Println("failed to send victory message to", ip)
// 			}
//
// 		}(uuid, ip)
// 	}
// 	// wait until all vicotry messages are send and ack is received
// 	s.becomeLeader()
// 	wg.Wait()
// 	s.logger.Println("Victory messages sent")
//
// }

// func (s *Server) becomeLeader() {
// 	go s.broadcaster.Start()
// 	s.mu.Lock()
// 	s.logger.Println("LEADER")
// 	s.state = LEADER
// 	s.leaderID = s.id
// 	s.mu.Unlock()
// 	// start heartbeating
// 	go func(interval time.Duration, quit chan interface{}) {
// 		t := time.NewTimer(interval)
// 		defer t.Stop()
//
// 		for {
// 			select {
// 			case <-quit:
// 				s.logger.Println("stopping heartbeat via quit channel")
// 				return
// 			case <-t.C:
// 				s.mu.Lock()
// 				if s.state != LEADER {
// 					s.mu.Unlock()
// 					s.logger.Println("stopping heartbeat as state is not LEADER")
// 					return
// 				}
// 				s.mu.Unlock()
// 				s.sendHearbeat()
// 				t.Reset(interval)
// 			}
// 		}
// 	}(250*time.Millisecond, s.quit)
// }

// func (s *Server) sendHearbeat() {
// 	hearbeat := message.NewHeartbeatMessage()
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
// 	id := s.id
// 	for uuid, ip := range s.peers {
// 		if id == uuid {
// 			continue
// 		}
// 		go func() {
// 			send := s.ru.SendMessage(ip, uuid, hearbeat)
// 			if !send {
// 				s.handleDeadServer(uuid, ip)
// 			}
// 		}()
// 	}
// }

// func (s *Server) becomeFollower(leaderID string, multicastPort uint32, peerIds []string) {
// 	s.logger.Println("become follower to", leaderID)
// 	s.mu.Lock()
// 	s.state = FOLLOWER
// 	oldLeaderID := s.leaderID
// 	s.leaderID = leaderID
// 	s.logger.Println(s.rmPort, multicastPort)
// 	if s.rmPort != multicastPort {
// 		if s.rm != nil {
// 			s.rm.Shutdown()
// 		}
// 		s.rmPort = multicastPort
// 		s.rm = multicast.NewReliableMulticast(s.id, s.rmPort, peerIds, s.rmMsgChan)
// 		go s.rm.StartListener()
// 	}
// 	s.mu.Unlock()
//
// 	if oldLeaderID != leaderID {
// 		go func(interval time.Duration) {
// 			t := time.NewTimer(interval)
// 			defer t.Stop()
//
// 			for {
// 				s.mu.Lock()
// 				if s.state != FOLLOWER {
// 					s.mu.Unlock()
// 					s.logger.Println("stopping hearbeat listener")
// 					return
// 				}
// 				s.mu.Unlock()
// 				<-t.C
// 				s.mu.Lock()
// 				diff := time.Now().Sub(s.leaderHeartbeatTimestamp)
// 				newLeader := s.leaderID != leaderID
// 				s.mu.Unlock()
// 				if newLeader {
// 					return
// 				}
// 				if diff > interval {
// 					s.logger.Println("election 4")
// 					go s.StartElection()
// 					return
// 				}
// 				t.Reset(interval)
// 			}
// 		}(time.Duration(rand.IntN(250)+250) * time.Millisecond)
// 	}
// }

func (s *Server) handleDeadServer(uuid string) {
	s.mu.Lock()
	ip, ok := s.peers[uuid]
	if ok {
		s.logger.Println("handling dead node", uuid, ip)
		delete(s.peers, uuid)
		delete(s.discoveredPeers, uuid)

		// s.mu.Unlock()
		// if s.rm != nil {
		// s.rm.HandleDeadNode(uuid)
		// s.rm.RemoveDeadNode(uuid)
		// s.rm.SendMessage(message.NewDeadNodeMessage(uuid, ip))
		// }
		// 	return
	}
	s.mu.Unlock()
	// s.ru.HandleDeadNode(uuid)
	// s.multicastPeerInfo()
}

func (s *Server) KillLeaderAfter(duration time.Duration) {
	s.logger.Println("leader will be killed")
	go func() {
		time.Sleep(duration)
		s.mu.Lock()
		if s.state == LEADER {
			s.mu.Unlock()
			s.logger.Fatal("Leader killed")
			// s.logger.Printf("Leader killed")
			// os.Exit(0)
		}
		s.mu.Unlock()
	}()
}

func (s *Server) KillFollowerAfter(duration time.Duration) {
	if rand.IntN(10) < 6 {
		s.logger.Println("Follower will not be killed")
		return
	}
	s.logger.Println("Follower will be killed")
	go func() {
		count := 0
		time.Sleep(duration)
		s.mu.Lock()
		ip := s.ip
		state := s.state
		s.mu.Unlock()
		for {
			if state == FOLLOWER {
				count++
				s.rm.SendMessage(message.NewApplicationMessage([]byte(fmt.Sprintf("%s: %d", ip, count))))
				// s.logger.Fatal("Follower killed")
				// s.logger.Printf("Follower killed")
				// os.Exit(0)
			}
			time.Sleep(1 * time.Second)

		}
	}()
}
func (s *Server) Shutdown() {
	s.mu.Lock()
	close(s.quit)
	s.broadcaster.Shutdown()
	s.rm.Shutdown()
	s.ru.Shutdown()
	s.mu.Unlock()
}

func getRandomUDPPort() (uint32, error) {
	for i := 0; i < 20; i++ {
		port := rand.IntN(65535-49152+1) + 49152
		addr := fmt.Sprintf(":%d", port)
		conn, err := net.ListenPacket("udp4", addr)
		if err == nil {
			conn.Close()
			return uint32(port), nil
		}
	}
	return 0, errors.New("could not find a free port after multiple attempts")
}

func emptyChannel(ch interface{}) {
	switch v := ch.(type) {
	case chan *broadcast.Message:
		for len(v) > 0 {
			<-v // Discard values
		}
	case chan *unicast.Message:
		for len(v) > 0 {
			<-v // Discard values
		}
	case chan bool:
		for len(v) > 0 {
			<-v // Discard values
		}
	case chan interface{}:
		for len(v) > 0 {
			<-v // Discard values
		}
	// Add more cases for other channel types as needed
	default:
		log.Println("Unsupported channel type")
	}
}
