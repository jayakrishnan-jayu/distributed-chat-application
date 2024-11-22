package server

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"sync"
)

type reliableUnicast struct {
	mu        sync.Mutex
	frame     uint64
	msgBuf    map[uint64]*UnicastMessage
	isSending bool
	lConn     net.PacketConn
	rConn     net.PacketConn
	msgChan   chan UnicastMessage
	failChan  chan UnicastMessage
	quit      chan interface{}
}

type UnicastMessage struct {
	frame   uint64
	tries   uint8
	uuid    string
	Ip      string
	Message []byte
}

func NewReliableUnicast() *reliableUnicast {
	s := new(reliableUnicast)
	s.frame = 0
	s.isSending = false
	s.msgBuf = make(map[uint64]*UnicastMessage)
	s.msgChan = make(chan UnicastMessage, 5)
	s.quit = make(chan interface{})
	return s
}

func (s *reliableUnicast) StartListener() {
	var err error

	s.lConn, err = net.ListenPacket("udp4", UNI_L_PORT)
	if err != nil {
		log.Panic(err)
	}
	defer s.lConn.Close()

	buf := make([]byte, 1024)

	for {
		n, addr, err := s.lConn.ReadFrom(buf)
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
		msg, err := decodeData(buf[:n], udpAddr.IP.String())
		if err != nil {
			log.Printf("Error decoding data %v", err)
		}
		log.Println("msg ", string(msg.Message), "from ", msg.Ip)

		s.msgChan <- *msg
	}
}

func (s *reliableUnicast) SendMessage(ip string, uuid string, data []byte) {
	var currFrame uint64
	// var n int
	s.mu.Lock()
	s.frame += 1
	currFrame = s.frame
	// inProgress := s.isSending
	s.msgBuf[currFrame] = &UnicastMessage{
		frame:   s.frame,
		tries:   0,
		uuid:    uuid,
		Ip:      ip,
		Message: data,
	}
	encodedData, err := encodeData(*s.msgBuf[currFrame])
	if err != nil {
		log.Panic(err)
	}
	s.mu.Unlock()

	// for len(s.msgChan) > 0 {
	// 	<-s.msgChan
	// }

	// if inProgress {
	// 	for {
	// 		s.mu.Lock()
	// 		n = len(s.msgBuf)
	// 		s.mu.Unlock()
	// 		log.Println("inProgress", n)
	// 		if n == 0 {
	// 			break
	// 		}
	// 		time.Sleep(50 * time.Millisecond)
	// 	}
	// 	return
	// }

	// for {
	// 	s.mu.Lock()
	// 	n = len(s.msgBuf)
	// 	s.isSending = true
	// 	if n == 0 {
	// 		s.isSending = false
	// 		s.mu.Unlock()
	// 		break
	// 	}
	// 	top := s.msgBuf[0]
	// 	encoded :=
	// 		s.mu.Unlock()
	// 	log.Println("sending", n)

	// }

	SendUDP(ip, UNI_S_PORT, UNI_L_PORT, encodedData)
}

func encodeData(msg UnicastMessage) ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.BigEndian, msg.frame); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.BigEndian, msg.tries); err != nil {
		return nil, err
	}

	uuidBytes := []byte(msg.uuid)
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

func decodeData(data []byte, ip string) (*UnicastMessage, error) {
	reader := bytes.NewReader(data)
	msg := &UnicastMessage{}

	var frame uint64
	if err := binary.Read(reader, binary.BigEndian, &frame); err != nil {
		return msg, err
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

	msg.uuid = uuidStr
	msg.Ip = ip
	msg.frame = frame
	msg.Message = strBytes

	return msg, nil
}

func (s *reliableUnicast) Shutdown() {
	close(s.quit)
	s.lConn.Close()
}
