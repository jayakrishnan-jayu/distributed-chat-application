package main

import (
	"dummy-rom/server"
	"encoding/binary"
	"errors"
	"log"
	"net"
	"time"
)

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

func main() {
	ip, err := LocalIP()
	if err != nil {
		log.Panic(err)
	}
	broadcast, err := BroadcastIP(ip)
	if err != nil {
		log.Panic(err)
	}
	s := server.NewServer(ip.String(), broadcast.String())
	log.Println(ip.String())
	s.BroadcastHello()
	stopCh := make(chan struct{})

	// Start the broadcast listener in a goroutine
	go s.StartBroadcastListener(stopCh)

	// Simulate some other work
	time.Sleep(5 * time.Second)

	// Signal the listener to stop and wait for cleanup
	close(stopCh)
	time.Sleep(1 * time.Second)

}
