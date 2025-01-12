package message

import (
	"bytes"
	"encoding/binary"
	"log"
)

type MessageType uint8

const (
	ConnectToLeader MessageType = iota
	Election
	ElectionAlive
	ElectionVictory
	Heartbeat
	PeerInfo
)

type Message struct {
	Type    MessageType
	PeerIds []string
	PeerIps []string
	Clock   []uint32
}

func NewConnectToLeaderMessage() []byte {
	em, err := Encode(Message{Type: ConnectToLeader})
	if err != nil {
		log.Panic(err)
	}
	return em
}

func NewElectionMessage() []byte {
	em, err := Encode(Message{Type: Election})
	if err != nil {
		log.Panic(err)
	}
	return em
}

func NewElectionVictoryMessage() []byte {
	em, err := Encode(Message{Type: ElectionVictory})
	if err != nil {
		log.Panic(err)
	}
	return em
}

func NewElectionAliveMessage() []byte {
	em, err := Encode(Message{Type: ElectionAlive})
	if err != nil {
		log.Panic(err)
	}
	return em
}

func NewHeartbeatMessage() []byte {
	em, err := Encode(Message{Type: Heartbeat})
	if err != nil {
		log.Panic(err)
	}
	return em
}

func NewPeerInfoMessage(PeerIds []string, PeerIps []string, Clock []uint32) []byte {
	em, err := Encode(Message{Type: PeerInfo, PeerIds: PeerIds, PeerIps: PeerIps, Clock: Clock})
	if err != nil {
		log.Panic(err)
	}
	return em
}

func Encode(msg Message) ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.BigEndian, uint8(msg.Type)); err != nil {
		return nil, err
	}
	switch msg.Type {
	case ConnectToLeader:
	case Election:
	case ElectionVictory:
	case ElectionAlive:
	case Heartbeat:
		return buf.Bytes(), nil
	case PeerInfo:
		if err := binary.Write(&buf, binary.BigEndian, uint32(len(msg.PeerIds))); err != nil {
			return nil, err
		}
		log.Println("index", len(msg.Clock), len(msg.PeerIds))
		for index := range len(msg.PeerIds) {
			id := msg.PeerIds[index]
			ip := msg.PeerIps[index]
			clock := msg.Clock[index]

			if err := binary.Write(&buf, binary.BigEndian, uint32(len(id))); err != nil {
				return nil, err
			}
			buf.WriteString(id)
			if err := binary.Write(&buf, binary.BigEndian, uint32(len(ip))); err != nil {
				return nil, err
			}
			buf.WriteString(ip)
			if err := binary.Write(&buf, binary.BigEndian, uint32(clock)); err != nil {
				return nil, err
			}
		}

	}

	return buf.Bytes(), nil
}

func Decode(data []byte) (*Message, error) {
	reader := bytes.NewReader(data)
	msg := &Message{}
	var Type uint8
	if err := binary.Read(reader, binary.BigEndian, &Type); err != nil {
		return nil, err
	}

	// Convert back to MessageType
	msg.Type = MessageType(Type)

	switch msg.Type {
	case ConnectToLeader:
	case Election:
	case ElectionAlive:
	case ElectionVictory:
	case Heartbeat:
		// No additional data for ConnectToLeader
		return msg, nil
	case PeerInfo:
		// Decode PeerIds
		var count uint32
		if err := binary.Read(reader, binary.BigEndian, &count); err != nil {
			return nil, err
		}
		msg.PeerIds = make([]string, count)
		msg.PeerIps = make([]string, count)
		msg.Clock = make([]uint32, count)

		for index := range count {
			var idLen uint32
			if err := binary.Read(reader, binary.BigEndian, &idLen); err != nil {
				return nil, err
			}
			id := make([]byte, idLen)
			if _, err := reader.Read(id); err != nil {
				return nil, err
			}
			msg.PeerIds[index] = string(id)
			if err := binary.Read(reader, binary.BigEndian, &idLen); err != nil {
				return nil, err
			}
			ip := make([]byte, idLen)
			if _, err := reader.Read(ip); err != nil {
				return nil, err
			}
			msg.PeerIps[index] = string(ip)
			if err := binary.Read(reader, binary.BigEndian, &idLen); err != nil {
				return nil, err
			}
			msg.Clock[index] = idLen
		}

		// Decode PeerIps
	}

	return msg, nil
}
