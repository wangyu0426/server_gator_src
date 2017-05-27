package proto

import (
	"fmt"
	"runtime"
	"strconv"
	"../logging"
	"time"
	"strings"
	"math"
	"github.com/jackc/pgx"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"encoding/json"
	"net/http"
	"io/ioutil"
)

type ResponseItem struct {
	rspCmdType uint16
	data []byte
}

type TPoint struct {
	Latitude float64
	LongtiTude float64
}

type timestruct struct{
	y, m, d, h, mi, s uint64
}

const (
	DEVICE_DATA_LOCATION = iota
	DEVICE_DATA_STATUS
	DEVICE_DATA_BATTERY
)

type RequestContext struct {
	IsDebug bool
	IP uint32
	Port int
	Pgpool *pgx.ConnPool
	MysqlPool *sql.DB
	WritebackChan chan *MsgData
	Msg *MsgData
	GetDeviceDataFunc  func (imei uint64, pgpool *pgx.ConnPool)  LocationData
	SetDeviceDataFunc  func (imei uint64, updateType int, deviceData LocationData)
	GetChatDataFunc  func (imei uint64)  []ChatInfo
	AddChatDataFunc  func (imei uint64, chatData ChatInfo)
}

type GmapWifi struct {
	MacAddress string `json:"macAddress"`
	SignalToNoiseRatio int `json:"signalToNoiseRatio"`
}

type GmapLBS struct {
	CellID int `json:"cellId"`
	LocationAreaCode int `json:"locationAreaCode"`
	MobileCountryCode int `json:"mobileCountryCode"`
	MobileNetworkCode int `json:"mobileNetworkCode"`
	SignalStrength int `json:"signalStrength"`
}

type GooglemapParams struct {
	RadioType string `json:"radioType"`
	ConsiderIP bool `json:"considerIP"`
	WifiAccessPoints []GmapWifi `json:"wifiAccessPoints"`
	CellTowers []GmapLBS `json:"cellTowers"`
}

type GooglemapResult struct {
	Location struct {
			 Lat float64 `json:"lat"`
			 Lng float64 `json:"lng"`
		 } `json:"location"`
	Accuracy int `json:"accuracy"`
}

type AmapResult struct {
	Status string `json:"status"`
	Info string `json:"info"`
	Infocode string `json:"infocode"`
	Result struct {
		       Type string `json:"type"`
		       Location string `json:"location"`
		       Radius string `json:"radius"`
		       Desc string `json:"desc"`
		       Country string `json:"country"`
		       Province string `json:"province"`
		       City string `json:"city"`
		       Citycode string `json:"citycode"`
		       Adcode string `json:"adcode"`
		       Road string `json:"road"`
		       Street string `json:"street"`
		       Poi string `json:"poi"`
	       } `json:"result"`
}


type LocationData struct {
	DataTime uint64 	`json:"datatime,omitempty"`
	Lat float64			`json:"lat,omitempty"`
	Lng float64			`json:"lng,omitempty"`
	Steps uint64  		`json:"steps"`
	Accracy uint32		 `json:"accracy"`
	Battery uint8		`json:"battery"`
	AlarmType uint8   	`json:"alarm"`
	LocateType uint8	`json:"locateType"`
	ReadFlag uint8  	 `json:"readflag"`
	ZoneAlarm uint8 	 `json:"zoneAlarm"`
	ZoneIndex int32 	`json:"zoneIndex"`
	ZoneName string 	`json:"zoneName"`

	OrigBattery uint8	`json:"org_battery,omitempty"`
	origBatteryOld uint8
}

const (
	ChatFromDeviceToApp = iota
	ChatFromAppToDevice
)

type ChatInfo struct {
	DateTime uint64
	VoiceMilisecs int
	SenderType uint8
	Sender string
	ReceiverType uint8
	Receiver string
	ContentType uint8
	Content string
	Flags  int
}

type DataBlock struct {
	MsgId uint64
	Imei uint64
	Cmd string
	Time uint64
	Phone string
	BlockCount, BlockIndex, BlockSize  int
	Data []byte
}

type DeviceCache struct {
	Imei uint64
	CurrentLocation LocationData
	AlarmCache []LocationData
	ChatCache []ChatInfo
	ResponseCache []ResponseItem
}

type GT06Service struct {
	imei uint64
	msgSize uint64
	cmd uint16
	old LocationData
	cur LocationData
	needSendLocation bool
	needSendChatNum bool
	needSendChat bool
	reqChatInfoNum int
	getSameWifi bool
	wifiInfoList [MAX_WIFI_NUM]WIFIInfo
	wiFiNum uint16
	wifiZoneIndex int16
	hasSetHomeWifi bool
	lbsNum uint16
	lbsInfoList [MAX_LBS_NUM]LBSINFO
	accracy uint32
	MCC int32
	MNC int32
	LACID int32
	CELLID int32
	RXLEVEL int32
	rspList []*ResponseItem
	reqCtx RequestContext
}

const DEVICEID_BIT_NUM = 15
const DEVICE_CMD_LENGTH = 4
const TIME_LEN  = 6

const MAX_WIFI_NAME_LEN = 64
const MAX_MACID_LEN = 17
const MAX_WIFI_NUM = 6
const MAX_LBS_NUM = 8

const PI  = 3.1415926
const BASE_TITUDE  =1000000
const EARTH_RADIUS = 6378.137

func HandleTcpRequest(reqCtx RequestContext)  bool{
	service := &GT06Service{reqCtx: reqCtx}
	ret := service.PreDoRequest()
	if ret == false {
		return false
	}

	ret = service.DoRequest(reqCtx.Msg)
	if ret == false {
		return false
	}
	data := service.DoResponse()
	if data == nil {
		return false
	}
	msgResp := &MsgData{Data: data}
	msgResp.Header.Header.Imei = service.imei

	reqCtx.WritebackChan <- msgResp
	return true
}

func (service *GT06Service)PreDoRequest() bool  {
	service.wifiZoneIndex = -1
	service.needSendLocation = false
	return true
}


func (service *GT06Service)DoRequest(msg *MsgData) bool  {
	logging.Log("Get Input Msg: " + string(msg.Data))
	logging.Log(fmt.Sprintf("imei: %d cmd: %s; go routines: %d", msg.Header.Header.Imei, StringCmd(msg.Header.Header.Cmd), runtime.NumGoroutine()))

	DeviceInfoListLock.RLock()
	_, ok := (*DeviceInfoList)[msg.Header.Header.Imei]
	if ok == false {
		DeviceInfoListLock.RUnlock()
		logging.Log(fmt.Sprintf("invalid deivce, imei: %d cmd: %s", msg.Header.Header.Imei, StringCmd(msg.Header.Header.Cmd)))
		return false
	}
	DeviceInfoListLock.RUnlock()

	bufOffset := uint32(0)
	service.msgSize = uint64(msg.Header.Header.Size)
	service.imei = msg.Header.Header.Imei
	service.cmd = msg.Header.Header.Cmd

	service.GetWatchDataInfo(service.imei)

	if IsDeviceInCompanyBlacklist(service.imei) {
		logging.Log(fmt.Sprintf("device %d  is in the company black list", service.imei))
		return false
	}


	if service.cmd == DRT_SYNC_TIME {  //BP00 对时
		resp := &ResponseItem{CMD_AP03, service.makeSyncTimeReplyMsg()}
		service.rspList = append(service.rspList, resp)
		bufOffset++
		szVersion := make([]byte, 64)
		index := 0
		for i := 0; i < len(msg.Data); i++ {
			if msg.Data[bufOffset] != ',' && index < 64 {
				szVersion[index] = msg.Data[bufOffset]
				index++
				bufOffset++
			}else {break}
		}

		logging.Log(fmt.Sprintf("%d|%s", service.imei, szVersion)) // report version
	}  else if service.cmd == DRT_SEND_LOCATION {
		//BP30 上报定位和报警等数据
		resp := &ResponseItem{CMD_AP30, []byte(fmt.Sprintf("(0019%dAP30)", service.imei))}
		service.rspList = append(service.rspList, resp)

		bufOffset++
		cLocateTag := msg.Data[bufOffset]
		bufOffset += 2
		service.cur.AlarmType = msg.Data[bufOffset] - '0'
		bufOffset++

		if msg.Data[bufOffset] != ',' {
			szAlarm := []byte{msg.Data[bufOffset - 1], msg.Data[bufOffset]}
			alarm, _ := strconv.ParseUint(string(szAlarm), 16, 0)
			service.cur.AlarmType = uint8(alarm)
			bufOffset++
		}

		bufOffset++
		//steps
		service.cur.Steps, _ = strconv.ParseUint(string(msg.Data[bufOffset: bufOffset + 8]), 16, 0)
		bufOffset += 9

		//battery
		service.cur.OrigBattery = msg.Data[bufOffset]- '0'
		bufOffset += 1

		//will need to send location
		if cLocateTag == 'G' || cLocateTag == 'g'  {
			service.needSendLocation = false
		}else {
			bufOffset++
			service.needSendLocation = (msg.Data[bufOffset] - '0') == 1
			if service.needSendLocation {
				resp := &ResponseItem{CMD_AP30, service.makeSendLocationReplyMsg()}
				service.rspList = append(service.rspList, resp)
			}
			bufOffset += 1
		}

		ret := service.ProcessLocate(msg.Data[bufOffset: ], cLocateTag)
		if ret == false {
			logging.Log("ProcessLocateInfo Error")
			return false
		}
	}else if service.cmd == DRT_DEVICE_LOGIN {
		//BP31, 设备登录服务器消息
		resp := &ResponseItem{CMD_AP31, service.makeDeviceLoginReplyMsg()}
		service.rspList = append(service.rspList, resp)
	}else if service.cmd == DRT_DEVICE_ACK {
		//BP04, 手表对服务器请求的应答
		if strings.Contains(string(msg.Data),  "AP11") {
			service.needSendChatNum = false
		}
	}else if service.cmd == DRT_MONITOR_ACK {
		//BP05, 手表对服务器请求电话监听的应答
	}else if service.cmd == DRT_SEND_MINICHAT {
		//BP34,手表发送微聊
		if len(msg.Data) <= 2 {
			logging.Log("ProcessMicChat Error for too small msg data length")
			return false
		}

		bufOffset++
		ret := service.ProcessMicChat(msg.Data[bufOffset: ])
		if ret == false {
			logging.Log("ProcessMicChat Error")
		}
	}else if service.cmd == DRT_FETCH_MINICHAT {
		//BP11, 手表获取未读微聊数目
		service.needSendChat = true
		service.needSendChatNum = true
		service.reqChatInfoNum = 0
		service.reqChatInfoNum = int(msg.Data[bufOffset] - '0') * 10 + int(msg.Data[bufOffset + 1]- '0')
	}else if service.cmd == DRT_FETCH_AGPS {
		//BP32, 手表请求EPO数据
		ret := service.ProcessRspAGPSInfo()
		if ret == false {
			logging.Log("ProcessRspAGPSInfo Error")
		}
	}else if service.cmd == DRT_HEART_BEAT {
		// heart beat
		bufOffset++
		service.cur.AlarmType = msg.Data[bufOffset] - '0'
		bufOffset++
		if msg.Data[bufOffset] != ',' {
			service.cur.AlarmType = uint8(Str2Num(string(msg.Data[bufOffset - 1 : bufOffset + 1]), 16))
			bufOffset++
		}

		bufOffset++

		//steps
		service.cur.Steps = Str2Num(string(msg.Data[bufOffset : bufOffset + 8]), 16)
		bufOffset += 9

		//battery
		service.cur.OrigBattery = msg.Data[bufOffset] - '0'
		bufOffset += 2

		//main base station
		//接下来解析"46000024820000DE127,"
		szMainLBS := make([]byte, 256)
		i := 0
		for msg.Data[bufOffset] != ',' {
			szMainLBS[i] = msg.Data[bufOffset]
			bufOffset++
			i++
		}
		fmt.Sscanf(string(szMainLBS), "%03d%03d%04X%07X%02X",
			&service.MCC, &service.MNC, &service.LACID, &service.CELLID, &service.RXLEVEL)
		service.RXLEVEL = service.RXLEVEL * 2 - 113
		if service.RXLEVEL > 0 {
			service.RXLEVEL = 0
		}

		ret := service.ProcessUpdateWatchStatus(msg.Data[bufOffset: ])
		if ret == false {
			logging.Log("ProcessUpdateWatchStatus Error")
		}
	}else {
		logging.Log(fmt.Sprintf("Error Device CMD(%d, %s)", service.imei, StringCmd(service.cmd)))
	}

	ret := service.PushCachedData()

	return ret
}


func (service *GT06Service)DoResponse() []byte  {
	if service.needSendChat {
		service.ProcessRspChat()
		//data := make([]byte, 200*1024)
		//offset := 0
		//for  _, respCmd := range service.rspList {
		//	if respCmd.rspCmdType == CMD_AP12 {
		//		fields := strings.SplitN(string(respCmd.data), ",", 7)
		//		size := int(Str2Num(fields[5], 10))
		//		copy(data[offset: offset + size], fields[6][0: size])
		//		offset += size
		//	}
		//}
		//
		//ioutil.WriteFile("/home/work/Documents/test2.txt", data[0: offset], 0666)
	}

	if service.needSendChatNum {
		service.PushChatNum()
	}

	offset, bufSize := 0, 256
	data := make([]byte, bufSize)

	for  _, respCmd := range service.rspList {
		if respCmd.data != nil && len(respCmd.data) > 0 {
			if offset + len(respCmd.data) > bufSize {
				bufSize += offset + len(respCmd.data)
				dataNew := make([]byte, bufSize)
				copy(dataNew[0: offset], data[0: offset])
				data = dataNew
			}
			copy(data[offset: offset + len(respCmd.data)], respCmd.data)
			offset += len(respCmd.data)
		}
	}

	if offset == 0 {
		return nil
	}else {
		return data[0: offset]
	}
}

func (service *GT06Service)UpdateDeviceTimeZone(imei uint64, timezone int) bool {
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

func (service *GT06Service)makeSyncTimeReplyMsg() []byte {
	//(002D357593060153353AP03,150728,152900,e0800)
	curTime := time.Now().UTC().Format("060102,150405")
	c, timezone := 'e', 0

	DeviceInfoListLock.RLock()
	device, ok := (*DeviceInfoList)[service.imei]
	if ok {
		timezone = device.TimeZone
	}
	DeviceInfoListLock.RUnlock()

	if timezone == INVALID_TIMEZONE {
		timezone = int(GetTimeZone(service.reqCtx.IP))
		service.UpdateDeviceTimeZone(service.imei, timezone)
	}

	if timezone < 0 {
		c = 'w'
	}

	body := fmt.Sprintf("%015dAP03,%s,%c%04d)", service.imei,curTime, c, timezone)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)  // + string(service.makeSendLocationReplyMsg()))
}

func (service *GT06Service)makeSendLocationReplyMsg() []byte {
	//(0056357593060153353AP1424.772816,121.022636,160,2015,11,12,08,00,00,0000000000000009)
	//(0051357593060571398AP140.000000,0.000000,0,2017,05,22,11,04,28,00000D99DE4C0826)
	curTime := time.Now().UTC().Format("2006,01,02,15,04,05")
	service.old.Lat, service.old.Lng = 22.587725123456,113.913641123456
	body := fmt.Sprintf("%015dAP14%06f,%06f,%d,%s,%016X)",
		service.imei, service.old.Lat, service.old.Lng,service.old.Accracy,
		curTime, makeId())
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}

func (service *GT06Service)makeDeviceLoginReplyMsg() []byte {
	//(0019357593060153353AP31)
	body := fmt.Sprintf("%015dAP31)", service.imei)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}

func (service *GT06Service)makeChatDataReplyMsg(voiceFile, phone string, datatime uint64) []*ResponseItem {
	//(03B5357593060153353AP12,1,170413163300,6,1,900,数据)

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

	return service.shardData("AP12", phone, datatime, voice)
}

func (service *GT06Service)shardData(cmd, phone string, datatime uint64, data []byte) []*ResponseItem {
	resps := []*ResponseItem{}
	bufOffset, iReadLen := 0, len(data)
	blockSize := 700
	packCount := int(iReadLen / blockSize + 1)
	if iReadLen % blockSize == 0 {
		packCount = iReadLen / blockSize
	}

	countLen, packIndex := iReadLen, 1
	for countLen > 0 {
		packSize := countLen
		if countLen >= blockSize {
			packSize = blockSize
		}

		block := &DataBlock{ MsgId:  makeId(),  Imei: service.imei,  Cmd: cmd,  Phone: phone,  Time: datatime,
			BlockCount: packCount, BlockIndex: packIndex, BlockSize: packSize, Data: data[bufOffset: bufOffset + packSize],
		}

		intCmd := CMD_AP12
		if cmd == "AP13" {
			intCmd = CMD_AP13
		}
		resp := &ResponseItem{uint16(intCmd),  service.makeDataBlockReplyMsg(block)}
		resps = append(resps, resp)

		bufOffset += packSize
		countLen -= packSize
		packIndex++
	}

	return resps
}

func (service *GT06Service)makeDataBlockReplyMsg(block *DataBlock) []byte {
	//(03B5357593060153353AP12,1,170413163300,6,1,900,数据)
	//(03A6357593060153353AP13,7,1,900,数据)
	body := ""
	if block.Cmd == "AP12" {
		body = fmt.Sprintf("%015d%s,%s,%d,%d,%d,%d,", block.Imei, block.Cmd, block.Phone, block.Time,
			block.BlockCount, block.BlockIndex, block.BlockSize)
	}else if block.Cmd == "AP13" {
		body = fmt.Sprintf("%015d%s,%d,%d,%d,", block.Imei, block.Cmd,
			block.BlockCount, block.BlockIndex, block.BlockSize)
	}

	tail := fmt.Sprintf(",%016X)", block.MsgId)
	sizeNum := 5 + len(body) + len(block.Data) + len(tail)
	size := fmt.Sprintf("(%04X", sizeNum)

	data := make([]byte, sizeNum)
	copy(data[0: 5 + len(body)], []byte(size + body))
	copy(data[5 + len(body) : 5 + len(body) + len(block.Data)], block.Data)
	copy(data[5 + len(body) + len(block.Data): ], []byte(tail))

	fmt.Print(size, body, tail)
	return data
}

func (service *GT06Service)makeChatNumReplyMsg(chatNum int) []byte {
	//(001B357593060153353AP1102)
	body := fmt.Sprintf("%015dAP11%02d)", service.imei, chatNum)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}

func makeId()  (uint64) {
	id := time.Now().UnixNano() / int64(time.Millisecond ) * 10
	//fmt.Println("make id: ", uint64(id))
	return uint64(id)
}

func (service *GT06Service) ProcessLocate(pszMsg []byte, cLocateTag uint8) bool {
	logging.Log(fmt.Sprintf("%d - begin: m_iAlarmStatu=%d", service.imei, service.cur.AlarmType))
	ret := true
	GetSafeZoneSettings(service.imei)

	if cLocateTag != 'G' && cLocateTag != 'g' {
		if IsDeviceInCompanyBlacklist(service.imei) {
		service.needSendLocation = false
		logging.Log(fmt.Sprintf("device %d comamy is in the black list", service.imei))
		return false
		}
	}

	if cLocateTag == 'G' || cLocateTag == 'g'  {
		ret = service.ProcessGPSInfo(pszMsg)
	} else if cLocateTag == 'W' || cLocateTag == 'w' {
		ret = service.ProcessWifiInfo(pszMsg)
		if service.wiFiNum <= 1 {
			service.needSendLocation = false
			logging.Log(fmt.Sprintf("device %d, ProcessWifiInfo m_shWiFiNum <= 1", service.imei))
			return false
		}
	} else if cLocateTag == 'L' || cLocateTag == 'l'  {
		 ret = service.ProcessLBSInfo(pszMsg)
	} else if cLocateTag == 'M' || cLocateTag == 'm'  {
	 	ret = service.ProcessMutilLocateInfo(pszMsg)
	} else {
		logging.Log(fmt.Sprintf("%d - Error Locate Info Tag(%c)", service.imei, cLocateTag))
		return false
	}

	logging.Log(fmt.Sprintf("%d - middle: m_iAlarmStatu=%d, parsed location:  m_DateTime=%d, m_lng=%f, m_lat=%f",
		service.imei, service.cur.AlarmType, service.cur.DataTime, service.cur.Lng, service.cur.Lat))

	if ret == false || service.cur.Lng == 0 && service.cur.Lat== 0 {
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
	if 0 == service.cur.AlarmType {
		if service.cur.origBatteryOld > 2 && service.cur.OrigBattery <= 2  {
			service.cur.AlarmType = ALARM_BATTERYLOW
		}
	} else if ALARM_BATTERYLOW == service.cur.AlarmType {
		if service.cur.origBatteryOld <= 2 || service.cur.OrigBattery > 2  {
			service.cur.AlarmType = 0
		}
	}

	logging.Log(fmt.Sprintf("end: m_iAlarmStatu=%d",  service.cur.AlarmType))
	logging.Log(fmt.Sprintf("Update database: DeviceID=%d, m_DateTime=%d, m_lng=%f, m_lat=%f",
		service.imei, service.cur.DataTime, service.cur.Lng, service.cur.Lat))

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

	return true
}

func (service *GT06Service) ProcessMicChat(pszMsg []byte) bool {
	//需要断点续传的支持
	if len(pszMsg) == 0 {
		return false
	}

	//(03BA357593060153353BP34,
	// 123456789,170413163300,2710,6,1,900,数据)
	fields := strings.SplitN(string(pszMsg), ",", 7)
	if len(fields) != 7 || len(fields[6]) == 0 {
		logging.Log("mini chat data bad length")
		return false
	}

	//// for test
	//{
	//	data := make([]byte, 200*1024)
	//	offset := 0
	//	for  _, respCmd := range service.rspList {
	//		if respCmd.rspCmdType == CMD_AP12 {
	//			fields := strings.SplitN(string(respCmd.data), ",", 7)
	//			size := int(Str2Num(fields[5], 10))
	//			copy(data[offset: offset + size], fields[6][0: size])
	//			offset += size
	//		}
	//	}
	//
	//	//收到所有数据以后，写入语音文件
	//	ioutil.WriteFile("/home/work/Documents/test2.txt", data[0: offset], 0666)
	//}

	return true
}


func utc_to_gps_hour (iYr, iMo, iDay, iHr int)  int {
	iYearsElapsed := 0     // Years since 1980
	iDaysElapsed := 0      // Days elapsed since Jan 6, 1980
	iLeapDays := 0         // Leap days since Jan 6, 1980
	i := 0

	// Number of days into the year at the start of each month (ignoring leap years)
	doy := [12]int{0,31,59,90,120,151,181,212,243,273,304,334}

	iYearsElapsed = iYr - 1980
	i = 0
	iLeapDays = 0

	for i <= iYearsElapsed {
		if (i % 100) == 20 {
			if (i % 400) == 20 {
				iLeapDays++
			}
		}else if (i % 4) == 0 {
			iLeapDays++
		}

		i++
	}

	if (iYearsElapsed % 100) == 20 {
		if ((iYearsElapsed % 400) == 20) && (iMo <= 2) {
			iLeapDays--
		}
	} else if ((iYearsElapsed % 4) == 0) && (iMo <= 2) {
		iLeapDays--
	}

	iDaysElapsed = iYearsElapsed * 365 + doy[iMo - 1] + iDay + iLeapDays - 6

	return  (iDaysElapsed * 24 + iHr)
}


func (service *GT06Service) ProcessRspAGPSInfo() bool {
	//需要断点续传的支持
	utcNow := time.Now().UTC()
	iCurrentGPSHour := utc_to_gps_hour(utcNow.Year(), int(utcNow.Month()),utcNow.Day(), utcNow.Hour())
	segment := (uint32(iCurrentGPSHour) - StartGPSHour) / 6
	if service.reqCtx.IsDebug == false && ( (segment < 0) || (segment >= MTKEPO_SEGMENT_NUM)) {
		logging.Log(fmt.Sprintf("[%d] EPO segment invalid, %d", service.imei, segment))
		return false
	}

	iReadLen := MTKEPO_DATA_ONETIME * MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER
	data := make([]byte, iReadLen)
	offset := 0
	EpoInfoListLock.RLock()
	for i := 0; i < MTKEPO_DATA_ONETIME; i++ {
		copy(data[offset: offset + MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER], EPOInfoList[i].EPOBuf)
		offset += MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER
	}
	EpoInfoListLock.RUnlock()

	service.rspList = append(service.rspList, service.shardData("AP13", "", 0, data)...)

	// for test
	//{
	//	data := make([]byte, 200 * 1024)
	//	offset := 0
	//	for _, respCmd := range service.rspList {
	//		if respCmd.rspCmdType == CMD_AP13 {
	//			//(03A6357593060153353AP13,7,1,900,数据)
	//			fields := strings.SplitN(string(respCmd.data), ",", 5)
	//			size := int(Str2Num(fields[3], 10))
	//			copy(data[offset: offset + size], fields[4][0: size])
	//			offset += size
	//		}
	//	}
	//
	//	ioutil.WriteFile("/home/work/Documents/test2.txt", data[0: offset], 0666)
	//}

	return true
}

func (service *GT06Service) ProcessRspChat() bool {
	//需要断点续传的支持
	if service.reqCtx.GetChatDataFunc != nil {
		chatData := service.reqCtx.GetChatDataFunc(service.imei)
		for _, chat := range chatData {
			if chat.Flags == 0 {  //未读
				voiceFileName :=fmt.Sprintf("/usr/share/nginx/html/tracker/web/upload/minichat/app/%d/%d.amr",
					service.imei, chat.DateTime)

				service.rspList = append(service.rspList, service.makeChatDataReplyMsg(voiceFileName, chat.Sender, chat.DateTime)...)
			}
		}
	}

	// for test
	//voiceFileName := "/home/work/Documents/test.txt"
	//service.rspList = append(service.rspList, service.makeChatDataReplyMsg(voiceFileName, "123456789", 170526134505)...)

	return true
}

func (service *GT06Service) PushChatNum() bool {
	if service.reqCtx.GetChatDataFunc != nil {
		chatData := service.reqCtx.GetChatDataFunc(service.imei)
		if len(chatData) > 0 {
			//通知终端有聊天信息
			resp := &ResponseItem{CMD_AP11, service.makeChatNumReplyMsg(len(chatData))}
			service.rspList = append(service.rspList, resp)
		}
	}

	return true
}

func (service *GT06Service) ProcessUpdateWatchStatus(pszMsgBuf []byte) bool {
	if len(pszMsgBuf) == 0 {
		return false
	}

	ucBattery := 1
	if service.cur.OrigBattery > 2 {
		ucBattery = int(service.cur.OrigBattery - 2)
	}

	ret := service.UpdateWatchBattery()
	if ret == false {
		logging.Log(fmt.Sprintf("UpdateWatchBattery failed, battery=%d", ucBattery))
		return ret
	}

	nStep := service.cur.Steps
	if  nStep >= 0x7fff - 1 || int(nStep) < 0 {
		nStep = 0
	}

	iLocateType := LBS_SMARTLOCATION

	bufOffset := 0
	bufOffset++

	iDayTime := service.GetIntValue(pszMsgBuf[bufOffset: ], TIME_LEN)
	iDayTime = iDayTime % 1000000
	bufOffset += TIME_LEN + 1

	iSecTime := service.GetIntValue(pszMsgBuf[bufOffset: ], TIME_LEN)
	i64Time := uint64(iDayTime * 1000000 + iSecTime)
	bufOffset += TIME_LEN + 1

	logging.Log(fmt.Sprintf("Update Watch %d, Step:%d, Battery:%d", service.imei, service.cur.Steps, ucBattery))

	stWatchStatus := &WatchStatus{}
	stWatchStatus.i64DeviceID = service.imei
	stWatchStatus.iLocateType = uint8(iLocateType)
	stWatchStatus.i64Time = i64Time
	stWatchStatus.Step = nStep
	ret = service.UpdateWatchStatus(stWatchStatus)
	if ret == false {
		logging.Log("Update Watch status failed ")
	}

	if ucBattery <=7 && ucBattery >= 0 {
		service.cur.Battery = uint8(ucBattery)
		ret = service.UpdateWatchBattery()
		if ret == false {
			logging.Log(fmt.Sprintf("UpdateWatchBattery failed, battery=%d", ucBattery))
			return ret
		}
	}

	return true
}

func (service *GT06Service) PushCachedData() bool {
	return true
}

func (service *GT06Service) ProcessGPSInfo(pszMsg []byte) bool {
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

	iTimeSec := service.GetIntValue(pszMsg[bufOffset: ], TIME_LEN)
	bufOffset += TIME_LEN

	iIOStatu := service.cur.OrigBattery - 2
	if iIOStatu < 1 {
		service.cur.AlarmType = ALARM_BATTERYLOW
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
	service.cur.Steps = uint64(iSpeed)
	service.cur.ReadFlag = 0
	service.cur.Battery = iIOStatu
	service.cur.LocateType = LBS_GPS

	return true
}

func (service *GT06Service) ProcessWifiInfo(pszMsgBuf []byte) bool {
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
	service.cur.Battery = service.cur.OrigBattery - 2
	if service.cur.Battery < 1  {
		service.cur.AlarmType = ALARM_BATTERYLOW
		service.cur.Battery = 1
	}

	//service.GetWatchDataInfo(service.imei)

	if service.wifiZoneIndex >= 0 {
		DeviceInfoListLock.RLock()
		safeZones := GetSafeZoneSettings(service.imei)
		if safeZones != nil {
			strLatLng := strings.Split(safeZones[service.wifiZoneIndex].Center, ",")
			service.cur.Lat = Str2Float(strLatLng[0])
			service.cur.Lng = Str2Float(strLatLng[1])
			service.accracy = 150

			logging.Log(fmt.Sprintf("Get location frome home zone by home wifi: %f, %f",
				service.cur.Lat, service.cur.Lng))
		}
		DeviceInfoListLock.RUnlock()
	} else {
		ret := service.GetLocation()
		if ret == false {
			service.needSendLocation = false
			logging.Log("GetLocation failed")
			return false
		}
	}

	service.cur.Steps = 0
	service.cur.ReadFlag = 0
	service.cur.LocateType = LBS_WIFI
	service.cur.Accracy = service.accracy

	service.UpdateLocationIntoDBCache()
	return true
}

func (service *GT06Service) ProcessLBSInfo(pszMsgBuf []byte) bool {
	if len(pszMsgBuf) == 0 {
		logging.Log(fmt.Sprintf("pszMsgBuf is NULL"))
		return false
	}

	bufOffset := uint32(0)
	bufOffset++
	shLBSLen := uint32(0)
	service.ParseLBSInfo(pszMsgBuf[bufOffset: ],  &shLBSLen)
	bufOffset += shLBSLen

	iDayTime := service.GetIntValue(pszMsgBuf[bufOffset: ], TIME_LEN)
	iDayTime = iDayTime % 1000000
	bufOffset += TIME_LEN + 1

	iSecTime := service.GetIntValue(pszMsgBuf[bufOffset: ], TIME_LEN)
	i64Time := uint64(iDayTime * 1000000 + iSecTime)

	service.cur.Battery = service.cur.OrigBattery - 2
	if service.cur.Battery < 1 {
		service.cur.AlarmType = ALARM_BATTERYLOW
		service.cur.Battery = 1
	}

	if service.lbsNum <= 1 {
		stWatchStatus := &WatchStatus{}
		stWatchStatus.i64DeviceID = service.imei
		stWatchStatus.i64Time = i64Time
		stWatchStatus.iLocateType = LBS_SMARTLOCATION
		stWatchStatus.Step = 0
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

	service.cur.DataTime = i64Time
	service.cur.Steps = 0
	service.cur.ReadFlag  = 0
	service.cur.Accracy = service.accracy
	service.cur.LocateType = LBS_JIZHAN  //uint8(service.accracy / 10) //LBS_JIZHAN;

	service.UpdateLocationIntoDBCache();

	return true
}

func (service *GT06Service) ProcessMutilLocateInfo(pszMsgBuf []byte) bool {
	if len(pszMsgBuf) == 0 {
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
	service.ParseLBSInfo(pszMsgBuf[bufOffset: ], &shBufLen)
	bufOffset += shBufLen

	service.ParseWifiInfo(pszMsgBuf[bufOffset: ], &shBufLen)
	bufOffset += shBufLen

	service.cur.DataTime = i64Time
	service.cur.Battery = service.cur.OrigBattery - 2
	if service.cur.Battery < 1 {
		service.cur.AlarmType = ALARM_BATTERYLOW
		service.cur.Battery = 1
	}

	//service.GetWatchDataInfo(service.imei)

	if service.wifiZoneIndex >= 0  {
		DeviceInfoListLock.RLock()
		safeZones := GetSafeZoneSettings(service.imei)
		if safeZones != nil {
			strLatLng := strings.Split(safeZones[service.wifiZoneIndex].Center, ",")
			service.cur.Lat = Str2Float(strLatLng[0])
			service.cur.Lng = Str2Float(strLatLng[1])
			service.accracy = 150

			logging.Log(fmt.Sprintf("Get location frome home zone by home wifi: %f, %f",
				service.cur.Lat, service.cur.Lng))
		}
		DeviceInfoListLock.RUnlock()
	} else {
		ret := service.GetLocation()
		if ret == false {
			service.needSendLocation = false
			logging.Log("GetDevicePostion for multilocation failed")
			return false
		}
	}

	service.cur.Steps = 0
	service.cur.ReadFlag = 0
	service.cur.LocateType = LBS_WIFI
	service.cur.Accracy = service.accracy

	if service.accracy >= 2000 {
		service.cur.LocateType = LBS_INVALID_LOCATION
	} else if service.wiFiNum >= 3 && service.accracy <= 400 {
		service.cur.LocateType = LBS_WIFI
	} else {
		service.cur.LocateType = LBS_JIZHAN // uint8(service.accracy / 10)  //LBS_JIZHAN
	}

	service.UpdateLocationIntoDBCache()
	return true
}

func (service *GT06Service) ProcessZoneAlarm() bool {
	//service.GetWatchDataInfo(service.imei)
	if service.old.DataTime > 0 {
		iZoneID := service.old.ZoneIndex
		iAlarmType := service.old.AlarmType
		iZoneAlarmType := service.old.ZoneAlarm

		if service.cur.Battery == 1 && service.old.Battery > 1 {
			service.cur.AlarmType = ALARM_BATTERYLOW
		}

		if iAlarmType == service.cur.AlarmType {
			service.cur.AlarmType = 0
		} else {
			iAlarmType = service.cur.AlarmType;
		}

		if iZoneID == 0 && iZoneAlarmType == 0 {
			iZoneID = 1
		}

		stCurPoint, stDstPoint := TPoint{}, TPoint{}
		if (service.cur.LocateType != LBS_GPS && service.wifiZoneIndex < 0) ||  service.cur.AlarmType > 0 {
			if service.old.LocateType == LBS_WIFI {
				stCurPoint.Latitude = service.cur.Lat
				stCurPoint.LongtiTude = service.cur.Lng
				stDstPoint.Latitude = service.old.Lat
				stDstPoint.LongtiTude = service.old.Lng
				iDistance := service.GetDisTance(&stCurPoint, &stDstPoint)

				if iDistance <= 200 && (service.cur.LocateType == LBS_JIZHAN ) {
					i64Time := service.cur.DataTime
					service.cur = service.old
					service.cur.DataTime = i64Time
					service.cur.LocateType = LBS_SMARTLOCATION
					service.getSameWifi = true
				}

				if iDistance >= 1800 && service.cur.DataTime >= service.old.DataTime {
					iTotalMin := deltaMinutes(service.old.DataTime, service.cur.DataTime)
					if iTotalMin == 0 || (iTotalMin > 0 && iTotalMin * 800 <= iDistance) {
						i64Time := service.cur.DataTime
						service.cur = service.old
						service.cur.DataTime = i64Time
						service.cur.LocateType = LBS_SMARTLOCATION
						service.getSameWifi = true
					}
				}
			}

			logging.Log(fmt.Sprintf("device %d, m_iAlarmStatu=%d, parsed location:  m_DateTime=%d, m_lng=%d, m_lat=%d",
				service.imei,  service.cur.AlarmType, service.cur.DataTime, service.cur.Lng, service.cur.Lat))

			return true
		}

		stCurPoint.Latitude = service.cur.Lat
		stCurPoint.LongtiTude = service.cur.Lng

		DeviceInfoListLock.RLock()
		safeZones := GetSafeZoneSettings(service.imei)
		strLatLng := strings.Split(safeZones[service.wifiZoneIndex].Center, ",")
		zoneLat := Str2Float(strLatLng[0])
		zoneLng := Str2Float(strLatLng[1])

		if safeZones != nil {
			for  i := 0; i < len(safeZones); i++ {
				stSafeZone := safeZones[i]
				if stSafeZone.On == "0" {
					continue
				}

				stDstPoint.Latitude = zoneLat
				stDstPoint.LongtiTude = zoneLng
				iRadiu := service.GetDisTance(&stCurPoint, &stDstPoint)
				if iRadiu < uint32(stSafeZone.Radius) {
					if iZoneID != stSafeZone.ZoneID || (iZoneID == stSafeZone.ZoneID && iZoneAlarmType != ALARM_INZONE) {
						if service.wifiZoneIndex < 0 && service.cur.LocateType == LBS_WIFI  {
							break
						}

						if service.hasSetHomeWifi && service.wifiZoneIndex >= 0 && strings.ToLower(stSafeZone.ZoneName) == "home" {
							continue
						}

						iZoneID = stSafeZone.ZoneID
						iZoneAlarmType = ALARM_INZONE
						iAlarmType = 0
						service.cur.AlarmType = iZoneAlarmType
						service.cur.ZoneName = stSafeZone.ZoneName
						logging.Log(fmt.Sprintf("Device[%d] Make a InZone Alarm[%s][%d,%d][%d,%d]",
							service.imei, stSafeZone.ZoneName, iRadiu, stSafeZone.Radius , iZoneAlarmType, iZoneID))
						break
					}
				} else {
					if iZoneID == stSafeZone.ZoneID && iZoneAlarmType != ALARM_OUTZONE {
						if service.wifiZoneIndex < 0 && service.cur.LocateType == LBS_WIFI  {
							break
						}

						iZoneID = stSafeZone.ZoneID
						iZoneAlarmType = ALARM_OUTZONE
						service.cur.AlarmType = iZoneAlarmType
						iAlarmType = 0
						service.cur.ZoneName = stSafeZone.ZoneName
						logging.Log(fmt.Sprintf("Device[%d] Make a OutZone Alarm[%s][%d,%d][%d,%d]",
							service.imei, stSafeZone.ZoneName, iRadiu, stSafeZone.Radius,  iZoneAlarmType,  iZoneID))
						break
					}
				}
			}
		}

		DeviceInfoListLock.RUnlock()
	} else{
		logging.Log(fmt.Sprintf("device %d, m_iAlarmStatu=%d, parsed location:  m_DateTime=%d, m_lng=%f, m_lat=%f",
			service.imei, service.cur.AlarmType, service.cur.DataTime, service.cur.Lng, service.cur.Lat))
	}

	return true
}

func (service *GT06Service) makeJson(data *LocationData) string  {
	strData, err := json.Marshal(data)
	if err != nil {
		logging.Log("make json failed, " + err.Error())
	}

	return string(strData)
	//return fmt.Sprintf("{\"steps\": %d,  \"readflag\": %d,  \"locateType\": %d, \"accracy\": %d,  \"battery\": %d,   \"alarm\": %d,  \"zoneAlarm\": %d, \"zoneIndex\": %d,  \"zoneName\": \"%s\"}",
	//data.Steps, data.ReadFlag, data.LocateType, data.Accracy, data.Battery, data.AlarmType, data.ZoneAlarm, data.ZoneIndex, data.ZoneName)
}

func (service *GT06Service) WatchDataUpdateDB() bool {
	strTime := fmt.Sprintf("20%d", service.cur.DataTime) //20170520114920
	strTableaName := fmt.Sprintf("device_location_%s_%s", string(strTime[0:4]), string(strTime[4: 6]))
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

func (service *GT06Service) UpdateWatchData() bool {
	if service.reqCtx.SetDeviceDataFunc != nil {
		service.reqCtx.SetDeviceDataFunc(service.imei, DEVICE_DATA_LOCATION, service.cur)
		return true
	}else{
		return false
	}
}

func (service *GT06Service) UpdateWatchStatus(watchStatus *WatchStatus) bool {
	deviceData := LocationData{LocateType: watchStatus.iLocateType,
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

func (service *GT06Service) UpdateWatchBattery() bool {
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

func (service *GT06Service) NotifyAlarmMsg() bool {
	strRequestBody := "r=app/auth/alarm&"
	strReqURL := "http://service.gatorcn.com/tracker/web/index.php"
	value := fmt.Sprintf("systemno=%d&data=%d,%d,%d,%d,%d,%d,%d,%d,%s,%s",
		service.imei % 100000000000, service.cur.DataTime, service.cur.Lat, service.cur.Lng,
		service.cur.Steps, service.cur.Battery, service.cur.AlarmType, service.cur.ReadFlag,
		service.cur.LocateType, service.cur.ZoneName, "")

	strRequestBody += value
	strReqURL += "?" +  strRequestBody
	logging.Log(strReqURL)
	resp, err := http.Get(strReqURL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] call php api to notify app failed  failed,  %s", service.imei, err.Error()))
		return false
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] php notify api  response has err, %s", service.imei, err.Error()))
		return false
	}

	logging.Log(fmt.Sprintf("[%d] read php notify api response:, %s", service.imei, body))

	return true
}

func (service *GT06Service) UpdateLocationIntoDBCache() bool {
	return true
}

func (service *GT06Service) rad(d float64) float64 {
	return d * PI / 180.0
}

func getTimeStruct(time uint64) *timestruct {
	t := &timestruct{}
	t.y = uint64(time / 10000000000)
	t.m = uint64((time - t.y * 10000000000) / 100000000)
	t.d = uint64((time - t.y * 10000000000 - t.m * 100000000) / 1000000)
	t.h = uint64((time - t.y * 10000000000 - t.m * 100000000 - t.d * 1000000) / 10000)
	t.mi = uint64((time - t.y * 10000000000 - t.m * 100000000 - t.d * 1000000 - t.h * 10000) / 100)
	t.s = uint64(0)
	return t
}

var daysForMonth = []uint16{
	//1  2   3   4   5   6   7   8   9  10  11  12
	31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31,
}

func isLeapYear(year uint64) bool {
	return (year % 4 == 0 && year % 100 != 0 || year % 400 == 0)
}

func deltaMinutes(time1, time2 uint64) uint32 {
	t1 := getTimeStruct(time1)
	t2 := getTimeStruct(time2)
	days1, days2 := uint32(0), uint32(0)

	for i := 0; i < int(t1.m - 1); i++ {
		leapYear := uint32(0)
		if i == 1 && isLeapYear(2000 + t1.y) {
			leapYear = 1
		}
		days1 += uint32(daysForMonth[i]) + leapYear
	}

	days1 += uint32(t1.d)

	for i := 0; i < int(t2.y - t1.y); i++ {
		for j := 0; j < 12; j++ {
			leapYear := uint32(0)
			if j == 1 && isLeapYear(2000 + t1.y + uint64(i)) {
				leapYear = 1
			}
			days2 += uint32(daysForMonth[j]) + leapYear
		}
	}

	for i := 0; i < int(t2.m - 1); i++ {
		leapYear := uint32(0)
		if i == 1 && isLeapYear(2000 + t2.y) {
			leapYear = 1
		}
		days2 += uint32(daysForMonth[i]) + leapYear
	}

	days2 += uint32(t2.d)

	return (days2 - days1) * 24 * 60 + uint32(t2.h - t1.h) * 60 + uint32(t2.mi - t1.mi)
}

func (service *GT06Service) GetDisTance(stPoint1, stPoint2 *TPoint) uint32 {
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

func (service *GT06Service) GetIntValue(pszMsgBuf []byte,  shLen uint32) uint64{
	value, _ := strconv.Atoi(string(pszMsgBuf[0: shLen]))
	return uint64(value)
}


func (service *GT06Service) GetValue(pszMsgBuf []byte, shMaxLen *int32) int32 {
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


func (service *GT06Service) GetFloatValue(pszMsgBuf []byte, shLen uint16) float64 {
	if shLen > 12 {
		shLen = 12
	}

	value, _ := strconv.ParseFloat(string(pszMsgBuf[0 : shLen]), 0)
	return value
}

func  (service *GT06Service) GetWatchDataInfo(imei uint64)  {
	if service.reqCtx.GetDeviceDataFunc != nil {
		service.old = service.reqCtx.GetDeviceDataFunc(imei, service.reqCtx.Pgpool)
	}
}
func  (service *GT06Service) GetLocation()  bool{
	useGooglemap := true
	DeviceInfoListLock.RLock()
	deviceInfo, ok := (*DeviceInfoList)[service.imei]
	if ok && strings.Contains(deviceInfo.CountryCode, "86") {
		useGooglemap = false
	}
	DeviceInfoListLock.RUnlock()

	if useGooglemap {
		return service.GetLocationByGoogle()
	}else {
		return service.GetLocationByAmap()
	}
}


func  (service *GT06Service) ParseWifiInfo(pszMsgBuf []byte, shBufLen *uint32)  bool {
	*shBufLen = 0
	bufOffset := uint32(0)
	if  len(pszMsgBuf) == 0 {
		logging.Log("pszMsgBuf is empty")
		return false
	}

	service.wiFiNum = 0
	service.wifiInfoList = [MAX_WIFI_NUM]WIFIInfo{}

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

	DeviceInfoListLock.RLock()
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
	DeviceInfoListLock.RUnlock()

	return true
}

func  (service *GT06Service) ParseLBSInfo(pszMsgBuf []byte, shBufLen *uint32)  bool {
	if len(pszMsgBuf) == 0 {
		logging.Log("pszMsgBuf is NULL")
		return false
	}

	bufOffset := 0
	service.lbsNum = 0
	*shBufLen = 0
	service.lbsInfoList = [MAX_LBS_NUM]LBSINFO{}

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
func  (service *GT06Service) GetLocationByGoogle() bool  {
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

	resp, err := http.NewRequest("POST",
		"https://www.googleapis.com/geolocation/v1/geolocate?key=AIzaSyAbvNjmCijnZv9od3cC0MwmTC6HTBG6R60",
		strings.NewReader(string(jsonStr)))
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] call google map api  failed,  %s", service.imei, err.Error()))
		return false
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] google map response has err, %s", service.imei, err.Error()))
		return false
	}

	result := GooglemapResult{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] parse google map response failed, %s", service.imei, err.Error()))
		return false
	}

	service.cur.Accracy = uint32(result.Accuracy)
	service.cur.Lat = result.Location.Lat
	service.cur.Lng = result.Location.Lng

	return true
}

//for amap(高德) api
func  (service *GT06Service) GetLocationByAmap() bool  {
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
	gcj_To_Gps84(dLatitude, dLontiTude, &service.cur.Lat, &service.cur.Lng)
	service.cur.LocateType = uint8(Str2Num(result.Result.Type, 10))
	service.cur.Accracy = uint32(Str2Num(result.Result.Radius, 10))

	logging.Log(fmt.Sprintf("amap TransForm location Is:%06f, %06f", service.cur.Lat, service.cur.Lng))

	return true
}


const pi = float64(3.1415926535897932384626)
const a = float64(6378245.0)
const ee = float64(0.00669342162296594323)
func transformLat(x, y float64) float64 {
	ret := -100.0 + 2.0 * x + 3.0 * y + 0.2 * y * y + 0.1 * x * y + 0.2 * math.Sqrt(math.Abs(x))
	ret += (20.0 * math.Sin(6.0 * x * pi) + 20.0 * math.Sin(2.0 * x * pi)) * 2.0 / 3.0
	ret += (20.0 * math.Sin(y * pi) + 40.0 * math.Sin(y / 3.0 * pi)) * 2.0 / 3.0
	ret += (160.0 * math.Sin(y / 12.0 * pi) + 320 * math.Sin(y * pi / 30.0)) * 2.0 / 3.0
	return ret
}

func  transformLon(x, y float64) float64 {
	ret := 300.0 + x + 2.0 * y + 0.1 * x * x + 0.1 * x * y + 0.1 * math.Sqrt(math.Abs(x))
	ret += (20.0 * math.Sin(6.0 * x * pi) + 20.0 * math.Sin(2.0 * x * pi)) * 2.0 / 3.0
	ret += (20.0 * math.Sin(x * pi) + 40.0 * math.Sin(x / 3.0 * pi)) * 2.0 / 3.0
	ret += (150.0 * math.Sin(x / 12.0 * pi) + 300.0 * math.Sin(x / 30.0 * pi)) * 2.0 / 3.0
	return ret
}

func transform(lat, lon float64, mgLat, mgLon *float64) {
	dLat := transformLat(lon - 105.0, lat - 35.0)
	dLon := transformLon(lon - 105.0, lat - 35.0)
	radLat := lat / 180.0 * pi
	magic := math.Sin(radLat)
	magic = 1 - ee * magic * magic
	sqrtMagic := (magic)
	dLat = (dLat * 180.0) / ((a * (1 - ee)) / (magic * sqrtMagic) * pi)
	dLon = (dLon * 180.0) / (a / sqrtMagic * math.Cos(radLat) * pi)
	*mgLat = lat + dLat
	*mgLon = lon + dLon
}

func gcj_To_Gps84(lat, lon float64, mgLat, mgLon *float64) {
	dTempLat,  dTempLon := float64(0), float64(0)
	transform(lat, lon, &dTempLat, &dTempLon)
	*mgLon = lon * 2 - dTempLon
	*mgLat = lat * 2 - dTempLat
}