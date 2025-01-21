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
const CLIENT_PORT = ":5006"

type ServerInterface interface {
	StartToInit(*StateMachineMessage)
	InitToBecomeFollower(*StateMachineMessage)
	InitToLeader(*StateMachineMessage)
	BecomeFollowerToInit(*StateMachineMessage)
	BecomeFollowerToFollower(*StateMachineMessage)
	LeaderToInit(*StateMachineMessage)
	FollowerToElection(*StateMachineMessage)
	FollowerToFollower(*StateMachineMessage)
	BecomeLeaderToLeader(*StateMachineMessage)
	ElectionToFollower(*StateMachineMessage)
	ElectionToBecomeLeader(*StateMachineMessage)
	ElectionToElection(*StateMachineMessage)
}

type Server struct {
	id       string
	ip       string
	leaderID string

	sm            *StateMachine
	state         State
	stateListener StateListener
	stateChan     chan State
	logger        *log.Logger

	hbSConn net.PacketConn
	hbLConn net.PacketConn

	ru        *unicast.ReliableUnicast
	ruMsgChan chan *unicast.Message
	uMsgChan  chan *unicast.Message

	rm        *multicast.ReliableMulticast
	rmMsgChan chan *multicast.Message
	rmPort    uint32

	broadcaster *broadcast.Broadcaster
	bMsgChan    chan *broadcast.Message

	applicationMsgChan chan string
	clientMsgChan      chan string
	clientServer       *ClientServer

	peers       map[string]string
	electionMap map[string]bool
	quit        chan interface{}
	mu          sync.Mutex
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

	s.applicationMsgChan = make(chan string, 10)
	s.clientMsgChan = make(chan string, 10)

	s.peers = make(map[string]string)
	s.quit = make(chan interface{})

	s.peers[s.id] = s.ip

	s.stateListener = NewStateListener(START, s)
	s.clientServer = NewClientServer(s.id, s.clientMsgChan, s.applicationMsgChan, s, s.getRandomPeerID)
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
	s.clientServer.Start()
	s.sm.ChangeTo(INIT, nil)
}

func (s *Server) StartToInit(_ *StateMachineMessage) {
	s.stateChan <- INIT
	go s.ru.StartListener()
	go s.rm.StartListener()
	go s.startUnicastMessageListener()
	go s.StartMulticastListener()

	// // randomDuration := time.Duration(rand.IntN(4)+1) * time.Second
	randomDuration := time.Duration(rand.IntN(6)+2) * time.Second
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
	s.Debug()
	s.mu.Unlock()
}

func (s *Server) LeaderToInit(msg *StateMachineMessage) {
	s.stateChan <- INIT
	s.sm.ChangeTo(BECOME_FOLLOWER, &StateMachineMessage{Broadcast: msg.Broadcast})
}

func (s *Server) FollowerToElection(msg *StateMachineMessage) {
	s.stateChan <- ELECTION
}
func (s *Server) FollowerToFollower(msg *StateMachineMessage) {
	s.stateChan <- FOLLOWER
}
func (s *Server) BecomeLeaderToLeader(msg *StateMachineMessage) {
	s.stateChan <- LEADER
}
func (s *Server) ElectionToFollower(msg *StateMachineMessage) {
	s.stateChan <- FOLLOWER
}
func (s *Server) ElectionToBecomeLeader(msg *StateMachineMessage) {
	s.stateChan <- BECOME_LEADER
	s.rm.SendMessage(message.NewElectionVictoryMessage())
	s.sm.ChangeTo(LEADER, nil)

}
func (s *Server) ElectionToElection(msg *StateMachineMessage) {
	s.stateChan <- ELECTION
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
	s.peers = make(map[string]string, len(msg.PeerIds))
	for index, peerId := range msg.PeerIds {
		s.peers[peerId] = msg.PeerIps[index]
	}

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
	s.Debug()
}

func (s *Server) connectToLeaderHandler(fromUUID string, fromIP string) {
	s.mu.Lock()
	s.peers[fromUUID] = fromIP
	peerIds := make([]string, 0, len(s.peers))
	peerIps := make([]string, 0, len(s.peers))
	clocks := make([]uint32, 0, len(s.peers))
	for uuid, ip := range s.peers {
		peerIds = append(peerIds, uuid)
		peerIps = append(peerIps, ip)
		clocks = append(clocks, 0)
	}
	leaderID := s.id
	s.mu.Unlock()
	randomPort, _ := getRandomUDPPort()
	peerInfoData := message.NewPeerInfoMessage(peerIds, peerIps, randomPort, clocks)
	send := s.ru.SendMessage(fromIP, fromUUID, peerInfoData)
	if !send {
		s.logger.Println("failed to send peer info to new node", fromIP)
		s.mu.Lock()
		delete(s.peers, fromUUID)
		s.mu.Unlock()
		return
	}

	msChangeData := message.NewMulticastSessionChangeMessage(peerIds, peerIps, clocks, randomPort)
	newMulticastSession := multicast.NewReliableMulticast(leaderID, randomPort, peerIds, s.rmMsgChan)
	for index, uuid := range peerIds {
		if uuid == leaderID || uuid == fromUUID {
			continue
		}
		go s.ru.SendMessage(peerIps[index], uuid, msChangeData)
	}
	s.rm.Shutdown()
	s.rm = newMulticastSession
	s.rmPort = randomPort
	go s.rm.StartListener()

	s.logger.Println("sending message to", fromUUID)
}

func (s *Server) handleDeadServer(uuid string) {
	s.mu.Lock()
	ip, ok := s.peers[uuid]
	if ok {
		s.logger.Println("handling dead node", uuid, ip)
		delete(s.peers, uuid)
		s.rm.SendMessage(message.NewDeadNodeMessage(uuid, ip))
	}
	s.mu.Unlock()
}

func (s *Server) getRandomPeerID() string {
	s.mu.Lock()
	n := rand.IntN(len(s.peers))
	i := 0
	for _, ip := range s.peers {
		if i == n {
			s.mu.Unlock()
			return ip
		}
		i++
	}
	s.mu.Unlock()
	return ""
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
	if rand.IntN(10) < 7 {
		s.logger.Println("Follower will not be killed")
		return
	}
	s.logger.Println("Follower will be killed")
	go func() {
		count := 0
		time.Sleep(duration)
		s.mu.Lock()
		// ip := s.ip
		state := s.state
		s.mu.Unlock()
		for {
			if state == FOLLOWER {
				count++
				// s.rm.SendMessage(message.NewApplicationMessage([]byte(fmt.Sprintf("%s: %d", ip, count))))
				s.logger.Fatal("Follower killed")
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
	s.clientServer.Shutdown()
	s.rm.Shutdown()
	s.ru.Shutdown()
	s.mu.Unlock()
}

func getRandomUDPPort() (uint32, error) {
	for i := 0; i < 20; i++ {
		port := rand.IntN(65535-5010+1) + 5010
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
