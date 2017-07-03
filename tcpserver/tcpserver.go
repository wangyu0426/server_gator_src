package tcpserver

import (
	"../logging"
	"../svrctx"
	"../proto"
	"net"
	"fmt"
	"io"
	"strconv"
	"strings"
	"os"
	"time"
)

const (
	MsgHeaderSize = 28
	MsgMinSize = 29
	DeviceMsgVersionSize = 4
)

var addConnChan chan *Connection
var delConnChan chan *Connection
var TcpClientTable = map[uint64]*Connection{}
var DevicePushCache = map[uint64]*[]*proto.MsgData{}

func init()  {
	logging.Log("tcpserver init")
	addConnChan = make(chan *Connection, 1024)
	delConnChan = make(chan *Connection, 1024)
}

func isCmdsMatched(reqCmd, ackCmd uint16) bool {
	return true
}

//for managing connection, 对内负责管理tcp连接对象，对外为APP server提供通信接口
func ConnManagerLoop(serverCtx *svrctx.ServerContext) {
	defer logging.PanicLogAndExit("")

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

				delete(TcpClientTable, connDel.imei)
				logging.Log("connection deleted from TcpClientTable")
			}else {
				logging.Log("will delete connection from TcpClientTable, but connection not found")
			}
		case msg := <-serverCtx.TcpServerChan:
			if msg == nil {
				logging.Log("main lopp exit, connection manager goroutine exit")
				return
			}

			isPushCache := msg.Header.Header.Version == proto.MSG_HEADER_PUSH_CACHE
			//if msg.Header.Header.Version == proto.MSG_HEADER_PUSH_CACHE {
			if len(msg.Data) == 0 { //没有数据，表示这是一个通知的消息
				logging.Log(fmt.Sprintf("msg to notify to push cached data to device %d ",  msg.Header.Header.Imei))
			}else {
				//if msg.Header.Header.From == proto.MsgFromAppToAppServer {
				//	//logging.Log("msg from app: " + string(msg.Data))
				//}else {
				//	//logging.Log("msg from tcp: " + string(msg.Data))
				//}

				_, ok1 := DevicePushCache[msg.Header.Header.Imei]
				if ok1 == false {
					DevicePushCache[msg.Header.Header.Imei] = &[]*proto.MsgData{}
				}

				*DevicePushCache[msg.Header.Header.Imei] = append(*DevicePushCache[msg.Header.Header.Imei], msg)
				//}else{
				//	cache = append(cache, msg)
				logging.Log(fmt.Sprintf("[%d] cache --  count: %d", msg.Header.Header.Imei, len(*DevicePushCache[msg.Header.Header.Imei])))
			}

			c, ok2 := TcpClientTable[msg.Header.Header.Imei]
			if ok2 {
				logging.Log(fmt.Sprintf("[%d] lastActiveTime: %d", msg.Header.Header.Imei, c.lastActiveTime))
				if (c.lastActiveTime - time.Now().Unix()) <= int64(serverCtx.MaxDeviceIdleTimeSecs) {
					//如果60秒内有数据，则认为连接良好
					cache, ok3 := DevicePushCache[msg.Header.Header.Imei]
					if (ok3 == false) {
						logging.Log(fmt.Sprintf("[%d] no data cached to send", msg.Header.Header.Imei ))
					}else{
						logging.Log(fmt.Sprintf("[%d] cache --////--  count: %d", msg.Header.Header.Imei, len(*cache)))
						tempCache := []*proto.MsgData{}
						for _, cachedMsg := range *cache {
							if cachedMsg.Header.Header.Status == 0 {
								//0， 不需要手表回复确认，直接发送完并删除
								c.responseChan <- cachedMsg
							}else if cachedMsg.Header.Header.Status == 1 {
								//1, 消息尚未发送，且需要确认，则首先发出消息，并将status置2,
								c.responseChan <- cachedMsg
								cachedMsg.Header.Header.Status = 2
								tempCache = append(tempCache, cachedMsg)
							}else if cachedMsg.Header.Header.Status == 2 {
								//2, 消息已经发送，并处于等待手表确认的状态
								if msg.Header.Header.Version == proto.MSG_HEADER_ACK_PARSED &&
									msg.Header.Header.Imei == cachedMsg.Header.Header.Imei &&
									msg.Header.Header.ID == cachedMsg.Header.Header.ID {
								}else{ //未收到确认，如果当前消息是通知继续推送数据并且时间已经超过30s，才会发送
									if isPushCache {
										timeout := (cachedMsg.Header.Header.ID - proto.NewMsgID()) / 10000
										if timeout > 30 {
											c.responseChan <- cachedMsg
											tempCache = append(tempCache, cachedMsg)
										}else{
											logging.Log(fmt.Sprintf("[%d] push data timeout less than 30s, no need to push", msg.Header.Header.Imei))
										}
									}
								}
							}else{
								tempCache = append(tempCache, cachedMsg)
							}

							logging.Log("send app data to write routine")
						}

						*cache = tempCache
						logging.Log(fmt.Sprintf("[%d] cache --/ ack parse /--  count: %d", msg.Header.Header.Imei, len(*cache)))
					}
				}else{
					logging.Log(fmt.Sprintf("[%d] device idle no data over %d seconds",
						msg.Header.Header.Imei, serverCtx.MaxDeviceIdleTimeSecs ))
				}
				//if msg.Header.Header.Version == proto.MSG_HEADER_PUSH_CACHE {
				//	//从缓存中读取数据，发送至手表
				//}else {
				//	c.responseChan <- msg
				//}
				//
				//logging.Log("send app data to write routine")
			}else {
				logging.Log(fmt.Sprintf("[%d]will send app data to tcp connection from TcpClientTable, but connection not found",
					msg.Header.Header.Imei))
			}
		}
	}

}

//for reading
func ConnReadLoop(c *Connection, serverCtx *svrctx.ServerContext) {
	defer logging.PanicLogAndExit("")

	defer c.closeOnce.Do(func() {
		logging.Log("client connection closed")
		delConnChan <- c
	})

	logging.Log("new connection from: " +  c.conn.RemoteAddr().String())

	for  {
		if len(c.buf) == 0 {
			c.buf = make([]byte, 256)
		}

		dataSize := uint16(0)
		headerBuf := c.buf[0: MsgHeaderSize]

		if serverCtx.IsDebug == false {
			c.conn.SetReadDeadline(time.Now().Add(time.Second * serverCtx.RecvTimeout))
		}

		if _, err := io.ReadFull(c.conn, headerBuf); err != nil {
			logging.Log("recv MsgHeaderSize bytes header failed, " + err.Error())
			break
		}

		c.lastActiveTime = time.Now().Unix()

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

		//将IMEI和cmd解析出来
		imei, _ := strconv.ParseUint(string(headerBuf[5 + DeviceMsgVersionSize: 20 + DeviceMsgVersionSize]), 0, 0)
		cmd := string(headerBuf[20 + DeviceMsgVersionSize: 24 + DeviceMsgVersionSize])

		logging.Log("data size: " + fmt.Sprintf("%d", dataSize))
		if len(c.buf) < int(dataSize) {
			buf := c.buf
			c.buf = make([]byte, dataSize)
			copy(c.buf[0: MsgHeaderSize], buf)
		}

		bufSize := dataSize - MsgHeaderSize
		dataBuf := c.buf[MsgHeaderSize: MsgHeaderSize + bufSize]
		//recvOffset := uint16(0)
		full := false

		if serverCtx.IsDebug == false {
			c.conn.SetReadDeadline(time.Now().Add(time.Second * serverCtx.RecvTimeout))
		}
		n, err := io.ReadFull(c.conn, dataBuf)
		if err == nil && n == len(dataBuf) {
			full = true
		}else {
			logging.Log(fmt.Sprintf("recv data failed, %s, recv %d bytes", err.Error(), n))
		}

		if n > 0 {
			c.lastActiveTime = time.Now().Unix()
		}

		//记录收到的是些什么数据
		logging.Log(string(c.buf))

		if !full {
			if serverCtx.IsDebug == true {
				logging.Log("data packet was not fully received ")
				break
			}
		}

		if full && dataBuf[bufSize - 1] != ')' {
			logging.Log("bad format of data packet, it must end with )")
			break
		}

		// 消息接收不完整，如果是微聊（BP34）等需要支持续传的请求，
		// 则将当前不完整的数据加入续传队列，等待下一次连接以后进行续传
		// 否则，为普通命令请求如BP01，BP09等，直接丢弃并关闭连接
		if !full && cmd != proto.StringCmd(proto.DRT_SEND_MINICHAT) {
			break  //退出循环和go routine，连接关闭
		}

		c.imei = imei
		if c.saved == false && full {
			c.saved = true
			addConnChan <- c
		}

		msg := &proto.MsgData{}
		msg.Header.Header.Version = proto.MSG_HEADER_VER_EX
		msg.Header.Header.Size = dataSize
		msg.Header.Header.From = proto.MsgFromDeviceToTcpServer
		msg.Header.Header.ID = proto.NewMsgID()
		msg.Header.Header.Imei = imei
		msg.Header.Header.Cmd = proto.IntCmd(cmd)
		msg.Data = make([]byte, n)
		copy(msg.Data, dataBuf[0: n]) //不包含头部28字节
		if  msg.Header.Header.Cmd ==  proto.DRT_SEND_MINICHAT {
			msg.Header.Header.Status = 1
		}else{
			msg.Header.Header.Status = 0
		}

		////首先通知ManagerLoop, 将上次缓存的未发送的数据发送给手表
		//msgNotify := &proto.MsgData{}
		//msgNotify.Header.Header.Version = proto.MSG_HEADER_PUSH_CACHE
		////logging.Log("MSG_HEADER_PUSH_CACHE, imei: " + proto.Num2Str(imei, 10))
		//msgNotify.Header.Header.Imei = imei
		//serverCtx.TcpServerChan <- msgNotify

		// 包可能接收未完整，但此时可以开始进行业务逻辑处理
		// 目前所有头部信息都已准备好，业务处理模块只需处理与断点续传相关逻辑
		// 以及命令的实际逻辑
		c.requestChan <- msg

		//业务逻辑只处理完整的请求和不完整的微聊请求，处理完请求以后，
		// 对于不完整的微聊请求，由于连接已经出错，需要将当前连接关闭
		if !full && cmd == proto.StringCmd(proto.DRT_SEND_MINICHAT) {
			break  //退出循环和go routine，连接关闭
		}
	}

}

//for writing, 写协程等待一个channel的数据，将channel收到的数据发送至客户端
func ConnWriteLoop(c *Connection) {
	defer logging.PanicLogAndExit("")

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
				logging.Log(fmt.Sprintf("send data to client failed: %s,  %d bytes sent",  err.Error(), n))
			}else{
				logging.Log(fmt.Sprintf("send data to client: %s,  %d bytes sent", string(data.Data), n))
			}
		}
	}

}

//for business handler，业务处理的协程
func BusinessHandleLoop(c *Connection, serverCtx *svrctx.ServerContext) {
	defer logging.PanicLogAndExit("")

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

			proto.HandleTcpRequest(proto.RequestContext{IsDebug: serverCtx.IsDebug, IP: c.IP, Port: c.Port,
				AvatarUploadDir: serverCtx.HttpStaticDir + serverCtx.HttpStaticAvatarDir,
				MinichatUploadDir: serverCtx.HttpStaticDir + serverCtx.HttpStaticMinichatDir,
				DeviceMinichatBaseUrl: fmt.Sprintf("%s:%d%s", serverCtx.HttpServerName,
					serverCtx.WSPort, serverCtx.HttpStaticURL + serverCtx.HttpStaticMinichatDir),
				AndroidAppURL: serverCtx.AndroidAppURL,
				IOSAppURL: serverCtx.IOSAppURL,
				Pgpool: serverCtx.PGPool,
				MysqlPool: serverCtx.MySQLPool,
				WritebackChan: serverCtx.TcpServerChan,
				AppNotifyChan: serverCtx.AppServerChan,
				Msg: data,
				GetDeviceDataFunc: svrctx.GetDeviceData,
				SetDeviceDataFunc: svrctx.SetDeviceData,
				GetChatDataFunc: svrctx.GetChatData,
				AddChatDataFunc: svrctx.AddChatData,
			})
		}
	}
}

func TcpServerRunLoop(serverCtx *svrctx.ServerContext)  {
	defer logging.PanicLogAndExit("")

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
	go ConnManagerLoop(serverCtx)

	for  {
		//listener.SetDeadline(time.Now().Add(time.Second * serverCtx.AcceptTimeout))
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
		c.lastActiveTime = time.Now().Unix()

		//for reading
		go ConnReadLoop(c, serverCtx)

		//for business handler，业务处理的协程
		go BusinessHandleLoop(c, serverCtx)

		//for writing, 写协程等待一个channel的数据，将channel收到的数据发送至客户端
		go ConnWriteLoop(c)
	}

}
