package tcpserver

import (
	"../logging"
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
	MsgHeaderSize = 24
	MsgMinSize = 25
)

var addConnChan chan *Connection
var delConnChan chan *Connection

func init()  {
	logging.Log("tcpserver init")
	addConnChan = make(chan *Connection, 1024)
	delConnChan = make(chan *Connection, 1024)
}


func TcpServerRunLoop(serverCtx *svrctx.ServerContext)  {
	tcpaddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", serverCtx.BindAddr, serverCtx.Port ))
	if err != nil {
		logging.Log("resolve tcp address failed, " + err.Error())
		os.Exit(1)
	}

	listener, err := net.ListenTCP("tcp", tcpaddr)
	if err != nil {
		logging.Log("listen tcp address failed, " + err.Error())
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
				logging.Log("new connection added")
				TcpClientTable[connAdd.imei] = connAdd
			case connDel := <-delConnChan:
				if connDel == nil {
					logging.Log("main lopp exit, connection manager goroutine exit")
					return
				}

				_, ok := TcpClientTable[connDel.imei]
				if ok {
					connDel.SetClosed()
					close(connDel.requestChan)
					close(connDel.responseChan)
					close(connDel.closeChan)
					connDel.conn.Close()

					delete(TcpClientTable, connDel.imei);
					logging.Log("connection deleted from TcpClientTable")
				}else {
					logging.Log("will delete connection from TcpClientTable, but connection not found")
				}
			case msg := <-serverCtx.TcpServerChan:
				if msg == nil {
					logging.Log("main lopp exit, connection manager goroutine exit")
					return
				}

				if msg.Header.Header.From == proto.MsgFromAppToAppServer{
					logging.Log("msg from app: " + string(msg.Data))
				}else {
					logging.Log("msg from tcp: " + string(msg.Data))
				}

				c, ok := TcpClientTable[msg.Header.Header.Imei]
				if ok {
					c.responseChan <- msg
					logging.Log("send app data to write routine")
				}else {
					logging.Log("will send app data to tcp connection from TcpClientTable, but connection not found")
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
				logging.Log("accept connection failed, " + err.Error())
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
				logging.Log("client connection closed")
				delConnChan <- c
			})

			logging.Log("new connection from: " +  c.conn.RemoteAddr().String())

			for  {
				dataSize := uint16(0)
				headerBuf := make([]byte, MsgHeaderSize)

				c.conn.SetReadDeadline(time.Now().Add(time.Second * serverCtx.RecvTimeout))
				if _, err := io.ReadFull(c.conn, headerBuf); err != nil {
					logging.Log("recv MsgHeaderSize bytes header failed, " + err.Error())
					break
				}

				logging.Log("recv: " + string(headerBuf))

				if headerBuf[0] != '(' {
					logging.Log("bad format of data packet, it must begin with (")
					break
				}

				num, _ := strconv.ParseUint("0x" + string(headerBuf[1:5]), 0, 0)
				dataSize = uint16(num)
				if dataSize < MsgMinSize {
					logging.Log(fmt.Sprintf("data size in header is bad of  %d, less than %d", dataSize, MsgMinSize))
					break
				}

				logging.Log("data size: " + fmt.Sprintf("%d", dataSize))

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
						logging.Log("recv data failed, " + err.Error())
						break
					}
				}

				//记录收到的是些什么数据
				logging.Log(fmt.Sprint(dataBuf))

				if recvOffset == bufSize && dataBuf[bufSize - 1] != ')' {
					logging.Log("bad format of data packet, it must end with )")
					break
				}

				//将IMEI和cmd解析出来
				imei, _ := strconv.ParseUint(string(headerBuf[5: 20]), 0, 0)
				cmd := string(headerBuf[20: 24])

				// 消息接收不完整，如果是微聊（BP34）等需要支持续传的请求，
				// 则将当前不完整的数据加入续传队列，等待下一次连接以后进行续传
				// 否则，为普通命令请求如BP01，BP09等，直接丢弃并关闭连接
				if recvOffset != bufSize && cmd != proto.StringCmd(proto.DRT_SEND_MINICHAT) {
					break  //退出循环和go routine，连接关闭
				}

				msg := &proto.MsgData{}
				msg.Header.Header.Version = proto.MSG_HEADER_VER_EX
				msg.Header.Header.Size = bufSize
				msg.Header.Header.From = proto.MsgFromDeviceToTcpServer
				msg.Header.Header.ID = proto.NewMsgID()
				msg.Header.Header.Imei = imei
				msg.Header.Header.Cmd = proto.IntCmd(cmd)
				msg.Data = dataBuf[0: recvOffset] //不包含头部24字节
				if  msg.Header.Header.Cmd ==  proto.DRT_SEND_MINICHAT {
					msg.Header.Header.Status = 1
				}else{
					msg.Header.Header.Status = 0
				}

				// 包可能接收未完整，但此时可以开始进行业务逻辑处理
				// 目前所有头部信息都已准备好，业务处理模块只需处理与断点续传相关逻辑
				// 以及命令的实际逻辑
				c.requestChan <- msg

			}

		}(c)

		//for business handler，业务处理的协程
		go func(c *Connection) {
			for {
				select {
				case <-c.closeChan:
					logging.Log("business goroutine exit")
					return
				case data := <-c.requestChan:
					if data == nil {
						logging.Log("connection closed, business goroutine exit")
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
					logging.Log("write goroutine exit")
					return
				case data := <-c.responseChan:
					if data == nil ||  c.IsClosed() { //连接关闭了，这里需要将响应数据推入续传队列
						logging.Log("connection closed, write goroutine exit")

						//连接关闭了，这里需要将响应数据推入续传队列
						return
					}

					if n, err := c.conn.Write([]byte(data.Data)); err != nil {
						logging.Log(fmt.Sprintf("send data to client failed: %s,  %d bytes sent" + err.Error(), n))
					}
				}
			}

		}(c)
	}

}