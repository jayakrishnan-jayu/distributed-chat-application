package server

import (
	"dummy-rom/server/broadcast"
	"dummy-rom/server/message"
	"dummy-rom/server/multicast"
	"dummy-rom/server/unicast"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"
)

const ROM_S_PORT = ":5004"
const ROM_L_PORT = ":5005"

const MIN_SERVERS = 3

type State int

const (
	INIT State = iota
	LEADER
	FOLLOWER
	ELECTION
)

type Server struct {
	id       string
	ip       string
	leaderID string
	state    State
	logger   *log.Logger

	ru        *unicast.ReliableUnicast
	ruMsgChan chan *unicast.Message

	rm        *multicast.ReliableMulticast
	rmMsgChan chan *multicast.Message

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
	rmMsgChan := make(chan *multicast.Message, 5)
	bMsgChan := make(chan *broadcast.Message, 5)

	s := new(Server)
	s.state = INIT
	s.id = id.String()
	s.ip = strings.Split(ip.String(), "/")[0]
	s.leaderID = ""
	s.logger = log.New(os.Stdout, fmt.Sprintf("[%s][%s] ", s.ip, s.id[:4]), log.Ltime)

	s.ruMsgChan = ruMsgChan
	s.ru = unicast.NewReliableUnicast(ruMsgChan)

	s.rmMsgChan = rmMsgChan
	s.rm = multicast.NewReliableMulticast(rmMsgChan, "239.0.0.0", "9999")

	s.bMsgChan = bMsgChan
	s.broadcaster = broadcast.NewBroadcaster(s.id, s.ip, broadcastIp.String(), bMsgChan)

	s.peers = make(map[string]string)
	s.discoveredPeers = make(map[string]bool)
	s.newServer = make(chan string, 5)
	s.quit = make(chan interface{})

	s.peers[s.id] = s.ip

	s.logger.Println("Current ID: ", s.id)

	return s, nil
}

func (s *Server) Debug() {
	s.logger.Printf("\nUUID: %s\nIP: %s\nPeers: %v\nState: %v\nDisovered: %v", s.id, s.ip, s.peers, s.state, s.discoveredPeers)
}

func (s *Server) StartUniCastSender() {
	for {
		select {
		case clientUUIDStr := <-s.newServer:
			s.mu.Lock()
			ip := s.peers[clientUUIDStr]
			s.logger.Printf("sending message to %s from %s", ip, s.ip)
			s.ru.SendMessage(ip, s.id, []byte("Hello"))
			s.mu.Unlock()

		}
	}
}

func (s *Server) connectToLeader(leaderIP string, leaderID string, id string) {
	data := message.NewConnectToLeaderMessage()
	send := s.ru.SendMessage(leaderIP, id, data)
	if !send {
		s.logger.Println("Failed to connect to leader")

		// TODO: delete all buffered messages and frame counts in reliable unicast
		// TODO: figure out how to handle node failure and recovery
		s.handleDeadServer(leaderID, leaderIP)
		s.mu.Lock()
		s.leaderID = ""
		s.state = INIT
		s.mu.Unlock()

		// if the server node is unreachable, restart the init
		s.StartInit()
		return
	}
	s.mu.Lock()
	s.discoveredPeers[leaderID] = true
	s.mu.Unlock()
	s.logger.Println("connect message send", leaderIP)
}

// Initlize server
func (s *Server) StartInit() {
	// s.logger.Println("StartInit")
	s.mu.Lock()
	s.leaderID = ""
	s.state = INIT
	s.mu.Unlock()
	s.broadcaster.Stop()
	// s.StopBroadcasting()
	// wait for broadcasts from other leader nodes
	// Wait for a random time between 150ms and 300ms
	// randomDelay := time.Duration(rand.IntN(350)+10) * time.Millisecond
	// time.Sleep(randomDelay)
	s.mu.Lock()
	// if the state is still on INIT, then there are no other nodes.
	// then, start the server as a leader node
	if s.state == INIT {
		s.logger.Println("Assuming Leader")
		s.state = LEADER
		s.leaderID = s.id
		s.mu.Unlock()
		s.broadcaster.Start()
		// go s.StartBroadcasting()
		return
	}
	s.mu.Unlock()
}

func (s *Server) StartElection() {
	// TODO
	// bully algorithm
	s.logger.Println("Starting election")
	s.mu.Lock()
	s.state = ELECTION
	peers := make(map[string]string, len(s.peers))
	for uuid, ip := range s.peers {
		peers[uuid] = ip
	}
	id := s.id
	s.discoveredPeers = make(map[string]bool)
	s.mu.Unlock()
	highestID := true

	for uuid, ip := range peers {
		if uuid > id || uuid == id {
			continue
		}
		highestID = false
		s.logger.Println(ip)
		// send election message
		// wait unitl a specified time for
		go func(uuid string, ip string) {
			s.logger.Println("sending election message to", ip)
			encodedData := message.NewElectionMessage()
			send := s.ru.SendMessage(ip, uuid, encodedData)
			if !send {
				s.logger.Println("failed to send election message to", ip)
				s.handleDeadServer(uuid, ip)
			}
		}(uuid, ip)

	}
	if highestID {
		// send out victory message
		s.logger.Println("sending victory messages")
		go s.SendElectionVictoryAndBecomeLeader()
		return
	}

	// check if we got response from
}

func (s *Server) SendElectionVictoryAndBecomeLeader() {
	var wg sync.WaitGroup
	peers := make(map[string]string, len(s.peers))
	s.mu.Lock()
	for uuid, ip := range s.peers {
		peers[uuid] = ip
	}
	ownID := s.id
	s.mu.Unlock()

	wg.Add(len(peers))

	for uuid, ip := range peers {
		if uuid == ownID {
			continue
		}
		go func(uuid string, ip string) {
			defer wg.Done()
			victoryMessage := message.NewElectionVictoryMessage()
			send := s.ru.SendMessage(ip, uuid, victoryMessage)
			if !send {
				// TODO: handle node failure
				s.handleDeadServer(uuid, ip)
				log.Println("failed to send victory message to", ip)
			}

		}(uuid, ip)
	}
	// wait until all vicotry messages are send and ack is received
	wg.Wait()
	s.logger.Println("Victory messages sent")
	s.mu.Lock()
	defer s.mu.Unlock()

	// if s.state == ELECTION {
	s.leaderID = s.id
	s.state = LEADER
	s.logger.Println("ELECTED LEADER")
	// s.StartBroadcasting()
	// return
	// }
}

func (s *Server) handleDeadServer(uuid string, ip string) {
	s.mu.Lock()
	delete(s.peers, uuid)
	delete(s.discoveredPeers, uuid)
	s.mu.Unlock()
}

func (s *Server) Shutdown() {
	close(s.quit)
	s.broadcaster.Shutdown()
	s.rm.Shutdown()
	s.ru.Shutdown()

}

// func (*Server) getRandomUDPPort() (*net.PacketConn, int, error) {
// 	for i := 0; i < 10; i++ {
// 		port := rand.Intn(65535-49152+1) + 49152
// 		addr := fmt.Sprintf(":%d", port)
// 		conn, err := net.ListenPacket("udp4", addr)
// 		if err == nil {
// 			return &conn, port, nil
// 		}
// 	}
// 	return nil, 0, errors.New("coulb not find a free port after multiple attempts")
// }
