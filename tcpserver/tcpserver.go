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

func init()  {
	logger.Log("tcpserver init")
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
		}
		serverCtx.WaitLock.Done()
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

		//for reading
		go func(c *Connection) {
			defer c.closeOnce.Do(func() {
				logger.Log("client connection closed")
				close(c.requestChan)
				close(c.responseChan)
				close(c.closeChan)
				c.conn.Close()
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
				c.requestChan<-&proto.MsgData{bufSize, dataBuf}
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

					proto.HandleMsg(c.responseChan, data)
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