package client

import "io"

type EventType int

const (
	EvUnixConnected EventType = iota
	EvUnixDisconnected
	EvRedisConnected
	EvRedisDisconnected
	EvUnixConnectDetached
	EvRedisConnectDetached
	EvBridgeStopped
)

type Event struct {
	typ EventType
	rwc io.ReadWriteCloser
}

type Eventer interface {
	SendEvent(e Event)
}

type State int

const (
	StateNoneUp State = iota
	StateUnixUpOnly
	StateRedisUpOnly
	StatePreparingBridge
	StateBridging
)

type WorkerCmdType int

const (
	CmdStartHeartbeat WorkerCmdType = iota
	CmdStopHeartbeat
	CmdDetachForBridge
	CmdShutdown
)

type WorkerCmd struct {
	typ WorkerCmdType
}

type Worker interface {
	Run()
	SendCmd(c WorkerCmd)
}

type WorkerMode int

const (
	WorkerModeDisconnected WorkerMode = iota
	WorkerModeConnectedIdle
	WorkerModeHeartbeat
	WorkerModeDetached
)
