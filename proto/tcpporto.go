package proto

import (
	"fmt"
	"runtime"
	"strconv"
	"../logging"
	"time"
	"strings"
	"unicode"
)

type ResponseItem struct {
	rspCmdType uint16
	data []byte
}

type TPoint struct {
	Latitude float64
	LongtiTude float64
}



type LocationData struct {
	dataTime uint64
	longtiTude float64
	latitude float64
	steps uint64
	readflag uint8
	locateType uint8
	battery uint8
	origBattery uint8
	origBatteryOld uint8
	alarm uint8
	zoneAlarm uint8
	zoneIndex int32
 	zoneName string
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
}

const DEVICEID_BIT_NUM = 15
const DEVICE_CMD_LENGTH = 4
const TIME_LEN  = 6

const MAX_WIFI_NAME_LEN = 64
const MAX_MACID_LEN = 17
const MAX_WIFI_NUM = 6
const MAX_LBS_NUM = 8

func HandleTcpRequest(tcpserverChan chan *MsgData, msg *MsgData)  bool{
	service := &GT06Service{}
	ret := service.PreDoRequest()
	if ret == false {
		return false
	}

	ret = service.DoRequest(msg)
	if ret == false {
		return false
	}
	data := service.DoResponse()
	if data == nil {
		return false
	}
	msgResp := &MsgData{Data: data}
	msgResp.Header.Header.Imei = service.imei

	tcpserverChan <- msgResp
	return true
}

func (service *GT06Service)PreDoRequest() bool  {
	service.wifiZoneIndex = -1
	return true
}


func (service *GT06Service)DoRequest(msg *MsgData) bool  {
	if msg.Data[0] != 0x28 {
		logging.Log(fmt.Sprintf("Error Msg Token[%s]", string(msg.Data[0: 1])))
		return false;
	}

	imei, _ := strconv.ParseUint(string(msg.Data[0: ImeiLen]), 0, 0)
	cmd := string(msg.Data[ImeiLen: ImeiLen + CmdLen])

	logging.Log(fmt.Sprintf("imei: %d cmd: %s; go routines: %d", imei, cmd, runtime.NumGoroutine()))

	bufOffset, shMsgLen := uint32(0), len(msg.Data)
	bufOffset++
	shMsgLen--
	service.msgSize, _ = strconv.ParseUint(string(msg.Data[bufOffset: bufOffset + 4]), 16, 0)
	bufOffset += 4
	shMsgLen -= 4

	service.imei, _ = strconv.ParseUint(string(msg.Data[bufOffset: bufOffset + DEVICEID_BIT_NUM]), 10, 0)
	bufOffset += DEVICEID_BIT_NUM
	shMsgLen -= DEVICEID_BIT_NUM

	service.cmd = IntCmd(string(msg.Data[bufOffset: bufOffset + DEVICE_CMD_LENGTH]))

	bufOffset += DEVICE_CMD_LENGTH
	shMsgLen -= DEVICE_CMD_LENGTH

	if IsDeviceInCompanyBlacklist(service.imei) {
		logging.Log(fmt.Sprintf("device %d  is in the company black list", service.imei))
		return false
	}


	if service.cmd == DRT_SYNC_TIME {  //BP00 对时
		resp := &ResponseItem{CMD_AP03, nil}
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
		service.cur.alarm = msg.Data[bufOffset] - '0'
		bufOffset++

		if msg.Data[bufOffset] != ',' {
			szAlarm := []byte{msg.Data[bufOffset - 1], msg.Data[bufOffset]}
			alarm, _ := strconv.ParseUint(string(szAlarm), 16, 0)
			service.cur.alarm = uint8(alarm)
			bufOffset++
		}

		bufOffset++
		//steps
		service.cur.steps, _ = strconv.ParseUint(string(msg.Data[bufOffset: bufOffset + 8]), 16, 0)
		bufOffset += 9

		//battery
		service.cur.origBattery = msg.Data[bufOffset]- '0'
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
	return nil
}


func (service *GT06Service) ProcessLocate(pszMsg []byte, cLocateTag uint8) bool {
	logging.Log(fmt.Sprintf("%d - begin: m_iAlarmStatu=%d", service.imei, service.cur.alarm))
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
		service.imei, service.cur.alarm, service.cur.dataTime, service.cur.longtiTude, service.cur.latitude))

	if ret == false || service.cur.longtiTude == 0 && service.cur.latitude == 0 {
		logging.Log(fmt.Sprintf("Error Locate(%d, %d, %d)", service.imei, service.cur.longtiTude, service.cur.latitude))
		return ret
	}

	if service.getSameWifi && service.cur.alarm <= 0  {
		logging.Log(fmt.Sprintf("return with: m_bGetSameWifi && m_iAlarmStatu <= 0(%d, %d, %d)",
			service.imei, service.cur.latitude, service.cur.longtiTude))
		return ret
	}

	//范围报警
	ret = service.ProcessZoneAlarm()
	if 0 == service.cur.alarm {
		if service.cur.origBatteryOld > 2 && service.cur.origBattery <= 2  {
			service.cur.alarm = ALARM_BATTERYLOW
		}
	} else if ALARM_BATTERYLOW == service.cur.alarm {
		if service.cur.origBatteryOld <= 2 || service.cur.origBattery > 2  {
			service.cur.alarm = 0
		}
	}

	logging.Log(fmt.Sprintf("end: m_iAlarmStatu=%d",  service.cur.alarm))
	logging.Log(fmt.Sprintf("Update database: DeviceID=%d, m_DateTime=%d, m_lng=%d, m_lat=%d",
		service.imei, service.cur.dataTime, service.cur.longtiTude, service.cur.latitude))

	ret = service.WatchDataUpdateDB()
	if ret == false {
		logging.Log(fmt.Sprintf("Update WatchData into Database failed"))
		return false
	}

	if service.cur.locateType != LBS_INVALID_LOCATION {
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
			logging.Log(fmt.Sprintf("UpdateWatchBattery failed, battery=%d", service.cur.battery))
			return false
		}
	}

	if service.cur.alarm > 0  {
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

	iIOStatu := service.cur.origBattery - 2
	if iIOStatu < 1 {
		service.cur.alarm = ALARM_BATTERYLOW
		iIOStatu = 1
	}

	i64Time :=  uint64(0)
	if bTrueTime {
		i64Time = uint64(iTimeDay * 1000000 + iTimeSec)
	} else {
		now := time.Now()
		year,mon,day := now.UTC().Date()
		hour,min,sec := now.UTC().Clock()
		i64Time = uint64(year) * uint64(10000000000) +  uint64(mon) * uint64(100000000) + uint64(day) * uint64(1000000) + uint64(hour * 10000 + min * 100 + sec)
	}

	service.cur.dataTime = i64Time
	service.cur.longtiTude = float64(iLongtitude / 100000.0)
	service.cur.latitude = float64(iLatitude / 100000.0)
	service.cur.steps = uint64(iSpeed)
	service.cur.readflag = 0
	service.cur.battery = iIOStatu
	service.cur.locateType = LBS_GPS

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

	service.cur.dataTime = i64Time
	service.cur.battery = service.cur.origBattery - 2
	if service.cur.battery < 1  {
		service.cur.alarm = ALARM_BATTERYLOW
		service.cur.battery = 1
	}

	service.GetWatchDataInfo(service.imei)

	if service.wifiZoneIndex >= 0 {
		DeviceInfoListLock.RLock()
		safeZones := GetSafeZoneSettings(service.imei)
		if safeZones != nil {
			service.cur.latitude = safeZones[service.wifiZoneIndex].LatiTude
			service.cur.longtiTude = safeZones[service.wifiZoneIndex].LongTitude
			service.accracy = 150

			logging.Log(fmt.Sprintf("Get location frome home zone by home wifi: %f, %f",
				service.cur.latitude, service.cur.longtiTude))
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

	service.cur.steps = 0
	service.cur.readflag = 0
	service.cur.locateType = LBS_WIFI

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

	service.cur.battery = service.cur.origBattery - 2
	if service.cur.battery < 1 {
		service.cur.alarm = ALARM_BATTERYLOW
		service.cur.battery = 1
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

	service.cur.dataTime = i64Time
	service.cur.steps = 0
	service.cur.readflag  = 0
	service.cur.locateType = uint8(service.accracy / 10) //LBS_JIZHAN;

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

	service.cur.dataTime = i64Time
	service.cur.battery = service.cur.origBattery - 2
	if service.cur.battery < 1 {
		service.cur.alarm = ALARM_BATTERYLOW
		service.cur.battery = 1
	}

	service.GetWatchDataInfo(service.imei)

	if service.wifiZoneIndex >= 0  {
		DeviceInfoListLock.RLock()
		safeZones := GetSafeZoneSettings(service.imei)
		if safeZones != nil {
			service.cur.latitude = safeZones[service.wifiZoneIndex].LatiTude
			service.cur.longtiTude = safeZones[service.wifiZoneIndex].LongTitude
			service.accracy = 150

			logging.Log(fmt.Sprintf("Get location frome home zone by home wifi: %f, %f",
				service.cur.latitude, service.cur.longtiTude))
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

	service.cur.steps = 0
	service.cur.readflag = 0
	service.cur.locateType = LBS_WIFI

	if service.accracy >= 2000 {
		service.cur.locateType = LBS_INVALID_LOCATION
	} else if service.wiFiNum >= 3 && service.accracy <= 400 {
		service.cur.locateType = LBS_WIFI
	} else {
		service.cur.locateType = uint8(service.accracy / 10)  //LBS_JIZHAN
	}

	service.UpdateLocationIntoDBCache()
	return true
}

func (service *GT06Service) ProcessZoneAlarm() bool {
	service.GetWatchDataInfo()
	if service.old.dataTime > 0 {
		iZoneID := service.old.zoneIndex
		iAlarmType := service.old.alarm
		iZoneAlarmType := service.old.zoneAlarm

		if service.cur.battery == 1 && service.old.battery > 1 {
			service.cur.alarm = ALARM_BATTERYLOW
		}

		if iAlarmType == service.cur.alarm {
			service.cur.alarm = 0
		} else {
			iAlarmType = service.cur.alarm;
		}

		if iZoneID == 0 && iZoneAlarmType == 0 {
			iZoneID = 1
		}

		stCurPoint, stDstPoint := TPoint{}, TPoint{}
		if (service.cur.locateType != LBS_GPS && service.wifiZoneIndex < 0) ||  service.cur.alarm > 0 {
			if (service.old.locateType != LBS_JIZHAN && service.old.locateType <= 40) && service.old.locateType != LBS_SMARTLOCATION  {
				stCurPoint.Latitude = service.cur.latitude
				stCurPoint.LongtiTude = service.cur.longtiTude
				stDstPoint.Latitude = service.old.latitude
				stDstPoint.LongtiTude = service.old.longtiTude
				iDistance := service.GetDisTance(&stCurPoint, &stDstPoint)

				if iDistance <= 200 && (service.cur.locateType == LBS_JIZHAN || (service.cur.locateType > 40 && service.cur.locateType < 200)) {
					i64Time := service.cur.dataTime
					service.cur = service.old
					service.cur.dataTime = i64Time
					service.cur.locateType = LBS_SMARTLOCATION
					service.getSameWifi = true
				}

				if iDistance >= 1800 && service.cur.dataTime >= service.old.dataTime {
					iTotalMin := deltaMinutes(service.old.dataTime, service.cur.dataTime)
					if iTotalMin == 0 || (iTotalMin > 0 && iTotalMin * 800 <= iDistance) {
						i64Time := service.cur.dataTime
						service.cur = service.old
						service.cur.dataTime = i64Time
						service.cur.locateType = LBS_SMARTLOCATION
						service.getSameWifi = true
					}
				}
			}

			logging.Log(fmt.Sprintf("device %d, m_iAlarmStatu=%d, parsed location:  m_DateTime=%lld, m_lng=%d, m_lat=%d",
				service.imei,  service.cur.alarm, service.cur.dataTime, service.cur.longtiTude, service.cur.latitude)

			return true
		}

		stCurPoint.Latitude = service.cur.latitude
		stCurPoint.LongtiTude = service.cur.longtiTude

		DeviceInfoListLock.RLock()
		safeZones := GetSafeZoneSettings(service.imei)
		if safeZones != nil {
			for  i := 0; i < len(safeZones); i++ {
				stSafeZone := safeZones[i]
				if stSafeZone.Flags == 0 {
					continue
				}

				stDstPoint.Latitude = stSafeZone.LatiTude
				stDstPoint.LongtiTude = stSafeZone.LongTitude
				iRadiu := service.GetDisTance(&stCurPoint, &stDstPoint)
				if iRadiu < stSafeZone.Radiu {
					if iZoneID != stSafeZone.ZoneID || (iZoneID == stSafeZone.ZoneID && iZoneAlarmType != ALARM_INZONE) {
						if service.wifiZoneIndex < 0 && service.cur.locateType == LBS_WIFI && service.cur.locateType != LBS_WIFI {
							break
						}

						if service.hasSetHomeWifi && service.wifiZoneIndex >= 0 && strings.ToLower(stSafeZone.ZoneName) == "home" {
							continue
						}

						iZoneID = stSafeZone.m_iZoneID;
						iZoneAlarmType = ALARM_INZONE;
						iAlarmType = 0;
						m_stDataInfo.m_ucAlarmStatu = iZoneAlarmType;
						m_iAlarmStatu = iZoneAlarmType;
						strncpy(m_szZoneName, stSafeZone.m_szZoneName, strlen(stSafeZone.m_szZoneName));
						strncpy(m_stDataInfo.m_szZoneName, m_szZoneName, sizeof(m_stDataInfo.m_szZoneName) - 1);
						log_file(INFO, ERR_SUCCESS, "Device[%lld] Make a InZone Alarm[%s][%d,%d][%d,%d]", m_i64SrcDeviceID, stSafeZone.m_szZoneName,
						iRadiu, stSafeZone.m_iRadiu, GetZoneAlarmType(m_stDataInfo.m_iReserve), iZoneAlarmType, GetZoneID(m_stOldDataInfo.m_iReserve), iZoneID);
						break;
						}
				}
				else
				{
				if (iZoneID == stSafeZone.m_iZoneID && iZoneAlarmType != ALARM_OUTZONE) //�������
				{
				if (m_iWifiZoneIndex < 0 && m_stDataInfo.m_cStationType == LBS_WIFI && m_stOldDataInfo.m_cStationType != LBS_WIFI)
				{
				break;
				log_file(INFO, ERR_SUCCESS, "device %lld, m_iAlarmStatu=%d, parsed location:  m_DateTime=%lld, m_lng=%d, m_lat=%d",
				m_i64DeviceID, m_iAlarmStatu, m_stDataInfo.m_uiDataTime, m_stDataInfo.m_iLongtiTude, m_stDataInfo.m_iLatitude);
				}

				iZoneID = stSafeZone.m_iZoneID;
				iZoneAlarmType = ALARM_OUTZONE;
				m_stDataInfo.m_ucAlarmStatu = iZoneAlarmType;
				m_iAlarmStatu = iZoneAlarmType;
				iAlarmType = 0;
				strncpy(m_szZoneName, stSafeZone.m_szZoneName, strlen(stSafeZone.m_szZoneName));
				strncpy(m_stDataInfo.m_szZoneName, m_szZoneName, sizeof(m_stDataInfo.m_szZoneName) - 1);
				log_file(INFO, ERR_SUCCESS, "Device[%lld] Make a OutZone Alarm[%s][%d,%d][%d,%d]", m_i64SrcDeviceID, stSafeZone.m_szZoneName, iRadiu,
				stSafeZone.m_iRadiu, GetZoneAlarmType(m_stDataInfo.m_iReserve), iZoneAlarmType, GetZoneID(m_stOldDataInfo.m_iReserve), iZoneID);
				break;
				}
				}
				}
		}
		DeviceInfoListLock.RUnlock()

m_stDataInfo.m_iReserve = GetReverseValue(iAlarmType, iZoneID, iZoneAlarmType);
}
else
log_file(INFO, ERR_SUCCESS, "device %lld, m_iAlarmStatu=%d, parsed location:  m_DateTime=%lld, m_lng=%d, m_lat=%d",
m_i64DeviceID, m_iAlarmStatu, m_stDataInfo.m_uiDataTime, m_stDataInfo.m_iLongtiTude, m_stDataInfo.m_iLatitude);
	return true
}

func (service *GT06Service) WatchDataUpdateDB() bool {
	return true
}

func (service *GT06Service) UpdateWatchData() bool {
	return true
}

func (service *GT06Service) UpdateWatchStatus(watchStatus *WatchStatus) bool {
	return true
}

func (service *GT06Service) UpdateWatchBattery() bool {
	return true
}

func (service *GT06Service) NotifyAlarmMsg() bool {
	return true
}

func (service *GT06Service) UpdateLocationIntoDBCache() bool {
	return true
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
				if string(service.wifiInfoList[i].MacID[0:]) == safeZones[j].WifiMACID {
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