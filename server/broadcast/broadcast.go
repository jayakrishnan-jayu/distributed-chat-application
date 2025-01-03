package broadcast

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type Broadcaster struct {
	id                     string
	ip                     string
	broadcastIP            string
	listenerPort           string
	senderPort             string
	handleBroadcastMessage func(string, string)
	stopping               bool
	stopChan               chan struct{}
	logger                 *log.Logger
	conn                   net.PacketConn
	quit                   chan interface{}
	mu                     sync.Mutex
	waitGroup              sync.WaitGroup
}

func NewBroadcaster(serverUUID string, serverIP string, broadcastIP string, listenerPort string, senderPort string, handleBroadcastMessage func(string, string)) *Broadcaster {
	return &Broadcaster{
		id:                     serverUUID,
		ip:                     serverIP,
		broadcastIP:            broadcastIP,
		listenerPort:           listenerPort,
		senderPort:             senderPort,
		handleBroadcastMessage: handleBroadcastMessage,
		logger:                 log.New(os.Stdout, fmt.Sprintf("[%s][%s][Broadcaster]", serverIP, serverUUID[:4]), log.Ltime),
		quit:                   make(chan interface{}),
		stopChan:               make(chan struct{}),
	}
}

func (b *Broadcaster) StartListener() {
	var err error

	b.conn, err = net.ListenPacket("udp4", b.listenerPort)
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
				b.logger.Println("Broadcast Connection closed")
				return
			default:
				b.logger.Printf("Error reading from connection: %v\n", err)
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

		go b.handleBroadcastMessage(string(buf[:n]), udpAddr.IP.String())
	}
}

func (b *Broadcaster) Start() {
	b.logger.Println("Start")
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
				SendUDP(b.broadcastIP, b.senderPort, b.listenerPort, []byte(b.id))
				time.Sleep(1 * time.Second)
			}
		}
	}()
}
func (b *Broadcaster) Stop() {
	b.mu.Lock()
	if b.stopping {
		b.mu.Unlock()
		return
	}
	b.stopping = true
	close(b.stopChan)
	b.mu.Unlock()

	b.waitGroup.Wait()
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
	close(b.quit)
	b.Stop()
	b.conn.Close()
}
