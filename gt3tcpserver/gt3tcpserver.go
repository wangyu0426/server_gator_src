package gt3tcpserver

import (
	"../logging"
	"../svrctx"
	"../proto"
	"net"
	"fmt"
	"strings"
	"os"
	"time"
	"encoding/json"
)

const (
	MsgHeaderSize = 28
	MsgMinSize = 29
	DeviceMsgVersionSize = 4
	MAX_TCP_REQUEST_LENGTH = 1024 * 64
	GT3_PROTO_INVALID = -1
	GT3_PROTO_V1 = 1
	GT3_PROTO_V2 = 2
)

var addConnChan chan *Connection
var delConnChan chan *Connection
var TcpClientTable = map[uint64]*Connection{}
var DevicePushCache = map[uint64]*[]*proto.MsgData{}

func init()  {
	logging.Log("gt3tcpserver init")
	addConnChan = make(chan *Connection, 10240)
	delConnChan = make(chan *Connection, 10240)
}

func isCmdsMatched(reqCmd, ackCmd uint16) bool {
	return true
}

//for managing connection, 对内负责管理tcp连接对象，对外为APP server提供通信接口
func ConnManagerLoop(serverCtx *svrctx.ServerContext) {
	defer logging.PanicLogAndExit("ConnManagerLoop")

	for   {
		select {
		case connAdd:=<-addConnChan:
			connDel, ok := TcpClientTable[connAdd.imei]
			if ok && connDel != nil && connDel.connid != connAdd.connid {
				connDel.SetClosed()
				close(connDel.requestChan)
				close(connDel.responseChan)
				close(connDel.closeChan)
				connDel.conn.Close()

				delete(TcpClientTable, connDel.imei)
				logging.Log(fmt.Sprintf("%d connection deleted from TcpClientTable", connAdd.imei))
			}else {
				logging.Log(fmt.Sprintf("%d will delete connection from TcpClientTable, but connection not found", connAdd.imei))
			}

			TcpClientTable[connAdd.imei] = connAdd
			logging.Log(fmt.Sprintf("%d new connection added", connAdd.imei))
		case connDel := <-delConnChan:
			if connDel == nil {
				logging.Log("main lopp exit, connection manager goroutine exit")
				return
			}

			theConn, ok := TcpClientTable[connDel.imei]
			if ok && theConn != nil && theConn.connid == connDel.connid {
				connDel.SetClosed()
				close(connDel.requestChan)
				close(connDel.responseChan)
				close(connDel.closeChan)
				connDel.conn.Close()

				delete(TcpClientTable, connDel.imei)
				logging.Log(fmt.Sprintf("%d connection deleted from TcpClientTable", connDel.imei))
			}else {
				logging.Log(fmt.Sprintf("%d will delete connection from TcpClientTable, but connection not found", connDel.imei))
			}
		case msg := <- serverCtx.GT3TcpServerChan:
			if msg == nil {
				logging.Log("main lopp exit, connection manager goroutine exit")
				return
			}

			//logging.Log("recv chan data: " + proto.MakeStructToJson(msg))

			if  msg.Header.Header.Version == proto.MSG_HEADER_VER_EX &&
				msg.Header.Header.From == proto.MsgFromAppServerToTcpServer {
				logging.Log(fmt.Sprintf("[%d] will active device: %s", msg.Header.Header.Imei, msg.Data))

				cmdAckName := ""
				c, ok2 := TcpClientTable[msg.Header.Header.Imei]
				if ok2 {
					curTimeSecs := time.Now().Unix()
					logging.Log(fmt.Sprintf("[%d] lastActiveTime: %d, curTimeSecs: %d, maxIdle: %d, result: %d", msg.Header.Header.Imei,
						c.lastActiveTime, curTimeSecs, serverCtx.MaxDeviceIdleTimeSecs, curTimeSecs - c.lastActiveTime))
					if (curTimeSecs - c.lastActiveTime) <= int64(serverCtx.MaxDeviceIdleTimeSecs) {
						//如果100秒内有数据，则认为连接良好
						var (
							data []byte
							requireAck bool
						)

						switch msg.Header.Header.Cmd {
						case proto.CMD_GT3_AP00_LOCATE_NOW:
							data = proto.MakeLocateNowReplyMsg(msg.Header.Header.Imei, false)
							requireAck = false
							cmdAckName = proto.DeviceLocateNowAckCmdName
						//case proto.CMD_AP16:
						//	data = proto.MakeSosReplyMsg(msg.Header.Header.Imei, proto.NewMsgID())
						//	requireAck = false
						//	cmdAckName = proto.ActiveDeviceSosAckCmdName
						}

						c.responseChan <-  proto.MakeReplyMsg(msg.Header.Header.Imei, requireAck, data, msg.Header.Header.ID)
						logging.Log(fmt.Sprintf("[%d] device active msg has been sent, app has no need to send sms",  msg.Header.Header.Imei ))
						break
					}else{
						//超过连接有效期，则通知APP需要发送短信激活
						logging.Log(fmt.Sprintf("[%d] device idle no data over %d seconds and will notify app to send sms",
							msg.Header.Header.Imei, serverCtx.MaxDeviceIdleTimeSecs ))
					}
				}else {
					logging.Log(fmt.Sprintf("[%d]will send app data to active device , but device tcp connection not found, will notify app to send sms",
						msg.Header.Header.Imei))
				}

				//params := proto.AppRequestTcpConnParams{}
				//err := json.Unmarshal(msg.Data, &params)
				//if err!= nil {
				//	logging.Log(fmt.Sprintf("[%d] parse json from msg.Data failed ,%s", msg.Header.Header.Imei, err.Error()))
				//	return
				//}

				//result := proto.HttpAPIResult{ErrCode: 1, Imei: proto.Num2Str(msg.Header.Header.Imei, 10), Data: msg.Data}

				reqParams := proto.AppRequestTcpConnParams{}
				switch msg.Header.Header.Cmd {
				case proto.CMD_GT3_AP00_LOCATE_NOW:
					cmdAckName = proto.DeviceLocateNowAckCmdName
					_ = json.Unmarshal(msg.Data, &reqParams)

				//case proto.CMD_AP16:
				//	cmdAckName = proto.ActiveDeviceSosAckCmdName
				}

				if reqParams.ConnID != 0 && reqParams.Params.UserName != "" {
					serverCtx.AppServerChan <- &proto.AppMsgData{Cmd: cmdAckName,
						Imei: msg.Header.Header.Imei,
						Data: string(msg.Data), ConnID: reqParams.ConnID, UserName: reqParams.Params.UserName}
				}

				break
			}

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
				//logging.Log(fmt.Sprintf("[%d] cache --  count: %d", msg.Header.Header.Imei, len(*DevicePushCache[msg.Header.Header.Imei])))
			}

			c, ok2 := TcpClientTable[msg.Header.Header.Imei]
			if ok2 {
				logging.Log(fmt.Sprintf("[%d] lastActiveTime: %d", msg.Header.Header.Imei, c.lastActiveTime))
				if (time.Now().Unix() - c.lastActiveTime) <= int64(serverCtx.MaxDeviceIdleTimeSecs) {
					//如果100秒内有数据，则认为连接良好
					cache, ok3 := DevicePushCache[msg.Header.Header.Imei]
					if (ok3 == false) {
						logging.Log(fmt.Sprintf("[%d] no data cached to send", msg.Header.Header.Imei ))
					}else{
						//logging.Log(fmt.Sprintf("[%d] cache --////--  count: %d", msg.Header.Header.Imei, len(*cache)))
						tempCache := []*proto.MsgData{}
						for _, cachedMsg := range *cache {
							if len(cachedMsg.Data) == 0 {
								tempCache = append(tempCache, cachedMsg)
								continue
							}

							//不需要手表回复确认，直接发送完并删除
							//对于AP15,需要加30s延迟，不能连续发送太快
							//改为3秒
							if cachedMsg.Header.Header.Cmd == proto.CMD_GT3_AP15_PUSH_CHAT_COUNT {
								if (int64(cachedMsg.Header.Header.ID) - int64(c.lastPushFileNumTime)) / int64(time.Second) >= 3 {
									c.responseChan <- cachedMsg
									c.lastPushFileNumTime = int64(cachedMsg.Header.Header.ID)
								}
							}else{
								c.responseChan <- cachedMsg
							}

							logging.Log("send app data to write routine")
						}

						if len(tempCache) > 0 {
							*cache = tempCache
						}else{
							delete(DevicePushCache, msg.Header.Header.Imei)
						}
						//logging.Log(fmt.Sprintf("[%d] cache --/ ack parse /--  count: %d, %s", msg.Header.Header.Imei, len(*cache),
							//proto.MakeStructToJson((*cache)[0])))
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
	defer logging.PanicLogAndExit("ConnReadLoop")

	defer func() {
		logging.Log(fmt.Sprintf("%d client connection closed", c.imei))
		delConnChan <- c
	}()

	logging.Log("new connection from: " +  c.conn.RemoteAddr().String())

	if len(c.buf) == 0 {
		c.buf = make([]byte, MAX_TCP_REQUEST_LENGTH )
	}

	for  {
		if c.recvEndPosition >= MAX_TCP_REQUEST_LENGTH{
			logging.Log(fmt.Sprintf("recvEndPosition >= MAX_TCP_REQUEST_LENGTH(%d)", MAX_TCP_REQUEST_LENGTH))
			break
		}

		if serverCtx.IsDebug == false {
			c.conn.SetReadDeadline(time.Now().Add(time.Second * serverCtx.RecvTimeout))
		}

		breakLoop := false
		n, err := c.conn.Read(c.buf[c.recvEndPosition: ])
		if n > 0 {
			c.recvEndPosition += n
		}else {
			logging.Log(fmt.Sprintf("recv data failed, %s, recv %d bytes", err.Error(), n))
			breakLoop = true
		}

		if n > 0 {
			c.lastActiveTime = time.Now().Unix()
		}

		//记录收到的是些什么数据
		logging.Log(string(c.buf[0: c.recvEndPosition]))

		//切割消息
		for {
			if c.recvEndPosition == 0 {
				break
			}

			parseLen, ret, gt3protoVer := ParaseClientMsg(c.buf[0: c.recvEndPosition], uint16(c.recvEndPosition))
			if ret < 0 {
				breakLoop = true
				break
			}else if ret == 1 {
				break
			}else{
				//分发完整消息
				//首先判断协议版本
				msgLen := parseLen
				packet := c.buf[0: msgLen]

				if gt3protoVer != GT3_PROTO_V1  && gt3protoVer != GT3_PROTO_V2 {
					logging.Log("bad proto of data packet")
					breakLoop = true
					break
				}else{
					if gt3protoVer == GT3_PROTO_V2{ //加密协议,需要解密数据
						pkt, err := proto.Gt3AesDecrypt(string(c.buf[8: parseLen]))
						if err != nil {
							logging.Log("Gt3AesDecrypt data packet failed, " + err.Error())
							breakLoop = true
							break
						}

						packet = pkt
						msgLen = uint16(len(packet))
					}
				}

				//记录解密后(如果需要解密)的是些什么数据
				logging.Log(string(packet))

				if packet[msgLen - 1] != ')' || msgLen < 21 {
					logging.Log("bad format of data packet, it must end with )")
					breakLoop = true
					break
				}

				imei := proto.Str2Num(string(packet[1: 16]), 10)
				cmd := string(packet[16:20])

				c.imei = imei
				if c.saved == false {
					c.saved = true
					addConnChan <- c
				}

				msg := &proto.MsgData{}
				msg.Header.Header.Version = proto.MSG_HEADER_VER_EX
				msg.Header.Header.Size = msgLen
				msg.Header.Header.From = proto.MsgFromDeviceToTcpServer
				msg.Header.Header.ID = proto.NewMsgID()
				msg.Header.Header.Imei = imei
				msg.Header.Header.Cmd = proto.Gt3IntCmd(cmd)
				msg.Data = make([]byte, msgLen)
				copy(msg.Data, packet[0: msgLen])

				c.requestChan <- msg

				c.FlushRecvBuf(int(parseLen))
			}
		}

		if breakLoop {
			break  //退出循环和go routine，连接关闭
		}
	}
}

//for writing, 写协程等待一个channel的数据，将channel收到的数据发送至客户端
func ConnWriteLoop(c *Connection) {
	defer logging.PanicLogAndExit("ConnWriteLoop")

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
				sentData := string(data.Data)
				cmd := proto.Gt3IntCmd(sentData[16: 20])
				count := 0
				if cmd == proto.CMD_GT3_AP16_PUSH_CHAT_DATA { //微聊，只打印80个字节
					count = 30
				}else if cmd == proto.CMD_GT3_AP17_PUSH_EPO_DATA  { //EPO，只打印40个字节
					count = 30
				}else{
					count = len(sentData)
				}

				if count > len(sentData){
					count = len(sentData)
				}

				logging.Log(fmt.Sprintf("send data to client: %s,  %d bytes sent", sentData[0: count], n))
			}
		}
	}

}

//for business handler，业务处理的协程
func BusinessHandleLoop(c *Connection, serverCtx *svrctx.ServerContext) {
	defer logging.PanicLogAndExit("BusinessHandleLoop")

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

			proto.HandleGt3TcpRequest(proto.RequestContext{IsDebug: serverCtx.IsDebug, IP: c.IP, Port: c.Port,
				AvatarUploadDir: serverCtx.HttpStaticDir + serverCtx.HttpStaticAvatarDir,
				MinichatUploadDir: serverCtx.HttpStaticDir + serverCtx.HttpStaticMinichatDir,
				DeviceMinichatBaseUrl: fmt.Sprintf("%s:%d%s", serverCtx.HttpServerName,
					serverCtx.WSPort, serverCtx.HttpStaticURL + serverCtx.HttpStaticMinichatDir),
				AndroidAppURL: serverCtx.AndroidAppURL,
				IOSAppURL: serverCtx.IOSAppURL,
				APNSServerBaseURL: serverCtx.APNSServerApiBase,
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
	defer logging.PanicLogAndExit("TcpServerRunLoop")

	tcpaddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", serverCtx.BindAddr, serverCtx.Gt3Port ))
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

	//负责定时清理缓存数据
	go BackgroundCleanerLoop(serverCtx)

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

func BackgroundCleanerLoop(serverCtx *svrctx.ServerContext) {
	defer logging.PanicLogAndExit("BackgroundCleanerLoop")

	//for  {
	//	proto.DeviceChatTaskTableLock.Lock()
	//	for imei, item := range proto.DeviceChatTaskTable{
	//		for fileid, subItem := range item {
	//			if subItem != nil {
	//				timeout := (proto.NewMsgID() -  subItem.Info.CreateTime) / uint64(time.Second)
	//				if timeout >= uint64(serverCtx.MaxMinichatKeepTimeSecs) {
	//					delete(item, fileid)
	//					logging.Log(fmt.Sprintf("%d device send chat timeout over %d hours, need to delete/remove",
	//						imei, timeout / 3600))
	//				}
	//			}
	//		}
	//
	//		if item != nil && len(item) == 0 {
	//			delete(proto.DeviceChatTaskTable, imei)
	//		}
	//	}
	//	proto.DeviceChatTaskTableLock.Unlock()
	//
	//	logging.Log("device chat list cleaned")
	//
	//	tempChatTaskList :=  []*proto.ChatTask{}
	//	proto.AppSendChatListLock.Lock()
	//	for imei, item := range proto.AppSendChatList{
	//		if item != nil{
	//			for _, subItem := range *item{
	//				if subItem != nil {
	//					timeout := (proto.NewMsgID() -  subItem.Info.CreateTime) / uint64(time.Second)
	//					if timeout >= uint64(serverCtx.MaxMinichatKeepTimeSecs) {
	//						logging.Log(fmt.Sprintf("%d AppSendChat timeout over %d hours, need to delete/remove",
	//							imei, timeout / 3600))
	//					}else{
	//						tempChatTaskList = append(tempChatTaskList, subItem)
	//						//logging.Log(fmt.Sprintf("%d after append tempChatTaskList(AppSendChatList) len: %d", imei, len(tempChatTaskList)))
	//					}
	//				}
	//			}
	//
	//			if len(tempChatTaskList) > 0 {
	//				proto.AppSendChatList[imei] = &tempChatTaskList
	//				//logging.Log(fmt.Sprintf("%d after append AppSendChatList len: %d", imei, len(*proto.AppSendChatList[imei])))
	//			}
	//
	//			if len(*item) == 0{
	//				delete(proto.AppSendChatList, imei)
	//			}
	//		}
	//	}
	//
	//	proto.AppSendChatListLock.Unlock()
	//
	//	logging.Log("app send chat list cleaned")
	//
	//	//
	//	//var AppChatList = map[uint64]map[uint64]ChatInfo{}
	//	proto.AppChatListLock.Lock()
	//	for imei, item := range  proto.AppChatList {
	//		for fileid, subItem := range item {
	//			timeout := (proto.NewMsgID() -  subItem.CreateTime) / uint64(time.Second)
	//			if timeout >= uint64(serverCtx.MaxMinichatKeepTimeSecs) {
	//				delete(item, fileid)
	//				logging.Log(fmt.Sprintf("%d APP chat timeout over %d hours, need to delete/remove",
	//					imei, timeout / 3600))
	//			}
	//		}
	//
	//		if item != nil && len(item) == 0 {
	//			delete(proto.AppChatList, imei)
	//		}
	//	}
	//	proto.AppChatListLock.Unlock()
	//
	//	logging.Log("app chat list cleaned")
	//
	//	tempPhotoList := []*proto.PhotoSettingTask{}
	//	proto.AppNewPhotoListLock.Lock()
	//	for imei, item := range proto.AppNewPhotoList{
	//		if item != nil{
	//			logging.Log(fmt.Sprintf("%d AppNewPhotoList begin len: %d", imei, len(*item)))
	//			for _, subItem := range *item{
	//				if subItem != nil {
	//					timeout := (proto.NewMsgID() -  subItem.Info.CreateTime) / uint64(time.Second)
	//					if timeout >= uint64(serverCtx.MaxMinichatKeepTimeSecs) {
	//						logging.Log(fmt.Sprintf("%d AppNewPhotoList timeout over %d hours, need to delete/remove",
	//							imei, timeout / 3600))
	//					}else{
	//						tempPhotoList = append(tempPhotoList, subItem)
	//					}
	//				}
	//			}
	//
	//			if len(tempPhotoList) > 0 {
	//				proto.AppNewPhotoList[imei] = &tempPhotoList
	//			}
	//
	//			if len(*item) == 0{
	//				delete(proto.AppNewPhotoList, imei)
	//			}
	//
	//			logging.Log(fmt.Sprintf("%d AppNewPhotoList end len: %d", imei, len(*item)))
	//		}
	//	}
	//	proto.AppNewPhotoListLock.Unlock()
	//
	//	logging.Log("app new photo list cleaned")
	//
	//	tempPhotoPendingList := []*proto.PhotoSettingTask{}
	//	proto.AppNewPhotoPendingListLock.Lock()
	//	for imei, item := range proto.AppNewPhotoPendingList {
	//		if item != nil{
	//			for _, subItem := range *item{
	//				if subItem != nil {
	//					timeout := (proto.NewMsgID() -  subItem.Info.CreateTime) / uint64(time.Second)
	//					if timeout >= uint64(serverCtx.MaxMinichatKeepTimeSecs) {
	//						logging.Log(fmt.Sprintf("%d AppNewPhotoPendingList timeout over %d hours, need to delete/remove",
	//							imei, timeout / 3600))
	//					}else{
	//						tempPhotoPendingList = append(tempPhotoPendingList, subItem)
	//					}
	//				}
	//			}
	//
	//			if len(tempPhotoList) > 0 {
	//				proto.AppNewPhotoPendingList[imei] = &tempPhotoPendingList
	//			}
	//
	//			if len(*item) == 0{
	//				delete(proto.AppNewPhotoPendingList, imei)
	//			}
	//		}
	//	}
	//	proto.AppNewPhotoPendingListLock.Unlock()
	//
	//	logging.Log("app new photo pending list cleaned")
	//
	//	time.Sleep(time.Duration(serverCtx.BackgroundCleanerDelayTimeSecs) * time.Second)
	//}
}

func ParaseClientMsg(pszCodeBuf []byte, shRecvBufLen uint16) (uint16, int, int) {
	shCodeLen, ret :=  uint16(0), 0
	gt3protoVer := GT3_PROTO_INVALID

	if pszCodeBuf == nil || len(pszCodeBuf) == 0 || (pszCodeBuf[0] != '('  &&
		pszCodeBuf[0] != 'G')  {
		logging.Log("pszCodeBuf is null or invalid")
		return 0, -1, gt3protoVer
	}

	gt3protoVer = GT3_PROTO_V1
	if pszCodeBuf[0] == 'G' {
		gt3protoVer = GT3_PROTO_V2
		//GT004802/iAwAUB3pGjkYw44lYotx/Ny+BskWacSFetg+NFGkUfW/zasg+susRsDKD5QZmw=
		if shRecvBufLen < 8 {
			ret = 1
			logging.Log("the data from client is not completed by recv, maybe need recv any more")
			return shCodeLen, ret, gt3protoVer
		}

		if pszCodeBuf[1] != 'T' {
			logging.Log("pszCodeBuf is null or invalid")
			return 0, -1, gt3protoVer
		}

		base64DataLen := proto.Str2Num(string(pszCodeBuf[2: 6]), 16)
		if base64DataLen >= MAX_TCP_REQUEST_LENGTH {
			logging.Log("data size is invalid in gt3 encrypted msg")
			return 0, -1, gt3protoVer
		}

		if  shRecvBufLen < uint16(base64DataLen) {
			ret = 1
			fmt.Println("shRecvBufLen: ", shRecvBufLen, "base64DataLen: ", base64DataLen)
			logging.Log("the data from client is not completed by recv, maybe need recv any more")
			return shCodeLen, ret, gt3protoVer
		}

		return (uint16(base64DataLen)), 0, gt3protoVer
	}

	find_bp06 := string(pszCodeBuf[16: 20]) == "BP06"
	find_bp11 := string(pszCodeBuf[16: 20]) == "BP11"

	if find_bp06 || find_bp11 {
		prefix_len, phone_len := (0), (0)

		if find_bp06{
			prefix_len = (len("(357593060153353BP0627D6,2710,15101016010000"))
		}else{
			//"(357593060153353BP11,xxxxx,27D6,2710,15101016010000"
			prefix_len = (len("(357593060153353BP11,"))

			for i := 0; i < 32; i++ {
				if pszCodeBuf[i + prefix_len] == ',' {
					phone_len = i
					break
				}
			}

			if(phone_len == 0) {
				ret = 1
				logging.Log("the BP11 data from client is not completed by recv, maybe need recv any more")
				return shCodeLen, ret, gt3protoVer
			}

			prefix_len += phone_len + len(",27D6,2710,15101016010000")
		}

		sz_amr_msg_size := make([]byte, 4)
		if find_bp06 {
			copy(sz_amr_msg_size[0: ], pszCodeBuf[16 + 4: 24])
		}else{
			copy(sz_amr_msg_size[0: ], pszCodeBuf[16 + 4 + phone_len + 2: 16 + 4 + phone_len + 2 + 4])
		}

		amr_msg_size, t := uint16(0), byte(0)
		for i :=0; i < 4; i++ {
			if sz_amr_msg_size[i] <= '9'	{
				t = sz_amr_msg_size[i]-'0'
			}else{
				t=sz_amr_msg_size[i]-'A'+10
			}

			amr_msg_size = amr_msg_size*16 + uint16(t)
		}

		if shRecvBufLen < amr_msg_size {
			ret = 1
			logging.Log("the data from client is not completed by recv, maybe need recv any more")
			return shCodeLen, ret, gt3protoVer
		}

		if shRecvBufLen >= amr_msg_size &&  pszCodeBuf[amr_msg_size - 1] != ')' {
			logging.Log("the data from client is completed but invalid")
			return 0, -1, gt3protoVer
		}

		shCodeLen = amr_msg_size
		return shCodeLen , ret, gt3protoVer
	}

	brackets_open := 1
	shCodeLen = 1

	for shCodeLen < shRecvBufLen && pszCodeBuf[shCodeLen] != 0 && brackets_open != 0 {
		if pszCodeBuf[shCodeLen] == '(' {
			brackets_open++
		}else if pszCodeBuf[shCodeLen] == ')' {
			brackets_open--
		}

		shCodeLen++
	}

	//	const char *end = strchr((char *)pszCodeBuf, ')');
	//	if(end)
	//	{
	//		shCodeLen = (unsigned short)((unsigned char *)end + 1 - pszCodeBuf);
	//	}
	if brackets_open != 0 {
		ret = 1
		logging.Log("the data from client is not completed by recv, maybe need recv any more")
	}

	return shCodeLen, ret, gt3protoVer
}

func (this *Connection)FlushRecvBuf(iFlushLength int ) {
	if iFlushLength > this.recvEndPosition {
		iFlushLength = this.recvEndPosition
	}

	if iFlushLength > 0{
		for i := 0; i < (this.recvEndPosition - iFlushLength); i++ {
			this.buf[i] = this.buf[i + iFlushLength]
		}

		this.recvEndPosition -= iFlushLength
	}
}