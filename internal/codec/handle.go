package codec

import (
	"net"

	"github.com/yzhlove/pedis/internal/packet"
	"google.golang.org/protobuf/proto"
)

const (
	msgNegotiate = "pedis-negotiate-message"
	msgHandshake = "pedis-handshake-message"
)

type (
	reqFunc  func() (proto.Message, error)
	respFunc func(msg proto.Message) error
)

func doHandle(c ClientCodec, cmd Cmd, conn net.Conn, req reqFunc, resp respFunc) error {
	msg, err := req()
	if err != nil {
		return err
	}
	payload, err := c.Encode(cmd, msg)
	if err != nil {
		return err
	}
	if err = packet.Pack(conn, payload); err != nil {
		return err
	}

	if payload, err = packet.Unpack(conn); err != nil {
		return err
	}

	msg, err = c.Decode(cmd, payload)
	if err != nil {
		return err
	}
	return resp(msg)
}

// Auth performs the IKE authentication handshake over conn using codec c.
func Auth(c ClientCodec, conn net.Conn) error {
	cli := c.(*clientCodec)
	return doHandle(c, AuthCmd, conn, cli.reqIKE, cli.respIKE)
}

// Heartbeat sends a PING/PONG exchange over conn using codec c.
func Heartbeat(c ClientCodec, conn net.Conn) error {
	cli := c.(*clientCodec)
	return doHandle(c, HeartbeatCmd, conn, cli.reqEcho, cli.respEcho)
}

func Hello(c ClientCodec, conn net.Conn) error {
	cli := c.(*clientCodec)
	return doHandle(c, HelloCmd, conn, cli.reqHello, cli.respHello)
}

func Free(c ClientCodec, conn net.Conn) error {
	cli := c.(*clientCodec)
	return doHandle(c, FreeCmd, conn, cli.reqFree, cli.respFree)
}
