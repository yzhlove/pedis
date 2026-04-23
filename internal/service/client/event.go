package client

import "net"

// EventType identifies what happened on a managed connection.
type EventType int

const (
	UnixConnected EventType = iota
	UnixDisconnected
	RedisConnected
	RedisDisconnected
	UnixConnectDetached
	RedisConnectDetached
	BridgeStopped
)

// Event carries state-change information from a worker to the manager.
type Event struct {
	typ  EventType
	conn net.Conn
}

// Eventer accepts asynchronous events.
type Eventer interface {
	SendEvent(e Event)
}

// State represents the current overall connectivity state.
type State int

const (
	NoneUp State = iota
	UnixUpOnly
	RedisUpOnly
	PreparingBridge
	Bridging
)

// WorkerCmdType is the set of commands the manager may send to a worker.
type WorkerCmdType int

const (
	CmdStartHeartbeat WorkerCmdType = iota
	CmdStopHeartbeat
	CmdDetachForBridge
	CmdShutdown
)

// WorkerCmd wraps a WorkerCmdType for channel delivery.
type WorkerCmd struct {
	typ WorkerCmdType
}

// Worker is the interface satisfied by each connection worker.
type Worker interface {
	Run()
	SendCmd(c WorkerCmd)
}

// WorkerMode tracks the current operating mode of a worker.
type WorkerMode int

const (
	ModeDisconnected WorkerMode = iota
	ModeConnectedIdle
	ModeHeartbeat
	ModeDetached
)
