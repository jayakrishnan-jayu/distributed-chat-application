package tests

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"
)

type MockPacketConn struct {
	addr        *net.UDPAddr
	Router      *MockRouter
	readBuffer  chan Packet
	writeBuffer map[string]*bytes.Buffer
	mu          sync.Mutex
}

type Packet struct {
	Data []byte
	Addr net.Addr
}

type MockRouter struct {
	connections map[string]*MockPacketConn
	mu          sync.Mutex
}

func NewMockRouter() *MockRouter {
	return &MockRouter{
		connections: make(map[string]*MockPacketConn),
	}
}

func (r *MockRouter) Register(conn *MockPacketConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	conn.Router = r
	r.connections[conn.LocalAddr().String()] = conn
}

func (r *MockRouter) Send(toAddr string, data []byte, fromAddr net.Addr) {
	r.mu.Lock()
	defer r.mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	if conn, ok := r.connections[toAddr]; ok {
		conn.readBuffer <- Packet{
			Data: data,
			Addr: fromAddr,
		}
	}
}

func NewMockPacketConn(addr string, port int) *MockPacketConn {
	return &MockPacketConn{
		addr:        &net.UDPAddr{IP: net.ParseIP(addr), Port: port},
		readBuffer:  make(chan Packet, 100),
		writeBuffer: make(map[string]*bytes.Buffer),
	}
}

func (m *MockPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	packet, ok := <-m.readBuffer
	if !ok {
		return 0, nil, net.ErrClosed
	}
	copy(p, packet.Data)
	return len(packet.Data), packet.Addr, nil
}

func (m *MockPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	if m.Router != nil {
		m.Router.Send(addr.String(), p, m.addr)
		return len(p), nil
	}
	return 0, fmt.Errorf("Router not set")
}

func (m *MockPacketConn) Close() error {
	close(m.readBuffer)
	return nil
}

func (m *MockPacketConn) LocalAddr() net.Addr {
	return m.addr
}

func (m *MockPacketConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *MockPacketConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *MockPacketConn) SetWriteDeadline(t time.Time) error {
	return nil
}
