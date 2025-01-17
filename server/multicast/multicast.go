package multicast

import (
	"bytes"
	"dummy-rom/server/message"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
)

const (
	maxDatagramSize = 8192
	ROM_ADDRESS     = "239.0.0.0:9999"
)

type State int

type ReliableMulticast struct {
	mu            sync.Mutex
	id            string
	sconn         *net.UDPConn
	lconn         *net.UDPConn
	port          uint32
	vectorClock   map[string]uint32
	holdBackQueue []*Message
	msgChan       chan<- *Message
	logger        *log.Logger
	quit          chan interface{}
}

type Message struct {
	IP          string
	UUID        string
	VectorClock map[string]uint32
	Message     []byte
}

func NewReliableMulticast(id string, port uint32, nodeIds []string, msgChan chan<- *Message) *ReliableMulticast {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("239.0.0.0:%d", port))
	if err != nil {
		log.Panic(err)
	}

	senderConn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		log.Panic(err)
	}

	listenerConn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		log.Fatal(err)
	}
	listenerConn.SetReadBuffer(maxDatagramSize)

	m := &ReliableMulticast{}
	m.id = id
	m.sconn = senderConn
	m.lconn = listenerConn
	m.port = port
	m.vectorClock = make(map[string]uint32)
	m.holdBackQueue = make([]*Message, 0)
	m.msgChan = msgChan
	m.logger = log.New(os.Stdout, fmt.Sprintf("[%s][%d][multicaster] ", m.id[:4], port), log.Ltime)
	m.quit = make(chan interface{})

	m.vectorClock[m.id] = 0
	for _, nodeId := range nodeIds {
		m.vectorClock[nodeId] = 0
	}
	m.logger.Println("New multicaster with %d nodes", len(nodeIds))

	return m
}

func (m *ReliableMulticast) CanDeliver(msg *Message) bool {
	m.logger.Println("checking msg from", msg.UUID, msg.VectorClock)
	j := msg.UUID
	ij, ok := m.vectorClock[j]
	if !ok {
		m.logger.Println("vector clock not found for id", j)
		return false
	}
	jj, ok := msg.VectorClock[j]
	if !ok {
		m.logger.Println("vector clock not found in msg for id", j, len(msg.VectorClock), len(m.vectorClock), msg.IP)
		return false
	}

	if jj != ij+1 {
		m.logger.Println("can not deliver message until", jj, ij, j)
		// TODO
		// send nack
		return false
	}

	for k, ik := range m.vectorClock {
		if k == j {
			continue
		}
		jk, ok := msg.VectorClock[k]
		if !ok {
			m.logger.Println("vector clock not found in msg", k)
			return false
		}
		if jk > ik {
			m.logger.Println("can not deliver messages until k", jk, ik, k)
			// send nack
			return false
		}

	}
	return true
}

func (m *ReliableMulticast) SendMessage(data []byte) bool {
	m.logger.Println("sending multicast")
	m.mu.Lock()
	_, ok := m.vectorClock[m.id]
	if !ok {
		log.Fatal("vector clock not found for self id")
		return false
	}
	m.vectorClock[m.id] += 1
	clockCopy := make(map[string]uint32)
	for uuid, clock := range m.vectorClock {
		clockCopy[uuid] = clock
	}
	m.mu.Unlock()
	encodedData, err := encodeMulticastMessage(Message{UUID: m.id, Message: data, VectorClock: clockCopy})
	if err != nil {
		log.Fatal(err)
	}
	_, err = m.sconn.Write(encodedData)
	if err != nil {
		log.Fatal(err)
	}
	return true

}

func (m *ReliableMulticast) StartListener() {

	m.logger.Println("multicast listening on port", m.port)
	buf := make([]byte, 1024)

	for {
		n, addr, err := m.lconn.ReadFrom(buf)
		if err != nil {
			select {
			case <-m.quit:
				m.logger.Println("Quiting Multicast listener")
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
		msg, err := decodeMulticastMessage(buf[:n], udpAddr.IP.String())
		if err != nil {
			log.Printf("Error decoding data %v", err)
		}
		if msg.UUID == m.id {
			continue
		}
		decodedMsg, err := message.Decode(msg.Message)
		if decodedMsg.Type == message.NewNode {
			m.logger.Println("got multicast to add node")
			m.AddNewNode(decodedMsg.UUID)
		}
		if decodedMsg.Type == message.DeadNode {
			// m.RemoveDeadNode(decodedMsg.UUID)
		}
		go func(msg Message) {
			// handle dead nodes
			m.mu.Lock()
			m.holdBackQueue = append(m.holdBackQueue, &msg)
			m.mu.Unlock()
			log.Println("trying to deliver message from multicast")
			m.DeliverMessages()
			log.Println("done delivering message from multicast")
		}(*msg)

	}
}

// func (m *ReliableMulticast) AddPeer(id string) {
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	if _, ok := m.vectorClock[id]; !ok {
// 		m.vectorClock[id] = 0
// 	}
// }

func (m *ReliableMulticast) HasPeer(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.vectorClock[id]
	return ok
}

// func (m *ReliableMulticast) AddPeers(ids []string, clocks []uint32) {
// 	m.logger.Println("adding peers")
// 	m.mu.Lock()
// 	defer m.mu.Unlock()
// 	for index, id := range ids {
// 		if _, ok := m.vectorClock[id]; !ok {
// 			m.vectorClock[id] = clocks[index]
// 		} else {
// 			m.logger.Println("don't know if this should happen", m.vectorClock[id], clocks[index])
// 		}
// 	}
// }

func (m *ReliableMulticast) RemoveDeadNode(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.vectorClock[id]; !ok {
		log.Println("node not found in vector clock to delete")
		return
	}
	delete(m.vectorClock, id)
	j := 0
	for i := range len(m.holdBackQueue) {
		if m.holdBackQueue[i].UUID == id {
			continue
		}
		m.holdBackQueue[j] = m.holdBackQueue[i]
		delete(m.holdBackQueue[j].VectorClock, id)
		j++
	}
	m.holdBackQueue = m.holdBackQueue[:j]

}

func (m *ReliableMulticast) AddNewNode(id string) {
	m.Debug()
	m.logger.Println("adding new Node", id)
	m.mu.Lock()
	if _, ok := m.vectorClock[id]; ok && id != m.id {
		log.Println("node already in vector clock %s | %s", m.id, id)
	} else {
		m.vectorClock[id] = 0
	}
	for i := range len(m.holdBackQueue) {
		if _, ok := m.holdBackQueue[i].VectorClock[id]; !ok {
			m.holdBackQueue[i].VectorClock[id] = 0
			if id == m.id {
				m.logger.Println("hold back queue doesn't have node clock")
			}
		}
	}
	m.logger.Println("done adding a new node")
	m.mu.Unlock()
	// m.Debug()
	m.DeliverMessages()
}

func (m *ReliableMulticast) VectorClock() map[string]uint32 {
	m.mu.Lock()
	result := make(map[string]uint32, len(m.vectorClock))
	for id, clock := range m.vectorClock {
		result[id] = clock
	}
	m.mu.Unlock()
	return result
}

func (m *ReliableMulticast) DeliverMessages() {
	m.logger.Println("trying to deliver")
	m.mu.Lock()
	m.logger.Println("trying to deliver message", len(m.holdBackQueue))

	delivered := true
	for delivered {
		delivered = false
		for i, msg := range m.holdBackQueue {
			if m.CanDeliver(msg) {
				m.msgChan <- msg
				log.Printf("Node %s delivered message from %s\n", m.id, msg.UUID)
				m.vectorClock[msg.UUID]++
				m.holdBackQueue = append(m.holdBackQueue[:i], m.holdBackQueue[i+1:]...)
				delivered = true
				break
			} else {

			}
		}
	}
	m.mu.Unlock()

	m.logger.Println("undelivered messages", len(m.holdBackQueue))
}

func encodeMulticastMessage(msg Message) ([]byte, error) {
	var buf bytes.Buffer

	uuidBytes := []byte(msg.UUID)
	if len(uuidBytes) > 36 {
		uuidBytes = uuidBytes[:36]
	}
	if len(uuidBytes) < 36 {
		uuidBytes = append(uuidBytes, make([]byte, 36-len(uuidBytes))...)
	}
	buf.Write(uuidBytes)

	if err := binary.Write(&buf, binary.BigEndian, uint32(len(msg.VectorClock))); err != nil {
		return nil, err
	}
	for id, clock := range msg.VectorClock {
		if err := binary.Write(&buf, binary.BigEndian, uint32(clock)); err != nil {
			return nil, err
		}
		idBytes := []byte(id)
		if len(idBytes) > 36 {
			idBytes = idBytes[:36]
		}
		if len(idBytes) < 36 {
			idBytes = append(idBytes, make([]byte, 36-len(idBytes))...)
		}
		buf.Write(idBytes)
	}

	msgLen := uint64(len(msg.Message))
	if err := binary.Write(&buf, binary.BigEndian, msgLen); err != nil {
		return nil, err
	}
	buf.Write(msg.Message)

	return buf.Bytes(), nil
}

func decodeMulticastMessage(data []byte, ip string) (*Message, error) {
	reader := bytes.NewReader(data)
	msg := &Message{}

	uuidBytes := make([]byte, 36)
	if _, err := reader.Read(uuidBytes); err != nil {
		return msg, err
	}
	uuidStr := string(uuidBytes)

	var vectorClockLen uint32
	if err := binary.Read(reader, binary.BigEndian, &vectorClockLen); err != nil {
		return msg, err
	}

	var clock uint32
	msg.VectorClock = make(map[string]uint32)
	for range vectorClockLen {
		if err := binary.Read(reader, binary.BigEndian, &clock); err != nil {
			return msg, err
		}
		if _, err := reader.Read(uuidBytes); err != nil {
			return msg, err
		}
		msg.VectorClock[string(uuidBytes)] = clock

	}

	var msgLen uint64
	if err := binary.Read(reader, binary.BigEndian, &msgLen); err != nil {
		return msg, err
	}

	strBytes := make([]byte, msgLen)
	if _, err := reader.Read(strBytes); err != nil {
		return msg, err
	}

	msg.IP = ip
	msg.UUID = uuidStr
	msg.Message = strBytes
	return msg, nil

}

func (s *ReliableMulticast) Debug() {
	count := 0
	for id, vc := range s.vectorClock {
		count++
		s.logger.Println(count, "Peer: ", id, " -", vc)
	}
	s.logger.Println("holdBackQueue")
	for _, msg := range s.holdBackQueue {
		s.logger.Println(msg.IP, msg.VectorClock)
	}

}
func (s *ReliableMulticast) Shutdown() {
	s.logger.Println("shutting down multicaster with port", s.port)
	close(s.quit)
	s.lconn.Close()
	s.sconn.Close()
}
