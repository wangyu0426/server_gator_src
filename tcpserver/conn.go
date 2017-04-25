package tcpserver

import (
	"net"
	"sync"
	"../proto"
)

type Connection struct {
	conn *net.TCPConn
	closeOnce sync.Once
	closeChan chan struct{}
	requestChan chan  *proto.MsgData
	responseChan chan  *proto.MsgData
}

func newConn(conn *net.TCPConn)  *Connection{
	return &Connection{
		conn: conn,
		closeChan: make(chan struct{}),
		requestChan: make(chan *proto.MsgData, 128),
		responseChan: make(chan *proto.MsgData, 128),
	}
}