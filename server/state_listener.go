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
	case ELECTION:
		st.ELectionStateStart()
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

		t := time.NewTimer(5 * time.Second)
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
					if time.Now().Sub(lastHB) > 1000*time.Millisecond {
						st.logger.Println("dead node last hb", time.Now().Sub(lastHB), uuid, st.server.peers[uuid])
						// delete(peers, uuid)
						go st.server.handleDeadServer(uuid)
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
		st.server.broadcaster.StartBroadcastListener(higherNodeBChan, st.quit)
		for {
			select {
			case <-st.quit:
				st.quit = nil
				st.logger.Println("quitting leader unicast listner")
				return

			case msg := <-higherNodeBChan:
				if msg.UUID < leaderID {
					continue
				}
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

	msgChan := make(chan *broadcast.Message, 5)
	// listen for heartbeat from leader broadcast
	st.server.broadcaster.StartBroadcastListener(msgChan, st.quit)

	go func(msgChan chan *broadcast.Message) {
		t := time.NewTicker(2 * time.Second)
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
					st.server.mu.Lock()
					currentLeaderID := st.server.leaderID
					st.server.mu.Unlock()
					if currentLeaderID == leaderID {
						st.server.sm.ChangeTo(ELECTION, nil)
						return
					}
					st.logger.Println("old leader heartbeat, not going to election because of new leader")
					return
				}
				t.Reset(1 * time.Second)

			case msg := <-msgChan:
				if msg.UUID == leaderID {
					lastHeartBeat = time.Now()
				}
			}
		}
	}(msgChan)
}

func (st *ServerStateListener) ELectionStateStart() {
	st.server.mu.Lock()
	st.server.electionMap = map[string]bool{}
	for uuid, ip := range st.server.peers {
		if uuid != st.server.id && uuid > st.server.id {
			st.server.electionMap[uuid] = false
			go func(uuid string, ip string) {
				st.server.logger.Println("sending election message to higher node", ip)
				send := st.server.ru.SendMessage(ip, uuid, message.NewElectionMessage())
				if send {
					st.server.logger.Println("successfulyy send to", ip)
					st.server.mu.Lock()
					st.server.electionMap[uuid] = true
					st.server.logger.Println("updated map", ip)
					st.server.mu.Unlock()
				}
			}(uuid, ip)
		}
	}
	st.server.mu.Unlock()

	st.server.logger.Println("waiting for reply")
	time.Sleep(1)
	st.server.mu.Lock()
	st.server.logger.Println("checking if we are still in election")
	if st.server.state != ELECTION {
		st.server.logger.Println("not in eleciton anymore", st.server.state, st.server.leaderID)
		st.server.mu.Unlock()
		return
	}
	recvdAliveFromHigherNode := false
	for _, rcvd := range st.server.electionMap {
		if rcvd {
			st.server.logger.Println("higher node exists, waiting for election victory")
			recvdAliveFromHigherNode = true
			break
		}
	}
	st.server.mu.Unlock()
	if recvdAliveFromHigherNode {
		st.server.logger.Println("recievd alive from higher node, waitin for one more second other wise going to election again")
		time.Sleep(1)
		st.server.mu.Lock()
		if st.server.state == ELECTION {
			st.server.mu.Unlock()
			st.server.logger.Println("still in election restarting election")
			st.server.sm.ChangeTo(ELECTION, nil)
			return
		}
		st.server.mu.Unlock()
		return
	}
	st.server.logger.Println("no other potential leader, becomeing leader")
	st.server.sm.ChangeTo(BECOME_LEADER, nil)
}
