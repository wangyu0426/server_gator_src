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
	"sync"
	"os"
	"os/exec"
	"bytes"
)

type ResponseItem struct {
	rspCmdType uint16
	msg *MsgData
	//id uint64
	//imei uint64
	//data []byte
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
	AvatarUploadDir string
	MinichatUploadDir string
	DeviceMinichatBaseUrl string
	AndroidAppURL string
	IOSAppURL string
	APNSServerBaseURL string
	Pgpool *pgx.ConnPool
	MysqlPool *sql.DB
	WritebackChan chan *MsgData
	AppNotifyChan chan *AppMsgData
	Msg *MsgData
	GetDeviceDataFunc  func (imei uint64, pgpool *pgx.ConnPool)  LocationData
	SetDeviceDataFunc  func (imei uint64, updateType int, deviceData LocationData)
	GetChatDataFunc  func (imei uint64, index int)  []ChatInfo
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
	Accuracy float64 `json:"accuracy"`
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

type AlarmInfo struct{
	zoneIndex int32
	zoneName string
	alarm  uint8
}

type LocationData struct {
	Imei  uint64 		`json:"imei"`
	DataTime uint64 	`json:"datatime"`
	Lat float64			`json:"lat"`
	Lng float64			`json:"lng"`
	Steps uint64  		`json:"steps"`
	Accracy uint32		 `json:"accracy"`
	Battery uint8		`json:"battery"`
	AlarmType uint8   	`json:"alarm"`
	LocateType uint8	`json:"locateType"`
	ReadFlag uint8  	 `json:"readflag"`
	//ZoneAlarm uint8 	 `json:"zoneAlarm"`
	ZoneIndex int32 	`json:"zoneIndex"`
	ZoneName string 	`json:"zoneName"`

	OrigBattery uint8		`json:"org_battery"`
	OrigAlarm uint8       		`json:"org_alarm"`
	LastAlarmType uint8      `json:"lastAlarmType"`
	LastZoneIndex int32 	`json:"lastZoneIndex"`
}

const (
	ChatFromDeviceToApp = iota
	ChatFromAppToDevice
)
const (
	ChatContentVoice = iota
	ChatContentPhoto
	ChatContentVideo
	ChatContentText
	ChatContentHyperLink
)


type ChatInfo struct {
	Imei  uint64 		`json:"imei"`
	DateTime uint64      	`json:"datetime"`   //客户端的发送时间
	FileID uint64 		`json:"fileid"`
	VoiceMilisecs int		`json:"milisecs"`
	SenderType uint8	`json:"senderType"`
	Sender string		`json:"sender"`
	SenderUser string		`json:"senderUser"`
	ReceiverType uint8
	Receiver string
	ContentType uint8
	Content string 		`json:"content"`	//如contentType是文件类型，则Content是以时间戳id命名的文件名
	Flags  int
	CreateTime  uint64  //服务器生成时间
}


type PhotoSettingInfo struct {
	Sender string
	Member FamilyMember
	ContentType uint8
	Content string
	MsgId uint64
	Flags  int
	CreateTime  uint64  //服务器生成时间
}

type DataBlock struct {
	MsgId uint64
	Imei uint64
	Cmd string
	Time uint64
	Phone string
	BlockCount, BlockIndex, BlockSize, blockSizeField, recvSized, nextIndex  int
	Data []byte
}

type ChatTask struct {
	Info ChatInfo
	Data DataBlock
}

type PhotoSettingTask struct {
	Info PhotoSettingInfo
	Data DataBlock
}

type DeviceCache struct {
	Imei uint64
	CurrentLocation LocationData
	AlarmCache []LocationData
	//ChatCache []ChatInfo
	ResponseCache []ResponseItem
}

type GT06Service struct {
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
const DATA_BLOCK_SIZE = 700

const MAX_WIFI_NAME_LEN = 64
const MAX_MACID_LEN = 17
const MAX_WIFI_NUM = 6
const MAX_LBS_NUM = 8

const PI  = 3.1415926
const BASE_TITUDE  =1
const EARTH_RADIUS = 6378.137

var DeviceChatTaskTable = map[uint64]map[uint64]*ChatTask{}
var DeviceChatTaskTableLock = &sync.Mutex{}

var EPOTaskTable = map[uint64]*DataBlock{}
var EPOTaskTableLock = &sync.Mutex{}

var AppSendChatList = map[uint64]*[]*ChatTask{}
var AppSendChatListLock = &sync.Mutex{}

var AppChatList = map[uint64]map[uint64]ChatInfo{}
var AppChatListLock = &sync.Mutex{}

var AppNewPhotoList = map[uint64]*[]*PhotoSettingTask{}
var AppNewPhotoListLock = &sync.Mutex{}

var AppNewPhotoPendingList = map[uint64]*[]*PhotoSettingTask{}
var AppNewPhotoPendingListLock = &sync.Mutex{}

func HandleTcpRequest(reqCtx RequestContext)  bool{
	service := &GT06Service{reqCtx: reqCtx}
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

func MakeLocateNowReplyMsg(imei uint64) []byte {
	//(0019357593060153353AP00)
	return []byte(fmt.Sprintf("(0019%dAP00)", imei))

}

func MakeSosReplyMsg(imei, id uint64) []byte {
	//(002C357593060153353AP16,1,0000000000000012)
	body := fmt.Sprintf("%015dAP16,1,%016X)", imei, id)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}

func MakeVoiceMonitorReplyMsg(imei, id uint64, phone string) []byte {
	//(0035357593060153353AP0513632782450,0000000000000004)
	body := fmt.Sprintf("%015dAP05%s,%016X)", imei, phone, id)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}

func MakeReplyMsg(imei uint64, requireAck bool, data []byte, id uint64) *MsgData{
	msg := MsgData{}
	msg.Data = data
	msg.Header.Header.Imei = imei
	msg.Header.Header.ID = id
	if requireAck {
		msg.Header.Header.Status = 1
	}
	return &msg
}


func (service *GT06Service)makeReplyMsg(requireAck bool, data []byte, id uint64) *MsgData{
	return MakeReplyMsg(service.imei, requireAck, data, id)
}


func (service *GT06Service)makeAckParsedMsg(id uint64) *MsgData{
	msg := MsgData{}
	msg.Header.Header.Version = MSG_HEADER_ACK_PARSED
	msg.Header.Header.Imei = service.imei
	msg.Header.Header.Cmd = service.cmd
	msg.Header.Header.ID = id

	return &msg
}

func (service *GT06Service)PreDoRequest() bool  {
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

func (service *GT06Service)DoRequest(msg *MsgData) bool  {
	//logging.Log("Get Input Msg: " + string(msg.Data))
	logging.Log(fmt.Sprintf("imei: %d cmd: %s; go routines: %d", msg.Header.Header.Imei, StringCmd(msg.Header.Header.Cmd), runtime.NumGoroutine()))

	ret := true
	bufOffset := uint32(0)
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
		if deviceInfo != nil  && deviceInfo.RedirectServer && deviceInfo.CompanyHost != "" && deviceInfo.CompanyPort != 0{
			id := makeId()
			resp := &ResponseItem{CMD_AP01, service.makeReplyMsg(false,
				service.makeResetHostPortReplyMsg(deviceInfo.CompanyHost, deviceInfo.CompanyPort, id), id)}
			service.rspList = append(service.rspList, resp)

			DeviceInfoListLock.Unlock()
			logging.Log(fmt.Sprintf("%d reset server to host:port %s:%d", msg.Header.Header.Imei,
				deviceInfo.CompanyHost, deviceInfo.CompanyPort))
			return true
		}
	}
	DeviceInfoListLock.Unlock()

	service.GetWatchDataInfo(service.imei)

	if service.cmd == DRT_SET_IP_PORT_ACK  ||       // 同BP01，手表设置服务器IP端口的ACK
		service.cmd == DRT_SET_APN_ACK     ||        // 同BP02，手表设置APN的ACK
		service.cmd == DRT_SYNC_TIME_ACK  ||   // 同BP03，手表请求对时ACK
		service.cmd == DRT_VOICE_MONITOR_ACK  ||      // 同BP05	，手表设置监听的ACK
		service.cmd == DRT_SET_PHONE_NUMBERS_ACK  ||      // 同BP06	，手表设置亲情号的ACK
		service.cmd == DRT_CLEAR_PHONE_NUMBERS_ACK  ||      // 同BP07	，手表清空亲情号的ACK
		service.cmd == DRT_SET_REBOOT_ENABLE_ACK    ||    // 同BP08	，手表设置是否能重启的ACK
		service.cmd == DRT_SET_TIMER_ALARM_ACK   ||    // 同BP09	，手表设置是否能重启的ACK
		service.cmd == DRT_SET_MUTE_ENABLE_ACK   ||    // 同BP10	，手表设置是否静音的ACK
		service.cmd == DRT_FETCH_LOCATION__ACK     ||   // 同BP14	，手表下载定位数据的ACK
		service.cmd == DRT_SET_POWEROFF_ENABLE_ACK    ||   // 同BP15	，手表设置是否能关机的ACK
		service.cmd == DRT_ACTIVE_SOS_ACK   ||    // 同BP16	，手表设置激活sos的ACK
		service.cmd == DRT_SET_OWNER_NAME_ACK   ||     // 同BP18	，手表设置名字的ACK
		service.cmd == DRT_SET_USE_DST_ACK   ||    // 同BP19	，手表设置夏令时的ACK
		service.cmd == DRT_SET_LANG_ACK   ||    // 同BP20	，手表设置语言的ACK
		service.cmd == DRT_SET_VOLUME_ACK   ||    // 同BP21	，手表设置音量的ACK
		service.cmd == DRT_SET_AIRPLANE_MODE_ACK  ||      // 同BP22	，手表设置隐身模式的ACK
		service.cmd == DRT_QUERY_TEL_USE_ACK      || 	// 同BP24	，手表对服务器查询短信条数的ACK -- 这个命令格式需另外处理
		service.cmd == DRT_DELETE_PHONE_PHOTO_ACK  ||    	// 同BP25	，手表对删除亲情号图片的ACK
		service.cmd == DRT_FETCH_APP_URL_ACK {     	//  同BP26	，手表获取app下载页面URL的ACK


		service.isCmdForAck = true

		if service.cmd == DRT_QUERY_TEL_USE_ACK {
			msgIdForAck := Str2Num(string(msg.Data[1: 17]), 16)
			service.msgAckId = msgIdForAck
			resp := &ResponseItem{CMD_ACK,  service.makeAckParsedMsg(msgIdForAck)}
			service.rspList = append(service.rspList, resp)
		}else {
			msgIdForAck := Str2Num(string(msg.Data[0: 16]), 16)
			service.msgAckId = msgIdForAck
			lastAckOK := Str2Num(string(msg.Data[16: 17]), 10)
			if lastAckOK == 1 {
				//回复成功，通知app成功
				if service.cmd == DRT_SET_PHONE_NUMBERS_ACK{
					ResolvePendingPhotoData(service.imei, msgIdForAck)
				}
			}else{
				//回复失败，通知APP失败
			}
			resp := &ResponseItem{CMD_ACK,  service.makeAckParsedMsg(msgIdForAck)}
			service.rspList = append(service.rspList, resp)
		}
	}else if service.cmd == DRT_SYNC_TIME {  //BP00 对时
		madeData, id := service.makeSyncTimeReplyMsg()
		resp := &ResponseItem{CMD_AP03,  service.makeReplyMsg(true, madeData, id)}
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
	}else if service.cmd == DRT_SEND_LOCATION {
		//BP30 上报定位和报警等数据
		resp := &ResponseItem{CMD_AP30, service.makeReplyMsg(false,
			[]byte(fmt.Sprintf("(0019%dAP30)", service.imei)), makeId())}
		service.rspList = append(service.rspList, resp)

		bufOffset++
		cLocateTag := msg.Data[bufOffset]
		bufOffset += 2

		//报警字段
		alarmBuf := []byte{}
		for msg.Data[bufOffset] != ',' {
			alarmBuf = append(alarmBuf, msg.Data[bufOffset])
			bufOffset++
		}

		if len(alarmBuf) > 0 {
			service.cur.OrigAlarm = uint8(Str2Num(string(alarmBuf), 16))
		}

		fmt.Println(string(alarmBuf), service.cur.OrigAlarm)

		service.cur.AlarmType = service.cur.OrigAlarm
		//去掉手表上报的低电和设备脱离状态，由服务器计算是否报警
		service.cur.AlarmType &= (^uint8(ALARM_BATTERYLOW))
		service.cur.AlarmType &= (^uint8(ALARM_DEVICE_DETACHED))

		//由于手表脱离可能是一种持续状态，手表会持续上报这个状态，所以实际报警是在第一次上报脱离的时候
		if service.old.OrigAlarm & ALARM_DEVICE_DETACHED == 0 && service.cur.OrigAlarm & ALARM_DEVICE_DETACHED != 0 {
			service.cur.AlarmType |= uint8(ALARM_DEVICE_DETACHED)
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
			ret = service.ProcessLocate(msg.Data[bufOffset: ], cLocateTag)
		}else {
			bufOffset++
			service.needSendLocation = (msg.Data[bufOffset] - '0') == 1
			bufOffset += 1
			ret = service.ProcessLocate(msg.Data[bufOffset: ], cLocateTag)

			if service.needSendLocation {
				madeData, id := service.makeSendLocationReplyMsg()
				resp := &ResponseItem{CMD_AP14, service.makeReplyMsg(true, madeData, id)}
				service.rspList = append(service.rspList, resp)
			}
		}

		if ret == false {
			logging.Log("ProcessLocateInfo Error")
			return false
		}
	}else if service.cmd == DRT_DEVICE_LOGIN {
		//BP31, 设备登录服务器消息
		resp := &ResponseItem{CMD_AP31,  service.makeReplyMsg(false, service.makeDeviceLoginReplyMsg(), makeId())}
		service.rspList = append(service.rspList, resp)
	//}else if service.cmd == DRT_DEVICE_ACK {
	//	//BP04, 手表对服务器请求的应答
	//	if strings.Contains(string(msg.Data),  "AP11") {
	//		service.needSendChatNum = false
	//	}
	}else if service.cmd == DRT_SEND_MINICHAT {
		//BP34,手表发送微聊
		if len(msg.Data) <= 2 {
			logging.Log("ProcessMicChat Error for too small msg data length")
			return false
		}

		bufOffset++
		ret = service.ProcessMicChat(msg.Data[bufOffset: ])
		if ret == false {
			logging.Log("ProcessMicChat Error")
			return false
		}
	}else if service.cmd == DRT_PUSH_MINICHAT_ACK {
		//BP12,手表发送微聊确认包
		bufOffset++
		ret = service.ProcessPushMicChatAck(msg.Data[bufOffset: ])
		if ret == false {
			logging.Log("ProcessPushMicChatAck Error")
			return false
		}
	}else if service.cmd == DRT_PUSH_PHOTO_ACK {
		//BP23,手表发送头像确认包
		bufOffset++
		ret = service.ProcessPushPhotoAck(msg.Data[bufOffset: ])
		if ret == false {
			logging.Log("ProcessPushPhotoAck Error")
			return false
		}
	}else if service.cmd == DRT_FETCH_FILE {
		//BP11, 手表获取未读微聊或亲情号头像设置数目
		logging.Log("ProcessRspFetchFile 1")
		bufOffset++
		ret = service.ProcessRspFetchFile(msg.Data[bufOffset: ])
		if ret == false {
			logging.Log("ProcessRspFetchFile Error")
			return false
		}
	}else if service.cmd == DRT_FETCH_AGPS { //DRT_FETCH_AGPS 和 DRT_FETCH_APP_URL 都通过BP32命令
		//BP32, 手表请求EPO数据或APP URL
		bufOffset++
		service.fetchType = string(msg.Data[bufOffset: bufOffset + 4])
		if service.fetchType == "AP13" {
			ret = service.ProcessRspAGPSInfo()
			if ret == false {
				logging.Log("ProcessRspAGPSInfo Error")
				return  false
			}
		}else if service.fetchType == "AP26" {
			madeData, id := service.makeAppURLReplyMsg()
			resp := &ResponseItem{CMD_AP26,  service.makeReplyMsg(true, madeData, id)}
			service.rspList = append(service.rspList, resp)
		}

	}else if service.cmd == DRT_EPO_ACK {
		//BP13, 手表回复EPO数据确认包
		bufOffset++
		ret = service.ProcessRspAGPSAck(msg.Data[bufOffset: ])
		if ret == false {
			logging.Log("ProcessRspAGPSAck Error")
			return  false
		}
	}else if service.cmd == DRT_HEART_BEAT {
		// heart beat
		bufOffset++

		//报警字段
		alarmBuf := []byte{}
		for msg.Data[bufOffset] != ',' {
			alarmBuf = append(alarmBuf, msg.Data[bufOffset])
			bufOffset++
		}

		if len(alarmBuf) > 0 {
			service.cur.OrigAlarm = uint8(Str2Num(string(alarmBuf), 16))
		}

		fmt.Println(string(alarmBuf), service.cur.OrigAlarm)

		service.cur.AlarmType = service.cur.OrigAlarm
		//去掉手表上报的低电和设备脱离状态，由服务器计算是否报警
		service.cur.AlarmType &= (^uint8(ALARM_BATTERYLOW))
		service.cur.AlarmType &= (^uint8(ALARM_DEVICE_DETACHED))

		//由于手表脱离可能是一种持续状态，手表会持续上报这个状态，所以实际报警是在第一次上报脱离的时候
		if service.old.OrigAlarm & ALARM_DEVICE_DETACHED == 0 && service.cur.OrigAlarm & ALARM_DEVICE_DETACHED != 0 {
			service.cur.AlarmType |= uint8(ALARM_DEVICE_DETACHED)
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

		ret = service.ProcessUpdateWatchStatus(msg.Data[bufOffset: ])
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

func (service *GT06Service)CountSteps() {
	//170520114920
	//if service.old.DataTime == 0 || (service.cur.DataTime / 1000000 ==  service.old.DataTime / 1000000) {
	//	service.cur.Steps = service.old.Steps
	//}
}

func (service *GT06Service)DoResponse() []*MsgData  {
	if service.needSendChat {
		logging.Log(fmt.Sprint("ProcessRspFetchFile 4, ", service.needSendChat))
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

	if service.needSendPhoto {
		logging.Log(fmt.Sprint("ProcessRspFetchFile 5, ", service.needSendPhoto,
			service.needSendPhotoNum, service.reqPhotoInfoNum))
		service.ProcessRspPhoto()
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

	if service.needSendPhotoNum {
		service.PushNewPhotoNum()
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


func MakeTimeZoneReplyMsg(imei , id uint64, deviceTimeZone string) []byte {
	//(002D357593060153353AP03,150728,152900,e0800)
	curTime := time.Now().UTC().Format("060102,150405")

	body := fmt.Sprintf("%015dAP03,%s,%s,%016X)", imei, curTime, deviceTimeZone, id)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}

func deviceTimeZoneString(tz string) string {
	tz = strings.Replace(tz, ":", "", 1)
	if tz[0] == '-' {
		tz = strings.Replace(tz, "-", "w", 1)
	}else if tz[0] == '+'{
		tz = strings.Replace(tz, "+", "e", 1)
	}else{
		tz = "e" + tz
	}
	return tz
}


func DeviceTimeZoneInt(tz string) int {
	tz = strings.Replace(tz, ":", "", 1)
	negative := 1
	if tz[0] == '-' {
		negative = -1
		tz = strings.Replace(tz, "-", "", 1)
	}else if tz[0] == '+'{
		tz = strings.Replace(tz, "+", "", 1)
	}else{
		tz = "0"
	}
	return int(Str2Num(tz, 10)) * negative
}


func makeDeviceFamilyPhoneNumbers(family *[MAX_FAMILY_MEMBER_NUM]FamilyMember) string {
	phoneNumbers := ""
	for i := 0; i < len(family); i++ {
		phoneNumbers += fmt.Sprintf("#%s#%s#%d",  family[i].Phone, family[i].Name, family[i].Type)
	}

	return phoneNumbers
}

func makeHideTimerReplyMsgString(imei, id uint64) string {
	//(006A357593060153353AP22,1,1,0900,1000,127,1,1032,1100,127,1,1400,1500,62,1,1530,1600,62,0000000000000017)
	body := fmt.Sprintf("%015dAP22,1,", imei)
	tail := fmt.Sprintf("%016X)", id)

	DeviceInfoListLock.Lock()
	device, ok := (*DeviceInfoList)[imei]
	if ok && device != nil {
		for _, timer := range device.HideTimerList {
			if len(timer.Begin) > 0{
				body += fmt.Sprintf("%d,%s,%s,%d,",  timer.Enabled, timer.Begin[0:4], timer.End[0:4], timer.Days)
			}else{
				body += fmt.Sprintf("%d,%s,%s,%d,",  0, "0", "0", 0)
			}
		}
	}
	DeviceInfoListLock.Unlock()

	return (fmt.Sprintf("(%04X", 5 + len(body) + len(tail)) + body + tail)
}

func MakeSetDeviceConfigReplyMsg(imei  uint64, params *DeviceSettingParams)  []*MsgData  {
	if len(params.Settings) > 0 {
		msgList := []*MsgData{}
		for _, setting := range params.Settings {
			fmt.Println("MakeSetDeviceConfigReplyMsg:", setting)
			msg := MsgData{}
			msg.Header.Header.Imei = imei
			msg.Header.Header.ID = NewMsgID()
			msg.Header.Header.Status = 1

			switch setting.FieldName {
			case OwnerNameFieldName:
				body := fmt.Sprintf("%015dAP18,%s,%016X)", imei, setting.NewValue, msg.Header.Header.ID)
				msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
			case TimeZoneFieldName:
				msg.Data = MakeTimeZoneReplyMsg(imei, msg.Header.Header.ID,
					deviceTimeZoneString(setting.NewValue))
			case VolumeFieldName:
				body := fmt.Sprintf("%015dAP21,%s,%016X)", imei, setting.NewValue, msg.Header.Header.ID)
				msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
			case LangFieldName:
				body := fmt.Sprintf("%015dAP20,%04d,%016X)", imei, Str2Num(setting.NewValue, 10), msg.Header.Header.ID)
				msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
			case UseDSTFieldName:
				body := fmt.Sprintf("%015dAP19,%s,%016X)", imei, setting.NewValue, msg.Header.Header.ID)
				msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
			case ChildPowerOffFieldName:
				body := fmt.Sprintf("%015dAP15,%s,%016X)", imei, setting.NewValue, msg.Header.Header.ID)
				msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
			case PhoneNumbersFieldName:
				msgId := params.MsgId
				if params.MsgId == 0 {
					msgId = msg.Header.Header.ID
				}else{
					msg.Header.Header.ID = params.MsgId
				}

				DeviceInfoListLock.Lock()
				deviceInfo, ok := (*DeviceInfoList)[imei]
				if ok && deviceInfo != nil {
					body := fmt.Sprintf("%015dAP06%s,%016X)", imei,
						makeDeviceFamilyPhoneNumbers(&deviceInfo.Family),   msgId)
					msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
				}
				DeviceInfoListLock.Unlock()

				if ok == false {
					return nil
				}
			case WatchAlarmFieldName + "0":
				fallthrough
			case WatchAlarmFieldName + "1":
				fallthrough
			case WatchAlarmFieldName + "2":
				fallthrough
			case WatchAlarmFieldName + "3":
				fallthrough
			case WatchAlarmFieldName + "4":
				// (0040357593060153353AP09,1,0,127,150728,152900,0000000000000008)
				body := ""
				if setting.NewValue == "delete" || setting.NewValue == "null" {
					curWatchAlarm := WatchAlarm{}
					err := json.Unmarshal([]byte(setting.CurValue), &curWatchAlarm)
					if err != nil {
						logging.Log(fmt.Sprintf("[%d] bad data of current watch alarm to delete, %s", imei, setting.CurValue))
						return  nil
					}else{
						body = fmt.Sprintf("%015dAP09,%d,%d,%d,%s,%s,%016X)", imei, 0, setting.Index, curWatchAlarm.Days,
							curWatchAlarm.Date, curWatchAlarm.Time, msg.Header.Header.ID)
					}
				}else{
					newWatchAlarm := WatchAlarm{}
					err := json.Unmarshal([]byte(setting.NewValue), &newWatchAlarm)
					if err != nil {
						logging.Log(fmt.Sprintf("[%d] bad data of new watch alarm to save, %s", imei, setting.NewValue))
						return  nil
					}else{
						body = fmt.Sprintf("%015dAP09,%d,%d,%d,%s,%s,%016X)", imei, 1, setting.Index, newWatchAlarm.Days,
							newWatchAlarm.Date, newWatchAlarm.Time, msg.Header.Header.ID)
					}
				}

				msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
				logging.Log(fmt.Sprintf("send watch alarm to [%d]:  %s", imei, string(msg.Data)))

			case HideSelfFieldName:
				if setting.NewValue == "0" {
					body := fmt.Sprintf("%015dAP22,0,%016X)",  imei, msg.Header.Header.ID)
					msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
				}else {
					msg.Data = []byte(makeHideTimerReplyMsgString(imei, msg.Header.Header.ID))
				}

				logging.Log(fmt.Sprintf("send hide self settings to [%d]:  %s", imei, string(msg.Data)))
			case HideTimer0FieldName:
				fallthrough
			case HideTimer1FieldName:
				fallthrough
			case HideTimer2FieldName:
				fallthrough
			case HideTimer3FieldName:
				msg.Data = []byte(makeHideTimerReplyMsgString(imei, msg.Header.Header.ID))
				logging.Log(fmt.Sprintf("send hide timer to [%d]:  %s", imei, string(msg.Data)))

			default:
				//if strings.Contains(setting.FieldName,  FenceFieldName) {
				//	//(0035357593060571398AP27,3C:46:D8:27:2E:63,48:3C:0C:F5:56:48,0000000000000021)
				//	body := fmt.Sprintf("%015dAP27,%s,%s,%016X)", imei,
				//		info.SafeZoneList[0].Wifi.BSSID, info.SafeZoneList[1].Wifi.BSSID,
				//		msg.Header.Header.ID)
				//	msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
				//
				//	//if setting.Index == 1 || setting.Index == 2 {
				//	//	DeviceInfoListLock.Lock()
				//	//	info, ok := (*DeviceInfoList)[imei]
				//	//	if ok && info != nil {
				//	//		newFence := SafeZone{}
				//	//		err := json.Unmarshal([]byte(setting.CurValue), &newFence)
				//	//		if err == nil {
				//	//			if info.SafeZoneList[setting.Index - 1].Wifi.BSSID != newFence.Wifi.BSSID {
				//	//				//(0035357593060571398AP27,3C:46:D8:27:2E:63,48:3C:0C:F5:56:48,0000000000000021)
				//	//				body := fmt.Sprintf("%015dAP27,%s,%s,%016X)", imei,
				//	//					info.SafeZoneList[0].Wifi.BSSID, info.SafeZoneList[1].Wifi.BSSID,
				//	//					msg.Header.Header.ID)
				//	//				msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
				//	//			}
				//	//		} else {
				//	//			logging.Log(fmt.Sprintf("%d new fence bad json data %s", imei, setting.NewValue))
				//	//		}
				//	//	}
				//	//	DeviceInfoListLock.Unlock()
				//	//}
				//}
			}

			msgList = append(msgList, &msg)
		}

		return msgList
	}else{
		return nil
	}

	//logging.Log("MakeSetDeviceConfigReplyMsg: " + data)
	//return []byte(data)
}

func (service *GT06Service)makeSyncTimeReplyMsg() ([]byte, uint64) {
	//(002D357593060153353AP03,150728,152900,e0800)
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
		service.UpdateDeviceTimeZone(service.imei, timezone)
	}

	if timezone < 0 {
		c = 'w'
		timezone = -timezone
	}

	id := makeId()
	body := fmt.Sprintf("%015dAP03,%s,%c%04d,%016X)", service.imei,curTime, c, timezone, id)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body) , id // + string(service.makeSendLocationReplyMsg()))
}

func (service *GT06Service)makeSendLocationReplyMsg() ([]byte, uint64) {
	//(0056357593060153353AP1424.772816,121.022636,160,2015,11,12,08,00,00,0000000000000009)
	//(0051357593060571398AP140.000000,0.000000,0,2017,05,22,11,04,28,00000D99DE4C0826)
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

	id := makeId()
	curTime := time.Now().UTC().Format("2006,01,02,15,04,05")
	body := fmt.Sprintf("%015dAP14%06f,%06f,%d,%s,%016X)",
		service.imei, lat, lng, accracy, curTime, id)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body) , id
}

func (service *GT06Service)makeDeviceLoginReplyMsg() []byte {
	//(0019357593060153353AP31)
	body := fmt.Sprintf("%015dAP31)", service.imei)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}

func (service *GT06Service)makeResetHostPortReplyMsg(host string, port int, id uint64) ([]byte) {
	//(003B357593060571398AP01221.18.79.110#123,0000000000000001)
	body := fmt.Sprintf("%015dAP01%s#%d,%016X)", service.imei, host, port, id)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}

func (service *GT06Service)makeAppURLReplyMsg() ([]byte, uint64) {
	id := makeId()
	body := fmt.Sprintf("%015dAP26,%d,%s,%d,%s,%016X)", service.imei,
		len(service.reqCtx.IOSAppURL), service.reqCtx.IOSAppURL,
		len(service.reqCtx.AndroidAppURL), service.reqCtx.AndroidAppURL, id)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body), id
}

func (service *GT06Service)makeDeviceChatAckMsg(phone, datatime, milisecs,
		blockCount, blockIndex string,  success int) []byte {
	//(003D357593060153353AP34,13026618172,170413163300,2710,6,1,1)
	body := fmt.Sprintf("%015dAP34,%s,%s,%s,%s,%s,%d)", service.imei, phone, datatime, milisecs,
		blockCount, blockIndex, success)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}


func (service *GT06Service)makeChatDataReplyMsg(voiceFile, phone string, datatime uint64,
	index int) ([]byte, []*ResponseItem) {
	//(03B5357593060153353AP12,1,170413163300,6,1,900,数据)

	//读取语音文件，并分包
	voice, err := ioutil.ReadFile(voiceFile)
	if err != nil {
		logging.Log("read voice file failed, " + voiceFile + "," + err.Error())
		return nil, nil
	}

	if len(voice) == 0 {
		logging.Log("voice file empty, 0 bytes, ")
		return nil, nil
	}

	return (voice), service.shardData("AP12", phone, datatime, voice, index)
}

func (service *GT06Service)shardData(cmd, phone string, datatime uint64, data []byte, index int) []*ResponseItem {
	resps := []*ResponseItem{}
	bufOffset, iReadLen := 0, len(data)
	blockSize := DATA_BLOCK_SIZE
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
		}else if cmd == "AP23" {
			intCmd = CMD_AP23
		}

		if index == -1 ||  index == packIndex {
			resp := &ResponseItem{uint16(intCmd),  service.makeReplyMsg(false,
				service.makeDataBlockReplyMsg(block), block.MsgId)}
			resps = append(resps, resp)
		}

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

func (service *GT06Service)makeFileNumReplyMsg(fileType, chatNum int) []byte {
	return MakeFileNumReplyMsg(service.imei, fileType, chatNum)
}


func MakeFileNumReplyMsg(imei uint64, fileType, chatNum int) []byte {
	//(001B357593060153353AP1102)
	cmd := ""
	switch fileType {
	case ChatContentVoice:
		cmd = "AP12"
	case ChatContentPhoto:
		cmd = "AP23"

	}

	body := fmt.Sprintf("%015dAP11,%s,%02d)", imei, cmd, chatNum)
	size := fmt.Sprintf("(%04X", 5 + len(body))

	return []byte(size + body)
}

func makeId()  (uint64) {
	//id := time.Now().UnixNano() / int64(time.Millisecond ) * 10
	////fmt.Println("make id: ", uint64(id))
	//return uint64(id)
	return NewMsgID()
}

func MakeTimestampIdString()  string {
	now := time.Now()
	id := fmt.Sprintf("%s%d", now.Format("20060102150405"), now.Nanosecond())
	return id[2:18]
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

	isDisableLBS :=  IsCompanyDisableLBS(service.imei)

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
		if false && isDisableLBS == false {
			ret = service.ProcessLBSInfo(pszMsg)
		}else{
			service.needSendLocation = false
			logging.Log(fmt.Sprintf("device %d, LBS location and need disable lbs", service.imei))
			return false
		}
	} else if cLocateTag == 'M' || cLocateTag == 'm'  {
	 	ret = service.ProcessMutilLocateInfo(pszMsg)
		if service.cur.LocateType == LBS_JIZHAN && isDisableLBS {
			service.needSendLocation = false
			logging.Log(fmt.Sprintf("device %d, LBS location and need disable lbs", service.imei))
			return false
		}
	} else {
		logging.Log(fmt.Sprintf("%d - Error Locate Info Tag(%c)", service.imei, cLocateTag))
		return false
	}

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

	service.CountSteps()

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

func (service *GT06Service)  NotifyAppWithNewLocation() bool  {
	result := HeartbeatResult{Timestamp: time.Now().Format("20060102150405")}
	result.Locations = append(result.Locations, service.cur)

	service.reqCtx.AppNotifyChan  <- &AppMsgData{Cmd: HearbeatAckCmdName,
		Imei: service.imei,
		Data: MakeStructToJson(result), ConnID: 0}
	return true
}

func (service *GT06Service)  NotifyAppWithNewMinichat(chat ChatInfo) bool  {
	return NotifyAppWithNewMinichat(service.reqCtx.APNSServerBaseURL, service.imei, service.reqCtx.AppNotifyChan, chat)
}


func NotifyAppWithNewMinichat(apiBaseURL string, imei uint64, appNotifyChan chan *AppMsgData,  chat ChatInfo) bool  {
	result := HeartbeatResult{Timestamp: time.Now().Format("20060102150405")}

	fmt.Println("heartbeat-ack: ", MakeStructToJson(result))

	result.Minichat = append(result.Minichat, GetChatListForApp(imei, "")...)

	appNotifyChan  <- &AppMsgData{Cmd: HearbeatAckCmdName,
		Imei: imei,
		Data: MakeStructToJson(result), ConnID: 0}

	if apiBaseURL != "" {
		DeviceInfoListLock.Lock()
		deviceInfo, ok := (*DeviceInfoList)[imei]
		if ok && deviceInfo != nil {
			ownerName := deviceInfo.OwnerName
			if ownerName == "" {
				ownerName = Num2Str(imei, 10)
			}

			PushNotificationToApp(apiBaseURL, imei, "",  ownerName, chat.DateTime, ALARM_NEW_MINICHAT, "")
		}

		DeviceInfoListLock.Unlock()
	}

	return true
}

//
//func (service *GT06Service) ProcessMicChat(pszMsg []byte) bool {
//	//需要断点续传的支持
//	if len(pszMsg) == 0 {
//		return false
//	}
//
//	//(03BA357593060153353BP34,
//	// 123456789,170413163300,2710,6,1,900,数据)
//	fields := strings.SplitN(string(pszMsg), ",", 7)
//	if len(fields) != 7 || len(fields[6]) == 0 {
//		logging.Log("mini chat data bad length")
//		return false
//	}
//
//	ret := true
//	timestamp :=  Str2Num(fields[1], 10)
//	milisecs := int(Str2Num(fields[2], 16))
//	fileId := uint64(timestamp * 10000) + uint64(milisecs)
//	blockIndex := int(Str2Num(fields[4], 10))
//	blockSize := Str2Num(fields[5], 10)
//	DeviceChatTaskTableLock.Lock()
//	chatTasks, found :=DeviceChatTaskTable[service.imei]
//	if !found {
//		if blockIndex != 0 {
//			ret = false
//		}else{
//			chat := &ChatTask{}
//			chat.Info.DateTime = timestamp
//			chat.Info.Receiver = fields[0]
//			chat.Info.VoiceMilisecs = milisecs
//			chat.Data.Imei = service.imei
//			chat.Data.Cmd = StringCmd(DRT_SEND_MINICHAT)
//			chat.Data.blockSizeField = int(blockSize)
//			if uint64(len(fields[6]) -  1) == blockSize || uint64(len(fields[6])) == blockSize { //完整
//				chat.Data.BlockSize = int(blockSize)
//				chat.Data.nextIndex = blockIndex + 1
//			}else { //包不完整
//				chat.Data.BlockSize = len(fields[6])
//				chat.Data.nextIndex = blockIndex
//				ret = false
//			}
//			chat.Data.BlockCount = int(Str2Num(fields[3], 10))
//			chat.Data.BlockIndex = blockIndex
//			chat.Data.Phone = fields[0]
//			chat.Data.Time = timestamp
//			chat.Data.Data = make([]byte, chat.Data.BlockCount * DATA_BLOCK_SIZE)
//			copy(chat.Data.Data[0: ], fields[6][0:chat.Data.BlockSize])
//			chat.Data.recvSized = chat.Data.BlockSize
//
//			chatTasks = map[uint64]*ChatTask{}
//			chatTasks[fileId] = chat
//			DeviceChatTaskTable[service.imei] = chatTasks
//		}
//	}else{
//		chat, _ := chatTasks[fileId]
//		if blockIndex == chat.Data.nextIndex {
//			if chat.Data.nextIndex == chat.Data.BlockIndex { //包不完整，继续接收上一次包的数据
//				if chat.Data.BlockSize + int(blockSize) > chat.Data.blockSizeField {
//					ret = false
//				}else if chat.Data.BlockSize + int(blockSize) == chat.Data.blockSizeField {//刚好完整
//				}else {//继续不完整。。。
//					ret = false
//				}
//			}else {//新包
//
//			}
//		}else {//包index不匹配
//			ret = false
//		}
//	}
//
//	DeviceChatTaskTableLock.Unlock()
//
//	//// for test
//	//{
//	//	data := make([]byte, 200*1024)
//	//	offset := 0
//	//	for  _, respCmd := range service.rspList {
//	//		if respCmd.rspCmdType == CMD_AP12 {
//	//			fields := strings.SplitN(string(respCmd.data), ",", 7)
//	//			size := int(Str2Num(fields[5], 10))
//	//			copy(data[offset: offset + size], fields[6][0: size])
//	//			offset += size
//	//		}
//	//	}
//	//
//	//	//收到所有数据以后，写入语音文件
//	//	ioutil.WriteFile("/home/work/Documents/test2.txt", data[0: offset], 0666)
//	//}
//
//	return ret
//}


func (service *GT06Service) ProcessMicChat(pszMsg []byte) bool {
	//需要断点续传的支持
	if len(pszMsg) == 0 {
		return false
	}

	//(03BA357593060153353BP34,
	// 123456789,170413163300,2710,6,1,700,数据)
	fields := strings.SplitN(string(pszMsg), ",", 7)
	if len(fields) != 7{ //字段个数不对
		if len(fields) >= 5 { //已经收到了时间和包序号，可以作出回复了
			resp := &ResponseItem{CMD_AP34,  service.makeReplyMsg(false, service.makeDeviceChatAckMsg(
				fields[0], fields[1], fields[2], fields[3],fields[4], 0), makeId())}
			service.rspList = append(service.rspList, resp)
		}
		logging.Log("mini chat data bad length")
		return false
	}

	ret := true
	fileFinished := false
	var fileData []byte
	timestamp :=  Str2Num(fields[1], 10)
	milisecs := int(Str2Num(fields[2], 16))
	fileId := uint64(timestamp * 10000) + uint64(milisecs)
	blockCount := int(Str2Num(fields[3], 10))
	blockIndex := int(Str2Num(fields[4], 10))
	blockSize := Str2Num(fields[5], 10)

	//字段个数正确的时候，错误的情况处理：
	if len(fields[6]) !=  int(blockSize + 1) { //数据接收不完整,+1表示包含包结尾的右括号')'
		resp := &ResponseItem{CMD_AP34,  service.makeReplyMsg(false, service.makeDeviceChatAckMsg(
			fields[0], fields[1], fields[2], fields[3],fields[4], 0), makeId())}
		service.rspList = append(service.rspList, resp)

		logging.Log("mini chat data bad length")
		return false
	}

	//以下是数据完整的处理,
	//首先查找当前任务是否已经存在
	newChatInfo := ChatInfo{}
	isFoundTable, isFoundTask := false, false
	DeviceChatTaskTableLock.Lock()
	chat := &ChatTask{}
	chatTasks, isFoundTable := DeviceChatTaskTable[service.imei]
	if isFoundTable { //首先查找是否存在该IMEI对应的任务字典，
		// 如存在则再查找该微聊对应的任务字典是否存在，不存在，则创建
		chat, isFoundTask = chatTasks[fileId]
		if isFoundTask == false {
			chat = &ChatTask{}
			chatTasks[fileId] = chat
		}
	}else{//IMEI对应的任务字典不存在，则创建，同时创建该微聊对应的任务字典
		chatTasks = map[uint64]*ChatTask{}
		chatTasks[fileId] = chat
		DeviceChatTaskTable[service.imei] = chatTasks
	}

	//如果任务表不存在，并且当前包序号不是第一包，则出错
	if isFoundTask == false && blockIndex != 1{
		DeviceChatTaskTableLock.Unlock()
		resp := &ResponseItem{CMD_AP34,  service.makeReplyMsg(false, service.makeDeviceChatAckMsg(
			fields[0], fields[1], fields[2], fields[3], "1", 0), makeId())}
		service.rspList = append(service.rspList, resp)
		return false
	}

	//如任务表存在，但当前包序号不是任务需要接收的包，则出错
	if isFoundTask == true && (blockIndex != chat.Data.nextIndex){
		DeviceChatTaskTableLock.Unlock()
		resp := &ResponseItem{CMD_AP34,  service.makeReplyMsg(false, service.makeDeviceChatAckMsg(
			fields[0], fields[1], fields[2], fields[3], Num2Str(uint64(chat.Data.BlockIndex), 10), 1), makeId())}
		service.rspList = append(service.rspList, resp)
		return false
	}

	if isFoundTask == false {
		chat := &ChatTask{}
		chat.Info.CreateTime = NewMsgID()
		chat.Info.Imei = service.imei
		chat.Info.Sender = Num2Str(service.imei, 10)
		chat.Info.SenderType = 0
		chat.Info.FileID = fileId
		chat.Info.Content = Num2Str(fileId, 10)
		chat.Info.DateTime = timestamp
		chat.Info.Receiver = fields[0]
		chat.Info.VoiceMilisecs = milisecs
		chat.Data.Imei = service.imei
		chat.Data.Cmd = StringCmd(DRT_SEND_MINICHAT)
		chat.Data.blockSizeField = int(blockSize)
		chat.Data.BlockSize = int(blockSize)
		chat.Data.nextIndex = blockIndex + 1

		chat.Data.BlockCount = blockCount
		chat.Data.BlockIndex = blockIndex
		chat.Data.Phone = fields[0]
		chat.Data.Time = timestamp
		chat.Data.Data = make([]byte, chat.Data.BlockCount * DATA_BLOCK_SIZE)
		copy(chat.Data.Data[0: ], fields[6][0:chat.Data.BlockSize])
		chat.Data.recvSized = chat.Data.BlockSize

		chatTasks = map[uint64]*ChatTask{}
		chatTasks[fileId] = chat
		DeviceChatTaskTable[service.imei] = chatTasks
	}else{
		if blockIndex == chat.Data.nextIndex && blockIndex <= chat.Data.BlockCount {
			chat.Data.blockSizeField = int(blockSize)
			chat.Data.BlockSize = int(blockSize)
			chat.Data.nextIndex = blockIndex + 1
			chat.Data.BlockIndex = blockIndex

			copy(chat.Data.Data[chat.Data.recvSized: ], fields[6][0:chat.Data.BlockSize])
			chat.Data.recvSized += chat.Data.BlockSize

			if blockIndex == chat.Data.BlockCount {
				fileFinished = true
				fileData = chat.Data.Data[0: chat.Data.recvSized]
				newChatInfo = chat.Info
				delete(DeviceChatTaskTable[service.imei], fileId)
			}
		}
	}

	DeviceChatTaskTableLock.Unlock()

	resp := &ResponseItem{CMD_AP34,  service.makeReplyMsg(false, service.makeDeviceChatAckMsg(
		fields[0], fields[1], fields[2], fields[3], fields[4], 1), makeId())}
	service.rspList = append(service.rspList, resp)


	if fileFinished {
		filePathDir := fmt.Sprintf("%s%d", service.reqCtx.MinichatUploadDir + "watch/", service.imei)
		os.MkdirAll(filePathDir, 0755)
		filePath := fmt.Sprintf("%s/%d.amr", filePathDir, fileId)
		ioutil.WriteFile(filePath, fileData[0: ], 0666)

		args := fmt.Sprintf("-i %s -acodec libfaac -ab 64k -ar 44100 %s/%d.aac", filePath, filePathDir, fileId)
		err2, _ := ExecCmd("ffmpeg",  strings.Split(args, " ")...)
		if err2 != nil {
			logging.Log(fmt.Sprintf("[%d] ffmpeg %s failed, %s", service.imei, args, err2.Error()))
		}

		//DeviceChatTaskTableLock.Lock()
		//chatTasks, _ :=DeviceChatTaskTable[service.imei]
		//delete(chatTasks, fileId)
		//DeviceChatTaskTableLock.Unlock()

		//通知APP有新的微聊信息。。。
		newChatInfo.Content = fmt.Sprintf("%swatch/%d/%d.aac", service.reqCtx.DeviceMinichatBaseUrl,
			service.imei, fileId)
		AddChatForApp(newChatInfo)
		service.NotifyAppWithNewMinichat(newChatInfo)
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

	return ret
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

func (service *GT06Service) ProcessRspFetchFile(pszMsg []byte) bool {
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

func (service *GT06Service) ProcessRspAGPSInfo() bool {
	utcNow := time.Now().UTC()
	iCurrentGPSHour := utc_to_gps_hour(utcNow.Year(), int(utcNow.Month()),utcNow.Day(), utcNow.Hour())
	segment := (uint32(iCurrentGPSHour) - StartGPSHour) / 6
	if service.reqCtx.IsDebug == false && ( (segment < 0) || (segment >= MTKEPO_SEGMENT_NUM)) {
		logging.Log(fmt.Sprintf("[%d] EPO segment invalid, %d", service.imei, segment))
		//return false
	}

	iReadLen := MTKEPO_DATA_ONETIME * MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER
	data := make([]byte, iReadLen)
	offset := 0
	EpoInfoListLock.Lock()
	for i := 0; i < MTKEPO_DATA_ONETIME; i++ {
		copy(data[offset: offset + MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER], EPOInfoList[i].EPOBuf)
		offset += MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER
	}
	EpoInfoListLock.Unlock()

	//接收到epo请求，首先清空之前的任务表，
	// 然后重建任务表，并发送第一包
	isFoundTask := false
	//epoData := &ChatTask{}
	EPOTaskTableLock.Lock()
	epoTask, isFoundTask := EPOTaskTable[service.imei]
	if isFoundTask == false { //首先查找是否存在该IMEI对应的任务，
		// 不存在，则创建
		epoTask = &DataBlock{}
		EPOTaskTable[service.imei] = epoTask
	}

	//如存在，则直接覆盖之前的任务状态和数据
	epoTask.Imei = service.imei
	epoTask.Cmd = StringCmd(DRT_FETCH_AGPS)
	epoTask.BlockIndex = 1

	epoTask.BlockCount = GetBlockCount(len(data))
	epoTask.BlockSize = GetBlockSize(len(data), 1)

	epoTask.Data = data
	epoTask.recvSized = 0

	EPOTaskTableLock.Unlock()

	//service.rspList = append(service.rspList, ...)
	service.rspList = append(service.rspList, service.shardData("AP13", "", 0, data, 1)[0])

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


func (service *GT06Service) ProcessRspAGPSAck(pszMsg []byte) bool {
	isFoundTask := false
	//epoData := &ChatTask{}
	EPOTaskTableLock.Lock()
	epoTask, isFoundTask := EPOTaskTable[service.imei]
	if isFoundTask == false { //首先查找是否存在该IMEI对应的任务，
		// 不存在，则出错，此时任务应该仍然存在，并且尚未完成
		EPOTaskTableLock.Unlock()
		return false
	}

	//如存在，则继续之前的任务状态和数据发送
	//首先解析出包序号和确认状态
	fields := strings.SplitN(string(pszMsg), ",", 3)
	if len(fields) != 3 {
		//字段个数不对
		EPOTaskTableLock.Unlock()
		return false
	}

	if len(fields[2]) != 2 || (fields[2][0] != '0' && fields[2][0] != '1') {
		EPOTaskTableLock.Unlock()
		return false
	}

	blockCount := Str2Num(fields[0], 10)
	blockIndex := Str2Num(fields[1], 10)
	lastBlockOk := (fields[2][0] - '0')
	if blockCount <= 0 || blockIndex <= 0 || blockCount < blockIndex || epoTask.BlockIndex != int(blockIndex){
		EPOTaskTableLock.Unlock()
		logging.Log(fmt.Sprintf("[%d] block count or index is not invalid, %d, %d for %d, %d",
			service.imei,  blockCount, blockIndex, epoTask.BlockCount, epoTask.BlockIndex))

		return false
	}

	if lastBlockOk != 1 {
		EPOTaskTableLock.Unlock()
		logging.Log(fmt.Sprintf("[%d] last block ask is not ok ", service.imei, lastBlockOk))
		return false
	}

	//service.rspList = append(service.rspList, ...)
	if int(blockIndex) == epoTask.BlockCount {
		delete(EPOTaskTable, service.imei)
	}else{
		service.rspList = append(service.rspList, service.shardData("AP13", "", 0, epoTask.Data, int(blockIndex + 1))[0])
		epoTask.BlockIndex = int(blockIndex + 1)
	}

	EPOTaskTableLock.Unlock()

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

func ProcessPushMicChat(voiceFile string, imei uint64, phone string) bool {
	//这里只推送未接收的微聊的

	return true
}

func (service *GT06Service) ProcessPushMicChatAck(pszMsg []byte) bool {
	//(03BF357593060153353AP12,13026618172,170413163300,6,1,700,数据)
	//(包长度 IMEI AP12,手机号,时间戳id,包总数,包序号,数据长度,数据)
	//(003C0103357593060153353BP12,
	// 13026618172,1704131633251234,6,1,0)
	//首先解析出包状态信息，如时间戳id，序号和确认状态
	fields := strings.SplitN(string(pszMsg), ",", 5)
	if len(fields) != 5 {
		//字段个数不对
		logging.Log(fmt.Sprintf("[%d] cmd field count %d is bad", service.imei, len(fields)))
		return false
	}

	if len(fields[4]) == 0 || len(fields[4]) != 2 || (fields[4][0] != '0' && fields[4][0] != '1') {
		logging.Log(fmt.Sprintf("[%d] the ack status %s is bad", service.imei, fields[4]))
		return false
	}

	timeId := Str2Num(fields[1], 10)
	blockCount := int(Str2Num(fields[2], 10))
	blockIndex := int(Str2Num(fields[3], 10))
	lastBlockOk := (fields[4][0] - '0')
	if blockCount <= 0 || blockIndex <= 0 || blockCount < blockIndex {
		logging.Log(fmt.Sprintf("[%d] block count,  index  %d is bad", service.imei, blockCount, blockIndex))
		return false
	}

	if lastBlockOk != 1 {
		logging.Log(fmt.Sprintf("[%d] last block failed,  status is  %d, %s", service.imei, lastBlockOk,  fields[4]))
		return false
	}

	//状态都OK了，继续发送下一包
	//同时修改分包信息
	AppSendChatListLock.Lock()
	chatTask, ok  := AppSendChatList[service.imei]
	if ok && chatTask != nil && len(*chatTask) > 0 {
		if timeId != (*chatTask)[0].Info.FileID {
			AppSendChatListLock.Unlock()
			logging.Log(fmt.Sprintf("[%d] time id  is not matched, %d != %s",
				service.imei, timeId, (*chatTask)[0].Info.Content))

			return false
		}

		if  (*chatTask)[0].Data.BlockIndex != int(blockIndex){
			AppSendChatListLock.Unlock()
			logging.Log(fmt.Sprintf("[%d] block count or index is not invalid, %d, %d for %d, %d",
				blockCount, blockIndex,
				(*chatTask)[0].Data.BlockCount, (*chatTask)[0].Data.BlockIndex))

			return false
		}

		if(blockIndex == (*chatTask)[0].Data.BlockCount) {
			//确认完毕，删除该微聊
			if len(*chatTask) > 1 {
				(*chatTask) = (*chatTask)[1:]
			}else{
				AppSendChatList[service.imei] = &[]*ChatTask{}
			}
			//service.needSendChatNum = true
		}else{
			service.needSendChatNum = false
			service.rspList = append(service.rspList, service.shardData("AP12", (*chatTask)[0].Data.Phone,
				(*chatTask)[0].Data.Time, (*chatTask)[0].Data.Data, blockIndex + 1)[0])
			(*chatTask)[0].Data.BlockIndex = blockIndex + 1
		}
	}
	AppSendChatListLock.Unlock()

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


func (service *GT06Service) ProcessPushPhotoAck(pszMsg []byte) bool {
	//失败：(002F0103357593060153353BP23,
	// 13026618172,6,1,0)
	//首先解析出包状态信息，亲情号，序号和确认状态
	fields := strings.SplitN(string(pszMsg), ",", 4)
	if len(fields) != 4 {
		//字段个数不对
		logging.Log(fmt.Sprintf("[%d] cmd field count %d is bad", service.imei, len(fields)))
		return false
	}

	if len(fields[3]) == 0 || len(fields[3]) != 2 || (fields[3][0] != '0' && fields[3][0] != '1') {
		logging.Log(fmt.Sprintf("[%d] the ack status %s is bad", service.imei, fields[3]))
		return false
	}

	logging.Log(fmt.Sprintf("[%d] recv photo of phone number %s ", service.imei, fields[0]))

	blockCount := int(Str2Num(fields[1], 10))
	blockIndex := int(Str2Num(fields[2], 10))
	lastBlockOk := (fields[3][0] - '0')
	if blockCount <= 0 || blockIndex <= 0 || blockCount < blockIndex {
		logging.Log(fmt.Sprintf("[%d] block count,  index  %d is bad", service.imei, blockCount, blockIndex))
		return false
	}

	if lastBlockOk != 1 {
		logging.Log(fmt.Sprintf("[%d] last block failed,  status is  %d, %s", service.imei, lastBlockOk,  fields[3]))
		return false
	}

	//状态都OK了，继续发送下一包
	//同时修改分包信息
	AppNewPhotoListLock.Lock()
	appNewPhotoList, ok := AppNewPhotoList[service.imei]
	if ok && appNewPhotoList != nil && len(*appNewPhotoList) > 0 {
		if  (*appNewPhotoList)[0].Data.BlockIndex != int(blockIndex){
			AppNewPhotoListLock.Unlock()
			logging.Log(fmt.Sprintf("[%d] block count or index is not invalid, %d, %d for %d, %d",
				service.imei,  blockCount, blockIndex,
				(*appNewPhotoList)[0].Data.BlockCount, (*appNewPhotoList)[0].Data.BlockIndex))

			return false
		}

		if(blockIndex == (*appNewPhotoList)[0].Data.BlockCount) {
			//确认完毕，删除该新头像通知信息
			if len(*appNewPhotoList) > 1 {
				logging.Log(fmt.Sprintf("%d AppNewPhotoList begin len: %d", service.imei, len(*appNewPhotoList)))
				(*appNewPhotoList) = (*appNewPhotoList)[1:]
				logging.Log(fmt.Sprintf("%d AppNewPhotoList end len: %d", service.imei, len(*appNewPhotoList)))
			}else{
				AppNewPhotoList[service.imei] = &[]*PhotoSettingTask{}
				logging.Log(fmt.Sprintf("%d AppNewPhotoList begin len: %d", service.imei, len(*AppNewPhotoList[service.imei])))
				logging.Log(fmt.Sprintf("%d AppNewPhotoList end len: %d", service.imei, len(*AppNewPhotoList[service.imei])))
			}
			logging.Log(fmt.Sprintf("[%d] block count %d is all finished", service.imei, blockIndex))
		}else{
			service.rspList = append(service.rspList, service.shardData("AP23",
				(*appNewPhotoList)[0].Info.Member.Phone,
				(*appNewPhotoList)[0].Data.Time, (*appNewPhotoList)[0].Data.Data, blockIndex + 1)[0])
			(*appNewPhotoList)[0].Data.BlockIndex = blockIndex + 1
			service.needSendChatNum = false
		}
	}
	AppNewPhotoListLock.Unlock()

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

func (service *GT06Service) ProcessRspPhoto() bool {
	//目前每次只下载一个图片文件，并且同一个亲情号只对应一份头像图片
	//并且对于BP11命令永远都是从头开始发送文件的第一包
	//发送之前首先创建任务表，如果已存在任务表，则直接覆盖

	AppNewPhotoListLock.Lock()
	appNewPhotoList, ok := AppNewPhotoList[service.imei]
	if ok && appNewPhotoList != nil && len(*appNewPhotoList) > 0 {
		avatarFileName :=fmt.Sprintf("%s%d/%s", service.reqCtx.AvatarUploadDir, service.imei, (*appNewPhotoList)[0].Info.Content)

		//读取图片文件，发送第一包
		photo, err := ioutil.ReadFile(avatarFileName)
		if err != nil {
			AppNewPhotoListLock.Unlock()
			logging.Log("read photo file failed, " + avatarFileName + "," + err.Error())
			return false
		}

		if len(photo) == 0 {
			AppNewPhotoListLock.Unlock()
			logging.Log("photo file empty, 0 bytes, ")
			return false
		}

		service.rspList = append(service.rspList,
			service.shardData("AP23", (*appNewPhotoList)[0].Info.Member.Phone, makeId(), photo, 1)[0])

		//修改分包信息
		(*appNewPhotoList)[0].Data.Imei = service.imei
		(*appNewPhotoList)[0].Data.Cmd = StringCmd(DRT_FETCH_FILE)
		(*appNewPhotoList)[0].Data.Phone = (*appNewPhotoList)[0].Info.Member.Phone
		(*appNewPhotoList)[0].Data.Time = Str2Num((*appNewPhotoList)[0].Info.Content, 10)
		(*appNewPhotoList)[0].Data.BlockCount = GetBlockCount(len(photo))
		(*appNewPhotoList)[0].Data.BlockSize = GetBlockSize(len(photo), 1)
		(*appNewPhotoList)[0].Data.BlockIndex =1
		(*appNewPhotoList)[0].Data.Data = photo
	}
	AppNewPhotoListLock.Unlock()

	return true
}

func GetBlockCount(totalSize int) int {
	if totalSize % DATA_BLOCK_SIZE == 0 {
		return (totalSize / DATA_BLOCK_SIZE)
	}else{
		return (totalSize / DATA_BLOCK_SIZE  + 1)
	}
}

func GetBlockSize(totalSize, blockIndex int) int{
	if totalSize == 0 || blockIndex == 0 {
		return 0
	}

	if blockIndex <= (totalSize / DATA_BLOCK_SIZE) {
		return DATA_BLOCK_SIZE
	}else{
		if blockIndex == (totalSize / DATA_BLOCK_SIZE + 1) {
			return (totalSize - (blockIndex - 1) * DATA_BLOCK_SIZE)
		}else{
			return 0
		}
	}
}

func (service *GT06Service) ProcessRspChat() bool {
	//目前每次只下载一条微聊文件
	//并且对于BP11命令永远都是从头开始发送第一个微聊文件的第一包
	//发送之前首先创建任务表，如果已存在任务表，则直接覆盖
	if service.reqCtx.GetChatDataFunc != nil {
		chatData := service.reqCtx.GetChatDataFunc(service.imei, 0)
		if len(chatData) > 0 {
			//voiceFileName :=fmt.Sprintf("/usr/share/nginx/html/web/upload/minichat/app/%d/%s.amr",
			//	service.imei, chatData[0].Content)
			voiceFileName :=fmt.Sprintf("%s%d/%d.amr", service.reqCtx.MinichatUploadDir,
				service.imei, chatData[0].FileID)

			voice, chatReplyMsg := service.makeChatDataReplyMsg(voiceFileName,
				chatData[0].Sender, chatData[0].FileID, 1)
			if chatReplyMsg == nil || len(chatReplyMsg) == 0 {
				return false
			}

			service.rspList = append(service.rspList, chatReplyMsg[0])
			//修改分包信息
			AppSendChatListLock.Lock()
			chatTask, ok  := AppSendChatList[service.imei]
			if ok && chatTask != nil && len(*chatTask) > 0 {
				(*chatTask)[0].Data.Imei = service.imei
				(*chatTask)[0].Data.Cmd = StringCmd(DRT_FETCH_FILE)
				(*chatTask)[0].Data.Phone = chatData[0].Sender
				(*chatTask)[0].Data.Time = chatData[0].FileID
				(*chatTask)[0].Data.BlockCount = GetBlockCount(len(voice))
				(*chatTask)[0].Data.BlockSize = GetBlockSize(len(voice), 1)
				(*chatTask)[0].Data.BlockIndex =1
				(*chatTask)[0].Data.Data = voice
			}
			AppSendChatListLock.Unlock()
		}
	}

	return true
}

func (service *GT06Service) PushChatNum() bool {
	if service.reqCtx.GetChatDataFunc != nil {
		chatData := service.reqCtx.GetChatDataFunc(service.imei, -1)
		if len(chatData) > 0 {
			//通知终端有聊天信息
			resp := &ResponseItem{CMD_AP11,  service.makeReplyMsg(false,
				service.makeFileNumReplyMsg(ChatContentVoice, len(chatData)), makeId())}
			service.rspList = append(service.rspList, resp)
		}
	}

	return true
}

func (service *GT06Service) PushNewPhotoNum() bool {
	newAvatars := 0
	AppNewPhotoListLock.Lock()
	newPhotoList, ok := AppNewPhotoList[service.imei]
	if ok && newPhotoList != nil {
		//for _, photo := range *newPhotoList {
		//	if photo.Info.MsgId == 0 {
		//		newAvatars++
		//	}
		//}

		newAvatars = len(*newPhotoList)
	}
	AppNewPhotoListLock.Unlock()

	if newAvatars > 0 {
		resp := &ResponseItem{CMD_AP11,  service.makeReplyMsg(false,
			service.makeFileNumReplyMsg(ChatContentPhoto, newAvatars), makeId())}
		service.rspList = append(service.rspList, resp)
	}

	return true
}

func (service *GT06Service) ProcessUpdateWatchStatus(pszMsgBuf []byte) bool {
	if len(pszMsgBuf) == 0 {
		return false
	}

	ucBattery := 1
	if service.cur.OrigBattery >= 3 {
		ucBattery = int(service.cur.OrigBattery - 2)
	}

	service.cur.Battery = uint8(ucBattery)

	//ret := service.UpdateWatchBattery()
	//if ret == false {
	//	logging.Log(fmt.Sprintf("UpdateWatchBattery failed, battery=%d", ucBattery))
	//	return ret
	//}

	//nStep := service.cur.Steps
	//if  nStep >= 0x7fff - 1 || int(nStep) < 0 {
	//	nStep = 0
	//	service.cur.Steps = nStep
	//}

	iLocateType := LBS_SMARTLOCATION

	bufOffset := 0
	bufOffset++

	iDayTime := service.GetIntValue(pszMsgBuf[bufOffset: ], TIME_LEN)
	iDayTime = iDayTime % 1000000
	bufOffset += TIME_LEN + 1

	iSecTime := service.GetIntValue(pszMsgBuf[bufOffset: ], TIME_LEN)
	i64Time := uint64(iDayTime * 1000000 + iSecTime)
	bufOffset += TIME_LEN + 1

	service.cur.DataTime = i64Time
	service.CountSteps()

	logging.Log(fmt.Sprintf("Update Watch %d, Step:%d, Battery:%d", service.imei, service.cur.Steps, ucBattery))

	stWatchStatus := &WatchStatus{}
	stWatchStatus.i64DeviceID = service.imei
	stWatchStatus.iLocateType = uint8(iLocateType)
	stWatchStatus.i64Time = i64Time
	stWatchStatus.Step = service.cur.Steps
	stWatchStatus.AlarmType = service.cur.AlarmType
	stWatchStatus.Battery = service.cur.Battery
	ret := service.UpdateWatchStatus(stWatchStatus)
	if ret == false {
		logging.Log("Update Watch status failed ")
	}

	//if ucBattery <=7 && ucBattery >= 0 {
	//	service.cur.Battery = uint8(ucBattery)
	//	ret = service.UpdateWatchBattery()
	//	if ret == false {
	//		logging.Log(fmt.Sprintf("UpdateWatchBattery failed, battery=%d", ucBattery))
	//		return ret
	//	}
	//}

	if(service.cur.AlarmType != 0 && service.old.Lat != 0){
		newData := service.old
		newData.AlarmType = service.cur.AlarmType
		newData.DataTime = i64Time
		newData.Steps = service.cur.Steps
		newData.Battery = service.cur.Battery
		service.cur = newData
		ret = service.WatchDataUpdateDB()
		if ret == false {
			logging.Log(fmt.Sprintf("[%d] Update WatchData into Database failed", service.imei))
			return false
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

	service.cur.Battery = 1
	if  service.cur.OrigBattery >= 3 {
		service.cur.Battery =  service.cur.OrigBattery - 2
	}

	if service.cur.Battery < 1 {
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
	//service.cur.Steps = 0
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
	service.cur.Battery = 1
	if  service.cur.OrigBattery >= 3 {
		service.cur.Battery =  service.cur.OrigBattery - 2
	}

	if service.cur.Battery < 1 {
		service.cur.Battery = 1
	}

	//service.GetWatchDataInfo(service.imei)

	if service.wifiZoneIndex >= 0  {
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

func (service *GT06Service) ProcessZoneAlarm() bool {
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

	DeviceInfoListLock.Lock()
	deviceInfo, ok := (*DeviceInfoList)[service.imei]
	if ok && deviceInfo != nil {
		ownerName := deviceInfo.OwnerName
		if ownerName == "" {
			ownerName = Num2Str(service.imei, 10)
		}

		PushNotificationToApp(service.reqCtx.APNSServerBaseURL, service.imei, "",  ownerName,
			service.cur.DataTime, service.cur.AlarmType, service.cur.ZoneName)
	}

	DeviceInfoListLock.Unlock()

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
		if service.old.DataTime == 0 || service.old.Lat == 0 {
			service.old.ZoneIndex = -1
			service.old.LastZoneIndex = -1
		}

		service.cur.LastAlarmType = service.old.LastAlarmType
		service.cur.LastZoneIndex = service.old.LastZoneIndex
	}
}

func  (service *GT06Service) GetLocation()  bool{
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

func AddChatForApp(chat ChatInfo){
	AppChatListLock.Lock()
	chatMap, ok := AppChatList[chat.Imei]
	if ok  &&  chatMap != nil {
		AppChatList[chat.Imei][chat.FileID] = chat
	}else{
		AppChatList[chat.Imei] = map[uint64]ChatInfo{}
		AppChatList[chat.Imei][chat.FileID] = chat
	}

	AppChatListLock.Unlock()
}

func DeleteVoicesForApp(imei uint64, chatList []ChatInfo) bool {
	ret := true
	AppChatListLock.Lock()
	chatMap, ok := AppChatList[imei]
	if ok  &&  chatMap != nil {
		for _, chat := range chatList {
			_, ok2 := AppChatList[imei][chat.FileID]
			if ok2 {
				delete(AppChatList[imei], chat.FileID)
			}else{
				logging.Log(fmt.Sprintf("[%d] delete voices not found file id %d", imei, chat.FileID))
				ret = false
			}
		}
	}else{
		logging.Log(fmt.Sprintf("[%d] delete voices not found items", imei))
		ret = false
	}

	AppChatListLock.Unlock()

	return ret
}

func GetChatListForApp(imei uint64, username string) []ChatInfo{
	chatList:= []ChatInfo{}
	AppChatListLock.Lock()
	chatMap, ok := AppChatList[imei]
	if ok  &&  chatMap != nil {
		for _, chat := range chatMap {
			if username == "" {
				chatList = append(chatList, chat)
			}else{
				if chat.SenderType == 0 || username == chat.SenderUser {
					chatList = append(chatList, chat)
				}
			}
		}
	}
	AppChatListLock.Unlock()

	return chatList
}

func ExecCmd(name string, arg ...string) (error, string){
	cmd := exec.Command(name,  arg...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Start()
	if err != nil {
		return err, ""
	}

	err = cmd.Wait()
	if err != nil {
		return err, ""
	}

	return nil, out.String()
}

func AddPhotoData(imei uint64, photoData PhotoSettingInfo) {
	photoTask := PhotoSettingTask{Info: photoData}
	AppNewPhotoListLock.Lock()
	photoList, ok := AppNewPhotoList[imei]
	if ok {
		logging.Log(fmt.Sprintf("%d AppNewPhotoList begin len: %d", imei, len(*photoList)))
		*photoList = append(*photoList, &photoTask)
		logging.Log(fmt.Sprintf("%d AppNewPhotoList end len: %d", imei, len(*photoList)))
	}else {
		AppNewPhotoList[imei] = &[]*PhotoSettingTask{}
		logging.Log(fmt.Sprintf("%d AppNewPhotoList begin len: %d", imei, len(*AppNewPhotoList[imei])))
		*AppNewPhotoList[imei] = append(*AppNewPhotoList[imei], &photoTask)
		logging.Log(fmt.Sprintf("%d AppNewPhotoList end len: %d", imei, len(*AppNewPhotoList[imei])))
	}
	AppNewPhotoListLock.Unlock()
}

func ResolvePendingPhotoData(imei, msgId uint64) {
	isFound := false
	var resolvedItem *PhotoSettingTask
	tempList:= []*PhotoSettingTask{}
	AppNewPhotoPendingListLock.Lock()
	photoList, ok := AppNewPhotoPendingList[imei]
	if ok && photoList != nil {
		for _, photo := range *photoList{
			if photo.Info.MsgId == msgId{
				resolvedItem = photo
				isFound = true
			}else{
				tempList = append(tempList, photo)
			}
		}
	}
	AppNewPhotoPendingListLock.Unlock()

	if isFound{
		AppNewPhotoPendingListLock.Lock()
		AppNewPhotoPendingList[imei] = &tempList
		AppNewPhotoPendingListLock.Unlock()

		AddPhotoData(imei, resolvedItem.Info)
	}
}
