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
	"time"

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

func (s *Server) StartUniCastListener() {
	go s.startUnicastMessageListener()
	s.ru.StartListener()
}

func (s *Server) StartMulticastListener() {
	go s.rm.StartListener()
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
				log.Panic(err)
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

				// encodedMessage := message.NewPeerInfoMessage(peerIds, peerIps)
				// send active peers
				log.Println("Sending active peers")
				s.rm.SendMessage(id, []byte("from leader sending active peers"))
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

				if id > msg.UUID || higherNodeExists {
					// s.StopBroadcasting()
					s.broadcaster.Stop()
					s.mu.Unlock()
					go s.StartElection()
					break
				}
				s.state = FOLLOWER
				s.leaderID = msg.UUID
				s.mu.Unlock()
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
						log.Println("failed to send alive message to", msg.IP)
					}
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
						s.StartElection()
					}
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
				s.logger.Println("got election victory from", msg.IP)
				s.mu.Lock()
				s.state = FOLLOWER
				s.leaderID = msg.UUID
				// s.StopBroadcasting()
				s.broadcaster.Stop()
				s.mu.Unlock()
				break
			}
			s.logger.Println(unicastMessage.Type)
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
	s.discoveredPeers[leaderIP] = true
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
