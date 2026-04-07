package handle

import "net"

type Handler interface {
	Auth(conn net.Conn) error
	Echo(conn net.Conn) error
}
