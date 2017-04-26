package tcpserver

import (
	"net"
	"sync"
	"../proto"
)

type Connection struct {
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