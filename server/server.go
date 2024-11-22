package server

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"time"
)

const BROADCAST_PORT = ":5000"

type Server struct {
	ip          string
	broadcastIP string
}

func LocalIP() (*net.IPNet, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet, nil
		}
	}
	return nil, errors.New("IP not found")
}

func BroadcastIP(ipv4 *net.IPNet) (net.IP, error) {
	if ipv4.IP.To4() == nil {
		return net.IP{}, errors.New("does not support IPv6 addresses.")
	}
	ip := make(net.IP, len(ipv4.IP.To4()))
	binary.BigEndian.PutUint32(ip, binary.BigEndian.Uint32(ipv4.IP.To4())|^binary.BigEndian.Uint32(net.IP(ipv4.Mask).To4()))
	return ip, nil
}

func NewServer() (*Server, error) {
	ip, err := LocalIP()
	if err != nil {
		return &Server{}, err
	}
	broadcast, err := BroadcastIP(ip)
	if err != nil {
		return &Server{}, err
	}
	return &Server{ip.String(), broadcast.String()}, nil
}

func (s *Server) StartBroadcastListener(stopCh <-chan struct{}) {
	pc, err := net.ListenPacket("udp4", BROADCAST_PORT)
	if err != nil {
		log.Panic(err)
	}
	defer pc.Close()

	buf := make([]byte, 1024)

	for {
		select {
		case <-stopCh:
			log.Println("Broadcast listener shutting down.")
			return
		default:
			// Set a read deadline to allow periodic checks of the stop channel
			if err := pc.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
				log.Printf("Failed to set read deadline: %v\n", err)
			}

			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					// Timeout is expected; check the stop channel and continue
					continue
				}
				log.Printf("Error reading from connection: %v\n", err)
				continue
			}

			log.Printf("Received from %s: %s\n", addr, buf[:n])
		}
	}
}

func (s *Server) BroadcastHello() {
	pc, err := net.ListenPacket("udp4", BROADCAST_PORT)
	if err != nil {
		log.Panic(err)
	}
	defer pc.Close()

	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s%s", s.broadcastIP, BROADCAST_PORT))
	if err != nil {
		log.Panic(err)
	}

	_, err = pc.WriteTo([]byte(fmt.Sprintf("hello from %s", s.ip)), addr)
	if err != nil {
		log.Panic(err)
	}
	log.Println("Sent hello")
}
