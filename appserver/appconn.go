package appserver

import (
	"../proto"
	"../models"
	"sync/atomic"
	"gopkg.in/kataras/iris.v6/adaptors/websocket"
	"sync"
)

type AppConnection struct {
	fenceIndex uint64
	closeFlag int32
	saved bool
	user models.User
	imeis []uint64
	conn *websocket.Connection
	closeOnce sync.Once
	closeChan chan struct{}
	requestChan chan  []byte
	responseChan chan  *proto.AppMsgData
}

func newAppConn(conn *websocket.Connection)  *AppConnection{
	return &AppConnection{
		//fenceIndex: fence,
		conn: conn,
		closeChan: make(chan struct{}),
		requestChan: make(chan []byte, 10240),
		responseChan: make(chan *proto.AppMsgData, 10240),
	}
}

// IsClosed indicates whether or not the connection is closed
func (c *AppConnection) IsClosed() bool {
	return atomic.LoadInt32(&c.closeFlag) == 1
}

// Close closes the connection
func (c *AppConnection) SetClosed() {
	atomic.StoreInt32(&c.closeFlag, 1)
}