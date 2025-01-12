package multicast

import (
	"log"
	"testing"

	"github.com/google/uuid"
)

func TestEncodeDecode(t *testing.T) {
	uuidStr := uuid.NewString()

	uuidStr1 := uuid.NewString()
	uuidStr2 := uuid.NewString()

	clock1 := uint32(22)
	clock2 := uint32(23)

	msg := Message{
		IP:          "192.168.0.0",
		UUID:        uuidStr,
		VectorClock: map[string]uint32{uuidStr1: clock1, uuidStr2: clock2},
		Message:     []byte("hello"),
	}

	data, err := encodeMulticastMessage(msg)
	if err != nil {
		t.Fatalf("failed to encode multicast message %v", err)
	}
	decodedMsg, err := decodeMulticastMessage(data, msg.IP)
	if err != nil {
		t.Fatalf("failed to decode multicast message %v", err)
	}
	if decodedMsg.IP != msg.IP {
		t.Fatalf("IP mismatch")
	}
	if decodedMsg.UUID != msg.UUID {
		t.Fatalf("uuid mismatch")
	}
	if string(decodedMsg.Message) != string(msg.Message) {
		t.Fatalf("message mismatch")
	}

	for key, value := range msg.VectorClock {
		log.Println(key)
		_, ok := decodedMsg.VectorClock[key]
		if !ok {
			t.Fatalf("vector mismatch")
		}
		clock := decodedMsg.VectorClock[key]
		if clock != value {
			t.Fatalf("vector mismatch")
		}

	}
}
