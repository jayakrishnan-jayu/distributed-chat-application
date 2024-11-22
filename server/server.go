package server

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/google/uuid"
)

const BROADCAST_S_PORT = ":5000"
const BROADCAST_L_PORT = ":5001"

const UNI_S_PORT = ":5002"
const UNI_L_PORT = ":5003"

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
	mu              sync.Mutex
	id              string
	ip              string
	broadcast       string
	state           State
	ru              *reliableUnicast
	broadConn       net.PacketConn
	peers           map[string]string
	discoveredPeers map[string]bool
	quit            chan interface{}
	newServer       chan string
}

func LocalIP() (*net.IPNet, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet, nil
		}
	}
	return nil, errors.New("IP not found")
}

func BroadcastIP(ipv4 *net.IPNet) (net.IP, error) {
	if ipv4.IP.To4() == nil {
		return net.IP{}, errors.New("does not support IPv6 addresses.")
	}
	ip := make(net.IP, len(ipv4.IP.To4()))
	binary.BigEndian.PutUint32(ip, binary.BigEndian.Uint32(ipv4.IP.To4())|^binary.BigEndian.Uint32(net.IP(ipv4.Mask).To4()))
	return ip, nil
}

func NewServer() (*Server, error) {
	id := uuid.New()
	ip, err := LocalIP()
	if err != nil {
		return &Server{}, err
	}
	broadcast, err := BroadcastIP(ip)
	if err != nil {
		return &Server{}, err
	}

	s := new(Server)
	s.state = INIT
	s.id = id.String()
	s.ip = ip.String()
	s.ru = NewReliableUnicast()
	s.broadcast = broadcast.String()
	s.peers = make(map[string]string)
	s.discoveredPeers = make(map[string]bool)
	s.newServer = make(chan string, 5)
	s.quit = make(chan interface{})

	return s, nil
}

func (s *Server) StartBroadcastListener() {
	var err error

	s.broadConn, err = net.ListenPacket("udp4", BROADCAST_L_PORT)
	if err != nil {
		log.Panic(err)
	}
	defer s.broadConn.Close()

	buf := make([]byte, 1024)

	for {
		n, addr, err := s.broadConn.ReadFrom(buf)
		if err != nil {
			select {
			case <-s.quit:
				log.Println("Broadcast Connection closed")
				return
			default:
				log.Printf("Error reading from connection: %v\n", err)
			}
		}
		udpAddr, ok := addr.(*net.UDPAddr)
		if !ok {
			log.Printf("Unexpected address type: %T", addr)
			continue
		}
		go s.connectNewPeer(string(buf[:n]), udpAddr.IP.String())
	}
}

func (s *Server) StartUniCastSender() {
	for {
		select {
		case clientUUIDStr := <-s.newServer:
			s.mu.Lock()
			defer s.mu.Unlock()
			ip := s.peers[clientUUIDStr]
			log.Printf("sending message to %s from %s", ip, s.ip)
			s.ru.SendMessage(ip, s.id, []byte("Hello"))

		}
	}
}

func (s *Server) StartUniCastListener() {
	s.ru.StartListener()
}

func (s *Server) connectNewPeer(clientUUIDStr string, ip string) {
	clientUUID, err := uuid.Parse(clientUUIDStr)
	if err != nil {
		log.Printf("Invalid UUID received from %s: %v\n", ip, err)
		return
	}
	clientUUIDStr = clientUUID.String()
	s.mu.Lock()
	_, ok := s.peers[clientUUIDStr]
	s.mu.Unlock()
	if !ok {
		log.Println("Adding new client ", ip)
		s.mu.Lock()
		s.peers[clientUUIDStr] = ip
		s.discoveredPeers[clientUUIDStr] = false
		s.mu.Unlock()
		s.newServer <- clientUUIDStr
	}

}

func (s *Server) BroadcastHello() {
	SendUDP(s.broadcast, BROADCAST_S_PORT, BROADCAST_L_PORT, []byte(s.id))
}

func (s *Server) Shutdown() {
	close(s.quit)
	s.broadConn.Close()
	s.ru.Shutdown()

}

func SendUDP(ip string, fromPort string, toPort string, data []byte) {
	pc, err := net.ListenPacket("udp4", fromPort)
	if err != nil {
		log.Panic(err)
	}
	defer pc.Close()

	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s%s", ip, toPort))
	if err != nil {
		log.Panic(err)
	}

	_, err = pc.WriteTo(data, addr)
	if err != nil {
		log.Panic(err)
	}
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
