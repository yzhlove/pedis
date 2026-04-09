package client

import (
	"net"

	"github.com/yzhlove/peids/app/handle"
	"github.com/yzhlove/peids/app/internal/packet"
	"google.golang.org/protobuf/proto"
)

type Codec interface {
	Encode(cmd handle.Cmd, message proto.Message) ([]byte, error)
	Decode(cmd handle.Cmd, payload []byte) (proto.Message, error)
}

type (
	reqFunc  func() (proto.Message, error)
	respFunc func(msg proto.Message) error
)

func doHandel(c Codec, cmd handle.Cmd, conn net.Conn, req reqFunc, resp respFunc) error {
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

func Auth(c Codec, conn net.Conn) error {
	cli := c.(*client)
	return doHandel(c, handle.AuthCmd, conn, cli.reqIke, cli.respIke)
}

func Heartbeat(c Codec, conn net.Conn) error {
	cli := c.(*client)
	return doHandel(c, handle.HeartbeatCmd, conn, cli.reqEcho, cli.respEcho)
}
