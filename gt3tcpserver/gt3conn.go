package gt3tcpserver

import (
	"net"
	"sync"
	"../proto"
	"sync/atomic"
	"encoding/binary"
)

type Connection struct {
	connid uint64
	closeFlag int32
	saved bool
	imei uint64
	IP   uint32
	Port int
	lastActiveTime int64
	lastPushFileNumTime int64
	conn *net.TCPConn
	buf []byte
	recvEndPosition int
	closeOnce sync.Once
	closeChan chan struct{}
	requestChan chan  *proto.MsgData
	responseChan chan  *proto.MsgData
}

func newConn(conn *net.TCPConn)  *Connection{
	addr,  _:= net.ResolveTCPAddr("tcp", conn.RemoteAddr().String())
	return &Connection{
		connid: proto.NewMsgID(),
		conn: conn,
		IP:  binary.BigEndian.Uint32(addr.IP.To4()),
		Port: addr.Port,
		closeChan: make(chan struct{}),
		requestChan: make(chan *proto.MsgData, 20*1024),
		responseChan: make(chan *proto.MsgData, 20*1024),
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