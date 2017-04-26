package tcpserver

import (
	"../logger"
	"../svrctx"
	"../proto"
	"net"
	"fmt"
	"io"
	"strconv"
	"time"
	"strings"
	"os"
)

const (
	MsgHeaderSize = 5
	MsgMinSize = 25
)

var addConnChan chan *Connection
var delConnChan chan *Connection

func init()  {
	logger.Log("tcpserver init")
	addConnChan = make(chan *Connection, 1024)
	delConnChan = make(chan *Connection, 1024)
}


func TcpServerRunLoop(serverCtx *svrctx.ServerContext)  {
	tcpaddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", serverCtx.BindAddr, serverCtx.Port ))
	if err != nil {
		logger.Log("resolve tcp address failed, " + err.Error())
		os.Exit(1)
	}

	listener, err := net.ListenTCP("tcp", tcpaddr)
	if err != nil {
		logger.Log("listen tcp address failed, " + err.Error())
		os.Exit(1)
	}

	defer func() {
		if listener != nil{
			listener.Close()
			close(addConnChan)
			close(delConnChan)
		}
		serverCtx.WaitLock.Done()
	}()

	//for managing connection, 对内负责管理tcp连接对象，对外为APP server提供通信接口
	go func() {
		for   {
			select {
			case connAdd:=<-addConnChan:
				logger.Log("new connection added")
				TcpClientTable[connAdd.imei] = connAdd
			case connDel := <-delConnChan:
				if connDel == nil {
					logger.Log("main lopp exit, connection manager goroutine exit")
					return
				}

				_, ok := TcpClientTable[connDel.imei]
				if ok {
					close(connDel.requestChan)
					close(connDel.responseChan)
					close(connDel.closeChan)
					connDel.conn.Close()

					delete(TcpClientTable, connDel.imei);
					logger.Log("connection deleted from TcpClientTable")
				}else {
					logger.Log("will delete connection from TcpClientTable, but connection not found")
				}
			case msg := <-serverCtx.TcpServerChan:
				if msg == nil {
					logger.Log("main lopp exit, connection manager goroutine exit")
					return
				}

				if msg.From == proto.MsgFromAppServer{
					logger.Log("msg from app: " + string(msg.Data))
				}else {
					logger.Log("msg from tcp: " + string(msg.Data))
				}

				c, ok := TcpClientTable[msg.Imei]
				if ok {
					c.responseChan <- msg
					logger.Log("send app data to write routine")
				}else {
					logger.Log("will send app data to tcp connection from TcpClientTable, but connection not found")
				}
			}
		}

	}()

	for  {
		listener.SetDeadline(time.Now().Add(time.Second * serverCtx.AcceptTimeout))
		conn, err := listener.AcceptTCP()
		if err != nil {
			err_ := err.(*net.OpError)
			if  strings.Contains(err_.Err.Error(), "timeout") == false {
				logger.Log("accept connection failed, " + err.Error())
				return
			}else{
				continue
			}
		}

		c := newConn(conn)
		addConnChan <- c

		//for reading
		go func(c *Connection) {
			defer c.closeOnce.Do(func() {
				logger.Log("client connection closed")
				delConnChan <- c
			})

			logger.Log("new connection from: " +  c.conn.RemoteAddr().String())

			for  {
				dataSize := uint16(0)
				recvSizeBuf := make([]byte, MsgHeaderSize)

				c.conn.SetReadDeadline(time.Now().Add(time.Second * serverCtx.RecvTimeout))
				if _, err := io.ReadFull(c.conn, recvSizeBuf); err != nil {
					logger.Log("recv MsgHeaderSize bytes header failed, " + err.Error())
					break
				}

				logger.Log("recv: " + string(recvSizeBuf))

				if recvSizeBuf[0] != '(' {
					logger.Log("bad format of data packet, it must begin with (")
					break
				}

				num, _ := strconv.ParseUint("0x" + string(recvSizeBuf[1:]), 0, 0)
				dataSize = uint16(num)
				if dataSize < MsgMinSize {
					logger.Log(fmt.Sprintf("data size in header is bad of  %d, less than %d", dataSize, MsgMinSize))
					break
				}

				logger.Log("data size: " + fmt.Sprintf("%d", dataSize))

				bufSize := dataSize - MsgHeaderSize
				dataBuf := make([]byte, bufSize)
				recvOffset := uint16(0)
				for {
					c.conn.SetReadDeadline(time.Now().Add(time.Second * serverCtx.RecvTimeout))
					n, err := c.conn.Read(dataBuf[recvOffset: ])
					if n > 0 {
						recvOffset += uint16(n)
						if recvOffset == bufSize {
							break
						}
					}else {
						logger.Log("recv data failed, " + err.Error())
						return
					}
				}

				if dataBuf[bufSize - 1] != ')' {
					logger.Log("bad format of data packet, it must end with )")
					break
				}

				//包接收完整，开始进行业务逻辑处理
				logger.Log(string(dataBuf))
				c.requestChan<-&proto.MsgData{bufSize, proto.MsgFromTcpServer, 0, 0,  dataBuf}
			}

		}(c)

		//for business handler，业务处理的协程
		go func(c *Connection) {
			for {
				select {
				case <-c.closeChan:
					logger.Log("business goroutine exit")
					return
				case data := <-c.requestChan:
					if data == nil {
						logger.Log("connection closed, business goroutine exit")
						return
					}

					proto.HandleRequest(serverCtx.TcpServerChan, data)
				}
			}
		}(c)

		//for writing, 写协程等待一个channel的数据，将channel收到的数据发送至客户端
		go func(c *Connection) {
			for   {
				select {
				case <-c.closeChan:
					logger.Log("write goroutine exit")
					return
				case data := <-c.responseChan:
					if data == nil {
						logger.Log("connection closed, write goroutine exit")
						return
					}

					if _, err := c.conn.Write([]byte(data.Data)); err != nil {
						logger.Log("send data to client failed, " + err.Error())
					}
				}
			}

		}(c)
	}

}