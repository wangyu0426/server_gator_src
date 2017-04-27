package tcpserver

import (
	"net"
	"sync"
	"../proto"
	"sync/atomic"
)

type Connection struct {
	closeFlag int32
	imei uint64
	conn *net.TCPConn
	closeOnce sync.Once
	closeChan chan struct{}
	requestChan chan  *proto.MsgData
	responseChan chan  *proto.MsgData
}

var TcpClientTable map[uint64]*Connection

func newConn(conn *net.TCPConn)  *Connection{
	return &Connection{
		conn: conn,
		closeChan: make(chan struct{}),
		requestChan: make(chan *proto.MsgData, 1024),
		responseChan: make(chan *proto.MsgData, 1024),
	}
}

// IsClosed indicates whether or not the connection is closed
func (c *Connection) IsClosed() bool {
	return atomic.LoadInt32(&c.closeFlag) == 1
}

// Close closes the connection
func (c *Connection) SetClosed() {
	atomic.StoreInt32(&c.closeFlag, 1)
}