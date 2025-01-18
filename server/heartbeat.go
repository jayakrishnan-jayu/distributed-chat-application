package server

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/google/uuid"
)

const HRT_S_PORT = ":5004"
const HRT_L_PORT = ":5005"

func (s *Server) StartHearbeat(leaderIP string, serverUUID string, interval time.Duration, quit chan interface{}) {

	go func() {
		addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s%s", leaderIP, HRT_L_PORT))
		if err != nil {
			log.Panic(err)
		}
		data := []byte(serverUUID)

		_, err = s.hbSConn.WriteTo(data, addr)
		if err != nil {
			return
		}
		t := time.NewTimer(interval)
		defer t.Stop()
		s.logger.Println("sending heartbeat to", leaderIP)
		for {
			select {
			case <-quit:
				return
			case <-t.C:
				// log.Println("sending heartbeat", leaderIP)
				_, err := s.hbSConn.WriteTo(data, addr)
				if err != nil {
					return
				}
				t.Reset(interval)
			}
		}
	}()

}

func (s *Server) StartHearbeatListener(uuidChan chan string, quit chan interface{}) {
	go func() {
		buf := make([]byte, 36)
		var uuidStr string
		for {
			select {
			case <-quit:
				return
			default:
				_, _, err := s.hbLConn.ReadFrom(buf)
				if err != nil {
					log.Printf("Error reading from connection: %v\n", err)
					return
				}
				uuidStr = string(buf)
				_, err = uuid.Parse(uuidStr)
				if err != nil {
					log.Printf("invalid heartbeat")
					break
				}
				uuidChan <- uuidStr
			}
		}
	}()
}
