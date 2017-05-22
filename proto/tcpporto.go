package proto

import (
	"fmt"
	"runtime"
	"strconv"
	"../logging"
	"time"
	"strings"
	"unicode"
	"math"
	"github.com/jackc/pgx"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"encoding/json"
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
	Pgpool *pgx.ConnPool
	MysqlPool *sql.DB

	WritebackChan chan *MsgData
	Msg *MsgData
	GetDeviceDataFunc  func (imei uint64)  LocationData
	SetDeviceDataFunc  func (imei uint64, updateType int, deviceData LocationData)
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

type ChatInfo struct {
	DateTime uint64
	SenderType uint8
	Sender []byte
	ReceiverType uint8
	Receiver []byte
	ContentType uint8
	Content []byte
}

type DeviceCache struct {
	Imei uint64
	CurrentLocation LocationData
	AlarmCache []LocationData
	ChatCache []ChatInfo
}

type GT06Service struct {
	imei uint64
	msgSize uint64
	cmd uint16
	old LocationData
	cur LocationData
	needSendLocation bool
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
	//if msg.Data[0] != 0x28 {
	//	logging.Log(fmt.Sprintf("Error Msg Token[%s]", string(msg.Data[0: ])))
	//	return false;
	//}

	logging.Log("Get Input Msg: " + string(msg.Data))

	//imei, _ := strconv.ParseUint(string(msg.Data[0: ImeiLen]), 0, 0)
	//cmd := string(msg.Data[ImeiLen: ImeiLen + CmdLen])

	logging.Log(fmt.Sprintf("imei: %d cmd: %s; go routines: %d", msg.Header.Header.Imei, StringCmd(msg.Header.Header.Cmd), runtime.NumGoroutine()))

	DeviceInfoListLock.RLock()
	_, ok := (*DeviceInfoList)[msg.Header.Header.Imei]
	if ok == false {
		DeviceInfoListLock.RUnlock()
		logging.Log(fmt.Sprintf("invalid deivce, imei: %d cmd: %s", msg.Header.Header.Imei, StringCmd(msg.Header.Header.Cmd)))
		return false
	}
	DeviceInfoListLock.RUnlock()
	service.GetWatchDataInfo(service.imei)

	bufOffset := uint32(0)
	service.msgSize = uint64(msg.Header.Header.Size)
	service.imei = msg.Header.Header.Imei
	service.cmd = msg.Header.Header.Cmd
	//bufOffset++
	//shMsgLen--
	//service.msgSize, _ = strconv.ParseUint(string(msg.Data[bufOffset: bufOffset + 4]), 16, 0)
	//bufOffset += 4
	//shMsgLen -= 4
	//
	//service.imei, _ = strconv.ParseUint(string(msg.Data[bufOffset: bufOffset + DEVICEID_BIT_NUM]), 10, 0)
	//bufOffset += DEVICEID_BIT_NUM
	//shMsgLen -= DEVICEID_BIT_NUM
	//
	//service.cmd = IntCmd(string(msg.Data[bufOffset: bufOffset + DEVICE_CMD_LENGTH]))
	//
	//bufOffset += DEVICE_CMD_LENGTH
	//shMsgLen -= DEVICE_CMD_LENGTH

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
		bufOffset += 2

		//will need to send location
		service.needSendLocation = (msg.Data[bufOffset] - '0') == 1
		bufOffset += 1

		ret := service.ProcessLocate(msg.Data[bufOffset: ], cLocateTag)
		if ret == false {
			logging.Log("ProcessLocateInfo Error")
			return false
		}
	}

	return true
}


func (service *GT06Service)DoResponse() []byte  {
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

	if service.needSendLocation {

	}

	if offset == 0 {
		return nil
	}else {
		return data[0: offset]
	}
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
	if timezone < 0 {
		c = 'w'
	}

	body := fmt.Sprintf("%015dAP03,%s,%c%04d)", service.imei,curTime, c, timezone)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body + string(service.makeSendLocationReplyMsg()))
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

func makeId()  (uint64) {
	id := time.Now().UnixNano() / int64(time.Millisecond ) * 10
	fmt.Println("make id: ", uint64(id))
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
		logging.Log(fmt.Sprintf("Error Locate(%d, %d, %d)", service.imei, service.cur.Lng, service.cur.Lat))
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
	logging.Log(fmt.Sprintf("Update database: DeviceID=%d, m_DateTime=%d, m_lng=%d, m_lat=%d",
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
	}
	fmt.Sscanf(string(szMainLBS), "%03d%03d%04X%07X%02X",
		&service.MCC, &service.MNC, &service.LACID, &service.CELLID, &service.RXLEVEL)
	service.RXLEVEL = service.RXLEVEL * 2 - 113
	if service.RXLEVEL > 0 {
		service.RXLEVEL = 0
	}
	bufOffset++

	bTrueTime := true
	iTimeDay := uint32(0)
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

	dLatitude := float64(iTemp / 60.0)
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

	dLontitude := float64(iTemp / 60.0)
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
	service.cur.Lng = float64(iLongtitude / 100000.0)
	service.cur.Lat = float64(iLatitude / 100000.0)
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

	if unicode.IsDigit(rune(pszMsgBuf[bufOffset])) {
		bufOffset++
	}

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

	shBufLen = 0
	if unicode.IsDigit(rune(pszMsgBuf[bufOffset])) {
		bufOffset++
	}

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
						if service.wifiZoneIndex < 0 && service.cur.LocateType == LBS_WIFI && service.cur.LocateType != LBS_WIFI {
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
						if service.wifiZoneIndex < 0 && service.cur.LocateType == LBS_WIFI && service.old.LocateType != LBS_WIFI  {
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
		logging.Log(fmt.Sprintf("device %d, m_iAlarmStatu=%d, parsed location:  m_DateTime=%d, m_lng=%d, m_lat=%d",
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
	strSQL := fmt.Sprintf("INSERT INTO %s VALUES(%d, %d, %f, %f, '%s'::jsonb)",
		strTableaName, service.imei, service.cur.DataTime, service.cur.Lat, service.cur.Lng, service.makeJson(&service.cur))

	logging.Log(fmt.Sprintf("SQL: %s", strSQL))

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
		service.reqCtx.SetDeviceDataFunc(service.imei, DEVICE_DATA_STATUS, deviceData)
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

func (service *GT06Service) GetIntValue(pszMsgBuf []byte,  shLen uint32) uint32{
	value, _ := strconv.Atoi(string(pszMsgBuf[0: shLen]))
	return uint32(value)
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
		service.old = service.reqCtx.GetDeviceDataFunc(imei)
	}
}
func  (service *GT06Service) GetLocation()  bool{
	return true
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
			if shIndex == 3 {
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