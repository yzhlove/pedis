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
	err error
	rwc io.ReadWriteCloser
}

type Eventer interface {
	SendEvent(e Event) bool
}

type State int

const (
	StateNoneUp State = iota
	StateUnixUp
	StateRedisUp
	StateBridging
)

type Cmd int

const (
	CmdNone Cmd = iota
)

type Worker interface {
	Run()
	Stop()
	SendCmd(c Cmd) bool
}
