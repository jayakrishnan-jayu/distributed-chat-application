package multicast

import (
	"bytes"
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

func NewReliableMulticast(id string, msgChan chan<- *Message) *ReliableMulticast {
	addr, err := net.ResolveUDPAddr("udp4", ROM_ADDRESS)
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
	m.vectorClock = make(map[string]uint32)
	m.holdBackQueue = make([]*Message, 0)
	m.msgChan = msgChan
	m.logger = log.New(os.Stdout, fmt.Sprintf("[%s]multicaster ", m.id[:4]), log.Ltime)
	m.quit = make(chan interface{})

	m.vectorClock[m.id] = 0

	return m
}

func (m *ReliableMulticast) CanDeliver(msg *Message) bool {
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
	m.mu.Unlock()
	encodedData, err := encodeMulticastMessage(Message{UUID: m.id, Message: data})
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
		m.logger.Println("got multicast")
		go func(msg Message) {
			// handle dead nodes
			m.mu.Lock()
			m.holdBackQueue = append(m.holdBackQueue, &msg)
			m.mu.Unlock()
			m.DeliverMessages()
		}(*msg)

	}
}

func (m *ReliableMulticast) AddPeer(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.vectorClock[id]; !ok {
		m.vectorClock[id] = 0
	}
}

func (m *ReliableMulticast) AddPeers(ids []string, clocks []uint32) {
	m.logger.Println("adding peers")
	m.mu.Lock()
	defer m.mu.Unlock()
	for index, id := range ids {
		if _, ok := m.vectorClock[id]; !ok {
			m.vectorClock[id] = clocks[index]
		} else {
			m.logger.Println("don't know if this should happen", m.vectorClock[id], clocks[index])
		}
	}
}

func (m *ReliableMulticast) HandleDeadNode(id string) {

}

func (m *ReliableMulticast) VectorClock() map[string]uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]uint32, len(m.vectorClock))
	for id, clock := range m.vectorClock {
		result[id] = clock
	}
	return result
}

func (m *ReliableMulticast) DeliverMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func (s *ReliableMulticast) Shutdown() {
	close(s.quit)
	s.lconn.Close()
	s.sconn.Close()
}
