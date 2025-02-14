package broadcast

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

const BROADCAST_S_PORT = ":5000"
const BROADCAST_L_PORT = ":5001"

type Broadcaster struct {
	id                string
	ip                string
	broadcastIP       string
	msgChan           chan<- *Message
	internalMsgChan   chan *Message
	stopping          bool
	stopChan          chan struct{}
	logger            *log.Logger
	conn              net.PacketConn
	anotherLeaderQuit chan interface{}
	quit              chan interface{}
	mu                sync.Mutex
	listening         bool
	waitGroup         sync.WaitGroup
}

type Message struct {
	UUID string
	IP   string
}

func NewBroadcaster(serverUUID string, serverIP string, broadcastIP string, msgChan chan<- *Message) *Broadcaster {
	logger := log.New(os.Stdout, fmt.Sprintf("[%s][%s][Broadcaster]", serverIP, serverUUID[:4]), log.Ltime)
	conn, err := net.ListenPacket("udp4", BROADCAST_L_PORT)
	if err != nil {
		logger.Panic(err)
	}
	return &Broadcaster{
		id:              serverUUID,
		ip:              serverIP,
		broadcastIP:     broadcastIP,
		msgChan:         msgChan,
		conn:            conn,
		internalMsgChan: make(chan *Message),
		logger:          log.New(os.Stdout, fmt.Sprintf("[%s][%s][Broadcaster]", serverIP, serverUUID[:4]), log.Ltime),
		quit:            make(chan interface{}),
		stopChan:        make(chan struct{}),
	}
}

func (b *Broadcaster) Shutdown() {
	b.conn.Close()
}

func (b *Broadcaster) listen(ch chan *Message, quit chan interface{}) {
	buf := make([]byte, 1024)
	b.logger.Println("New broadcast listener")
	for {
		select {
		case <-quit:
			b.logger.Println("Broadcast Connection closed")
			return
		default:
			n, addr, err := b.conn.ReadFrom(buf)
			if err != nil {
				b.logger.Printf("Error reading from connection: %v\n", n, err)
				return
			}
			udpAddr, ok := addr.(*net.UDPAddr)
			if !ok {
				b.logger.Printf("Unexpected address type: %T", addr)
				continue
			}
			ip := udpAddr.IP.String()
			clientUUIDStr := string(buf[:n])
			_, err = uuid.Parse(clientUUIDStr)
			if err != nil {
				log.Printf("Invalid UUID received from %s: %v\n", err)
				continue
			}
			if clientUUIDStr == b.id {
				continue
			}
			ch <- &Message{UUID: clientUUIDStr, IP: ip}
		}
	}
}

func (b *Broadcaster) IsAnyOneBroadcasting(duration time.Duration) (bool, *Message) {
	ch := make(chan *Message)
	quit := make(chan interface{})
	go b.listen(ch, quit)
	t := time.NewTimer(duration)
	defer t.Stop()
	select {
	case <-t.C:
		close(quit)
		return false, nil
	case msg := <-ch:
		close(quit)
		b.logger.Println("got broadcast", msg)
		return true, msg
	}
}

func (b *Broadcaster) StartBroadcastListener(msgChan chan *Message, quit chan interface{}) {
	go b.listen(msgChan, quit)
}

func (b *Broadcaster) Start() {
	b.logger.Println("Starting")
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.stopping {
		// If broadcasting is already active, stop it first
		close(b.stopChan)
		b.waitGroup.Wait()
	}

	// Reset the state for a new broadcast
	b.stopChan = make(chan struct{})
	b.stopping = false

	b.waitGroup.Add(1)
	go func() {
		defer b.waitGroup.Done()
		for {
			select {
			case <-b.stopChan:
				return
			default:
				SendUDP(b.broadcastIP, BROADCAST_S_PORT, BROADCAST_L_PORT, []byte(b.id))
				b.logger.Println("send")
				time.Sleep(200 * time.Millisecond)
			}
		}
	}()
}
func (b *Broadcaster) Stop() {
	b.logger.Println("Stopping")
	b.mu.Lock()
	if b.stopping {
		b.mu.Unlock()
		return
	}
	b.stopping = true
	close(b.stopChan)
	b.mu.Unlock()

	b.waitGroup.Wait()
	b.logger.Println("Stopped")
}

func SendUDP(ip string, fromPort string, toPort string, data []byte) {
	pc, err := net.ListenPacket("udp4", fromPort)
	if err != nil {
		log.Panic(err)
	}
	defer pc.Close()

	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s%s", ip, toPort))
	if err != nil {
		log.Panic(err)
	}

	_, err = pc.WriteTo(data, addr)
	if err != nil {
		log.Panic(err)

	}
}
