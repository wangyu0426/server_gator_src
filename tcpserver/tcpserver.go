package tcpserver

import (
	"../logging"
	"../svrctx"
	"../proto"
	"net"
	"fmt"
	"io"
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
	GT6_PROTO_INVALID = -1
	GT6_PROTO_V1 = 1
	GT6_PROTO_V2 = 2
)

var addConnChan chan *Connection
var delConnChan chan *Connection
var TcpClientTable = map[uint64]*Connection{}
var DevicePushCache = map[uint64]*[]*proto.MsgData{}

func init()  {
	//fmt.Println(string(MakeLatestTimeLocationReplyMsg(proto.CMD_AP03, 357593060571398, 0x14CE9D5CF576B1DF, []byte("(003E357593060571398AP03,170706,023907,e0630,14CE9D5C99BF7158)"))))
	//fmt.Println(string(MakeLatestTimeLocationReplyMsg(proto.CMD_AP14, 357593060571398, 0x00000DA2DB7AFA20, []byte("(0054357593060571398AP1422.587725,113.913641,0,2017,07,06,03,30,13,00000DA2DB7AFA18)"))))

	logging.Log("tcpserver init")
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
				logging.Log(fmt.Sprintf("%d addConnChan connection deleted from TcpClientTable", connAdd.imei))
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
				logging.Log(fmt.Sprintf("%d delConnChan will delete connection from TcpClientTable, but connection not found", connDel.imei))
			}
		case msg := <- serverCtx.GT6TcpServerChan:
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
						case proto.CMD_AP00:
							data = proto.MakeLocateNowReplyMsg(msg.Header.Header.Imei, true)
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
				case proto.CMD_AP00:
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
					syncTimeMsg := &proto.MsgData{}
					syncTimeMsg.Header.Header.Cmd = proto.CMD_AP03
					*DevicePushCache[msg.Header.Header.Imei] = append(*DevicePushCache[msg.Header.Header.Imei], syncTimeMsg)

					sendLocationMsg := &proto.MsgData{}
					sendLocationMsg.Header.Header.Cmd = proto.CMD_AP14
					*DevicePushCache[msg.Header.Header.Imei] = append(*DevicePushCache[msg.Header.Header.Imei], sendLocationMsg)
				}

				if len(*DevicePushCache[msg.Header.Header.Imei]) == 1 {
					sendLocationMsg := &proto.MsgData{}
					sendLocationMsg.Header.Header.Cmd = proto.CMD_AP14
					*DevicePushCache[msg.Header.Header.Imei] = append(*DevicePushCache[msg.Header.Header.Imei], sendLocationMsg)
				}

				if msg.Header.Header.Cmd == proto.CMD_AP03 {
					if (*DevicePushCache[msg.Header.Header.Imei])[0].Header.Header.ID != 0 {
						tempID := (*DevicePushCache[msg.Header.Header.Imei])[0].Header.Header.ID
						(*DevicePushCache[msg.Header.Header.Imei])[0] = msg
						(*DevicePushCache[msg.Header.Header.Imei])[0].Header.Header.ID = tempID
					}else{
						(*DevicePushCache[msg.Header.Header.Imei])[0] = msg
					}
				}else if msg.Header.Header.Cmd == proto.CMD_AP14 {
					if (*DevicePushCache[msg.Header.Header.Imei])[1].Header.Header.ID != 0 {
						tempID := (*DevicePushCache[msg.Header.Header.Imei])[1].Header.Header.ID
						(*DevicePushCache[msg.Header.Header.Imei])[1] = msg
						(*DevicePushCache[msg.Header.Header.Imei])[1].Header.Header.ID = tempID
					}else{
						(*DevicePushCache[msg.Header.Header.Imei])[1] = msg
					}
				}else{
					if msg.Header.Header.Type == proto.TYPE_CMD_AP11 {
						bFound := false
						index := 0
						for index,_ = range *DevicePushCache[msg.Header.Header.Imei] {
							if (*DevicePushCache[msg.Header.Header.Imei])[index].Header.Header.Type == proto.TYPE_CMD_AP11 {
								bFound = true
								break
							}
						}

						if !bFound {
							*DevicePushCache[msg.Header.Header.Imei] = append(*DevicePushCache[msg.Header.Header.Imei], msg)
						}else {
							//replace
							if (*DevicePushCache[msg.Header.Header.Imei])[index].Header.Header.Count < msg.Header.Header.Count {
								logging.Log(fmt.Sprintf("replace:%d,%d",
									(*DevicePushCache[msg.Header.Header.Imei])[index].Header.Header.Count,
									msg.Header.Header.Count))
								(*DevicePushCache[msg.Header.Header.Imei])[index] = msg
							}
						}
					} else {
						*DevicePushCache[msg.Header.Header.Imei] = append(*DevicePushCache[msg.Header.Header.Imei], msg)
					}
				}

				//}else{
				//	cache = append(cache, msg)
				//logging.Log(fmt.Sprintf("[%d] cache --  count: %d", msg.Header.Header.Imei, len(*DevicePushCache[msg.Header.Header.Imei])))
			}

			c, ok2 := TcpClientTable[msg.Header.Header.Imei]
			if ok2 {
				logging.Log(fmt.Sprintf("[%d] lastActiveTime: %d", msg.Header.Header.Imei, c.lastActiveTime))
				//if (time.Now().Unix() - c.lastActiveTime) <= int64(serverCtx.MaxDeviceIdleTimeSecs) {
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
							logging.Log(fmt.Sprintf("[%d] tempcache:%d,%t,%d,%d",
								msg.Header.Header.Imei,cachedMsg.Header.Header.Cmd,isPushCache,cachedMsg.Header.Header.Status,len(*cache)))
							if cachedMsg.Header.Header.Status == 0 {
								//0， 不需要手表回复确认，直接发送完并删除
								//对于AP11,需要加30s延迟，不能连续发送太快
								//尽量把AP11命令发送间隔时间缩短
								if cachedMsg.Header.Header.Cmd == proto.CMD_AP11 {
									//if (int64(cachedMsg.Header.Header.ID) - int64(c.lastPushFileNumTime)) / int64(time.Second) >= 3 {
										c.responseChan <- cachedMsg
										c.lastPushFileNumTime = int64(cachedMsg.Header.Header.ID)
									//}else {
									//	logging.Log(fmt.Sprintf("[%d] AP11 cmd send too much!",msg.Header.Header.Imei))
									//}
								}else{
									c.responseChan <- cachedMsg
								}
							}else if cachedMsg.Header.Header.Status == 1 {
								//1, 消息尚未发送，且需要确认，则首先发出消息，并将status置2,
								//如果是AP03和AP14，则需要实时推送务器当前时间和手表最新定位
								if cachedMsg.Header.Header.Cmd == proto.CMD_AP03 ||  cachedMsg.Header.Header.Cmd == proto.CMD_AP14 {
									cachedMsg.Data = MakeLatestTimeLocationReplyMsg(cachedMsg.Header.Header.Cmd,
										cachedMsg.Header.Header.Imei, cachedMsg.Header.Header.ID, cachedMsg.Data)
								}

								c.responseChan <- cachedMsg
								cachedMsg.Header.Header.Status = 2
								cachedMsg.Header.Header.LastPushTime = proto.NewMsgID()
								tempCache = append(tempCache, cachedMsg)
							}else if cachedMsg.Header.Header.Status == 2 {
								//2, 消息已经发送，并处于等待手表确认的状态
								if msg.Header.Header.Version == proto.MSG_HEADER_ACK_PARSED &&
									msg.Header.Header.Imei == cachedMsg.Header.Header.Imei &&
									msg.Header.Header.ID == cachedMsg.Header.Header.ID {
									if msg.Header.Header.Cmd == proto.CMD_AP03 {
										syncTimeMsg := &proto.MsgData{}
										syncTimeMsg.Header.Header.Cmd = proto.CMD_AP03
										cachedMsg = syncTimeMsg
										tempCache = append(tempCache, cachedMsg)
									}else if msg.Header.Header.Cmd == proto.CMD_AP14 {
										sendLocationMsg := &proto.MsgData{}
										sendLocationMsg.Header.Header.Cmd = proto.CMD_AP14
										cachedMsg = sendLocationMsg
										tempCache = append(tempCache, cachedMsg)
									}else{
									}
								}else{ //未收到确认，如果当前消息是通知继续推送数据并且时间已经超过30s，才会发送
									if isPushCache {
										logging.Log(fmt.Sprintf("[%d] pushcache msg:%d",cachedMsg.Header.Header.Imei,cachedMsg.Header.Header.Cmd))
										timeout := (proto.NewMsgID() - cachedMsg.Header.Header.LastPushTime) / uint64(time.Second)
										if timeout >=  uint64(serverCtx.MaxPushDataTimeSecs) {
											logging.Log(fmt.Sprintf("[%d] push data timeout more than one day, need to remove/delete", msg.Header.Header.Imei))
										}else if  timeout > uint64(serverCtx.MinPushDataDelayTimeSecs) {
											//如果是AP03和AP14，则需要实时推送务器当前时间和手表最新定位
											if cachedMsg.Header.Header.Cmd == proto.CMD_AP03 ||  cachedMsg.Header.Header.Cmd == proto.CMD_AP14 {
												cachedMsg.Data = MakeLatestTimeLocationReplyMsg(cachedMsg.Header.Header.Cmd,
													cachedMsg.Header.Header.Imei, cachedMsg.Header.Header.ID, cachedMsg.Data)
											}

											c.responseChan <- cachedMsg
											cachedMsg.Header.Header.LastPushTime = proto.NewMsgID()
										}else{
											logging.Log(fmt.Sprintf("[%d] push data timeout less than 30s, no need to push,timeout = %d", msg.Header.Header.Imei,timeout))
											//还是要继续推送之
											//c.responseChan <- cachedMsg
											//cachedMsg.Header.Header.LastPushTime = proto.NewMsgID()
										}
									}
									//tempCache每次都初始化了，所以这里也要加上AP03和AP14消息,实时推送
									tempCache = append(tempCache, cachedMsg)
								}
							}else{
								tempCache = append(tempCache, cachedMsg)
							}

							//logging.Log("send app data to write routine")
						}

						if len(tempCache) > 0 {
							*cache = tempCache
						}else{
							delete(DevicePushCache, msg.Header.Header.Imei)
						}
						//logging.Log(fmt.Sprintf("[%d] cache --/ ack parse /--  count: %d", msg.Header.Header.Imei, len(*cache)))
					}
				/*}else{
					logging.Log(fmt.Sprintf("[%d] device idle no data over %d seconds",
						msg.Header.Header.Imei, serverCtx.MaxDeviceIdleTimeSecs ))
				}*/
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

//for reading read data command from watch
func ConnReadLoop(c *Connection, serverCtx *svrctx.ServerContext) {
	defer logging.PanicLogAndExit("ConnReadLoop")

	defer func() {
		logging.Log(fmt.Sprintf("[%d] client %d connection closed",c.imei,c.connid))
		delConnChan <- c
	}()

	logging.Log(fmt.Sprintf("new client connection %d from: ",c.connid) +  c.conn.RemoteAddr().String())

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

		//logging.Log(fmt.Sprintf("headerbuf:%s",headerBuf))

		//首先判断是加密协议还是明文协议
		protoVer := preparseMsgHeader(string(headerBuf))
		if protoVer != GT6_PROTO_V1 && protoVer != GT6_PROTO_V2 { //headerBuf[0] != '(' {
			logging.Log(fmt.Sprintf("[%d]bad format of data packet, it must begin with ( or G,headbuf:%s",c.imei,headerBuf))
			break
		}

		if protoVer == GT6_PROTO_V1 { //明文版本协议
			dataSize = uint16(proto.Str2Num(string(headerBuf[1:5]), 16))
		}else if protoVer == GT6_PROTO_V2 { // 加密版本协议
			dataSize = uint16(proto.Str2Num(string(headerBuf[2:6]), 16))
		}

		if dataSize < MsgMinSize || int(dataSize) >= MAX_TCP_REQUEST_LENGTH {
			logging.Log(fmt.Sprintf("data size in header is bad of  %d", dataSize))
			break
		}

		if len(c.buf) < int(dataSize) {
			buf := c.buf
			c.buf = make([]byte, dataSize)
			copy(c.buf[0: MsgHeaderSize], buf)
		}

		full := false
		bufSize := dataSize - MsgHeaderSize
		dataBuf := c.buf[MsgHeaderSize: MsgHeaderSize + bufSize]

		if serverCtx.IsDebug == false {
			c.conn.SetReadDeadline(time.Now().Add(time.Second * serverCtx.RecvTimeout))
		}
		n, err := io.ReadFull(c.conn, dataBuf)
		logging.Log(fmt.Sprintf("dataBuf:%s,len(dataBuf):%d,n = %d",dataBuf,len(dataBuf),n))
		if err == nil && n == len(dataBuf) {
			full = true
		}else {
			logging.Log(fmt.Sprintf("gt6 recv data failed, %s, recv %d bytes", err.Error(), n))
		}

		if !full  {
			break  //退出循环和go routine，连接关闭
		}

		msgLen := dataSize
		packet := c.buf[0: msgLen]

		if protoVer == GT6_PROTO_V2 { //加密协议则首先解密
			pkt, err := proto.Gt3AesDecrypt(string(c.buf[8: dataSize]))
			if err != nil {
				logging.Log("GT06 Gt3AesDecrypt data packet failed, " + err.Error())
				break
			}

			packet = pkt
			msgLen = uint16(len(packet))
		}

		//记录解密后(如果需要解密)的是些什么数据
		logging.Log("from watch data decryted: " + string(packet))

		if packet[msgLen - 1] != ')' || msgLen < 29 {
			logging.Log("bad format of data packet, it must end with )")
			break
		}

		//将IMEI和cmd解析出来
		imei := proto.Str2Num(string(packet[9:  24]), 10)
		cmd := string(packet[24 : 28])

		//记录收到的是些什么数据
		//if cmd == proto.StringCmd(proto.DRT_SEND_MINICHAT) {
		//	logging.Log("DRT_SEND_MINICHAT" + string(packet[0: msgLen]))
		//}else{
		//	logging.Log("NOT DRT_SEND_MINICHAT" + string(packet[0: msgLen]))
		//}

		//// 消息接收不完整，如果是微聊（BP34）等需要支持续传的请求，
		//// 则将当前不完整的数据加入续传队列，等待下一次连接以后进行续传
		//// 否则，为普通命令请求如BP01，BP09等，直接丢弃并关闭连接
		//if !full && cmd != proto.StringCmd(proto.DRT_SEND_MINICHAT) {
		//	break  //退出循环和go routine，连接关闭
		//}

		c.lastActiveTime = time.Now().Unix()

		c.imei = imei
		if c.saved == false && full {
			logging.Log("new TCP connect")
			c.saved = true
			addConnChan <- c
		}

		msg := &proto.MsgData{}
		msg.Header.Header.Version = proto.MSG_HEADER_VER_EX
		msg.Header.Header.Size = msgLen
		msg.Header.Header.From = proto.MsgFromDeviceToTcpServer
		msg.Header.Header.ID = proto.NewMsgID()
		msg.Header.Header.Imei = imei
		msg.Header.Header.Cmd = proto.IntCmd(cmd)
		msg.Header.Header.DevVersion = string(packet[5:9])
		msg.Data = make([]byte, msgLen - MsgHeaderSize)
		copy(msg.Data, packet[MsgHeaderSize: msgLen]) //不包含头部28字节
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
		if c.IsClosed(){
			logging.Log(fmt.Sprintf("[%d] requsetChan is closed",msg.Header.Header.Imei))
			break
		}
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
	defer logging.PanicLogAndExit("ConnWriteLoop")

	for   {
		select {
		case <-c.closeChan:
			logging.Log(fmt.Sprintf("[%d] write goroutine exit",c.imei))
			return
		case data := <-c.responseChan:
			if data == nil ||  c.IsClosed() { //连接关闭了，这里需要将响应数据推入续传队列
				logging.Log(fmt.Sprintf("[%d] connection closed, write goroutine exit",c.imei))

				//连接关闭了，这里需要将响应数据推入续传队列
				return
			}

			if n, err := c.conn.Write([]byte(data.Data)); err != nil {
				logging.Log(fmt.Sprintf("send data to client failed: %s,  %d bytes sent",  err.Error(), n))
			}else{
				sentData := string(data.Data)
				cmd := sentData[20: 24]
				count := 0
				if cmd == "AP12"  { //微聊，只打印80个字节
					count = 80
				}else if cmd == "AP13"  { //EPO，只打印40个字节
					count = 40
				}else if cmd == "AP23" { //亲情号头像，只打印60个字节
					count = 60
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
			logging.Log(fmt.Sprintf("[%d] business goroutine exit",c.imei))
			return
		case data := <-c.requestChan:
			if data == nil {
				logging.Log(fmt.Sprintf("[%d] gt06 connection closed, business goroutine exit",c.imei))
				return
			}

			proto.HandleTcpRequest(proto.RequestContext{IsDebug: serverCtx.IsDebug, IP: c.IP, Port: c.Port,
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
				GetAddressFunc:svrctx.GetAddress,
				FriendAvatar:serverCtx.FriendAvatar,
			})
		}
	}
}

func TcpServerRunLoop(serverCtx *svrctx.ServerContext)  {
	defer logging.PanicLogAndExit("TcpServerRunLoop")

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

		c := NewConn(conn)
		c.lastActiveTime = time.Now().Unix()

		//for reading
		go ConnReadLoop(c, serverCtx)

		//for business handler，业务处理的协程
		go BusinessHandleLoop(c, serverCtx)

		//for writing, 写协程等待一个channel的数据，将channel收到的数据发送至客户端
		go ConnWriteLoop(c)
	}
}

func MakeLatestTimeLocationReplyMsg(cmd uint16, imei, id uint64, data []byte) []byte{
	if len(data) == 0 {
		return data
	}

	offset := 0
	if cmd == proto.CMD_AP03 {
		//(003E357593060571398AP03,170706,024012,e0630,14CE9D6BDC6263B7)
		replyData := make([]byte, len(data))
		curTime := time.Now().UTC().Format("060102,150405")
		copy(replyData[offset: offset + 25], data[offset: offset + 25])
		offset += 25

		copy(replyData[offset: offset + 13], curTime[0: ])
		offset += 13

		copy(replyData[offset: offset + 6], data[offset: offset + 6])
		offset += 6

		copy(replyData[offset: offset + 18],  []byte(fmt.Sprintf(",%016X)", id)))
		offset += 18

		return replyData
	}else if cmd == proto.CMD_AP14{
		//(0054357593060081018AP1422.587725,113.913641,0,2017,07,06,03,30,13,00000DA2DB7AFA18)

		replyData, body  := "", ""
		curTime := time.Now().UTC().Format("2006,01,02,15,04,05")

		location := svrctx.GetDeviceData(imei, svrctx.Get().PGPool)
		body = string(data[5: 24]) + fmt.Sprintf("%06f,%06f,%d,", location.Lat, location.Lng, 200) + curTime +
			fmt.Sprintf(",%016X)", id)

		replyData = fmt.Sprintf("(%04X", 5 + len(body)) + body
		return []byte(replyData)
	}

	return data
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

func preparseMsgHeader(header string) int {
	if header[0] == '(' {//明文
		return GT6_PROTO_V1
	}else if header[0] == 'G' && header[1] == 'T' {//加密版本的协议，先解密
		return GT6_PROTO_V2
	}else{//非法
		logging.Log("bad header for GT06: " + header)
	}

	return GT6_PROTO_INVALID
}