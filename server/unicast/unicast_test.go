package unicast_test

import (
	"dummy-rom/server/tests"
	"dummy-rom/server/unicast"
	"log"
	"testing"
	"time"

	"github.com/google/uuid"
)

const UNI_S_PORT = 5002
const UNI_L_PORT = 5003

// mockPacketConn simulates a network connection for testing

func TestNew(t *testing.T) {
	msgChan1 := make(chan *unicast.Message, 10)
	msgChan2 := make(chan *unicast.Message, 10)

	router := tests.NewMockRouter()

	listnerConn1 := tests.NewMockPacketConn("192.168.1.1", UNI_L_PORT, router)
	senderConn1 := tests.NewMockPacketConn("192.168.1.1", UNI_S_PORT, router)

	listnerConn2 := tests.NewMockPacketConn("192.168.1.2", UNI_L_PORT, router)
	senderConn2 := tests.NewMockPacketConn("192.168.1.2", UNI_S_PORT, router)

	uuid1 := uuid.NewString()
	uuid2 := uuid.NewString()

	uni1 := unicast.NewReliableUnicast(msgChan1, uuid1, senderConn1, listnerConn1)
	uni2 := unicast.NewReliableUnicast(msgChan2, uuid2, senderConn2, listnerConn2)

	go uni1.StartListener()
	go uni2.StartListener()

	helloWorld := "helloWorld"
	helloThere := "hello there"

	if !uni1.SendMessage("192.168.1.2", uuid2, []byte(helloWorld)) {
		t.Errorf("failed to send message")
	}

	for i := 0; i < 3; i++ {
		select {
		case msg1 := <-msgChan1:
			// log.Println("message to node 1:", msg1, string(msg1.Message) == helloWorld)
			if string(msg1.Message) == helloWorld {
				if !uni1.SendMessage("192.168.1.2", uuid2, []byte(helloThere)) {
					t.Error("failed to send message")
				}
			}

		case msg2 := <-msgChan2:
			// log.Println("message to node 2:", msg2)
			if string(msg2.Message) == helloWorld {
				if !uni2.SendMessage("192.168.1.1", uuid1, []byte(helloWorld)) {
					t.Error("failed to send message")
				}
				continue
			}
			if string(msg2.Message) == helloThere {
				// log.Println("helloThere recieved")
			}
		case <-time.After(time.Second):
			t.Error("Timeout waiting for message")
		}
	}
	uni1.Shutdown()
	time.Sleep(200 * time.Millisecond)

	bye := "bye"
	// goodBye := "good bye"

	if uni2.SendMessage("192.168.1.1", uuid1, []byte(bye)) {
		t.Error("Message send to a closed node")
	}
	log.Println("testing communcation after node failure")

	// case in which existing node sends to a respawned node
	// the respawned node will have a new uuid
	if uni2.SendMessage("192.168.1.1", uuid1, []byte(bye)) {
		t.Error("old uuid should not have been accepted")
	}

	uuid1 = uuid.NewString()
	listnerConn1 = tests.NewMockPacketConn("192.168.1.1", UNI_L_PORT, router)
	senderConn1 = tests.NewMockPacketConn("192.168.1.1", UNI_S_PORT, router)

	msgChan1 = make(chan *unicast.Message)
	uni1 = unicast.NewReliableUnicast(msgChan1, uuid1, senderConn1, listnerConn1)
	go uni1.StartListener()

	if !uni2.SendMessage("192.168.1.1", uuid1, []byte(bye)) {
		t.Error("failed to send to a respawned node")
	}
	select {
	case msg1 := <-msgChan1:
		log.Println("message to node 1:", msg1)
	case msg2 := <-msgChan2:
		log.Println("message to node 2:", msg2)
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for message")
	}

	log.Println("done")

}
