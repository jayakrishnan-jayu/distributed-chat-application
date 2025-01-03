package unicast

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

const UNI_S_PORT = ":5002"
const UNI_L_PORT = ":5003"

type ReliableUnicast struct {
	sender   reliableSender
	listener reliableListener

	sAckChan chan string
	lAckChan chan *Message
	// ordered message for application layer
	msgChan chan<- *Message

	quit chan interface{}
}

type reliableSender struct {
	mu sync.Mutex

	conn net.PacketConn
	// message buffer for retransmission in case of transmission loss
	// map[ip_string+fram]Message
	msgBuf map[string]*Message

	// last send frame size for a client: map[ip_string]frame_count
	peerFrame map[string]uint64

	// acknoledgement for sent frames: map[ip_string+frame_count]bool
	ackFrame map[string]bool

	sAckChan <-chan string
	lAckChan <-chan *Message

	quit <-chan interface{}
}

type reliableListener struct {
	mu sync.Mutex

	//listener connection
	conn net.PacketConn

	// buffer to hold messages that are head of frame count in application layer
	// map[ip_string+fram]Message
	msgBuf map[string]*Message

	// last received frame size from a client that is send to applciation
	// layer
	// map[ip_string]frame_count
	peerFrame map[string]uint64

	// ordered message for application layer
	msgChan chan<- *Message

	// channel to send ack
	sAckChan chan<- string

	lAckChan chan<- *Message
	quit     <-chan interface{}
}

type Message struct {
	Response byte
	Frame    uint64
	Tries    uint8
	UUID     string
	IP       string
	Message  []byte
}

func NewReliableUnicast(msgChan chan<- *Message) *ReliableUnicast {
	s := new(ReliableUnicast)
	pc, err := net.ListenPacket("udp4", UNI_S_PORT)
	if err != nil {
		log.Panic(err)
	}

	s.sAckChan = make(chan string, 5)
	s.lAckChan = make(chan *Message, 5)
	s.msgChan = msgChan
	s.quit = make(chan interface{})

	s.sender.conn = pc
	s.sender.msgBuf = make(map[string]*Message)
	s.sender.peerFrame = make(map[string]uint64)
	s.sender.ackFrame = make(map[string]bool)
	s.sender.sAckChan = s.sAckChan
	s.sender.lAckChan = s.lAckChan
	s.sender.quit = s.quit

	s.listener.msgBuf = make(map[string]*Message)
	s.listener.peerFrame = make(map[string]uint64)
	s.listener.msgChan = s.msgChan
	s.listener.sAckChan = s.sAckChan
	s.listener.lAckChan = s.lAckChan
	s.listener.quit = s.quit
	return s
}

func (s *ReliableUnicast) StartListener() {
	go s.listener.start()
	go s.sender.ackListener()
}

func (s *ReliableUnicast) SendMessage(ip string, uuid string, data []byte) bool {
	return s.sender.send(ip, uuid, data)
}

func (s *ReliableUnicast) Shutdown() {
	close(s.quit)
	s.listener.Shutdown()
	s.sender.Shutdown()
}

func (s *reliableListener) start() {
	var err error

	s.conn, err = net.ListenPacket("udp4", UNI_L_PORT)
	if err != nil {
		log.Panic(err)
	}
	defer s.conn.Close()

	buf := make([]byte, 1024)

	for {
		n, addr, err := s.conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-s.quit:
				log.Println("Quiting Unicast listener")
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
		msg, err := decodeUnicastMessage(buf[:n], udpAddr.IP.String())
		if err != nil {
			log.Printf("Error decoding data %v", err)
		}
		msgIp := msg.IP
		msgIndex := fmt.Sprintf("%s%d", msgIp, msg.Frame)

		// if the message is an ack for a frame
		if msg.Response == 1 {
			s.sAckChan <- msgIndex
			continue
		}

		s.lAckChan <- &Message{
			IP:       msgIp,
			Frame:    msg.Frame,
			Tries:    msg.Tries,
			Response: 1,
		}

		// message from a peer
		s.mu.Lock()
		lastAckFrame, ok := s.peerFrame[msg.IP]
		if !ok {
			// first message from a new peer
			s.peerFrame[msgIp] = 0
		}
		if msg.Frame > lastAckFrame+1 {
			// previous frame/frames missing
			s.msgBuf[msgIndex] = msg
			s.mu.Unlock()
			continue
		}
		log.Println(msg.Frame, lastAckFrame)
		if msg.Frame < lastAckFrame+1 {
			log.Printf("discarding duplicate message %s, ip: %s, tries: %d, frame: %d", string(msg.Message), msgIp, msg.Tries, msg.Frame)
			s.mu.Unlock()
			continue
		}
		s.peerFrame[msgIp] += 1
		s.msgChan <- msg
		if len(s.msgBuf) > 0 {
			for {
				msgIndex = fmt.Sprintf("%s%d", msgIp, s.peerFrame[msgIp])
				m, ok := s.msgBuf[msgIndex]
				if !ok {
					break
				}
				delete(s.msgBuf, msgIndex)
				log.Println("sending buffered messages")
				s.msgChan <- m
			}
		}
		s.mu.Unlock()
	}
}

func (s *reliableSender) send(ip string, uuid string, data []byte) bool {
	s.mu.Lock()
	currFrame, ok := s.peerFrame[ip]
	if !ok {
		s.peerFrame[ip] = 0
	}
	s.peerFrame[ip] += 1
	currFrame = s.peerFrame[ip]

	msgIndex := fmt.Sprintf("%s%d", ip, currFrame)

	s.msgBuf[msgIndex] = &Message{
		Response: 0,
		Frame:    currFrame,
		Tries:    0,
		UUID:     uuid,
		IP:       ip,
		Message:  data,
	}
	encodedData, err := encodeUnicastMessage(*s.msgBuf[msgIndex])
	if err != nil {
		log.Panic(err)
	}
	s.ackFrame[msgIndex] = false
	s.mu.Unlock()

	s.sendUDP(ip, encodedData)

	for {
		time.Sleep(10 * time.Millisecond)
		s.mu.Lock()
		_, ok := s.ackFrame[msgIndex]
		s.mu.Unlock()
		if !ok {
			return true
		}
		decodedData, err := decodeUnicastMessage(encodedData, ip)
		if err != nil {
			log.Panic(err)
		}
		if decodedData.Tries == 20 {
			log.Println(decodedData)
			return false
		}
		decodedData.Tries += 1
		encodedData, err := encodeUnicastMessage(*decodedData)
		s.sendUDP(ip, encodedData)
	}
}

func (s *reliableSender) sendUDP(ip string, data []byte) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s%s", ip, UNI_L_PORT))
	if err != nil {
		log.Panic(err)
	}

	_, err = s.conn.WriteTo(data, addr)
	if err != nil {
		log.Panic(err)
	}
}

func (s *reliableSender) ackListener() {
	for {
		select {
		case <-s.quit:
			log.Println("Quiting Unicast Ack listener")
			return
		case msg := <-s.sAckChan:
			// log.Println("ack for", msg)
			s.mu.Lock()
			delete(s.ackFrame, msg)
			s.mu.Unlock()
		case msg := <-s.lAckChan:
			d, err := encodeUnicastMessage(*msg)
			if err != nil {
				log.Panic(err)
			}
			// log.Printf("Sending ACK to %s for frame %d", msg.Ip, msg.frame)
			// time.Sleep(50 * time.Millisecond)
			s.sendUDP(msg.IP, d)
		}
	}
}

func encodeUnicastMessage(msg Message) ([]byte, error) {
	var buf bytes.Buffer
	if err := buf.WriteByte(msg.Response); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.BigEndian, msg.Frame); err != nil {
		return nil, err
	}

	if msg.Response == 1 {
		return buf.Bytes(), nil
	}

	if err := binary.Write(&buf, binary.BigEndian, msg.Tries); err != nil {
		return nil, err
	}

	uuidBytes := []byte(msg.UUID)
	if len(uuidBytes) > 36 {
		uuidBytes = uuidBytes[:36]
	}
	if len(uuidBytes) < 36 {
		uuidBytes = append(uuidBytes, make([]byte, 36-len(uuidBytes))...)
	}
	buf.Write(uuidBytes)

	// ipBytes := []byte(msg.Ip)
	// log.Println("Ip bytes", len(ipBytes))
	// if len(ipBytes) > 15 {
	// 	return nil, fmt.Errorf("IP too long: %s", msg.Ip)
	// }
	// buf.Write(ipBytes)
	// buf.Write(make([]byte, 15-len(ipBytes)))

	messageBytes := msg.Message
	msgLen := uint64(len(messageBytes))
	if err := binary.Write(&buf, binary.BigEndian, msgLen); err != nil {
		return nil, err
	}
	buf.Write(messageBytes)

	return buf.Bytes(), nil
}

func decodeUnicastMessage(data []byte, ip string) (*Message, error) {
	reader := bytes.NewReader(data)
	msg := &Message{}

	response, err := reader.ReadByte()
	if err != nil {
		return msg, err
	}

	var frame uint64
	if err := binary.Read(reader, binary.BigEndian, &frame); err != nil {
		return msg, err
	}

	if response == 1 {
		msg.Response = 1
		msg.Frame = frame
		msg.IP = ip
		return msg, nil
	}

	var tries uint8
	if err := binary.Read(reader, binary.BigEndian, &tries); err != nil {
		return msg, err
	}

	uuidBytes := make([]byte, 36)
	if _, err := reader.Read(uuidBytes); err != nil {
		return msg, err
	}
	uuidStr := string(uuidBytes)

	// ipBytes := make([]byte, 15)
	// if _, err := reader.Read(ipBytes); err != nil {
	// 	return msg, err
	// }
	// ipStr := string(ipBytes)

	var msgLen uint64
	if err := binary.Read(reader, binary.BigEndian, &msgLen); err != nil {
		return msg, err
	}

	strBytes := make([]byte, msgLen)
	if _, err := reader.Read(strBytes); err != nil {
		return msg, err
	}

	msg.UUID = uuidStr
	msg.IP = ip
	msg.Frame = frame
	msg.Message = strBytes

	return msg, nil
}

func generateAck(ip string, frame uint64, tries uint8) []byte {
	d, err := encodeUnicastMessage(Message{IP: ip, Frame: frame, Tries: tries, Response: 1})
	if err != nil {
		log.Panic(err)
	}
	return d
}

func (s *reliableListener) Shutdown() {
	//race condition
	s.conn.Close()
}

func (s *reliableSender) Shutdown() {
	s.conn.Close()
}
