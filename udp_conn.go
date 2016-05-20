package quic

import "net"

type connection interface {
	write([]byte) error
	setCurrentRemoteAddr(interface{})
}

type udpConn struct {
	conn        *net.UDPConn
	currentAddr *net.UDPAddr
	server      *Server
}

var _ connection = &udpConn{}

func (c *udpConn) write(p []byte) error {
	c.server.packetsToSend <- packetToSend{c.currentAddr, p}
	// _, err := c.conn.WriteToUDP(p, c.currentAddr)
	// return err
	return nil
}

func (c *udpConn) setCurrentRemoteAddr(addr interface{}) {
	c.currentAddr = addr.(*net.UDPAddr)
}
