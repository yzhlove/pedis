package server

import (
	"fmt"
	"net"

	"github.com/yzhlove/peids/app/handle"
	"github.com/yzhlove/peids/app/internal/packet"
	"google.golang.org/protobuf/proto"
)

type Codec interface {
	Encode(cmd handle.Cmd, message proto.Message) ([]byte, error)
	Decode(payload []byte) (handle.Cmd, proto.Message, error)
}

type Handler interface {
	Auth(msg proto.Message) (proto.Message, error)
	Heartbeat(msg proto.Message) (proto.Message, error)
}

func Handle(h Handler, conn net.Conn) error {
	payload, err := packet.Unpack(conn)
	if err != nil {
		return err
	}
	cmd, msg, err := h.(Codec).Decode(payload)
	if err != nil {
		return err
	}

	if msg, err = router(h, cmd, msg); err != nil {
		return err
	}

	if payload, err = h.(Codec).Encode(cmd, msg); err != nil {
		return err
	}
	return packet.Pack(conn, payload)
}

func router(h Handler, cmd handle.Cmd, msg proto.Message) (proto.Message, error) {
	switch cmd {
	case handle.AuthCmd:
		return h.Auth(msg)
	case handle.HeartbeatCmd:
		return h.Heartbeat(msg)
	}
	return nil, fmt.Errorf("router: unknown command:%d", cmd)
}
