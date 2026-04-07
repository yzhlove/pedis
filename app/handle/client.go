package handle

import "net"

type client struct{}

func (c *client) Auth(conn net.Conn) error {

	return nil
}
