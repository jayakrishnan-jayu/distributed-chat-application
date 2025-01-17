package server

import (
	"dummy-rom/server/broadcast"
	"dummy-rom/server/message"
	"fmt"
	"log"
	"os"
	"time"
)

type StateListener interface {
	Start()
	Stop()
}

type ServerStateListener struct {
	state  State
	server *Server
	logger *log.Logger
	quit   chan interface{}
}

func NewStateListener(state State, server *Server) *ServerStateListener {
	return &ServerStateListener{
		server: server,
		state:  state,
		logger: log.New(os.Stdout, fmt.Sprintf("[%s][%s][%s] ", server.ip, server.id[:4], state), log.Ltime),
	}
}

func (st *ServerStateListener) Start() {
	st.logger.Println("starting listner for", st.state)
	st.quit = make(chan interface{})
	switch st.state {
	case INIT:
		st.InitStateListener()
	case LEADER:
		st.LeaderStateStart()
	case FOLLOWER:
		st.FollowerStateStart()
		// case BECOME_LEADER:
		// 	st.BecomeLeaderStateStart()
	}
}

func (st *ServerStateListener) Stop() {
	if st.quit != nil {
		close(st.quit)
	}
	switch st.state {
	case LEADER:
		st.LeaderStateStop()
	}
}

func (st *ServerStateListener) InitStateListener() {
	st.server.leaderID = ""
}

func (st *ServerStateListener) LeaderStateStart() {
	st.server.leaderID = st.server.id
	leaderID := st.server.id
	st.server.broadcaster.Start()
	st.logger.Println("broadcaster started")

	go func() {
		hbChan := make(chan string, 10)
		peers := make(map[string]time.Time)
		quitChan := make(chan interface{})
		st.server.StartHearbeatListener(hbChan, quitChan)

		st.server.mu.Lock()
		for uuid, _ := range st.server.peers {
			if uuid == st.server.id {
				continue
			}
			peers[uuid] = time.Now()
		}
		st.server.mu.Unlock()

		t := time.NewTimer(1 * time.Second)
		for {
			select {
			case <-st.quit:
				st.logger.Println("quitting heartbeat listner")
				close(quitChan)
				return
			case <-t.C:
				t.Reset(500 * time.Millisecond)
				st.server.mu.Lock()
				for uuid, _ := range st.server.peers {
					if uuid == st.server.id {
						continue
					}
					lastHB, ok := peers[uuid]
					if !ok {
						//new node maybe, check again
						peers[uuid] = time.Now()
						break
					}
					st.logger.Println("node hb", time.Now().Sub(lastHB), uuid, st.server.peers[uuid])
					if time.Now().Sub(lastHB) > 700*time.Millisecond {
						st.logger.Println("dead node last hb", time.Now().Sub(lastHB), uuid, st.server.peers[uuid])
						// delete(peers, uuid)
						// go st.server.handleDeadServer(uuid)
					}
				}
				st.server.mu.Unlock()
			case uuidh := <-hbChan:
				peers[uuidh] = time.Now()
			}
		}
	}()

	go func() {
		higherNodeBChan := make(chan *broadcast.Message)
		st.server.broadcaster.HigherNodeBroadcasting(leaderID, higherNodeBChan, st.quit)
		for {
			select {
			case <-st.quit:
				st.quit = nil
				st.logger.Println("quitting leader unicast listner")
				return

			case msg := <-higherNodeBChan:
				st.logger.Println("another leader node is broadcasting", msg.UUID)
				st.server.sm.ChangeTo(INIT, &StateMachineMessage{Broadcast: msg})
				return

			case msg := <-st.server.uMsgChan:
				uMsg, err := message.Decode(msg.Message)
				if err != nil {
					st.logger.Panic(err)
				}
				switch uMsg.Type {
				case message.ConnectToLeader:
					go st.server.connectToLeaderHandler(msg.FromUUID, msg.IP)
				case message.ElectionVictory:
					msgCopy := *msg
					st.server.sm.ChangeTo(FOLLOWER, &StateMachineMessage{Unicast: &msgCopy})

				}
			}
		}
	}()

}

func (st *ServerStateListener) LeaderStateStop() {
	st.server.broadcaster.Stop()
}

func (st *ServerStateListener) FollowerStateStart() {
	leaderID := st.server.leaderID
	go st.server.StartHearbeat(st.server.peers[leaderID], st.server.id, 180*time.Millisecond, st.quit)

	st.server.broadcaster.StartHeartbeatListener(st.quit)

	go func() {
		st.logger.Println("emptying chan")
		emptyChannel(st.server.bMsgChan)
		st.logger.Println("done emptying chan")
		t := time.NewTicker(1 * time.Second)
		lastHeartBeat := time.Now()
		defer t.Stop()
		for {
			select {
			case <-st.quit:
				st.quit = nil
				return
			case <-t.C:
				st.logger.Println("heartbeat", time.Now().Sub(lastHeartBeat))
				if time.Now().Sub(lastHeartBeat) > 500*time.Millisecond {
					st.server.sm.ChangeTo(ELECTION, nil)
					return
				}
				t.Reset(1 * time.Second)

			case msg := <-st.server.bMsgChan:
				if msg.UUID == leaderID {
					lastHeartBeat = time.Now()
				}
			case msg := <-st.server.uMsgChan:
				dMsg, _ := message.Decode(msg.Message)
				switch dMsg.Type {
				case message.ElectionVictory:
					st.server.mu.Lock()
					_, ok := st.server.peers[dMsg.UUID]
					st.server.mu.Unlock()
					st.logger.Println("go election victory from", dMsg.IP, ok)

				}
			}
		}
	}()
}

func (st *ServerStateListener) BecomeLeaderStateStart() {
	st.logger.Println("BecomeLeadeStateStart")
	vc := st.server.rm.VectorClock()
	st.logger.Println("VectorClockj")
	st.server.mu.Lock()
	st.logger.Println("server mu lock")
	peerIds := make([]string, 0, len(st.server.peers))
	peerIps := make([]string, 0, len(st.server.peers))
	clocks := make([]uint32, 0, len(st.server.peers))

	rmPort := st.server.rmPort
	for uuid, ip := range st.server.peers {
		clock, ok := vc[uuid]
		if !ok {
			st.logger.Panicf("failed to find frame count for uuid in vc: %s", uuid)
		}
		peerIds = append(peerIds, uuid)
		peerIps = append(peerIps, ip)
		clocks = append(clocks, clock)
	}
	// var wg sync.WaitGroup

	for uuid, ip := range st.server.peers {
		if uuid == st.server.id {
			continue
		}
		go func(uuid string, ip string) {
			// defer wg.Done()
			victoryMessage := message.NewElectionVictoryMessage(peerIds, peerIps, rmPort, clocks)
			st.server.logger.Println("sending election victory to", ip)
			send := st.server.ru.SendMessage(ip, uuid, victoryMessage)
			if !send {
				// TODO: handle node failure
				// s.handleDeadServer(uuid, ip)
				log.Panic("TODO: failed to send victory message to ", ip)
			}

			st.server.logger.Println("done sending", ip)
		}(uuid, ip)
	}
	st.logger.Println("server mu unlocking")
	st.server.mu.Unlock()

	st.logger.Println("server mu unlock")
	st.server.sm.ChangeTo(LEADER, nil)
}
