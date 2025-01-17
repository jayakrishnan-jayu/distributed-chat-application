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
	return &Broadcaster{
		id:              serverUUID,
		ip:              serverIP,
		broadcastIP:     broadcastIP,
		msgChan:         msgChan,
		internalMsgChan: make(chan *Message),
		logger:          log.New(os.Stdout, fmt.Sprintf("[%s][%s][Broadcaster]", serverIP, serverUUID[:4]), log.Ltime),
		quit:            make(chan interface{}),
		stopChan:        make(chan struct{}),
	}
}

func (b *Broadcaster) StartListener() {
	b.logger.Println("start listner")
	go func() {
		var err error
		if b.quit == nil {
			b.quit = make(chan interface{})
		}

		b.conn, err = net.ListenPacket("udp4", BROADCAST_L_PORT)
		if err != nil {
			b.logger.Panic(err)
		}
		defer b.conn.Close()

		buf := make([]byte, 1024)

		for {
			n, addr, err := b.conn.ReadFrom(buf)
			if err != nil {
				select {
				case <-b.quit:
					b.quit = nil
					b.logger.Println("Broadcast Connection closed", n)
					return
				default:
					b.conn.Close()
					b.logger.Printf("Error reading from connection: %v\n", err)
					return
				}
			}
			udpAddr, ok := addr.(*net.UDPAddr)
			if !ok {
				b.logger.Printf("Unexpected address type: %T", addr)
				continue
			}

			// if broadcast from self, discard
			if udpAddr.IP.String() == b.ip {
				continue
			}

			clientUUIDStr := string(buf[:n])
			ip := udpAddr.IP.String()

			_, err = uuid.Parse(clientUUIDStr)
			if err != nil {
				log.Printf("Invalid UUID received from %s: %v\n", ip, err)
				break
			}

			if clientUUIDStr == b.id {
				continue
			}
			b.mu.Lock()
			listening := b.listening
			b.mu.Unlock()
			if listening {
				b.internalMsgChan <- &Message{clientUUIDStr, ip}
			}
		}
	}()
}

func (b *Broadcaster) listeningToBroadcast() {
	b.mu.Lock()
	b.listening = true
	b.logger.Println("set it as true")
	b.mu.Unlock()
}

func (b *Broadcaster) notListeningToBroadcast() {
	b.mu.Lock()
	b.listening = false
	b.mu.Unlock()
}

func (b *Broadcaster) IsAnyOneBroadcasting(duration time.Duration) (bool, *Message) {
	b.listeningToBroadcast()
	t := time.NewTimer(duration)
	defer t.Stop()
	select {
	case <-t.C:
		b.notListeningToBroadcast()
		return false, nil
	case msg := <-b.internalMsgChan:
		b.notListeningToBroadcast()
		b.logger.Println("got broadcast", msg)
		return true, msg
	}
}

func (b *Broadcaster) StartIsAnotherLeaderBroadcasting(id string) *Message {
	b.anotherLeaderQuit = make(chan interface{})
	b.listeningToBroadcast()
	for {
		select {
		case <-b.anotherLeaderQuit:
			b.anotherLeaderQuit = nil
			return nil
		case msg := <-b.internalMsgChan:
			if msg.UUID == id {
				break
			}
			return msg
		}
	}
}

func (b *Broadcaster) StopIsAnotherLeaderBroadcasting() {
	close(b.anotherLeaderQuit)
	b.notListeningToBroadcast()
}

func (b *Broadcaster) StartHeartbeatListener(quit chan interface{}) {
	b.anotherLeaderQuit = make(chan interface{})
	b.listeningToBroadcast()
	b.logger.Println("listening for broadcast hb")
	go func() {
		for {
			select {
			case <-quit:
				b.notListeningToBroadcast()
				return
			case msg := <-b.internalMsgChan:
				b.msgChan <- msg
			}
		}
	}()
}

func (b *Broadcaster) HigherNodeBroadcasting(id string, msgChan chan *Message, quit chan interface{}) {
	b.anotherLeaderQuit = make(chan interface{})
	b.listeningToBroadcast()
	b.logger.Println("listening for broadcast higher node")
	go func() {
		for {
			select {
			case <-quit:
				b.notListeningToBroadcast()
				return
			case msg := <-b.internalMsgChan:
				if msg.UUID > id {
					msgChan <- msg
				}
			}
		}
	}()
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

func (b *Broadcaster) Shutdown() {
	if b.quit != nil {
		close(b.quit)
		b.conn.Close()
	}
	b.Stop()
}
