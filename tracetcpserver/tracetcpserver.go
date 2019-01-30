package tracetcpserver

import (
	"../svrctx"
	"../logging"
	"net"
	"fmt"
	"os"
	"../proto"
	"strings"
	"time"
	"io"
	"encoding/hex"
)

var addGpsChan chan *Connection
var delGpsChan chan *Connection
var TcpGpsClientTable = map[uint64]*Connection{}
var GpsDevicePushCache = map[uint64]*[]*proto.MsgData{}
const (
	GPSMsgHeaderSize = 5
	GPSMsgMinSize = 13
	MAX_TCP_REQUEST_LENGTH = 1024 * 64
	GPS_PROTO_INVALID = -1
	GPS_PROTO_V1 = 1
	GPS_PROTO_V2 = 2

)

func init()  {
	logging.Log("gpstcpserver init")
	addGpsChan = make(chan *Connection, 10240)
	delGpsChan = make(chan *Connection, 10240)
}

//for managing connection, 对内负责管理tcp连接对象，对外为APP server提供通信接口
func ConnManagerLoop(serverCtx *svrctx.ServerContext)  {
	defer logging.PanicLogAndExit("gps ConnManagerLoop")

	for  {
		select {
		case connAdd:=<-addGpsChan:
			connDel,ok := TcpGpsClientTable[connAdd.imei]
			if ok && connDel != nil && connDel.connid != connAdd.connid{
				connDel.SetClosed()
				close(connDel.requestChan)
				close(connDel.responseChan)
				close(connDel.closeChan)
				connDel.conn.Close()
				delete(TcpGpsClientTable,connDel.imei)
				logging.Log(fmt.Sprintf("%d gps addConnChan connection deleted from TcpClientTable", connAdd.imei))
			}else {
				logging.Log(fmt.Sprintf("%d will delete connection from gps TcpClientTable, but connection not found", connAdd.imei))
			}
			TcpGpsClientTable[connAdd.imei] = connAdd
		case connDel := <-delGpsChan:
			if connDel == nil {
				logging.Log("main lopp exit, gps connection manager goroutine exit")
				return
			}
			theConn,ok := TcpGpsClientTable[connDel.imei]
			if ok && theConn != nil && theConn.connid == connDel.connid{
				connDel.SetClosed()
				close(connDel.requestChan)
				close(connDel.responseChan)
				close(connDel.closeChan)
				connDel.conn.Close()
				delete(TcpGpsClientTable,connDel.imei)
				logging.Log(fmt.Sprintf("%d gps connection deleted from TcpClientTable", connDel.imei))
			}else {
				logging.Log(fmt.Sprintf("%d delgpsConnChan will delete connection from TcpClientTable, but connection not found", connDel.imei))
			}

		case msg := <- serverCtx.GTGPSTcpServerChan:
			if msg == nil{
				logging.Log("gps main lopp exit, connection manager goroutine exit")
				return
			}

			//from APP to gps gator
			if msg.Header.Header.Version == proto.MsgFromAppServerToTcpServer{
				logging.Log(fmt.Sprintf("[%d] will send to device: %x", msg.Header.Header.Imei, msg.Data))
				c,ok1 := TcpGpsClientTable[msg.Header.Header.Imei]
				if ok1{
					//如果100秒内有数据，则认为连接良好
					curTimeSecs := time.Now().Unix()
					logging.Log(fmt.Sprintf("[%d] lastActiveTime: %d, curTimeSecs: %d, maxIdle: %d, result: %d", msg.Header.Header.Imei,
						c.lastActiveTime, curTimeSecs, serverCtx.MaxDeviceIdleTimeSecs, curTimeSecs - c.lastActiveTime))
					c.responseChan <-  proto.MakeReplyMsg(msg.Header.Header.Imei, false, msg.Data, msg.Header.Header.ID)
					logging.Log(fmt.Sprintf("[%d] gps msg has been sent, app has no need to send",  msg.Header.Header.Imei ))
				}else {
					logging.Log(fmt.Sprintf("[%d]will send app gps data to active device , " +
						"but device tcp connection not found, will notify app to send sms",
						msg.Header.Header.Imei))
				}
				break
			}
			isPushCache := msg.Header.Header.Version == proto.MSG_HEADER_PUSH_CACHE
			if len(msg.Data) == 0{
				//没有数据，表示这是一个通知的消息
				logging.Log(fmt.Sprintf("gps msg to notify to push cached data to device %d ",  msg.Header.Header.Imei))
			}else {
				 _,ok := GpsDevicePushCache[msg.Header.Header.Imei]
				if ok == false{
					GpsDevicePushCache[msg.Header.Header.Imei] = &[]*proto.MsgData{}
					syncTimeMsg := &proto.MsgData{}
					syncTimeMsg.Header.Header.Cmd = proto.CMD_GPS_SET_SYNC
					*GpsDevicePushCache[msg.Header.Header.Imei] = append(*GpsDevicePushCache[msg.Header.Header.Imei],syncTimeMsg)
				}

				if msg.Header.Header.Cmd == proto.CMD_GPS_SET_SYNC{
					if (*GpsDevicePushCache[msg.Header.Header.Imei])[0].Header.Header.ID != 0{
						tempID := (*GpsDevicePushCache[msg.Header.Header.Imei])[0].Header.Header.ID
						(*GpsDevicePushCache[msg.Header.Header.Imei])[0] = msg
						(*GpsDevicePushCache[msg.Header.Header.Imei])[0].Header.Header.ID = tempID
					}else {
						*GpsDevicePushCache[msg.Header.Header.Imei] = append(*GpsDevicePushCache[msg.Header.Header.Imei],msg)
					}
				}else {
					*GpsDevicePushCache[msg.Header.Header.Imei] = append(*GpsDevicePushCache[msg.Header.Header.Imei],msg)
				}
			}

			//取出连接conn
			c,ok := TcpGpsClientTable[msg.Header.Header.Imei]
			if !ok{
				logging.Log(fmt.Sprintf("[%d]will send app data to tcp connection from TcpGpsClientTable, but connection not found",
					msg.Header.Header.Imei))
			}else {
				cache,ok1 := GpsDevicePushCache[msg.Header.Header.Imei]
				if ok1 == false{
					logging.Log(fmt.Sprintf("[%d] no data gps cached to send", msg.Header.Header.Imei ))
				}else {
					tempCache := []*proto.MsgData{}
					for _,cachedMsg := range *cache{
						if len(cachedMsg.Data) == 0{
							tempCache = append(tempCache,cachedMsg)
							continue
						}
						logging.Log(fmt.Sprintf("gps tempcache:%d,%t,%d,%d",cachedMsg.Header.Header.Cmd,isPushCache,cachedMsg.Header.Header.Status,len(*cache)))
						if cachedMsg.Header.Header.Status == 0{
							c.responseChan <- cachedMsg
						}else if cachedMsg.Header.Header.Status == 1{
							//1, 消息尚未发送，且需要确认，则首先发出消息，并将status置2,
							if cachedMsg.Header.Header.Cmd == proto.CMD_GPS_SET_SYNC {
								cachedMsg.Data = MakeLatestTimeLocationReplyMsg(cachedMsg.Header.Header.Cmd,
									cachedMsg.Header.Header.Imei,cachedMsg.Header.Header.ID, cachedMsg.Data)
							}
							c.responseChan <- cachedMsg
							cachedMsg.Header.Header.Status = 2
							cachedMsg.Header.Header.LastPushTime = proto.NewMsgID()
							tempCache = append(tempCache,cachedMsg)
						}else if cachedMsg.Header.Header.Status == 2{
							//2, 消息已经发送，并处于等待手表确认的状态
							logging.Log(fmt.Sprintf("cachedMsg:%d,%d,%d,%d,%d,%t",
								msg.Header.Header.Version,msg.Header.Header.ID,cachedMsg.Header.Header.ID,
								msg.Header.Header.Imei,cachedMsg.Header.Header.Imei,isPushCache))
							if msg.Header.Header.Version == proto.MSG_HEADER_ACK_PARSED &&
								msg.Header.Header.Imei == cachedMsg.Header.Header.Imei &&
								msg.Header.Header.ID == cachedMsg.Header.Header.ID{
								//APP的设置消息成功推送至手表,不用继续推送，否则走else流程
								logging.Log(fmt.Sprintf("[%d] gps msg cmd:%d,%t",msg.Header.Header.Imei,msg.Header.Header.Cmd,isPushCache))
							}else {
								if isPushCache {
									timeout := (proto.NewMsgID() - cachedMsg.Header.Header.LastPushTime) / uint64(time.Second)
									if timeout > uint64(serverCtx.MaxPushDataTimeSecs) {
										logging.Log(fmt.Sprintf("[%d] push gps data timeout more than one day, need to remove/delete", msg.Header.Header.Imei))
									} else if timeout > uint64(serverCtx.MinPushDataDelayTimeSecs) {
										//如果是CMD_GPS_SYNC和CMD_GPS_LOCATION，则需要实时推送务器当前时间和手表最新定位
										if cachedMsg.Header.Header.Cmd == proto.CMD_GPS_SET_SYNC {
											cachedMsg.Data = MakeLatestTimeLocationReplyMsg(cachedMsg.Header.Header.Cmd,
												cachedMsg.Header.Header.Imei, cachedMsg.Header.Header.ID, cachedMsg.Data)
										}
										c.responseChan <- cachedMsg
										cachedMsg.Header.Header.LastPushTime = proto.NewMsgID()

									} else {
										logging.Log(fmt.Sprintf("[%d] push gps data timeout less than 30s, no need to push,timeout = %d",
											msg.Header.Header.Imei, timeout))
									}
								}
								tempCache = append(tempCache, cachedMsg)
							}
						}else {
							tempCache = append(tempCache,cachedMsg)
						}
					}

					if len(tempCache) > 0 {
						*cache = tempCache
					}else {
						delete(GpsDevicePushCache,msg.Header.Header.Imei)
					}
				}
			}
		}
	}
}

func MakeLatestTimeLocationReplyMsg(cmd uint16,imei,id uint64,data []byte) []byte {
	if len(data) == 0{
		return data
	}
	if cmd == proto.CMD_GPS_SET_SYNC {
		curtime := time.Now().UTC().Format("060102,150405")
		data[20] = byte(proto.Str2Num(curtime[0:2], 10))
		data[21] = byte(proto.Str2Num(curtime[2:4], 10))
		data[22] = byte(proto.Str2Num(curtime[4:6], 10))
		data[23] = byte(proto.Str2Num(curtime[7:9], 10))
		data[24] = byte(proto.Str2Num(curtime[9:11], 10))
		data[25] = byte(proto.Str2Num(curtime[11:], 10))

		return data
	}else {
		return data
	}
}

func Tracegpsserver(serverCtx *svrctx.ServerContext)  {
	defer logging.PanicLogAndExit("Tracegpsserver")
	tcpaddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", serverCtx.BindAddr, serverCtx.TracegpsPort ))
	if err != nil {
		logging.Log("Tracegpsserver resolve tcp address failed, " + err.Error())
		os.Exit(1)
	}
	listener,err := net.ListenTCP("tcp",tcpaddr)
	if err != nil {
		logging.Log("listen tcp address failed, " + err.Error())
		os.Exit(1)
	}

	defer func() {
		if listener != nil{
			listener.Close()
			close(addGpsChan)
			close(delGpsChan)
		}
		serverCtx.WaitLock.Done()
	}()


	//for managing connection, 对内负责管理tcp连接对象，对外为APP server提供通信接口
	go ConnManagerLoop(serverCtx)

	for  {
		conn,err := listener.AcceptTCP()
		if err != nil{
			err_ := err.(*net.OpError)
			if strings.Contains(err_.Err.Error(),"timeout") == false{
				logging.Log("accept connection failed, " + err.Error())
				return
			}else {
				continue
			}
		}
		c := NewConn(conn)
		c.lastActiveTime = time.Now().Unix()

		//for reading
		go ConnReadLoop(c,serverCtx)
		//for business handler，业务处理的协程
		go BusinessHandleLoop(c,serverCtx)

		//for writing, 写协程等待一个channel的数据，将channel收到的数据发送至客户端
		go ConnWriteLoop(c)
	}
}

//reading read data command from watch
func ConnReadLoop(c *Connection,serverCtx *svrctx.ServerContext)  {
	defer logging.PanicLogAndExit("gps ConnReadLoop")
	defer func() {
		logging.Log(fmt.Sprintf("gps client %d connection closed",c.connid))
		delGpsChan <- c
	}()

	logging.Log(fmt.Sprintf("new client connection %d from: ",c.connid) + c.conn.RemoteAddr().String())

	for{
		if len(c.buf) == 0{
			c.buf = make([]byte,256)
		}
		dataSize := uint16(0)
		headbuf := c.buf[0:GPSMsgHeaderSize]
		if serverCtx.IsDebug == false {
			c.conn.SetReadDeadline(time.Now().Add(time.Second * serverCtx.RecvTimeout))
		}
		if _,err := io.ReadFull(c.conn,headbuf);err != nil{
			logging.Log("recv gps MsgHeaderSize bytes header failed, " + err.Error())
			break
		}
		c.lastActiveTime = time.Now().Unix()
		//首先判断是加密协议还是明文协议
		protoVer := parseHeader(headbuf)
		//logging.Log(fmt.Sprintf("headbuf:%x",headbuf,protoVer))
		if protoVer != GPS_PROTO_V1 && protoVer != GPS_PROTO_V2{
			logging.Log("bad format of data packet, it must begin with 0x24 or 0x26")
			break
		}
		if protoVer == GPS_PROTO_V1{//明文版本协议
			dataSize = uint16(headbuf[2]) << 8 + uint16(headbuf[3])

		}else if protoVer == GPS_PROTO_V2{ // 加密版本协议
			dataSize = uint16(headbuf[3]) << 8 + uint16(headbuf[4]) + 5
		}
		if dataSize < GPSMsgMinSize || int(dataSize) >= MAX_TCP_REQUEST_LENGTH {
			logging.Log(fmt.Sprintf("gps data size in header is bad of  %d", dataSize))
			break
		}
		if len(c.buf) < int(dataSize){
			buf := c.buf
			c.buf = make([]byte, dataSize)
			copy(c.buf[0: GPSMsgHeaderSize], buf)
		}

		full := false
		bufSize := dataSize - GPSMsgHeaderSize
		dataBuf := c.buf[GPSMsgHeaderSize: GPSMsgHeaderSize + bufSize]
		if serverCtx.IsDebug == false {
			c.conn.SetReadDeadline(time.Now().Add(time.Second * serverCtx.RecvTimeout))
		}
		//把剩下的数据读出来
		n,err := io.ReadFull(c.conn,dataBuf)

		if err == nil && n == len(dataBuf){
			full = true
		}else {
			logging.Log(fmt.Sprintf("gps recv data failed, %s, recv %d bytes", err.Error(), n))
		}
		if !full {
			break
		}
		//把超时取消
		if serverCtx.IsDebug == false {
			c.conn.SetReadDeadline(time.Time{})
		}
		msgLen := dataSize
		packet := c.buf[0: msgLen]
		//logging.Log(fmt.Sprintf("packet:%x\n",packet))
		if protoVer == GPS_PROTO_V2{//加密协议则首先解密
			pkt,err := proto.GPSAesDecrypt(c.buf[5:dataSize])
			if err != nil {
				logging.Log("gps Gt3AesDecrypt data packet failed, " + err.Error())
				break
			}
			packet = pkt
			msgLen = uint16(len(packet))
		}

		//记录解密后(如果需要解密)的是些什么数据
		logging.Log(fmt.Sprintf("gps from watch data decryted: %x",packet))
		if packet[msgLen - 2] != 0x0D || packet[msgLen - 1] != 0x0A{
			logging.Log("gps bad format of data packet, it must end with 0x0D 0x0A")
			break
		}

		//将IMEI和cmd解析出来

			imei := proto.Str2Num(hex.EncodeToString(packet[4:12]), 10)
			//协议号
			cmd := hex.EncodeToString(packet[12:14])
			c.lastActiveTime = time.Now().Unix()
			c.imei = imei
			if c.saved == false && full {
				logging.Log("new TCP connect")
				c.saved = true
				addGpsChan <- c
			}
			fmt.Println("addGpsChan", imei, cmd)
			msg := &proto.MsgData{}
			msg.Header.Header.Version = proto.MSG_HEADER_VER_EX
			msg.Header.Header.Size = msgLen
			msg.Header.Header.From = proto.MsgFromDeviceToTcpServer
			msg.Header.Header.ID = proto.NewMsgID()
			msg.Header.Header.Imei = imei
			msg.Header.Header.Cmd = proto.IntGpsCmd(cmd)
			msg.Data = make([]byte, msgLen-12)
			copy(msg.Data, packet[12: msgLen]) //不包含头部12字节
			//没有语音发送
			msg.Header.Header.Status = 0

			if c.IsClosed(){
				logging.Log(fmt.Sprintf("[%d] gps requsetChan is closed",msg.Header.Header.Imei))
				break
			}
			c.requestChan <- msg

	}
}

func parseHeader(header []byte) int{
	if len(header) <= 2{
		return GPS_PROTO_INVALID
	}
	if header[0] == 0x24 && header[1] == 0x24{
		return GPS_PROTO_V1

	}else if header[0] == 0x26 && header[1] == 0x26{
		return GPS_PROTO_V2
	}else {
		logging.Log("bad header for GPS: " + string(header))
		return GPS_PROTO_INVALID
	}
}

func BusinessHandleLoop(c *Connection,serverCtx *svrctx.ServerContext)  {
	defer logging.PanicLogAndExit("gps BusinessHandleLoop")
	for  {
		select {
		case <- c.closeChan:
			logging.Log("gps business goroutine exit")
			return
		case data := <- c.requestChan:
			if data == nil {
				logging.Log("gps connection closed, business goroutine exit")
				return
			}

			proto.HandleGpsRequest(proto.RequestContext{IsDebug: serverCtx.IsDebug, IP: c.IP, Port: c.Port,
				APNSServerBaseURL: serverCtx.APNSServerApiBase,
				Pgpool: serverCtx.PGPool,
				MysqlPool: serverCtx.MySQLPool,
				WritebackChan: serverCtx.TcpServerChan,
				AppNotifyChan: serverCtx.AppServerChan,
				Msg: data,
				GetDeviceDataFunc: svrctx.GetDeviceData,
				SetDeviceDataFunc: svrctx.SetDeviceData,
				GetAddressFunc:svrctx.GetAddress,})
		}
	}
}

func ConnWriteLoop(c *Connection)  {
	defer logging.PanicLogAndExit("gps ConnWriteLoop")
	for{
		select {
		case <- c.closeChan:
			logging.Log("gps write goroutine exit")
			return
		case data := <- c.responseChan:
			if data == nil ||  c.IsClosed() { //连接关闭了，这里需要将响应数据推入续传队列
				logging.Log("gps connection closed, write goroutine exit")

				//连接关闭了，这里需要将响应数据推入续传队列
				return
			}

			logging.Log(fmt.Sprintf("send gps data:%x",data.Data))
			if n,err := c.conn.Write([]byte(data.Data));err != nil{
				logging.Log(fmt.Sprintf("gps send data to client failed: %s,  %d bytes sent",  err.Error(), n))
			}
		}
	}
}
