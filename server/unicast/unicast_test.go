package unicast_test

import (
	"dummy-rom/server/tests"
	"dummy-rom/server/unicast"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/google/uuid"
)

const UNI_S_PORT = 5002
const UNI_L_PORT = 5003

// mockPacketConn simulates a network connection for testing

func TestNew(t *testing.T) {
	fmt.Println("testing")
	msgChan1 := make(chan *unicast.Message, 10)
	msgChan2 := make(chan *unicast.Message, 10)

	router := tests.NewMockRouter()

	listnerConn1 := tests.NewMockPacketConn("192.168.1.1", UNI_L_PORT)
	senderConn1 := tests.NewMockPacketConn("192.168.1.1", UNI_S_PORT)

	listnerConn2 := tests.NewMockPacketConn("192.168.1.2", UNI_L_PORT)
	senderConn2 := tests.NewMockPacketConn("192.168.1.2", UNI_S_PORT)

	router.Register(listnerConn1)
	router.Register(listnerConn2)
	router.Register(senderConn1)
	router.Register(senderConn2)

	uni1 := unicast.NewReliableUnicast(msgChan1, senderConn1, listnerConn1)
	uni2 := unicast.NewReliableUnicast(msgChan2, senderConn2, listnerConn2)

	uuid1 := uuid.NewString()
	uuid2 := uuid.NewString()

	go uni1.StartListener()
	go uni2.StartListener()

	helloWorld := "helloWorld"
	helloThere := "hello there"

	send := uni1.SendMessage("192.168.1.2", uuid1, []byte(helloWorld))
	if !send {
		t.Errorf("failed to send message")
	}

	for {
		select {

		case msg1 := <-msgChan1:
			log.Println("message to node 1:", msg1, string(msg1.Message) == helloWorld)
			if string(msg1.Message) == helloWorld {
				log.Println("sending hello there")
				send = uni1.SendMessage("192.168.1.2", uuid1, []byte(helloThere))
				log.Println("send hello there", send)
				if !send {
					t.Error("failed to send message")
				}
			}

		case msg2 := <-msgChan2:
			log.Println("message to node 2:", msg2)
			if string(msg2.Message) == helloWorld {
				send = uni2.SendMessage("192.168.1.1", uuid2, []byte(helloWorld))
				if !send {
					t.Error("failed to send message")
				}
				continue
			}
			if string(msg2.Message) == helloThere {
				log.Println("helloThere recieved")
				return
			}
		case <-time.After(time.Second):
			t.Error("Timeout waiting for message")
		}
	}

	log.Println("done")
}
