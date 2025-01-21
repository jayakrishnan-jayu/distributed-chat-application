package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

type ClientServer struct {
	id     string
	logger *log.Logger

	clientListner      net.Listener
	applicationMsgChan chan string
	clientMsgChan      chan string
	clients            []net.Conn
	quit               chan interface{}
	mu                 sync.Mutex
	server             *Server
	getRandomPeerID    func() string
}

func NewClientServer(uuid string, clientMsgChan chan string, applicationMsgChan chan string, server *Server, getRandomPeerID func() string) *ClientServer {
	clientListner, err := net.Listen("tcp", CLIENT_PORT)
	if err != nil {
		log.Panic(err)
	}
	return &ClientServer{
		id:                 uuid,
		logger:             log.New(os.Stdout, fmt.Sprintf("[%s][clientserver] ", uuid[:4]), log.Ltime),
		clientListner:      clientListner,
		clientMsgChan:      clientMsgChan,
		applicationMsgChan: applicationMsgChan,
		quit:               make(chan interface{}),
		server:             server,
		getRandomPeerID:    getRandomPeerID,
	}
}

func (s *ClientServer) Shutdown() {
	close(s.quit)
}

func (s *ClientServer) Start() {
	go func() {
		// ownID := s.id
		s.logger.Println("starting client listner")
		for {
			select {
			case <-s.quit:
				s.logger.Println("clossing conn")
				s.clientListner.Close()
				return
			default:
				conn, err := s.clientListner.Accept()
				s.logger.Println("new client conn from", conn.LocalAddr())
				if err != nil {
					s.logger.Println(err)
					return
				}
				s.server.mu.Lock()
				state := s.server.state
				// n := len(s.server.peers)
				s.server.mu.Unlock()
				// don't accept client connectoin if server is neither a leader nor a follower
				if state != LEADER && state != FOLLOWER {
					s.logger.Println("server state", state)
					conn.Close()
					break
				}
				// if state == LEADER {
				// 	randomPeerID := s.getRandomPeerID()
				// 	s.logger.Println(n, n > 1, randomPeerID, ownID)
				// 	if n > 1 && randomPeerID != ownID {
				// 		s.logger.Println("writing peerid", randomPeerID)
				// 		conn.Write([]byte(randomPeerID))
				// 		conn.Close()
				// 		break
				// 	}
				// }
				go s.handleConn(conn)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-s.quit:
				return
			case msg := <-s.applicationMsgChan:
				s.sendMessageToConnectedClients(msg)
			}
		}
	}()
}

func (s *ClientServer) sendMessageToConnectedClients(msg string) {
	s.mu.Lock()
	for _, c := range s.clients {
		c.Write([]byte(msg + "\n"))
	}
	s.mu.Unlock()
}

func (s *ClientServer) addClient(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients = append(s.clients, conn)
}

func (s *ClientServer) removeClient(conn net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := false
	for i, c := range s.clients {
		if c == conn {
			removed = true
			s.clients = append(s.clients[:i], s.clients[i+1:]...)
			break
		}
	}
	return removed
}

func (s *ClientServer) handleConn(conn net.Conn) {
	s.addClient(conn)
	defer func() {
		if s.removeClient(conn) {
			s.logger.Printf("Connection closed for %s\n", conn.RemoteAddr().String())
			conn.Close()
		}
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		message := scanner.Text()
		s.logger.Printf("Message from %s: %s\n", conn.RemoteAddr().String(), message)
		s.clientMsgChan <- message
		s.applicationMsgChan <- message
	}

	if err := scanner.Err(); err != nil {
		s.logger.Printf("Error reading from connection %s: %v\n", conn.RemoteAddr().String(), err)
	}
}
