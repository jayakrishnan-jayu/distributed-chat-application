package multicast

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
)

const (
	maxDatagramSize = 8192
)

type ReliableMulticast struct {
	sender   reliableSender
	listener reliableListener
	quit     chan interface{}
}

type Message struct {
	ip      string
	uuid    string
	Message []byte
}

type reliableSender struct {
	mu   sync.Mutex
	conn *net.UDPConn
	quit <-chan interface{}
}

type reliableListener struct {
	mu      sync.Mutex
	conn    *net.UDPConn
	msgChan chan<- *Message
	quit    <-chan interface{}
}

func NewReliableMulticast(msgChan chan<- *Message, ip string, port string) *ReliableMulticast {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%s", ip, port))
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

	m := new(ReliableMulticast)
	m.quit = make(chan interface{})

	m.sender.conn = senderConn
	m.sender.quit = m.quit

	m.listener.conn = listenerConn
	m.listener.quit = m.quit
	m.listener.msgChan = msgChan

	return m
}

func (m *ReliableMulticast) StartListener() {
	go m.listener.start()
}

func (m *ReliableMulticast) SendMessage(uuid string, data []byte) bool {
	return m.sender.send(uuid, data)
}

func (b *reliableSender) send(uuid string, data []byte) bool {
	encodedData, err := encodeMulticastMessage(Message{uuid: uuid, Message: data})
	if err != nil {
		log.Fatal(err)
	}
	_, err = b.conn.Write(encodedData)
	if err != nil {
		log.Fatal(err)
	}
	return true

}

func (s *reliableListener) start() {

	buf := make([]byte, 1024)

	for {
		n, addr, err := s.conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-s.quit:
				log.Println("Quiting Multicast listener")
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
		s.msgChan <- msg
	}
}

func encodeMulticastMessage(msg Message) ([]byte, error) {
	var buf bytes.Buffer

	uuidBytes := []byte(msg.uuid)
	if len(uuidBytes) > 36 {
		uuidBytes = uuidBytes[:36]
	}
	if len(uuidBytes) < 36 {
		uuidBytes = append(uuidBytes, make([]byte, 36-len(uuidBytes))...)
	}
	buf.Write(uuidBytes)

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

	var msgLen uint64
	if err := binary.Read(reader, binary.BigEndian, &msgLen); err != nil {
		return msg, err
	}

	strBytes := make([]byte, msgLen)
	if _, err := reader.Read(strBytes); err != nil {
		return msg, err
	}

	msg.ip = ip
	msg.uuid = uuidStr
	msg.Message = strBytes
	return msg, nil

}

func (s *ReliableMulticast) Shutdown() {
	close(s.quit)
	s.listener.Shutdown()
	s.sender.Shutdown()
}

func (s *reliableListener) Shutdown() {
	//race condition
	s.conn.Close()
}

func (s *reliableSender) Shutdown() {
	s.conn.Close()
}
