package server

import (
	"dummy-rom/server/broadcast"
	"dummy-rom/server/unicast"
	"fmt"
)

type State string

const (
	START           State = "START"
	INIT            State = "INIT"
	LEADER          State = "LEADER"
	FOLLOWER        State = "FOLLOWER"
	BECOME_LEADER   State = "BECOME_LEADER"
	BECOME_FOLLOWER State = "BECOME_FOLLOWER"
	ELECTION        State = "ELECTION"
)

type Action func(arg *StateMachineMessage)

func WrapAction(fn func(*StateMachineMessage)) Action {
	return func(msg *StateMachineMessage) {
		fn(msg) // Pass the *StateMachineMessage to the wrapped function
	}
}

type StateMachineMessage struct {
	Broadcast  *broadcast.Message
	Unicast    *unicast.Message
	ElectionID string
}

type StateMachine struct {
	state       State
	server      ServerInterface
	transitions map[State]map[State]Action
}

func NewStateMachine(server ServerInterface) *StateMachine {
	sm := &StateMachine{
		state:       START,
		server:      server,
		transitions: make(map[State]map[State]Action),
	}

	sm.transitions[START] = map[State]Action{
		INIT: WrapAction(sm.server.StartToInit),
	}

	sm.transitions[INIT] = map[State]Action{
		BECOME_FOLLOWER: sm.server.InitToBecomeFollower,
		LEADER:          sm.server.InitToLeader,
	}

	sm.transitions[BECOME_FOLLOWER] = map[State]Action{
		INIT:     sm.server.BecomeFollowerToInit,
		FOLLOWER: sm.server.BecomeFollowerToFollower,
	}

	sm.transitions[LEADER] = map[State]Action{
		INIT: sm.server.LeaderToInit,
	}

	sm.transitions[FOLLOWER] = map[State]Action{
		ELECTION: sm.server.FollowerToElection,
		FOLLOWER: sm.server.FollowerToFollower,
	}
	sm.transitions[BECOME_LEADER] = map[State]Action{
		LEADER: sm.server.BecomeLeaderToLeader,
	}
	sm.transitions[ELECTION] = map[State]Action{
		FOLLOWER:      sm.server.ElectionToFollower,
		BECOME_LEADER: sm.server.ElectionToBecomeLeader,
		ELECTION:      sm.server.ElectionToElection,
	}

	return sm
}

func (sm *StateMachine) ChangeTo(state State, args *StateMachineMessage) {
	if action, ok := sm.transitions[sm.state][state]; ok {
		sm.state = state
		go action(args)
	} else {
		fmt.Println("Invalid transition", sm.state, state)
	}
}
