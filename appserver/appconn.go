package appserver

import (
	"sync"
	"../proto"
	"../models"
	"sync/atomic"
	"gopkg.in/kataras/iris.v6/adaptors/websocket"
)

type AppConnection struct {
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

var AppClientTable map[uint64]map[string]*AppConnection

func newAppConn(conn *websocket.Connection)  *AppConnection{
	return &AppConnection{
		conn: conn,
		closeChan: make(chan struct{}),
		requestChan: make(chan []byte, 1024),
		responseChan: make(chan *proto.AppMsgData, 1024),
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