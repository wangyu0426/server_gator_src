package proto

import (
	"fmt"
	"strconv"
	"../logging"
	"time"
	"strings"
	"math"
	_ "github.com/go-sql-driver/mysql"
	"encoding/json"
	"net/http"
	"io/ioutil"
	"os"
	"bytes"
)

type GT03Service struct {
	imei uint64
	msgSize uint64
	cmd uint16
	isCmdForAck bool
	msgAckId uint64
	old LocationData
	cur LocationData
	fetchType string
	needSendLocation bool
	needSendChatNum bool
	needSendChat bool
	reqChatInfoNum int
	needSendPhotoNum bool
	needSendPhoto bool
	reqPhotoInfoNum int
	getSameWifi bool
	wifiInfoList [MAX_WIFI_NUM + 1]WIFIInfo
	wiFiNum uint16
	wifiZoneIndex int16
	hasSetHomeWifi bool
	lbsNum uint16
	lbsInfoList [MAX_LBS_NUM + 1]LBSINFO
	accracy uint32
	MCC int32
	MNC int32
	LACID int32
	CELLID int32
	RXLEVEL int32
	chat  ChatInfo
	chatData []byte
	rspList []*ResponseItem
	reqCtx RequestContext
}

func HandleGt3TcpRequest(reqCtx RequestContext)  bool{
	service := &GT03Service{reqCtx: reqCtx}
	ret := service.PreDoRequest()
	if ret == false {
		return false
	}

	ret = service.DoRequest(reqCtx.Msg)
	//if ret == false {
	//	return false
	//}

	msgReplyList := service.DoResponse()
	if msgReplyList  != nil && len(msgReplyList) > 0 {
		for _, msg := range msgReplyList {
			reqCtx.WritebackChan <- msg
		}
	}

	//通知ManagerLoop, 将上次缓存的未发送的数据发送给手表
	msgNotify := &MsgData{}
	msgNotify.Header.Header.Version = MSG_HEADER_PUSH_CACHE
	//logging.Log("MSG_HEADER_PUSH_CACHE, imei: " + proto.Num2Str(imei, 10))
	msgNotify.Header.Header.Imei = service.imei
	reqCtx.WritebackChan <- msgNotify

	return true
}

func (service *GT03Service)makeReplyMsg(data []byte) *MsgData{
	return MakeGt3ReplyMsg(service.imei, data)
}


func (service *GT03Service)makeAckParsedMsg(id uint64) *MsgData{
	msg := MsgData{}
	msg.Header.Header.Version = MSG_HEADER_ACK_PARSED
	msg.Header.Header.Imei = service.imei
	msg.Header.Header.Cmd = service.cmd
	msg.Header.Header.ID = id

	return &msg
}

func (service *GT03Service)PreDoRequest() bool  {
	service.wifiZoneIndex = -1
	service.cur.ZoneIndex = -1
	service.old.ZoneIndex = -1
	service.cur.LastZoneIndex = -1
	service.old.LastZoneIndex = -1
	service.needSendLocation = false
	service.needSendChatNum = true
	service.needSendPhotoNum = true
	service.isCmdForAck = false

	return true
}

func (service *GT03Service)DoRequest(msg *MsgData) bool  {
	//logging.Log(fmt.Sprintf("imei: %d cmd: %s; go routines: %d", msg.Header.Header.Imei, StringCmd(msg.Header.Header.Cmd), runtime.NumGoroutine()))

	ret := true
	//bufOffset := uint32(20)
	service.msgSize = uint64(msg.Header.Header.Size)
	service.imei = msg.Header.Header.Imei
	service.cmd = msg.Header.Header.Cmd

	service.cur.Imei = service.imei

	if IsDeviceInCompanyBlacklist(service.imei) {
		logging.Log(fmt.Sprintf("device %d  is in the company black list", service.imei))
		return false
	}

	DeviceInfoListLock.Lock()
	deviceInfo, ok := (*DeviceInfoList)[msg.Header.Header.Imei]
	if ok == false {
		DeviceInfoListLock.Unlock()
		logging.Log(fmt.Sprintf("invalid deivce, imei: %d cmd: %s", msg.Header.Header.Imei, StringCmd(msg.Header.Header.Cmd)))
		return false
	}else{
		//if deviceInfo != nil {
		//	resp := &ResponseItem{CMD_GT3_AP01_RESET_IP_PORT, service.makeReplyMsg(
		//		service.makeResetHostPortReplyMsg("72.55.174.166", 7014))}
		//	service.rspList = append(service.rspList, resp)
		//
		//	DeviceInfoListLock.Unlock()
		//	logging.Log(fmt.Sprintf("%d reset gator 2 server to host:port %s:%d", msg.Header.Header.Imei,
		//		deviceInfo.CompanyHost, deviceInfo.CompanyPort))
		//	return true
		//}
		if deviceInfo != nil  && (deviceInfo.RedirectServer || deviceInfo.RedirectIPPort) && deviceInfo.CompanyHost != "" && deviceInfo.CompanyPort != 0{
			resp := &ResponseItem{CMD_GT3_AP01_RESET_IP_PORT, service.makeReplyMsg(
				service.makeResetHostPortReplyMsg(deviceInfo.CompanyHost, deviceInfo.CompanyPort))}
			service.rspList = append(service.rspList, resp)

			DeviceInfoListLock.Unlock()
			logging.Log(fmt.Sprintf("%d reset server to host:port %s:%d", msg.Header.Header.Imei,
				deviceInfo.CompanyHost, deviceInfo.CompanyPort))
			return true
		}
	}
	DeviceInfoListLock.Unlock()

	service.GetWatchDataInfo(service.imei)

	if service.cmd >=  DRT_GT3_BP01_LOCATION && service.cmd <=  DRT_GT3_BPM2_LOCATION_ALARM {
		//定位数据：经纬度、时间、步数、电量、报警
		//首先解析消息中的各字段数据
		parseMsgString((msg.Data[0:]), service)
		ret = service.ProcessLocate()
		if ret == false{
			logging.Log("ProcessLocate failed")
		}else{
			if true || service.needSendLocation {
				madeData := service.makeSendLocationReplyMsg()
				resp := &ResponseItem{CMD_GT3_AP18_SET_TIME_AND_LOCATION, service.makeReplyMsg(madeData)}
				service.rspList = append(service.rspList, resp)
			}
		}
	}else if service.cmd == DRT_GT3_BP00_SYNC_TIME {
		//开机对时
		madeData := service.makeSyncTimeReplyMsg()
		resp := &ResponseItem{CMD_GT3_AP03_SET_TIMEZONE,  service.makeReplyMsg(madeData)}
		service.rspList = append(service.rspList, resp)
		//bufOffset++
		//szVersion := make([]byte, 64)
		//index := 0
		//for i := 0; i < len(msg.Data); i++ {
		//	if msg.Data[bufOffset] != ',' && index < 64 {
		//		szVersion[index] = msg.Data[bufOffset]
		//		index++
		//		bufOffset++
		//	}else {break}
		//}
		//
		//logging.Log(fmt.Sprintf("%d|%s", service.imei, szVersion)) // report version
	}else if service.cmd ==  DRT_GT3_BP04_LAST_CMD_ACK {     	//  bp04 ACK
		if strings.Contains(string(msg.Data[0:]), Gt3StringCmd(CMD_GT3_AP15_PUSH_CHAT_COUNT)) {
			service.needSendChatNum = false
		}
	}else if service.cmd == DRT_GT3_BP06_SEND_CHAT {
		//BP06,手表发送微聊
		//首先解析消息中的各字段数据
		parseMsgString((msg.Data[0:]), service)

		ret = service.ProcessMicChat()
		if ret == false {
			logging.Log("ProcessMicChat Error")
			return false
		}
	}else if service.cmd == DRT_GT3_BP07_REQUEST_CHAT {
		//BP07,手表请求下载微聊
		//解析消息中的各字段数据
		parseMsgString((msg.Data[0:]), service)
		service.needSendChat = true
	}else if service.cmd == DRT_GT3_BP08_REQUEST_EPO {
		//BP08, 手表请求EPO数据
		ret = service.ProcessRspAGPSInfo()
		if ret == false {
			logging.Log("ProcessRspAGPSInfo Error")
			return  false
		}
	}else if service.cmd == DRT_GT3_BP09_HEART_BEAT {
		//BP09  heart beat
		//解析消息中的各字段数据
		parseMsgString((msg.Data[0:]), service)
		if service.cur.DataTime == 0 {
			service.cur.DataTime = service.old.DataTime
		}

		ret = service.ProcessUpdateWatchStatus()
		if ret == false {
			logging.Log("ProcessUpdateWatchStatus Error")
			return false
		}
	}else {
		logging.Log(fmt.Sprintf("Error Device CMD(%d, %s)", service.imei, StringCmd(service.cmd)))
	}

	if ret {
		ret = service.PushCachedData()
	}

	return ret
}

func (service *GT03Service)CountSteps() {
	//170520114920
	if service.old.DataTime == 0 || (service.cur.DataTime / 1000000 ==  service.old.DataTime / 1000000) {
		service.cur.Steps += service.old.Steps
	}
}

func (service *GT03Service)DoResponse() []*MsgData  {
	if service.needSendChat {
		logging.Log(fmt.Sprint("ProcessRspFetchFile 4, ", service.needSendChat))
		service.ProcessRspChat()
	}

	if service.needSendChatNum {
		service.PushChatNum()
	}

	//offset, bufSize := 0, 256
	//data := make([]byte, bufSize)
	//
	//for  _, respCmd := range service.rspList {
	//	if respCmd.data != nil && len(respCmd.data) > 0 {
	//		if offset + len(respCmd.data) > bufSize {
	//			bufSize += offset + len(respCmd.data)
	//			dataNew := make([]byte, bufSize)
	//			copy(dataNew[0: offset], data[0: offset])
	//			data = dataNew
	//		}
	//		copy(data[offset: offset + len(respCmd.data)], respCmd.data)
	//		offset += len(respCmd.data)
	//	}
	//}
	//
	//if offset == 0 {
	//	return nil
	//}else {
	//	return data[0: offset]
	//}
	if len(service.rspList) > 0 {
		msgList := []*MsgData{}
		for  _, respCmd := range service.rspList {
			msg := MsgData{}
			msg = *respCmd.msg
			msg.Header.Header.Cmd = respCmd.rspCmdType
			msg.Data = make([]byte, len(respCmd.msg.Data))
			copy(msg.Data[0:], respCmd.msg.Data[0:])
			msgList = append(msgList, &msg)
		}

		return msgList
	}else{
		return nil
	}
}

func (service *GT03Service) PushChatNum() bool {
	if service.reqCtx.GetChatDataFunc != nil {
		chatData := service.reqCtx.GetChatDataFunc(service.imei, -1)
		if len(chatData) > 0 {
			//通知终端有聊天信息
			//（357593060153353AP1502）
			body := fmt.Sprintf("(%015dAP15%02d)", service.imei, len(chatData))
			resp := &ResponseItem{CMD_GT3_AP15_PUSH_CHAT_COUNT,
				service.makeReplyMsg([]byte(body))}
			service.rspList = append(service.rspList, resp)
		}
	}

	return true
}

func (service *GT03Service)UpdateDeviceTimeZone(imei uint64, timezone int) bool {
	DeviceInfoListLock.Lock()
	_, ok := (*DeviceInfoList)[service.imei]
	if ok {
		(*DeviceInfoList)[service.imei].TimeZone = timezone
	}
	DeviceInfoListLock.Unlock()

	//时区写入数据库
	szTimeZone, signed,hour, min := "", "", int(timezone / 100), timezone % 100
	if timezone > 0 {
		signed = "+"
	}else{
		signed = "-"
		if timezone == 0 {
			signed = ""
		}

		hour, min = int(-timezone / 100), -timezone % 100
	}

	szTimeZone = fmt.Sprintf("%s%02d:%02d", signed,  hour, min)

	strSQL := fmt.Sprintf("UPDATE watchinfo SET TimeZone='%s'  where IMEI='%d'", szTimeZone, service.imei)

	logging.Log("SQL: " + strSQL)
	_, err := service.reqCtx.MysqlPool.Exec(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] update time zone into db failed, %s", service.imei, err.Error()))
		return false
	}

	return true
}

func (service *GT03Service)makeSyncTimeReplyMsg() ([]byte) {
	//(357593060153353AP03,150728,152900,e0800)
	curTime := time.Now().UTC().Format("060102,150405")
	c, timezone := 'e', 0

	DeviceInfoListLock.Lock()
	device, ok := (*DeviceInfoList)[service.imei]
	if ok {
		timezone = device.TimeZone
	}
	DeviceInfoListLock.Unlock()

	if timezone == INVALID_TIMEZONE {
		timezone = int(GetTimeZone(service.reqCtx.IP))
		logging.Log(fmt.Sprintf("imei %d, IP %d  GetTimeZone: %d", service.imei, service.reqCtx.IP, timezone))
		service.UpdateDeviceTimeZone(service.imei, timezone)
	}

	if timezone < 0 {
		c = 'w'
		timezone = -timezone
	}

	body := fmt.Sprintf("(%015dAP03,%s,%c%04d)", service.imei,curTime, c, timezone)

	return []byte(body)
}

func (service *GT03Service)makeSendLocationReplyMsg() ([]byte) {
	//(357593060153353AP1824.772816,121.022636,160,2015,11,12,08,00,00)

	var lat, lng float64
	accracy := uint32(200)
	if service.old.Lat == 0{
		lat = service.cur.Lat
		lng = service.cur.Lng
		//accracy = service.cur.Accracy
	}else{
		lat = service.old.Lat
		lng = service.old.Lng
		//accracy = service.old.Accracy
	}

	curTime := time.Now().UTC().Format("2006,01,02,15,04,05")
	body := fmt.Sprintf("(%015dAP18%06f,%06f,%d,%s)",
		service.imei, lat, lng, accracy, curTime)

	return []byte(body)
}

func (service *GT03Service)makeDeviceLoginReplyMsg() []byte {
	//(0019357593060153353AP31)
	body := fmt.Sprintf("%015dAP31)", service.imei)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}

func (service *GT03Service)makeResetHostPortReplyMsg(host string, port int) ([]byte) {
	//(357593060571398AP01221.18.79.110#123)
	body := fmt.Sprintf("(%015dAP01%s#%d)", service.imei, host, port)

	return []byte(body)
}

func (service *GT03Service)makeAppURLReplyMsg() ([]byte, uint64) {
	id := makeId()
	body := fmt.Sprintf("%015dAP26,%d,%s,%d,%s,%016X)", service.imei,
		len(service.reqCtx.IOSAppURL), service.reqCtx.IOSAppURL,
		len(service.reqCtx.AndroidAppURL), service.reqCtx.AndroidAppURL, id)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body), id
}

func (service *GT03Service)makeDeviceChatAckMsg(phone, datatime, milisecs,
		blockCount, blockIndex string,  success int) []byte {
	//(003D357593060153353AP34,13026618172,170413163300,2710,6,1,1)
	body := fmt.Sprintf("%015dAP34,%s,%s,%s,%s,%s,%d)", service.imei, phone, datatime, milisecs,
		blockCount, blockIndex, success)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}


func (service *GT03Service)makeChatDataReplyMsg(voiceFile, phone string, datatime uint64,
	index int) (*ResponseItem) {
	//(357593060153353AP1627D6+数据)

	//读取语音文件，并分包
	voice, err := ioutil.ReadFile(voiceFile)
	if err != nil {
		logging.Log("read voice file failed, " + voiceFile + "," + err.Error())
		return nil
	}

	if len(voice) == 0 {
		logging.Log("voice file empty, 0 bytes, ")
		return nil
	}

	iReadLen := len(voice)
	body := make([]byte, 24 + iReadLen + 1)
	header := fmt.Sprintf("(%015dAP16%04X", service.imei, iReadLen + 25)
	copy(body[0: 24], header[0: ])
	copy(body[24: 24 + iReadLen], voice)

	body[24 + iReadLen] = ')'
	return &ResponseItem{CMD_GT3_AP16_PUSH_CHAT_DATA,  service.makeReplyMsg(body)}
}

func (service *GT03Service)makeDataBlockReplyMsg(block *DataBlock) []byte {
	//(03B5357593060153353AP12,1,170413163300,6,1,900,数据)
	//(03A6357593060153353AP13,7,1,900,数据)
	body := ""
	if block.Cmd == "AP12" {
		body = fmt.Sprintf("%015d%s,%s,%d,%d,%d,%d,", block.Imei, block.Cmd, block.Phone, block.Time,
			block.BlockCount, block.BlockIndex, block.BlockSize)
	}else if block.Cmd == "AP13" {
		body = fmt.Sprintf("%015d%s,%d,%d,%d,", block.Imei, block.Cmd,
			block.BlockCount, block.BlockIndex, block.BlockSize)
	}else if block.Cmd == "AP23" {
		//(03B2357593060153353AP23,13026618172,6,1,700,数据)
		body = fmt.Sprintf("%015d%s,%s,%d,%d,%d,", block.Imei, block.Cmd, block.Phone,
			block.BlockCount, block.BlockIndex, block.BlockSize)
	}

	//tail := fmt.Sprintf(",%016X)", block.MsgId)
	tail := ")"
	sizeNum := 5 + len(body) + len(block.Data) + len(tail)
	size := fmt.Sprintf("(%04X", sizeNum)

	data := make([]byte, sizeNum)
	copy(data[0: 5 + len(body)], []byte(size + body))
	copy(data[5 + len(body) : 5 + len(body) + len(block.Data)], block.Data)
	copy(data[5 + len(body) + len(block.Data): ], []byte(tail))

	fmt.Print(size, body, tail)
	return data
}

func (service *GT03Service)makeFileNumReplyMsg(fileType, chatNum int) []byte {
	return MakeFileNumReplyMsg(service.imei, fileType, chatNum, false)
}

func (service *GT03Service) ProcessLocate() bool {
	logging.Log(fmt.Sprintf("%d - begin: m_iAlarmStatu=%d", service.imei, service.cur.AlarmType))
	ret := true

	if IsDeviceInCompanyBlacklist(service.imei) {
		service.needSendLocation = false
		logging.Log(fmt.Sprintf("device %d comamy is in the black list", service.imei))
		return false
	}

	isDisableLBS :=  IsCompanyDisableLBS(service.imei)

	 if service.cur.LocateType == LBS_WIFI {
		isDisableWiFi := IsDisableWiFi(service.imei)
		if isDisableWiFi {
			service.needSendLocation = false
			logging.Log(fmt.Sprintf("device %d, wifi location  need disabled", service.imei))
			return false
		}

		if service.wiFiNum <= 1 {
			service.needSendLocation = false
			logging.Log(fmt.Sprintf("device %d, ProcessWifiInfo m_shWiFiNum <= 1", service.imei))
			return false
		}
	}else if service.cur.LocateType == LBS_JIZHAN {
		if  isDisableLBS {
			service.needSendLocation = false
			logging.Log(fmt.Sprintf("device %d, LBS location and need disable lbs", service.imei))
			return false
		}
	}else if service.cur.LocateType != LBS_GPS  && service.cur.LocateType != LBS_SMARTLOCATION{
		logging.Log(fmt.Sprintf("%d - Error Locate Info %d", service.imei,  service.cur.LocateType))
		return false
	}

	service.cur.AlarmType &= (^uint8(ALARM_BATTERYLOW))

	//process location
	if service.cur.LocateType == LBS_JIZHAN {
		ret = service.ProcessLBSInfo()
	} else if service.cur.LocateType == LBS_WIFI {
		ret = service.ProcessMutilLocateInfo()
	}else if service.cur.LocateType == LBS_GPS {
		service.cur.Battery = 1
		if  service.cur.OrigBattery >= 3 {
			service.cur.Battery =  service.cur.OrigBattery - 2
		}
	}

	service.CountSteps()

	logging.Log(fmt.Sprintf("%d - middle: m_iAlarmStatu=%d, parsed location:  m_DateTime=%d, m_lng=%f, m_lat=%f",
		service.imei, service.cur.AlarmType, service.cur.DataTime, service.cur.Lng, service.cur.Lat))

	if ret == false ||  service.cur.LocateType == LBS_INVALID_LOCATION  || service.cur.Lng == 0 && service.cur.Lat== 0 {
		service.needSendLocation = false
		logging.Log(fmt.Sprintf("Error Locate(%d, %06f, %06f)", service.imei, service.cur.Lng, service.cur.Lat))
		return ret
	}

	if service.getSameWifi && service.cur.AlarmType <= 0  {
		logging.Log(fmt.Sprintf("return with: m_bGetSameWifi && m_iAlarmStatu <= 0(%d, %d, %d)",
			service.imei, service.cur.Lat, service.cur.Lng))
		return ret
	}

	//范围报警
	ret = service.ProcessZoneAlarm()
	logging.Log(fmt.Sprintf("end: m_iAlarmStatu=%d",  service.cur.AlarmType))
	logging.Log(fmt.Sprintf("Update database: DeviceID=%d, m_DateTime=%d, m_lng=%f, m_lat=%f",
		service.imei, service.cur.DataTime, service.cur.Lng, service.cur.Lat))

	if service.cur.AlarmType & ALARM_INZONE  != 0 || service.cur.AlarmType & ALARM_OUTZONE  != 0 {
		service.cur.LastAlarmType = service.cur.AlarmType
		if service.cur.ZoneIndex >= 0 {
			service.cur.LastZoneIndex = service.cur.ZoneIndex
		}
	}

	ret = service.WatchDataUpdateDB()
	if ret == false {
		logging.Log(fmt.Sprintf("Update WatchData into Database failed"))
		return false
	}

	if service.cur.LocateType != LBS_INVALID_LOCATION {
		//同时，把数据写入内存服务器
		ret  = service.UpdateWatchData()
		if ret == false  {
			logging.Log(fmt.Sprintf("UpdateWatchData failed"))
			return false
		}
	} else {
		//更新电池状态
		ret = service.UpdateWatchBattery()
		if ret == false {
			logging.Log(fmt.Sprintf("UpdateWatchBattery failed, battery=%d", service.cur.Battery))
			return false
		}
	}

	if service.cur.AlarmType > 0  {
		//推送报警
		service.NotifyAlarmMsg()
	}

	service.NotifyAppWithNewLocation()

	return true
}

func (service *GT03Service)  NotifyAppWithNewLocation() bool  {
	result := HeartbeatResult{Timestamp: time.Now().Format("20060102150405")}
	result.Locations = append(result.Locations, service.cur)

	service.reqCtx.AppNotifyChan  <- &AppMsgData{Cmd: HearbeatAckCmdName,
		Imei: service.imei,
		Data: MakeStructToJson(result), ConnID: 0}
	return true
}

func (service *GT03Service)  NotifyAppWithNewMinichat(chat ChatInfo) bool  {
	return NotifyAppWithNewMinichat(service.reqCtx.APNSServerBaseURL, service.imei, service.reqCtx.AppNotifyChan, chat)
}

func (service *GT03Service) ProcessMicChat() bool {
	//(357593060153353BP0627D6,2710,15101016010000+数据)
	ret := true
	fileId := uint64(service.chat.DateTime * 10000) + uint64(service.chat.VoiceMilisecs)
	newChatInfo := ChatInfo{}
	newChatInfo.CreateTime = NewMsgID()
	newChatInfo.Imei = service.imei
	newChatInfo.Sender = Num2Str(service.imei, 10)
	newChatInfo.SenderType = 0
	newChatInfo.FileID = uint64(service.chat.DateTime * 10000) + uint64(service.chat.VoiceMilisecs)
	newChatInfo.Content = Num2Str(fileId, 10)
	newChatInfo.DateTime = service.chat.DateTime
	newChatInfo.DateTime = newChatInfo.DateTime / 10000
	newChatInfo.Receiver = ""
	newChatInfo.VoiceMilisecs = service.chat.VoiceMilisecs

	filePathDir := fmt.Sprintf("%s%d", service.reqCtx.MinichatUploadDir + "watch/", service.imei)
	os.MkdirAll(filePathDir, 0755)
	filePath := fmt.Sprintf("%s/%d.amr", filePathDir, fileId)
	ioutil.WriteFile(filePath, service.chatData, 0666)

	args := fmt.Sprintf("-i %s -acodec libfaac -ab 64k -ar 44100 %s/%d.aac", filePath, filePathDir, fileId)
	err2, _ := ExecCmd("ffmpeg",  strings.Split(args, " ")...)
	if err2 != nil {
		logging.Log(fmt.Sprintf("[%d] ffmpeg %s failed, %s", service.imei, args, err2.Error()))
	}

	//通知APP有新的微聊信息。。。
	newChatInfo.Content = fmt.Sprintf("%swatch/%d/%d.aac", service.reqCtx.DeviceMinichatBaseUrl,
		service.imei, fileId)
	AddChatForApp(newChatInfo)
	service.NotifyAppWithNewMinichat(newChatInfo)

	return ret
}

func (service *GT03Service) ProcessRspFetchFile(pszMsg []byte) bool {
	//(00250103357593060153353BP11,AP12,01)  chat
	//(00250103357593060153353BP11,AP23,01)  photo
	fields := strings.SplitN(string(pszMsg), ",", 2)
	if len(fields) != 2 {
		logging.Log(fmt.Sprintf("[%d] cmd field count %d is bad", service.imei, len(fields)))
		return false
	}

	if fields[0] != "AP12" && fields[0] != "AP23" {
		logging.Log(fmt.Sprintf("[%d] cmd type %s is bad", service.imei, fields[0]))
		return false
	}

	fileNum := Str2Num(fields[1][0:2], 10)
	if fileNum  != 1  {
		logging.Log(fmt.Sprintf("[%d] require file count  %d is bad", service.imei, fileNum))
		return false
	}

	if fields[0] ==  "AP12" { //chat
		service.needSendChat = true
		service.needSendChatNum = false
		service.reqChatInfoNum = int(fileNum)
		logging.Log(fmt.Sprint("ProcessRspFetchFile 2, ", service.needSendChat,
			service.needSendChatNum, service.reqChatInfoNum))
	}else if  fields[0] ==  "AP23" {
		//photo
		service.needSendPhoto = true
		service.needSendPhotoNum = false
		service.reqPhotoInfoNum = int(fileNum)
		logging.Log(fmt.Sprint("ProcessRspFetchFile 3, ", service.needSendPhoto,
			service.needSendPhotoNum, service.reqPhotoInfoNum))
	}

	return true
}

func (service *GT03Service) ProcessRspAGPSInfo() bool {
	//(357593060153353AP176C00+数据)
	utcNow := time.Now().UTC()
	iCurrentGPSHour := utc_to_gps_hour(utcNow.Year(), int(utcNow.Month()),utcNow.Day(), utcNow.Hour())
	segment := (uint32(iCurrentGPSHour) - StartGPSHour) / 6
	if service.reqCtx.IsDebug == false && ( (segment < 0) || (segment >= MTKEPO_SEGMENT_NUM)) {
		logging.Log(fmt.Sprintf("[%d] EPO segment invalid, %d", service.imei, segment))
		//return false
	}

	iReadLen := MTKEPO_DATA_ONETIME * MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER
	body := make([]byte, 24 + iReadLen + 1)
	header := fmt.Sprintf("(%015dAP17%04X", service.imei, iReadLen + 25)
	copy(body[0: 24], header[0: ])

	data := body[24: 24 + iReadLen]
	offset := 0
	EpoInfoListLock.Lock()
	for i := 0; i < MTKEPO_DATA_ONETIME; i++ {
		copy(data[offset: offset + MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER], EPOInfoList[i].EPOBuf)
		offset += MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER
	}
	EpoInfoListLock.Unlock()

	body[24 + iReadLen] = ')'
	service.rspList = append(service.rspList, &ResponseItem{CMD_GT3_AP17_PUSH_EPO_DATA,  service.makeReplyMsg(body)})

	return true
}

func (service *GT03Service) ProcessRspChat() bool {
	//(357593060153353AP1627D6+数据)
	if service.reqCtx.GetChatDataFunc != nil {
		chatData := service.reqCtx.GetChatDataFunc(service.imei, 0)
		if len(chatData) > 0 {
			//voiceFileName :=fmt.Sprintf("/usr/share/nginx/html/web/upload/minichat/app/%d/%s.amr",
			//	service.imei, chatData[0].Content)
			voiceFileName :=fmt.Sprintf("%s%d/%d.amr", service.reqCtx.MinichatUploadDir,
				service.imei, chatData[0].FileID)

			chatReplyMsg := service.makeChatDataReplyMsg(voiceFileName,
				chatData[0].Sender, chatData[0].FileID, 1)
			if chatReplyMsg == nil {
				return false
			}

			service.rspList = append(service.rspList, chatReplyMsg)
			//删除此条微聊
			AppSendChatListLock.Lock()
			chatTask, ok  := AppSendChatList[service.imei]
			if ok && chatTask != nil && len(*chatTask) > 0 {
				//确认完毕，删除该微聊
				if len(*chatTask) > 1 {
					(*chatTask) = (*chatTask)[1:]
				}else{
					AppSendChatList[service.imei] = &[]*ChatTask{}
				}
			}
			AppSendChatListLock.Unlock()
		}
	}

	return true
}

func (service *GT03Service) ProcessUpdateWatchStatus() bool {
	ucBattery := 1
	if service.cur.OrigBattery >= 3 {
		ucBattery = int(service.cur.OrigBattery - 2)
	}

	service.cur.Battery = uint8(ucBattery)
	service.cur.LocateType = LBS_SMARTLOCATION

	service.CountSteps()

	logging.Log(fmt.Sprintf("Update Watch %d, Step:%d, Battery:%d", service.imei, service.cur.Steps, ucBattery))

	stWatchStatus := &WatchStatus{}
	stWatchStatus.i64DeviceID = service.imei
	stWatchStatus.iLocateType = service.cur.LocateType
	stWatchStatus.i64Time = service.cur.DataTime
	stWatchStatus.Step = service.cur.Steps
	stWatchStatus.AlarmType = service.cur.AlarmType
	stWatchStatus.Battery = service.cur.Battery
	ret := service.UpdateWatchStatus(stWatchStatus)
	if ret == false {
		logging.Log("Update Watch status failed ")
	}

	return true
}

func (service *GT03Service) PushCachedData() bool {
	return true
}

func (service *GT03Service) ProcessGPSInfo(pszMsg []byte) bool {
	if len(pszMsg) == 0  {
		return false
	}

	bufOffset := int32(0)
	bufOffset++
	//接下来解析"46000024820000DE127,"
	szMainLBS := make([]byte, 256)
	i := 0
	for pszMsg[bufOffset] != ',' {
		szMainLBS[i] = pszMsg[bufOffset]
		bufOffset++
		i++
	}
	fmt.Sscanf(string(szMainLBS), "%03d%03d%04X%07X%02X",
		&service.MCC, &service.MNC, &service.LACID, &service.CELLID, &service.RXLEVEL)
	service.RXLEVEL = service.RXLEVEL * 2 - 113
	if service.RXLEVEL > 0 {
		service.RXLEVEL = 0
	}
	bufOffset++

	bTrueTime := true
	iTimeDay := uint64(0)
	if pszMsg[bufOffset] == '-' {
		bufOffset++
		bTrueTime = false
		bufOffset += 8
	} else {
		iTimeDay = service.GetIntValue(pszMsg[bufOffset: ], TIME_LEN)
		bufOffset += TIME_LEN
	}

	cTag := pszMsg[bufOffset]
	if cTag != 'A' && cTag != 'V'  {
		logging.Log(fmt.Sprintf("Ivalid Pos cTag Data [%c]", cTag))
		return false
	}
	bufOffset++;

	shMaxTitudeLen := int32(10)
	iLatitude := service.GetValue(pszMsg[bufOffset: ], &shMaxTitudeLen)
	bufOffset += shMaxTitudeLen

	iTemp := iLatitude % 1000000
        iLatitude = iLatitude - iTemp

	dLatitude := float64(float64(iTemp) / 60.0)
        iSubLatitude := int32(dLatitude * 100);
	iLatitude += iSubLatitude

	cTag = pszMsg[bufOffset]
	if cTag == 'S' {  //如果是南纬的存为负数
		iLatitude = 0 - iLatitude
	}
	bufOffset++

	shMaxTitudeLen = 10
	iLongtitude := service.GetValue(pszMsg[bufOffset: ], &shMaxTitudeLen)

	iTemp = iLongtitude % 1000000
	iLongtitude = iLongtitude - iTemp

	dLontitude := float64(float64(iTemp) / 60.0)
	iSubLontitude := int32(dLontitude * 100)

	iLongtitude += iSubLontitude
	bufOffset += shMaxTitudeLen
	cTag = pszMsg[bufOffset]
	if cTag == 'W' { //如果是西经的存为负数
		iLongtitude = 0 - iLongtitude
	}
	bufOffset++

	fSpeed := service.GetFloatValue(pszMsg[bufOffset: ], 5)
	iSpeed := int32(fSpeed * 10)
	bufOffset += 5

	logging.Log(fmt.Sprintf("[%d] fSpeed: %f, iSpeed: %d", service.imei, fSpeed, iSpeed))

	iTimeSec := service.GetIntValue(pszMsg[bufOffset: ], TIME_LEN)
	bufOffset += TIME_LEN

	iIOStatu := service.cur.OrigBattery
	if iIOStatu > 2 {
		iIOStatu -= 2
	}

	if iIOStatu <= 1 {
		iIOStatu = 1
	}

	i64Time :=  uint64(0)
	if bTrueTime {
		i64Time = uint64(iTimeDay * 1000000 + iTimeSec)
	} else {
		now := time.Now()
		year,mon,day := now.UTC().Date()
		year = int(year / 100)
		hour,min,sec := now.UTC().Clock()
		i64Time = uint64(year) * uint64(10000000000) +  uint64(mon) * uint64(100000000) + uint64(day) * uint64(1000000) + uint64(hour * 10000 + min * 100 + sec)
	}

	service.cur.DataTime = i64Time
	service.cur.Lng = float64(float64(iLongtitude) / 1000000.0)
	service.cur.Lat = float64(float64(iLatitude) / 1000000.0)
	service.cur.ReadFlag = 0
	service.cur.Battery = iIOStatu
	service.cur.LocateType = LBS_GPS

	return true
}

func (service *GT03Service) ProcessWifiInfo(pszMsgBuf []byte) bool {
	if len(pszMsgBuf) == 0  {
		logging.Log(fmt.Sprintf("pszMsgBuf is empty"))
		return false
	}

	bufOffset := uint32(0)
	bufOffset++
	iDayTime := service.GetIntValue(pszMsgBuf[bufOffset: ], TIME_LEN)
	iDayTime = iDayTime % 1000000
	bufOffset += TIME_LEN + 1

	iSecTime := service.GetIntValue(pszMsgBuf[bufOffset: ], TIME_LEN)
	i64Time := uint64(iDayTime * 1000000 + iSecTime)
	bufOffset += TIME_LEN + 1

	shBufLen := uint32(0)
	service.ParseWifiInfo(pszMsgBuf[bufOffset: ], &shBufLen)
	bufOffset += shBufLen

	service.cur.DataTime = i64Time
	service.cur.Battery = 1
	if service.cur.OrigBattery >= 3 {
		service.cur.Battery = service.cur.OrigBattery - 2
	}

	if service.cur.Battery <= 1  {
		service.cur.Battery = 1
	}

	//service.GetWatchDataInfo(service.imei)

	if service.wifiZoneIndex >= 0 {
		DeviceInfoListLock.Lock()
		safeZones := GetSafeZoneSettings(service.imei)
		if safeZones != nil {
			strLatLng := strings.Split(safeZones[service.wifiZoneIndex].Center, ",")
			service.cur.Lat = Str2Float(strLatLng[0])
			service.cur.Lng = Str2Float(strLatLng[1])
			service.accracy = 150

			logging.Log(fmt.Sprintf("Get location frome home zone by home wifi: %f, %f",
				service.cur.Lat, service.cur.Lng))
		}
		DeviceInfoListLock.Unlock()
	} else {
		ret := service.GetLocation()
		if ret == false {
			service.needSendLocation = false
			logging.Log("GetLocation failed")
			return false
		}
	}

	//service.cur.Steps = 0
	service.cur.ReadFlag = 0
	service.cur.LocateType = LBS_WIFI
	service.cur.Accracy = service.accracy

	service.UpdateLocationIntoDBCache()
	return true
}

func (service *GT03Service) ProcessLBSInfo() bool {
	service.cur.Battery = 1
	if  service.cur.OrigBattery >= 3 {
		service.cur.Battery =  service.cur.OrigBattery - 2
	}

	service.CountSteps()

	if service.lbsNum <= 1 {
		stWatchStatus := &WatchStatus{}
		stWatchStatus.i64DeviceID = service.imei
		stWatchStatus.i64Time = service.cur.DataTime
		stWatchStatus.iLocateType = LBS_SMARTLOCATION
		stWatchStatus.Step = service.cur.Steps
		ret := service.UpdateWatchStatus(stWatchStatus)
		if ret == false {
			logging.Log("Update LBS Watch status failed")
			return false
		}

		service.getSameWifi = true
		return  true
	}

	ret := service.GetLocation()
	if ret == false {
		logging.Log("GetLocation of LBS failed")
		return false
	}

	service.cur.ReadFlag  = 0
	service.cur.Accracy = service.accracy
	service.cur.LocateType = LBS_JIZHAN  //uint8(service.accracy / 10) //LBS_JIZHAN;

	service.UpdateLocationIntoDBCache();

	return true
}

func (service *GT03Service) ProcessMutilLocateInfo() bool {
	service.cur.Battery = 1
	if  service.cur.OrigBattery >= 3 {
		service.cur.Battery =  service.cur.OrigBattery - 2
	}

	DeviceInfoListLock.Lock()
	safeZones := GetSafeZoneSettings(service.imei)
	if safeZones != nil {
		for j := 0; j < len(safeZones); j++ {
			for i := 0; i <  int(service.wiFiNum); i++ {
				if string(service.wifiInfoList[i].MacID[0:]) == safeZones[j].Wifi.BSSID {
					service.wifiZoneIndex = int16(j)
					strLatLng := strings.Split(safeZones[j].Center, ",")
					service.cur.Lat = Str2Float(strLatLng[0])
					service.cur.Lng = Str2Float(strLatLng[1])
					service.accracy = 150
					break
				}
			}
		}
	}
	DeviceInfoListLock.Unlock()

	if service.wifiZoneIndex >= 0  {
		logging.Log(fmt.Sprintf("Get location frome home zone by home wifi: %f, %f",
			service.cur.Lat, service.cur.Lng))
	} else {
		ret := service.GetLocation()
		if ret == false {
			service.needSendLocation = false
			logging.Log("GetDevicePostion for multilocation failed")
			return false
		}
	}

	//service.cur.Steps = 0
	service.cur.ReadFlag = 0
	service.cur.LocateType = LBS_WIFI
	service.cur.Accracy = service.accracy

	if service.accracy >= 2000 {
		service.cur.LocateType = LBS_INVALID_LOCATION
	} else if service.wiFiNum >= 3 && service.accracy <= 400 {
		service.cur.LocateType = LBS_WIFI
	} else {
		service.cur.LocateType = LBS_INVALID_LOCATION //LBS_JIZHAN // uint8(service.accracy / 10)  //LBS_JIZHAN
	}

	service.UpdateLocationIntoDBCache()
	return true
}

func (service *GT03Service) ProcessZoneAlarm() bool {
	//service.GetWatchDataInfo(service.imei)
	//报警的处理逻辑：
	//首先，报警的类型有：
	//1.无需计算，手表直接上报的报警：sos，低电，脱离——这种类型的报警已经在前面读取到了

	//因此，此方法用来处理需要计算的报警类型
	//2.需要计算，区域范围距离计算是否进入或者走出
	//3.需要计算，通过手表上报的电量计算是否低电报警
	//4.需要计算，通过判断设置的WiFi位置决定是否进入区域
	//以上报警可能同时兼有，即一次定位数据中可能产生多个报警类型

	//需要计算的报警也有两种情况，第一种需要跟上一次的报警做比较，
	//有可能上一次报过相同的报警类型，需要上次的数据做比较，避免相同报警连续重复上报
	//第二种可以不需要上一次的数据，或者需要考虑上一次还没有数据的情况

	//上次有数据，那么可以计算所有的报警类型，低电，GPS出入界，WiFi入界和避免重复报警
	//上次没有数据，那么这是第一次上报数据，这时可以计算低电，GPS入界，WiFi入界
	//归纳之，没有依赖前一次数据的报警类型，始终可以计算，
	// 只有gps出界这一种情况，需要依赖上次的数据才能进行计算
	//还有一些容错的情况，比如短时间内（一分钟？）距离超过1800米，不作为出界报警，可能上报的数据不正常

	//低电报警已经在处理定位数据的方法计算出来了，因此这里只需要考虑是否是低电重复报警
	//对于计算低电报警，唯一的条件就是上次的电量大于2，而这次的电量小于2

	//第一步，先来计算是否低电报警
	if  service.old.DataTime == 0 && service.cur.OrigBattery <= 2 || //上次没有数据，这次电量低于2，报低电
		service.old.DataTime > 0 && service.old.OrigBattery >=3  && service.cur.OrigBattery <= 2 {
		//上次有数据，上次电量大于等于3，而这次电量小于等于2，报低电
		service.cur.AlarmType |= ALARM_BATTERYLOW
	}

	//在进行出入届计算之前，先做容错的处理，仅对上次有数据的情况
	stCurPoint, stDstPoint := TPoint{}, TPoint{}
	//if service.old.DataTime > 0{
	//	stCurPoint.Latitude = service.cur.Lat
	//	stCurPoint.LongtiTude = service.cur.Lng
	//	stDstPoint.Latitude = service.old.Lat
	//	stDstPoint.LongtiTude = service.old.Lng
	//	iDistance := service.GetDisTance(&stCurPoint, &stDstPoint)
	//	if service.cur.LocateType == LBS_JIZHAN && service.old.LocateType == LBS_WIFI {
	//		//如果上一次是WiFi定位，这一次是基站定位，并且距离不超过200米，则认为是智能定位，不更新位置
	//		if iDistance <= 200 {
	//			i64Time := service.cur.DataTime
	//			service.cur = service.old
	//			service.cur.DataTime = i64Time
	//			service.cur.LocateType = LBS_SMARTLOCATION
	//			service.getSameWifi = true
	//			logging.Log(fmt.Sprintf("[%d] distance <= 200, it is the same wifi location", service.imei))
	//			return true
	//		}
	//	}
	//
	//	if iDistance >= 1800 && service.cur.LocateType != LBS_GPS {//GPS数据是完全可能1分钟移动800米距离的，比如在外开车
	//		iTotalMin := uint32(0)
	//		if service.cur.DataTime >= service.old.DataTime {
	//			iTotalMin = deltaMinutes(service.old.DataTime, service.cur.DataTime)
	//		}
	//
	//		if  iDistance >= iTotalMin * 800 { //如果两次距离超过1800，并且1分钟内距离超过800，
	//			// 则认为是数据异常跳跃，不更新位置
	//			i64Time := service.cur.DataTime
	//			service.cur = service.old
	//			service.cur.DataTime = i64Time
	//			service.cur.LocateType = LBS_SMARTLOCATION
	//			service.getSameWifi = true
	//			logging.Log(fmt.Sprintf("[%d] distance %d >= 1800, total delta minutes %d, no need update location",
	//				service.imei, iDistance, iTotalMin))
	//			return true
	//		}
	//	}
	//}


	//第二步，计算WiFi入界（WiFi不需要报出界）
	//首先如果上报的WiFi数据中，包含了安全区域的WiFi，其次上一次的报警不是进入同一个安全区域
	//那么上报这次的WiFi入界报警
	//如果上一次并没有数据，那么直接报WiFi入界
	if service.wifiZoneIndex >= 0  && service.wifiZoneIndex < MAX_SAFE_ZONE_NUM{
		if service.old.DataTime == 0  || (service.old.LastAlarmType & ALARM_INZONE  == 0) ||
			(service.old.LastAlarmType & ALARM_INZONE  != 0) && int16(service.old.LastZoneIndex) != service.wifiZoneIndex {
			DeviceInfoListLock.Lock()
			safeZones := GetSafeZoneSettings(service.imei)
			if safeZones != nil {
				service.cur.ZoneIndex = int32(service.wifiZoneIndex)
				service.cur.ZoneName = safeZones[service.wifiZoneIndex].Name
				service.cur.AlarmType |= ALARM_INZONE
				logging.Log(fmt.Sprintf("Device[%d] Make a InZone Alarm by wifi [%s][%d,%d][%d,%d]",
					service.imei, service.cur.ZoneName, safeZones[service.wifiZoneIndex].Radius,
					service.cur.AlarmType,  service.cur.ZoneIndex, service.old.AlarmType,  service.old.ZoneIndex))
				logging.Log("service.old:  " + MakeStructToJson(&service.old) + ";   service.cur:  " + MakeStructToJson(&service.cur))
			}
			DeviceInfoListLock.Unlock()
			return true
		}
	}

	//第三步，计算GPS经纬度距离来决定出入界报警和是否重复报警
	//首先，计算当前经纬度与每个安全区域的中心点距离，

	//非GPS定位不做区域报警
	if service.cur.LocateType != LBS_GPS {
		return true
	}

	stCurPoint.Latitude = service.cur.Lat
	stCurPoint.LongtiTude = service.cur.Lng

	DeviceInfoListLock.Lock()
	safeZones := GetSafeZoneSettings(service.imei)

	if safeZones != nil {
		for  i := 0; i < len(safeZones); i++ {
			stSafeZone := safeZones[i]
			if stSafeZone.On == "0" {
				continue
			}

			strLatLng := strings.Split(stSafeZone.Center, ",")
			if len(strLatLng) != 2 {
				continue
			}

			zoneLat := Str2Float(strLatLng[0])
			zoneLng := Str2Float(strLatLng[1])
			if zoneLat == 0 || zoneLng == 0{
				continue
			}

			stDstPoint.Latitude = zoneLat
			stDstPoint.LongtiTude = zoneLng
			iRadiu := service.GetDisTance(&stCurPoint, &stDstPoint)
			logging.Log(fmt.Sprint(service.imei, " iRadiu, stCurPoint, stDstPoint: ", iRadiu, stCurPoint, stDstPoint))

			//判断报警，需根据上次的报警数据进行决定
			//1.上次无报警，那么这次只考虑入界报警
			if service.old.LastAlarmType & ALARM_INZONE == 0 && service.old.LastAlarmType & ALARM_OUTZONE == 0 {
				if iRadiu < uint32(stSafeZone.Radius) { //判断是否入界
					service.cur.ZoneIndex = stSafeZone.ZoneID
					service.cur.AlarmType |= ALARM_INZONE
					service.cur.ZoneName = stSafeZone.Name
					logging.Log(fmt.Sprintf("Device[%d] Make a InZone Alarm[%s][%d,%d][%d,%d]",
						service.imei, stSafeZone.Name, iRadiu, stSafeZone.Radius , service.cur.AlarmType, service.cur.ZoneIndex))
					logging.Log("service.old:  " + MakeStructToJson(&service.old) + ";   service.cur:  " + MakeStructToJson(&service.cur))
					break
				}
			}else{
				if service.old.LastAlarmType & ALARM_INZONE  != 0{ //上次是入界报警
					//那么这次需要计算是否有出界，并且同时是否有另一个入界
					if  service.old.LastZoneIndex == stSafeZone.ZoneID && iRadiu >= uint32(stSafeZone.Radius){
						service.cur.AlarmType |= ALARM_OUTZONE
						if len(service.cur.ZoneName) == 0 {
							service.cur.ZoneName = stSafeZone.Name
							service.cur.ZoneIndex = stSafeZone.ZoneID
						}else{
							service.cur.ZoneName = Num2Str(ALARM_INZONE, 10)  + service.cur.ZoneName +"," + Num2Str(ALARM_OUTZONE, 10)  + stSafeZone.Name
						}

						logging.Log(fmt.Sprintf("Device[%d] Make a OutZone Alarm[%s][%d,%d][%d,%d]",
							service.imei, stSafeZone.Name, iRadiu, stSafeZone.Radius , service.cur.AlarmType, service.cur.ZoneIndex))
						logging.Log("service.old:  " + MakeStructToJson(&service.old) + ";   service.cur:  " + MakeStructToJson(&service.cur))

						if (service.cur.AlarmType & ALARM_OUTZONE)  != 0 && (service.cur.AlarmType & ALARM_INZONE)  != 0{
							break
						}
					}

					if service.old.LastZoneIndex != stSafeZone.ZoneID && iRadiu < uint32(stSafeZone.Radius){
						service.cur.ZoneIndex = stSafeZone.ZoneID
						service.cur.AlarmType |= ALARM_INZONE
						if len(service.cur.ZoneName) == 0 {
							service.cur.ZoneName = stSafeZone.Name
						}else{
							service.cur.ZoneName = Num2Str(ALARM_OUTZONE, 10)  + service.cur.ZoneName + "," + Num2Str(ALARM_INZONE, 10)  + stSafeZone.Name
						}

						logging.Log(fmt.Sprintf("Device[%d] Make a InZone Alarm[%s][%d,%d][%d,%d]",
							service.imei, stSafeZone.Name, iRadiu, stSafeZone.Radius , service.cur.AlarmType, service.cur.ZoneIndex))
						logging.Log("service.old:  " + MakeStructToJson(&service.old) + ";   service.cur:  " + MakeStructToJson(&service.cur))

						if (service.cur.AlarmType & ALARM_OUTZONE)  != 0 && (service.cur.AlarmType & ALARM_INZONE)  != 0{
							break
						}
					}

				}else if service.old.LastAlarmType & ALARM_OUTZONE  != 0 {
					//上次是出界报警,那么这次只需要考虑是否有入界报警
					if iRadiu < uint32(stSafeZone.Radius) { //判断是否入界
						service.cur.ZoneIndex = stSafeZone.ZoneID
						service.cur.AlarmType |= ALARM_INZONE
						service.cur.ZoneName = stSafeZone.Name
						logging.Log(fmt.Sprintf("Device[%d] Make a InZone Alarm[%s][%d,%d][%d,%d]",
							service.imei, stSafeZone.Name, iRadiu, stSafeZone.Radius , service.cur.AlarmType, service.cur.ZoneIndex))
						logging.Log("service.old:  " + MakeStructToJson(&service.old) + ";   service.cur:  " + MakeStructToJson(&service.cur))
						break
					}
				}
			}
		}
	}

	DeviceInfoListLock.Unlock()

	return true
}

func (service *GT03Service) makeJson(data *LocationData) string  {
	strData, err := json.Marshal(data)
	if err != nil {
		logging.Log("make json failed, " + err.Error())
	}

	return string(strData)
	//return fmt.Sprintf("{\"steps\": %d,  \"readflag\": %d,  \"locateType\": %d, \"accracy\": %d,  \"battery\": %d,   \"alarm\": %d,  \"zoneAlarm\": %d, \"zoneIndex\": %d,  \"zoneName\": \"%s\"}",
	//data.Steps, data.ReadFlag, data.LocateType, data.Accracy, data.Battery, data.AlarmType, data.ZoneAlarm, data.ZoneIndex, data.ZoneName)
}

func (service *GT03Service) WatchDataUpdateDB() bool {
	//strTime := fmt.Sprintf("20%d", service.cur.DataTime) //20170520114920
	strTableaName := "gator3_device_location" //fmt.Sprintf("device_location_%s_%s", string(strTime[0:4]), string(strTime[4: 6]))
	strSQL := fmt.Sprintf("INSERT INTO %s VALUES(%d, %d, %06f, %06f, '%s'::jsonb)",
		strTableaName, service.imei, 20000000000000 + service.cur.DataTime, service.cur.Lat, service.cur.Lng, service.makeJson(&service.cur))

	logging.Log(fmt.Sprintf("SQL: %s", strSQL))

	strSQLTest := fmt.Sprintf("update %s set location_time=%d,data=jsonb_set(data,'{datatime}','%d'::jsonb,'{steps}','%d'::jsonb,'{locateType}','%d'::jsonb,true) ",
		strTableaName, 20000000000000 + service.cur.DataTime, service.cur.DataTime, service.cur.Steps, service.cur.LocateType)

	logging.Log(fmt.Sprintf("SQL Test: %s", strSQLTest))

	_, err := service.reqCtx.Pgpool.Exec(strSQL)
	if err != nil {
		logging.Log("pg pool exec sql failed, " + err.Error())
		return false
	}

	return true
}

func (service *GT03Service) UpdateWatchData() bool {
	if service.reqCtx.SetDeviceDataFunc != nil {
		service.reqCtx.SetDeviceDataFunc(service.imei, DEVICE_DATA_LOCATION, service.cur)
		return true
	}else{
		return false
	}
}

func (service *GT03Service) UpdateWatchStatus(watchStatus *WatchStatus) bool {
	deviceData := LocationData{LocateType: watchStatus.iLocateType,
		Battery: watchStatus.Battery,
		AlarmType: watchStatus.AlarmType,
		Steps: watchStatus.Step,
		DataTime: watchStatus.i64Time,
	}

	if service.reqCtx.SetDeviceDataFunc != nil  {
		service.reqCtx.SetDeviceDataFunc(watchStatus.i64DeviceID, DEVICE_DATA_STATUS, deviceData)

		//strTime := fmt.Sprintf("20%d", deviceData.DataTime) //20170520114920
		//strTableaName := fmt.Sprintf("device_location_%s_%s", string(strTime[0:4]), string(strTime[4: 6]))
		//strSQL := fmt.Sprintf("update %s set location_time=%d,data=jsonb_set(data,'{datatime}','%d'::jsonb,'{steps}','%d'::jsonb,'{locateType}','%d'::jsonb,true) ",
		//	strTableaName, 20000000000000 + deviceData.DataTime, deviceData.DataTime, deviceData.Steps, deviceData.LocateType)
		//
		//logging.Log(fmt.Sprintf("SQL: %s", strSQL))
		//
		//_, err := service.reqCtx.Pgpool.Exec(strSQL)
		//if err != nil {
		//	logging.Log("pg pool exec sql failed, " + err.Error())
		//	return false
		//}

		return true
	}else{
		return false
	}
}

func (service *GT03Service) UpdateWatchBattery() bool {
	deviceData := LocationData{Battery: service.cur.Battery,
		OrigBattery: service.cur.OrigBattery,
	}

	if service.reqCtx.SetDeviceDataFunc  != nil {
		service.reqCtx.SetDeviceDataFunc(service.imei, DEVICE_DATA_BATTERY, deviceData)
		return true
	}else{
		return false
	}
}

func (service *GT03Service) NotifyAlarmMsg() bool {
	//strRequestBody := "r=app/auth/alarm&"
	//strReqURL := "http://service.gatorcn.com/web/index.php"
	//value := fmt.Sprintf("systemno=%d&data=%d,%d,%d,%d,%d,%d,%d,%d,%s,%s",
	//	service.imei % 100000000000, service.cur.DataTime, int(service.cur.Lat * 1000000), int(service.cur.Lng * 1000000),
	//	service.cur.Steps, service.cur.Battery, service.cur.AlarmType, service.cur.ReadFlag,
	//	service.cur.LocateType, service.cur.ZoneName, "")
	//
	//strRequestBody += value
	//strReqURL += "?" +  strRequestBody
	//logging.Log(strReqURL)
	//resp, err := http.Get(strReqURL)
	//if err != nil {
	//	logging.Log(fmt.Sprintf("[%d] call php api to notify app failed  failed,  %s", service.imei, err.Error()))
	//	return false
	//}
	//
	//defer resp.Body.Close()
	//
	//body, err := ioutil.ReadAll(resp.Body)
	//if err != nil {
	//	logging.Log(fmt.Sprintf("[%d] php notify api  response has err, %s", service.imei, err.Error()))
	//	return false
	//}
	//
	//logging.Log(fmt.Sprintf("[%d] read php notify api response:, %s", service.imei, body))

	ownerName := Num2Str(service.imei, 10)
	DeviceInfoListLock.Lock()
	deviceInfo, ok := (*DeviceInfoList)[service.imei]
	if ok && deviceInfo != nil {
		if deviceInfo.OwnerName != "" {
			ownerName = deviceInfo.OwnerName
		}
	}

	DeviceInfoListLock.Unlock()

	PushNotificationToApp(service.reqCtx.APNSServerBaseURL, service.imei, "",  ownerName,
		service.cur.DataTime, service.cur.AlarmType, service.cur.ZoneName)

	return true
}

func (service *GT03Service) UpdateLocationIntoDBCache() bool {
	return true
}

func (service *GT03Service) rad(d float64) float64 {
	return d * PI / 180.0
}

func (service *GT03Service) GetDisTance(stPoint1, stPoint2 *TPoint) uint32 {
	lng1 := stPoint1.LongtiTude / BASE_TITUDE
	lat1  := stPoint1.Latitude / BASE_TITUDE
	lng2 := stPoint2.LongtiTude / BASE_TITUDE
	lat2 := stPoint2.Latitude / BASE_TITUDE

	radLat1 := service.rad(lat1)
	radLat2 := service.rad(lat2)
	a := radLat1 - radLat2
	b := service.rad(lng1) - service.rad(lng2)
	dDistance := 2 * math.Sin(math.Sqrt(math.Pow(math.Sin(a / 2), 2) + math.Cos(radLat1) * math.Cos(radLat2) * math.Pow(math.Sin(b / 2), 2)))
	//2 * math.Sin((math.Sqrt(math.Pow(math.Sin(a / 2), 2) + math.Cos(radLat1) * math.Cos(radLat2) * math.Pow(math.Sin(b / 2), 2)))

	dDistance = dDistance * EARTH_RADIUS
	dDistance = dDistance * 1000

	return uint32(dDistance)
}

func (service *GT03Service) GetIntValue(pszMsgBuf []byte,  shLen uint32) uint64{
	value, _ := strconv.Atoi(string(pszMsgBuf[0: shLen]))
	return uint64(value)
}


func (service *GT03Service) GetValue(pszMsgBuf []byte, shMaxLen *int32) int32 {
	shTempLen := *shMaxLen
	*shMaxLen = 0
	if len(pszMsgBuf) == 0  {
		return -1
	}

	 iRetValue := int32(0)
	for i := 0; i < len(pszMsgBuf); i++ {
		if pszMsgBuf[i] >= '0' && pszMsgBuf[i] <= '9' {
			iRetValue = iRetValue * 10 + int32(int32(pszMsgBuf[i]) - '0')
			*shMaxLen = *shMaxLen + 1
		} else if pszMsgBuf[i] == '.' {
			*shMaxLen = *shMaxLen + 1
		} else {
			break
		}

		if *shMaxLen >= shTempLen {
			break
		}
	}

	return iRetValue
}


func (service *GT03Service) GetFloatValue(pszMsgBuf []byte, shLen uint16) float64 {
	if shLen > 12 {
		shLen = 12
	}

	value, _ := strconv.ParseFloat(string(pszMsgBuf[0 : shLen]), 0)
	return value
}

func  (service *GT03Service) GetWatchDataInfo(imei uint64)  {
	if service.reqCtx.GetDeviceDataFunc != nil {
		service.old = service.reqCtx.GetDeviceDataFunc(imei, service.reqCtx.Pgpool)
		if service.old.DataTime == 0 || service.old.Lat == 0 {
			service.old.ZoneIndex = -1
			service.old.LastZoneIndex = -1
		}

		service.cur.LastAlarmType = service.old.LastAlarmType
		service.cur.LastZoneIndex = service.old.LastZoneIndex
	}
}

func  (service *GT03Service) GetLocation()  bool{
	useGooglemap := true
	DeviceInfoListLock.Lock()
	deviceInfo, ok := (*DeviceInfoList)[service.imei]
	if ok && strings.Contains(deviceInfo.CountryCode, "86") {
		useGooglemap = false
	}
	DeviceInfoListLock.Unlock()

	if useGooglemap {
		return service.GetLocationByGoogle()
	}else {
		return service.GetLocationByAmap()
	}
}


func  (service *GT03Service) ParseWifiInfo(pszMsgBuf []byte, shBufLen *uint32)  bool {
	*shBufLen = 0
	bufOffset := uint32(0)
	if  len(pszMsgBuf) == 0 {
		logging.Log("pszMsgBuf is empty")
		return false
	}

	service.wiFiNum = 0
	service.wifiInfoList = [MAX_WIFI_NUM + 1]WIFIInfo{}

	shWifiNum := pszMsgBuf[bufOffset] - '0'
	bufOffset += 2

	if shWifiNum > MAX_WIFI_NUM {
		shWifiNum = MAX_WIFI_NUM
	}

	shIndex,  shMacIDLen := 0, 0

	for bufOffset < uint32(len(pszMsgBuf))  && shWifiNum > 0 {
		stWifiInfo := &service.wifiInfoList[service.wiFiNum]
		if shIndex == 0  {
			hi, low := byte(0), byte(0)
			if pszMsgBuf[bufOffset] >= 'A' && pszMsgBuf[bufOffset] <= 'F' {
				hi = pszMsgBuf[bufOffset] - 'A' + 10
			}else{
				hi = pszMsgBuf[bufOffset] - '0'
			}

			if pszMsgBuf[bufOffset + 1] >= 'A' && pszMsgBuf[bufOffset + 1] <= 'F' {
				low = pszMsgBuf[bufOffset + 1] - 'A' + 10
			}else{
				low = pszMsgBuf[bufOffset + 1] - '0'
			}

			 nameLen := 16 * hi + low
			if nameLen < MAX_WIFI_NAME_LEN  {
				stWifiInfo.WIFIName = string(pszMsgBuf[bufOffset + 3:  bufOffset + 3 + uint32(nameLen)])
				bufOffset += 4 + uint32(nameLen)
				shIndex++
			}
		}

		if pszMsgBuf[bufOffset] == ',' {
			shIndex++
			if shIndex == 3 || shIndex == 2 && shWifiNum == 1 {
				if strings.Contains(stWifiInfo.WIFIName, "iPhone") || strings.Contains(stWifiInfo.WIFIName, "Android") {
					//DONOTING
				}else {
					if service.wiFiNum < MAX_WIFI_NUM {
						service.wiFiNum++
					}
				}

				shIndex, shMacIDLen = 0, 0
				if len(stWifiInfo.WIFIName) == 0 {
					stWifiInfo.WIFIName = "mark"
				}

				shWifiNum--
			}
		} else {
			switch shIndex {
			case 0:
			case 1:
				stWifiInfo.MacID[shMacIDLen] = pszMsgBuf[bufOffset]
				shMacIDLen++
			case 2:
				if pszMsgBuf[bufOffset] != '-'  {
					stWifiInfo.Ratio = stWifiInfo.Ratio * 10 + int16(pszMsgBuf[bufOffset] - '0')
				}
			}

		}

		bufOffset++
	}

	*shBufLen = bufOffset

	if service.wiFiNum > MAX_WIFI_NUM {
		service.wiFiNum = MAX_WIFI_NUM
	}

	DeviceInfoListLock.Lock()
	safeZones := GetSafeZoneSettings(service.imei)
	if safeZones != nil {
		for j := 0; j < len(safeZones); j++ {
			for i := 0; i <  int(service.wiFiNum); i++ {
				if string(service.wifiInfoList[i].MacID[0:]) == safeZones[j].Wifi.BSSID {
					service.wifiZoneIndex = int16(j)
					break
				}
			}
		}
	}
	DeviceInfoListLock.Unlock()

	return true
}

func  (service *GT03Service) ParseLBSInfo(pszMsgBuf []byte, shBufLen *uint32)  bool {
	if len(pszMsgBuf) == 0 {
		logging.Log("pszMsgBuf is NULL")
		return false
	}

	bufOffset := 0
	service.lbsNum = 0
	*shBufLen = 0
	service.lbsInfoList = [MAX_LBS_NUM + 1]LBSINFO{}

	shLBSNum := pszMsgBuf[bufOffset] - '0'
	if shLBSNum > MAX_LBS_NUM {
		shLBSNum = MAX_LBS_NUM
	}
	bufOffset += 2
	szLBSInfo := [64]byte{}
	shIndex := 0
	for bufOffset < len(pszMsgBuf) && shLBSNum > 0 {
		if pszMsgBuf[bufOffset] == ',' {
			stLBSInfo := &service.lbsInfoList[service.lbsNum]
			fmt.Sscanf(string(szLBSInfo[0: shIndex]), "%03d%03d%04X%07X%02X",
				&stLBSInfo.Mcc, &stLBSInfo.Mnc, &stLBSInfo.Lac,
				&stLBSInfo.CellID, &stLBSInfo.Signal)

			stLBSInfo.Signal = stLBSInfo.Signal * 2 - 113
			if stLBSInfo.Signal > 0 {
				stLBSInfo.Signal = 0
			}

			service.lbsNum++
			shLBSNum--
			shIndex = 0
		} else {
			szLBSInfo[shIndex] = pszMsgBuf[bufOffset]
			shIndex++
		}

		bufOffset++
	}

	if service.lbsNum >= MAX_LBS_NUM {
		service.lbsNum = MAX_LBS_NUM
	}

	*shBufLen = uint32(bufOffset)

	return true
}

//for google api
func  (service *GT03Service) GetLocationByGoogle() bool  {
	params := &GooglemapParams{}
	params.RadioType = "gsm"
	params.ConsiderIP = true

	for  i := 0; i < int(service.wiFiNum); i++ {
		params.WifiAccessPoints = append(params.WifiAccessPoints, GmapWifi{string(service.wifiInfoList[i].MacID[0:]),
			int(service.wifiInfoList[i].Ratio),
		})
	}

	for  i := 0; i < int(service.lbsNum); i++ {
		params.CellTowers = append(params.CellTowers, GmapLBS{int(service.lbsInfoList[i].CellID),
			int(service.lbsInfoList[i].Lac),
			int(service.lbsInfoList[i].Mcc),
			int(service.lbsInfoList[i].Mnc),
			int(service.lbsInfoList[i].Signal),
		})
	}

	jsonStr, err := json.Marshal(params)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] make json failed, %s", service.imei, err.Error()))
		return false
	}

	req, err := http.NewRequest("POST",
		"https://www.googleapis.com/geolocation/v1/geolocate?key=AIzaSyAbvNjmCijnZv9od3cC0MwmTC6HTBG6R60",
		bytes.NewBuffer(jsonStr))
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] call google map api  failed,  %s", service.imei, err.Error()))
		return false
	}

	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] do request of google map api  failed,  %s", service.imei, err.Error()))
		return false
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] google map response has err, %s", service.imei, err.Error()))
		return false
	}

	logging.Log(fmt.Sprintf("%d google location result: %s", service.imei, string(body)))

	bodyStr := strings.Replace(string(body), "\n", "", -1)

	result := GooglemapResult{}
	err = json.Unmarshal([]byte(bodyStr), &result)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] parse google map response failed, %s", service.imei, err.Error()))
		return false
	}

	if result.Location.Lat == 0 || result.Location.Lng == 0 {
		logging.Log(fmt.Sprintf("%d bad location :%06f, %06f", service.imei,
			result.Location.Lat, result.Location.Lng))
		return false
	}

	service.cur.Accracy = uint32(result.Accuracy)
	service.cur.Lat = result.Location.Lat
	service.cur.Lng = result.Location.Lng

	return true
}

//for amap(高德) api
func  (service *GT03Service) GetLocationByAmap() bool  {
	strReqURL := "http://apilocate.amap.com/position"
	strRequestBody, strWifiReqParam, strLBSReqParam := "", "", ""

	for i := 0; i < int(service.wiFiNum); i++ {
		if i==0 {
			strWifiReqParam += fmt.Sprintf("mmac=%s,%d,%s", string(service.wifiInfoList[i].MacID[0:]),
				0 - service.wifiInfoList[i].Ratio, service.wifiInfoList[i].WIFIName)
		} else if i == 1 {
			strWifiReqParam += fmt.Sprintf("&macs=%s,%d,%s", string(service.wifiInfoList[i].MacID[0:]),
				0 - service.wifiInfoList[i].Ratio, service.wifiInfoList[i].WIFIName)
		} else {
			strWifiReqParam += fmt.Sprintf("|%s,%d,%s", string(service.wifiInfoList[i].MacID[0:]),
				0 - service.wifiInfoList[i].Ratio, service.wifiInfoList[i].WIFIName)
		}
	}

	for i := 0; i < int(service.lbsNum); i++ {
		if i == 0 {
			strLBSReqParam += fmt.Sprintf("bts=%d,%02d,%d,%d,%d&nearbts=", service.lbsInfoList[i].Mcc,
				service.lbsInfoList[i].Mnc, service.lbsInfoList[i].Lac, service.lbsInfoList[i].CellID, service.lbsInfoList[i].Signal)
		} else if i == 1 {
			strLBSReqParam += fmt.Sprintf("%d,%02d,%d,%d,%d", service.lbsInfoList[i].Mcc,
				service.lbsInfoList[i].Mnc, service.lbsInfoList[i].Lac, service.lbsInfoList[i].CellID, service.lbsInfoList[i].Signal)
		} else {
			strLBSReqParam += fmt.Sprintf("|%d,%02d,%d,%d,%d", service.lbsInfoList[i].Mcc,
				service.lbsInfoList[i].Mnc, service.lbsInfoList[i].Lac, service.lbsInfoList[i].CellID, service.lbsInfoList[i].Signal)
		}
	}

	if service.lbsNum == 0 {
		strRequestBody = fmt.Sprintf("accesstype=1&imei=%d&cdma=0&", service.imei) + strWifiReqParam
	} else {
		strRequestBody = fmt.Sprintf("accesstype=0&imei=%d&cdma=0&", service.imei) +  strLBSReqParam
		if service.wiFiNum > 0 {
			strRequestBody += "&" +  strWifiReqParam
		}
	}

	strRequestBody += "&key=7fdbbeb16c30e7660f8588afee2bf9ba"

	//http://apilocate.amap.com/position?accesstype=1&imei=357593060571398&cdma=0&mmac=1C:78:57:08:A5:CA,-88,wifi2462&macs=D8:15:0D:5C:4D:74,-87,HDB-H&key=7fdbbeb16c30e7660f8588afee2bf9ba
	//http://apilocate.amap.com/position?accesstype=0&imei=357593060571398&cdma=0&bts=460,00,9360,4880,-7&nearbts=460,00,9360,4183,-29|460,00,9360,4191,-41|460,00,9360,4072,-41&mmac=48:3C:0C:F5:56:48,-46,Gator&macs=FC:37:2B:50:A7:49,-55,ChinaNet-kWqN|14:B8:37:23:71:57,-57,ChinaNet-qi67|D8:24:BD:77:4D:6E,-61,gatorgroup|FC:37:2B:4B:06:11,-69,ChinaNet-z6QU|82:89:17:78:E2:21,-74,cfbb.com.cn&key=7fdbbeb16c30e7660f8588afee2bf9ba
	//http://apilocate.amap.com/position?accesstype=0&imei=357593060571398&cdma=0&mmac=48:3C:0C:F5:56:48,-46,Gator&macs=FC:37:2B:50:A7:49,-55,ChinaNet-kWqN|14:B8:37:23:71:57,-57,ChinaNet-qi67|D8:24:BD:77:4D:6E,-61,gatorgroup|FC:37:2B:4B:06:11,-69,ChinaNet-z6QU|82:89:17:78:E2:21,-74,cfbb.com.cn&key=7fdbbeb16c30e7660f8588afee2bf9ba
	strReqURL += "?" +  strRequestBody
	logging.Log(strReqURL)
	resp, err := http.Get(strReqURL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] call amap api  failed,  %s", service.imei, err.Error()))
		return false
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] amap response has err, %s", service.imei, err.Error()))
		return false
	}

	logging.Log(fmt.Sprintf("[%d] parse amap response:, %s", service.imei, body))

	result := AmapResult{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] parse amap response failed, %s", service.imei, err.Error()))
		return false
	}

	//解析返回参数
	latlng := strings.Split(result.Result.Location, ",")
	if len(latlng) < 2 {
		logging.Log(fmt.Sprintf("[%d] location from amap does not have lat and lng", service.imei))
		return false
	}

	dLontiTude := Str2Float(latlng[0]) //经度
	dLatitude := Str2Float(latlng[1]) //纬度
	if dLatitude == 0 || dLontiTude == 0 {
		logging.Log(fmt.Sprintf("%d bad location :%06f, %06f", service.imei, dLatitude, dLontiTude))
		return false
	}

	gcj_To_Gps84(dLatitude, dLontiTude, &service.cur.Lat, &service.cur.Lng)
	service.cur.LocateType = uint8(Str2Num(result.Result.Type, 10))
	service.cur.Accracy = uint32(Str2Num(result.Result.Radius, 10))

	logging.Log(fmt.Sprintf("amap TransForm location Is:%06f, %06f", service.cur.Lat, service.cur.Lng))

	return true
}

func  (service *GT03Service) Test() {
	fmt.Println("gt3tcpproto test")
}