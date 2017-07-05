package proto

import (
	"encoding/binary"
	"time"
	"os"
	"bufio"
	"io"
	"strconv"
	"sync/atomic"
	"sync"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"fmt"
	"strings"
	"encoding/json"
)

const (
	ImeiLen = 15
	CmdLen = 4
	MTKEPO_DATA_ONETIME =12
	MTKEPO_SV_NUMBER   = 32
	MTKEPO_RECORD_SIZE    =  72
	MTKEPO_SEGMENT_NUM  = (30 * 4)
	INVALID_TIMEZONE = -999
)

const (
	MsgFromDeviceToTcpServer = iota
	MsgFromTcpServerToAppServer
	MsgFromAppServerToApp
	MsgFromAppToAppServer
	MsgFromAppServerToTcpServer
	MsgFromTcpServerToDevice
)


const (
	MSG_HEADER_VER   =  0x0100  //普通消息头部版本
	MSG_HEADER_VER_EX  = 0x0101  //支持分片和断点续传功能的增强型消息头部版本
	MSG_HEADER_PUSH_CACHE = 0x0102  //此消息头部版本用于通知ManagerLoop推送缓存中的数据给手表
	MSG_HEADER_ACK_PARSED = 0x0103  //此消息头部版本用于通知ManagerLoop已收到ack消息
	SHARDING_SIZE  = 700     //分片大小为700字节，这是一个比较合适的经验值。
)

const (
	DRT_MIN = iota

	DRT_SYNC_TIME       // 同BP00，手表请求对时
	DRT_SEND_LOCATION      // 同BP30，手表上报定位(报警)数据
	DRT_DEVICE_LOGIN    	   // 同BP31，手表登录服务器
	DRT_SEND_MINICHAT      // 同BP34，手表发送语音微聊
	DRT_FETCH_FILE		  // 同BP11，手表获取语音微聊或亲情号图片
	DRT_PUSH_MINICHAT_ACK         // 同BP12，手表回复微聊确认包
	DRT_EPO_ACK			  // 同BP13，手表回复epo数据确认包
	DRT_PUSH_PHOTO_ACK         // 同BP23，手表回复头像确认包
	DRT_FETCH_AGPS        	  // 同BP32，手表获取AGPS或APP URL 数据
	DRT_HEART_BEAT       	  // 同BP33，手表发送状态数据

	//确认消息类型
	DRT_SET_IP_PORT_ACK       // 同BP01，手表设置服务器IP端口的ACK
	DRT_SET_APN_ACK             // 同BP02，手表设置APN的ACK
	DRT_SYNC_TIME_ACK       // 同BP03，手表请求对时ACK
	DRT_VOICE_MONITOR_ACK       // 同BP05	，手表设置监听的ACK
	DRT_SET_PHONE_NUMBERS_ACK       // 同BP06	，手表设置亲情号的ACK
	DRT_CLEAR_PHONE_NUMBERS_ACK       // 同BP07	，手表清空亲情号的ACK
	DRT_SET_REBOOT_ENABLE_ACK       // 同BP08	，手表设置是否能重启的ACK
	DRT_SET_TIMER_ALARM_ACK       // 同BP09	，手表设置是否能重启的ACK
	DRT_SET_MUTE_ENABLE_ACK       // 同BP10	，手表设置是否静音的ACK
	DRT_FETCH_LOCATION__ACK       // 同BP14	，手表下载定位数据的ACK
	DRT_SET_POWEROFF_ENABLE_ACK       // 同BP15	，手表设置是否能关机的ACK
	DRT_ACTIVE_SOS_ACK       // 同BP16	，手表设置激活sos的ACK
	DRT_SET_OWNER_NAME_ACK       // 同BP18	，手表设置名字的ACK
	DRT_SET_USE_DST_ACK       // 同BP19	，手表设置夏令时的ACK
	DRT_SET_LANG_ACK       // 同BP20	，手表设置语言的ACK
	DRT_SET_VOLUME_ACK       // 同BP21	，手表设置音量的ACK
	DRT_SET_AIRPLANE_MODE_ACK       // 同BP22	，手表设置隐身模式的ACK
	DRT_QUERY_TEL_USE_ACK       	// 同BP24	，手表对服务器查询短信条数的ACK
	DRT_DELETE_PHONE_PHOTO_ACK       	// 同BP25	，手表对删除亲情号图片的ACK
	DRT_FETCH_APP_URL_ACK       	// 同BP26	，手表获取app下载页面URL的ACK

	DRT_MAX
)

const DRT_FETCH_APP_URL = DRT_FETCH_AGPS

 const (
	CMD_AP00 = iota
	CMD_AP01
	CMD_AP02
	CMD_AP03
	CMD_AP04
	CMD_AP05
	CMD_AP06
	CMD_AP07
	CMD_AP08
	CMD_AP09
	 CMD_AP11
	 CMD_AP12
	 CMD_AP13
	 CMD_AP23
	 CMD_AP14
	 CMD_AP26
	 CMD_AP30
	 CMD_AP31
	 CMD_AP34

	 CMD_ACK
 )



var commands = []string{
	"",
	"BP00",
	"BP30",
	"BP31",
	"BP34",
	"BP11",
	"BP12",
	"BP13",
	"BP23",
	"BP32",
	"BP33",

	"BP01",
	"BP02",
	"BP03",
	"BP05",
	"BP06",
	"BP07",
	"BP08",
	"BP09",
	"BP10",
	"BP14",
	"BP15",
	"BP16",
	"BP18",
	"BP19",
	"BP20",
	"BP21",
	"BP22",
	"BP24",
	"BP25",
	"BP26",
}

/*普通的消息头部*/
type MsgHeader struct {

	Version  uint16  /*版本号，目前取值为 MSG_HEADER_VER 或 MSG_HEADER_VER_EX。
                              由于GT03和WH01的协议以左括弧"("作为起始标识，
                              因此这里需要使用一个不等于左括弧的值。
                              MSG_HEADER_VER    - 表示普通消息头部版本，用于简化处理不需要支持断点续传的消息请求
                              MSG_HEADER_VER_EX - 表示支持分片和断点续传功能的增强型消息头部版本
                              */

	Size  uint16   /*当前这条消息的总字节数*/

	From  uint8  /*当前消息的类型，0 - 客户端请求；1 - 服务器响应；2 - 服务器请求；3 - 客户端响应*/

	Status  uint8  /*消息请求处理的状态，
                                  0 - 表示完成并OK；如果是不做分片和断点续传并且不需要ack的请求，则status应始终赋值为0
                                  1 - 表示请求已经开始处理，但尚未完成，需要继续进行后续通信；
                                 -1 - 表示此次请求非正常结束终止，不再进行后续通信*/

	Cmd  uint16  /*消息的请求类型，等同于GT03上的BP01，BP09等命令*/

	SrcIP    uint32   /*消息的源IP地址*/
	DestIP  uint32  /*消息的目的IP地址*/

	ID  uint64   /*消息ID，精确到4位毫秒的时间戳，用于唯一标识一条完整的消息请求。如果某个请求含有多个分片，
                               那么所有的分片都使用同一个msgId*/

	Imei  uint64 /*设备的IMEI*/

}

type MsgResumeHeader struct {
	TotalShardings uint32   /*全部分片的总数，不做分片也即是只有一个分片时，此字段等于1*/

	TotalDataSize uint32   /*如果请求包含多个分片，表示所有数据内容的总字节数，不包含消息头的大小，
                                  例如发送一个10K大小的语音，那么这个值就是10240*/
	CurShardingIdx uint32   /*当前数据所属分片的序号*/

	CurDataOffset uint32   /*如果请求包含多个分片，表示当前发送的分片中数据内容的偏移量。正常情况下都是从0开始，
                                  如果之前发生过断点，那么重新续传时这个值将是最后一次客户端发送的确认包中收到的数据量。*/
}

/*支持分片和断点续传功能的增强型消息头部*/
type MsgHeaderEx struct {
	Header   MsgHeader       /*普通消息头部*/
	ResumeHeader   MsgResumeHeader  /*用于断点续传的头部*/
}

type MsgData struct {
	Header MsgHeaderEx
	Data []byte
}

type AppMsgData struct {
	ID uint64		 `json:"id,omitempty"`
	Cmd string  	`json:"cmd"`
	Imei uint64		`json:"imei,omitempty"`
	Data string		`json:"data"`
	UserName string `json:"username,omitempty"`
	AccessToken string `json:"accessTken,omitempty"`
	Conn interface{}  `json:"-"`
}

type HeartbeatParams struct {
	UserName string `json:"username"`
	AccessToken string `json:"accessToken"`
	Timestamp int64 `json:"timestamp"`
	Devices []string `json:"devices"`
}

type HeartbeatResult struct {
	Timestamp string `json:"timestamp"`
	Locations []LocationData`json:"locations"`
	Minichat []ChatInfo`json:"minichat"`
}


type DeviceSettingResult struct {
	Settings []SettingParam `json:"settings"`
}

type HttpAPIResult struct {
	ErrCode int				`json:"errcode"`
	ErrMsg string  			`json:"errmsg"`
	Imei string  				`json:"imei"`
	Data  string  			`json:"data"`
}

type LoginParams struct {
	UserName string  	`json:"username"`
	Password string		`json:"password"`
}

type DeviceBaseParams struct {
	Imei string  				`json:"imei"`
	UserName string		`json:"username"`
	AccessToken string		`json:"accessToken"`
}

type DeviceAddParams struct {
	Imei string  				`json:"imei"`
	UserName string		`json:"username"`
	AccessToken string		`json:"accessToken"`
	UserId string			`json:"userId"`
	OwnerName string		`json:"ownerName"`
	DeviceSimCountryCode string		`json:"deviceSimCountryCode"`
	DeviceSimID string		`json:"deviceSimID"`
	MySimCountryCode string		`json:"mySimCountryCode"`
	MySimID string		`json:"mySimID"`
	PhoneType int		`json:"phoneType"`
	MyName string		`json:"myName"`
	VerifyCode string		`json:"verifyCode"`
	IsAdmin int		`json:"isAdmin"`
	TimeZone  string 	`json:"timezone"`
}

type SettingParam struct {
	FieldName string 		`json:"fieldname"`
	CurValue string 		`json:"curvalue"`
	NewValue string 		`json:"newvalue"`
	Index int
}

type DeviceSettingParams struct {
	Imei string  				`json:"imei"`
	UserName string		`json:"username"`
	AccessToken string		`json:"accessToken"`
	MsgId uint64 			`json:"msgId"`
	Settings []SettingParam `json:"settings"`
}

type DeleteVoicesParams struct {
	Imei string `json:"imei"`
	Username string `json:"username"`
	AccessToken string `json:"accessToken"`
	DeleteVoices []ChatInfo  `json:"deleteVoices"`
}

type IPInfo struct {
	StartIP uint32
	EndIP uint32
	TimeZone int32
}

type EPOInfo struct {
	EPOBuf[]byte
}

const DM_MIN = -1
const (
	DM_WH01 = iota
	DM_GT03
	DM_GTI3
	DM_GT06

	DM_MAX
)

var ModelNameList = []string{ "WH01", "GT03","GTI3", "GT06"}

const (
	ALARM_INZONE = 1
	ALARM_SOS = 2
	ALARM_OUTZONE = 6
	ALARM_BATTERYLOW = 8
)

const (
	LBS_NORMAL = 0
	LBS_GPS = 1 //gps直接定位
	LBS_JIZHAN = 2 //基站定位
	LBS_WIFI = 3  //WIFI 定位
	LBS_SMARTLOCATION = 4  //智能定位
	LBS_INVALID_LOCATION //无效定位
)

const (
	MAX_SAFE_ZONE_NUM = 10
	MAX_WATCH_ALARM_NUM = 5
	MAX_HIDE_TIMER_NUM = 4
	MAX_FAMILY_MEMBER_NUM = 13
)

var	ReloadEPOFileName 		= "epo"

var	ModelFieldName 			= "Model"

var	AvatarFieldName 			= "Avatar"
var	OwnerNameFieldName 	= "OwnerName"
var	TimeZoneFieldName 		= "TimeZone"
var	SimIDFieldName 			= "SimID"
var	VolumeFieldName 			= "Volume"
var	LangFieldName 			= "Lang"
var	UseDSTFieldName 			= "UseDST"
var	ChildPowerOffFieldName 	= "ChildPowerOff"
var PhoneNumbersFieldName 	= "PhoneNumbers"
var ContactAvatarsFieldName 	= "ContactAvatar"
var WatchAlarmFieldName 	= "WatchAlarm"
var HideSelfFieldName 		= "HideSelf"
var HideTimer0FieldName 		= "HideTimer0"
var HideTimer1FieldName 		= "HideTimer1"
var HideTimer2FieldName 		= "HideTimer2"
var HideTimer3FieldName 		= "HideTimer3"
var FenceFieldName 			= "Fence"

var CountryCodeFieldName	= "CountryCode"

var CmdOKTail 				= "-ok"
var CmdAckTail 				= "-ack"

var LoginCmdName  			= "login"
var LoginAckCmdName  		= LoginCmdName + CmdAckTail

var RegisterCmdName  		= "register"
var RegisterAckCmdName  		= RegisterCmdName + CmdAckTail

var ResetPasswordCmdName  		= "reset-password"
var ResetPasswordAckCmdName  	= ResetPasswordCmdName + CmdAckTail

var FeedbackCmdName  		= "feedback"
var FeedbackAckCmdName  	= FeedbackCmdName + CmdAckTail

var ModifyPasswordCmdName  		= "modify-password"
var ModifyPasswordAckCmdName  	= ModifyPasswordCmdName + CmdAckTail

var HearbeatCmdName  		= "heartbeat"
var HearbeatAckCmdName  	= HearbeatCmdName + CmdAckTail

var VerifyCodeCmdName  		= "verify-code"
var VerifyCodeAckCmdName  	= VerifyCodeCmdName + CmdAckTail

var SetDeviceCmdName  		= "set-device"
var SetDeviceAckCmdName  	= SetDeviceCmdName + CmdAckTail

var GetDeviceByImeiCmdName  		= "get-device-by-imei"
var GetDeviceByImeiAckCmdName  	= GetDeviceByImeiCmdName + CmdAckTail

var AddDeviceCmdName  			= "add-device"
var AddDeviceAckCmdName  		= AddDeviceCmdName + CmdAckTail
var AddDeviceOKAckCmdName  	= AddDeviceCmdName + CmdOKTail +  CmdAckTail

var DeleteDeviceCmdName  			= "delete-device"
var DeleteDeviceAckCmdName  		= DeleteDeviceCmdName + CmdAckTail

var DeleteVoicesCmdName  			= "delete-voices"
var DeleteVoicesAckCmdName  		= DeleteVoicesCmdName + CmdAckTail


type SafeZone struct {
	ZoneID int32
	ZoneName string
	//LatiTude float64
	//LongTitude float64
	//Radiu uint32
	//WifiMACID string
	//Flags int32
	Radius int `json:"Radius"`
	Name string `json:"Name"`
	Center string `json:"Center"`
	On string `json:"On"`
	Wifi struct {
		       SSID string `json:"SSID"`
		       BSSID string `json:"BSSID"`
	     } `json:"Wifi"`
}

type FamilyMember struct {
	CountryCode string `json:"countryCode"`
	Phone string `json:"phone"`
	Name string `json:"name"`
	Avatar string `json:"avatar"`
	Type int `json:"type"`
	Index int `json:"index"`
}

type ContactAvatars struct {
	ContactAvatars[] FamilyMember
}

type HideiTimer struct {
	Idx string `json:"Idx"`
	Date string `json:"Date"`
	Days int `json:"Days"`
	Begin string `json:"Begin"`
	End string `json:"End"`
	Enabled int `json:"Enabled"`
}

type WatchAlarm struct {
	Idx string `json:"Idx"`
	Date string `json:"Date"`
	Days int `json:"Days"`
	Time string `json:"Time"`
}

type DeviceInfo struct {
	Imei uint64
	Model int
	TimeZone int
	OwnerName string
	Company string
	CountryCode string
	Avatar string
	SimID string
	Lang string
	Volume uint8
	ChildPowerOff bool
	UseDST bool
	SocketModeOff bool
	VerifyCode string
	IsAdmin int
	WatchAlarmList [MAX_WATCH_ALARM_NUM]WatchAlarm
	SafeZoneList [MAX_SAFE_ZONE_NUM]SafeZone
	Family [MAX_FAMILY_MEMBER_NUM]FamilyMember
	HideTimerOn bool
	HideTimerList [MAX_HIDE_TIMER_NUM]HideiTimer
}

type DeviceInfoResult struct {
	IMEI,
	Model,
	OwnerName,
	PhoneNumbers,
	FamilyNumber,
	Name,
	AlarmEmail,
	LocateInterval,
	TimeZone,
	CountryCode,
	Avatar,
	Fence1,
	Fence2,
	VerifyCode,
	WatchAlarm0,
	WatchAlarm1,
	WatchAlarm2,
	WatchAlarm3,
	WatchAlarm4,
	Fence3,
	Fence4,
	Fence5,
	Fence6,
	Fence7,
	Fence8,
	Fence9,
	Fence10,
	Lang,
	HideTimer0,
	HideTimer1,
	HideTimer2,
	HideTimer3 string
	HideSelf,
	ChildPowerOff,
	AlertSMS,
	UseDST,
	SocketModeOff,
	Volume uint8
	ContactAvatar [MAX_FAMILY_MEMBER_NUM]string
}

type WIFIInfo  struct  {
	WIFIName string
	MacID [MAX_MACID_LEN]byte
	Ratio int16
}

type LBSINFO struct  {
	Mcc int32
	Mnc int32
	Lac int32
	CellID int32
	Signal int32
}

type WatchStatus struct {
	i64DeviceID uint64
	iLocateType uint8
	i64Time uint64
	Step uint64
}


var offsets = []uint8{2, 2, 1, 1, 2, 8, 8, 4, 4, 4, 4}
var IPInfoList []*IPInfo
var ipinfoListLock = sync.RWMutex{}
var StartGPSHour uint32
var EPOInfoList []*EPOInfo
var EpoInfoListLock = sync.RWMutex{}
var DeviceInfoList = &map[uint64]*DeviceInfo{}
var DeviceInfoListLock =  sync.RWMutex{}
var company_blacklist = []string {
	"UES",
}

func init()  {
	//fmt.Println(time.Now().Unix())
	//fmt.Println(time.Now().UnixNano() / int64(time.Millisecond )* 10)
	//a := []byte("123456")
	//b := a[0: 3]
	//a[0] = '6'
	//fmt.Println(string(a), string(b))
	//test := LocationData{}
	//data, _ := json.Marshal(&test)
	//fmt.Println(string(data))
	//os.Exit(1)

	//a := int32(133)
	//c := float64(float64(a) / 60.0)
	//fmt.Println("c: ", c * 100)
	//fmt.Println(strings.SplitN("(03B5357593060153353AP12,1,170413163300,6,1,900,数据,数据)", ",", 7))
	//a:=[]int{1,2,3}
	//a = append(a, nil...)
	//var a []byte
	//a := []byte{1,2,3}
	////b := []byte{4,5,6,7,8,9}
	//c := a
	//c[0] = 6
	//a = make([]byte, 9)
	//copy(a[0: 3], c)
	//copy(a[3: ], b)
	//fmt.Println(a, len(a))
	//m1 := map[int]map[int]int{}
	//a := map[int]int{6: 6}
	//m1[5] = a
	//
	//b, _ :=m1[6]
	//b = map[int]int{}
	//m1[6] = b
	//
	//
	////a := []int{1,2}
	////m1 := a
	//fmt.Println(a)
	//fmt.Println(m1)
	//
	//b[6] = 7
	//fmt.Println(a)
	//fmt.Println(m1)
	//
	//tz := "-01:30"
	//tz = strings.Replace(tz, ":", "", 1)
	//if tz[0] == '-' {
	//	tz = strings.Replace(tz, "-", "w", 1)
	//}else if tz[0] == '+'{
	//	tz = strings.Replace(tz, "+", "e", 1)
	//}else{
	//	tz = "e" + tz
	//}
	//
	//fmt.Println("tz", DeviceTimeZoneInt(tz))
	//imei := 357593060571398
	//fmt.Println("sn: ")
	//fmt.Println(imei % 100000000000)
	//fmt.Println(DM_MIN , DM_WH01, DM_GT03, DM_GTI3 , DM_GT06, DM_MAX)
	//fmt.Println(DRT_MIN,  DRT_SYNC_TIME)
	//fmt.Println(MsgFromDeviceToTcpServer, CMD_AP00,  DEVICE_DATA_LOCATION,  ChatFromDeviceToApp,)
	//now := time.Now()
	//fmt.Println(now.UnixNano(),  now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond())
	//fmt.Println(now.UnixNano(),  now.Format("20060102150405"), now.Nanosecond())
	//id := fmt.Sprintf("%s%d", now.Format("20060102150405"), now.Nanosecond())
	//fmt.Println(now.UnixNano(),  id[2:18])
	//avatar := "http://service.gatorcn.com/xxx/xxx/upload/avatar/aaaaa.jpg"
	//s:=strings.SplitN(avatar, "upload/avatar/", 2)
	//fmt.Println(MakeStructToJson(&FamilyMember{}))
	//f1 := [MAX_FAMILY_MEMBER_NUM]FamilyMember{}
	//f2 := [MAX_FAMILY_MEMBER_NUM]FamilyMember{}
	//f1[0].Phone = "123"
	//f1[1].Phone = "234"
	//fmt.Println(f1)
	//f2[0] = f1[0]
	//f1 = f2
	//fmt.Println(f1)
	//name := "Fence"
	//switch {
	//case strings.Contains(name, "Fence"):
	//	fmt.Println("OK")
	//}
	//args := fmt.Sprintf("-i %s -acodec libfaac -ab 64k -ar 44100 %s.aac",
	//	"static/upload/minichat/watch/357593060571398/1707031627033000.amr",
	//	"static/upload/minichat/watch/357593060571398/test")
	//argList := strings.Split(args, " ")
	//err2, _ := ExecCmd("ffmpeg",  argList...)
	//if err2 != nil {
	//	fmt.Println(fmt.Sprintf(" ffmpeg %s failed, %s",  args, err2.Error()))
	//}
	//
	//os.Exit(0)

	LoadIPInfosFromFile()
	LoadEPOFromFile(false)
	//utcNow := time.Now().UTC()
	//iCurrentGPSHour := utc_to_gps_hour(utcNow.Year(), int(utcNow.Month()),utcNow.Day(), utcNow.Hour())
	//segment := (uint32(iCurrentGPSHour) - StartGPSHour) / 6
	//fmt.Printf("EPO y,m,d,h: %d, %d,%d, %d\n", utcNow.Year(), utcNow.Month(),utcNow.Day(), utcNow.Hour())
	//fmt.Printf("EPO hour: %d, %d(%d), %d\n", iCurrentGPSHour, StartGPSHour, Str2Num("01050028", 16)  & 0x00FFFFFF, segment)
	//os.Exit(0)

}

func LoadIPInfosFromFile()  {
	rw,err := os.Open("./ipconfig.ini")
	if err != nil {
		panic(err)
	}
	defer rw.Close()
	tmpIPInfoList := []*IPInfo{}
	rb := bufio.NewReader(rw)
	for {
		line, _, err := rb.ReadLine()
	 if err == io.EOF { break }
		ipInfo := &IPInfo{}
		wordCount, wordStart, ip := 0, -1, 0
		for i := 0; i < len(line); i++ {
			if i >= len(line) - 1 || line[i] == ' ' || line[i] == '\t' || line[i] == '\r' || line[i] == '\n' {
				if wordStart != -1 {
					if wordCount ==  0 {
						ip, _ = strconv.Atoi(string(line[wordStart: i]))
						ipInfo.StartIP = uint32(ip)
						if ipInfo.StartIP <= 0 {
							break
						}
					} else if wordCount == 1 {
						ip, _ = strconv.Atoi(string(line[wordStart: i]))
						ipInfo.EndIP = uint32(ip)
						if ipInfo.EndIP <= 0 {
							break
						}
					} else if wordCount == 2 {
						 bSignal := true
						szTimeZone :=  line[wordStart: i]
						if i >= len(line) - 1{
							szTimeZone = line[wordStart: i+1]
						}

						if (szTimeZone[0] == '-'){
							bSignal = false
						}

						ipInfo.TimeZone = (int32(szTimeZone[1]) - '0')*1000 + (int32(szTimeZone[2]) - '0')*100 + (int32(szTimeZone[4]) - '0') * 10

						if (!bSignal) {
							ipInfo.TimeZone = 0 - ipInfo.TimeZone
						}

					}
					wordStart = -1
					wordCount++
				}else{
					continue
				}
			}else{
				if wordStart == -1{
					wordStart = i
				}
			}
		}

		tmpIPInfoList = append(tmpIPInfoList, ipInfo)
	}

	ipinfoListLock.Lock()
	IPInfoList = tmpIPInfoList
	ipinfoListLock.Unlock()
}

func GetTimeZone(uiIP uint32) int32 {
	ipinfoListLock.RLock()
	defer ipinfoListLock.RUnlock()

	uiStartIP := IPInfoList[0].StartIP
	uiEndIP := IPInfoList[len(IPInfoList) - 1].EndIP
	if uiIP <= uiStartIP {
		return IPInfoList[0].TimeZone
	}

	if uiIP >= uiEndIP {
		return IPInfoList[len(IPInfoList) - 1].TimeZone
	}

	iLowIndex := 1
	iHighIndex := (len(IPInfoList)) - 2
	uiMidIndex := uint32(0)

	for iLowIndex < iHighIndex {
		uiMidIndex = uint32((iLowIndex + iHighIndex ) / 2)

		if uiIP < IPInfoList[uiMidIndex].StartIP {
			if uiIP >= IPInfoList[uiMidIndex - 1].StartIP {
				return IPInfoList[uiMidIndex - 1].TimeZone
			} else {
				iHighIndex = int(uiMidIndex - 1)
			}
		} else {
			if uiIP < IPInfoList[uiMidIndex + 1].StartIP {
				return IPInfoList[uiMidIndex].TimeZone
			} else {
				iLowIndex = int(uiMidIndex + 1)
			}
		}
	}

	return 0
}

func LoadEPOFromFile(reload bool) error {

	mtk30,err := os.Open("./EPO/MTK30.EPO")
	if err != nil {
		if reload {
			return err
		}

		panic(err)
	}
	defer mtk30.Close()

	epo36h, err := os.OpenFile("./EPO/36H.EPO", os.O_CREATE | os.O_WRONLY | os.O_TRUNC, 0666)
	if err != nil {
		if reload {
			return err
		}

		panic(err)
	}
	defer epo36h.Close()

	tmpEPOInfoList := []*EPOInfo{}
	tmpStartGPSHour := uint32(0)
	hourBuf := make([]byte, 4)
	mtk30.Read(hourBuf)
	tmpStartGPSHour = binary.LittleEndian.Uint32(hourBuf)
	tmpStartGPSHour &= 0x00FFFFFF
	atomic.StoreUint32(&StartGPSHour, tmpStartGPSHour)
	mtk30.Seek(0,  os.SEEK_SET)

	for  i := 0; i < MTKEPO_DATA_ONETIME; i++ {
		epoInfo := &EPOInfo{}
		epoInfo.EPOBuf = make([]byte, MTKEPO_SV_NUMBER*MTKEPO_RECORD_SIZE)
		tmpEPOInfoList = append(tmpEPOInfoList, epoInfo)
		mtk30.Read(tmpEPOInfoList[i].EPOBuf)
		epo36h.Write(tmpEPOInfoList[i].EPOBuf)
	}

	EpoInfoListLock.Lock()
	EPOInfoList = tmpEPOInfoList
	EpoInfoListLock.Unlock()

	return nil
}

func parseUint8Array(data interface{}) string {
	if data == nil {
		return ""
	}

	return string([]byte(data.([]uint8)))
}

func makeDBTimeZoneString(tz int) string  {
	if tz == 0 {
		return "00:00"
	}else if tz < 0 {
		return fmt.Sprintf("%02d:%02d", int(tz / 100), tz % 100)
	}else{
		return fmt.Sprintf("+%02d:%02d", int(tz / 100), tz % 100)
	}
}

func MakeStructToJson(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func Bool2UInt8(b bool)   uint8 {
	if b {
		return 1
	}

	return 0
}


func MakeFamilyPhoneNumbers(family *[MAX_FAMILY_MEMBER_NUM]FamilyMember) string {
	phoneNumbers := ""
	for i := 0; i < len(family); i++ {
		if  i > 0 {
			phoneNumbers += ","
		}

		phoneNumbers += fmt.Sprintf("%s|%d|%s",  family[i].Phone, family[i].Type, family[i].Name)
	}

	return phoneNumbers
}

func MakeDeviceInfoResult(deviceInfo *DeviceInfo) DeviceInfoResult {
	result := DeviceInfoResult{}
	result.IMEI = Num2Str(deviceInfo.Imei, 10)
	result.Model = ModelNameList[deviceInfo.Model]
	result.OwnerName = deviceInfo.OwnerName
	result.PhoneNumbers = MakeFamilyPhoneNumbers(&deviceInfo.Family)

	for i, m := range deviceInfo.Family {
		result.ContactAvatar[i] = m.Avatar
	}

	result.TimeZone = makeDBTimeZoneString(deviceInfo.TimeZone)
	result.CountryCode = deviceInfo.CountryCode
	result.Avatar = deviceInfo.Avatar
	result.VerifyCode = deviceInfo.VerifyCode
	result.Lang = deviceInfo.Lang
	result.Volume = deviceInfo.Volume
	result.HideSelf = Bool2UInt8(deviceInfo.HideTimerOn)
	result.ChildPowerOff = Bool2UInt8(deviceInfo.ChildPowerOff)
	result.UseDST = Bool2UInt8(deviceInfo.UseDST)
	result.SocketModeOff = Bool2UInt8(deviceInfo.SocketModeOff)

	result.Fence1 = MakeStructToJson(&deviceInfo.SafeZoneList[0])
	result.Fence2 = MakeStructToJson(&deviceInfo.SafeZoneList[1])
	result.Fence3 = MakeStructToJson(&deviceInfo.SafeZoneList[2])
	result.Fence4 = MakeStructToJson(&deviceInfo.SafeZoneList[3])
	result.Fence5 = MakeStructToJson(&deviceInfo.SafeZoneList[4])
	result.Fence6 = MakeStructToJson(&deviceInfo.SafeZoneList[5])
	result.Fence7 = MakeStructToJson(&deviceInfo.SafeZoneList[6])
	result.Fence8 = MakeStructToJson(&deviceInfo.SafeZoneList[7])
	result.Fence9 = MakeStructToJson(&deviceInfo.SafeZoneList[8])
	result.Fence10 = MakeStructToJson(&deviceInfo.SafeZoneList[9])

	result.WatchAlarm0 = MakeStructToJson(&deviceInfo.WatchAlarmList[0])
	result.WatchAlarm1 = MakeStructToJson(&deviceInfo.WatchAlarmList[1])
	result.WatchAlarm2 = MakeStructToJson(&deviceInfo.WatchAlarmList[2])
	result.WatchAlarm3 = MakeStructToJson(&deviceInfo.WatchAlarmList[3])
	result.WatchAlarm4 = MakeStructToJson(&deviceInfo.WatchAlarmList[4])

	result.HideTimer0 = MakeStructToJson(&deviceInfo.HideTimerList[0])
	result.HideTimer1 = MakeStructToJson(&deviceInfo.HideTimerList[1])
	result.HideTimer2 = MakeStructToJson(&deviceInfo.HideTimerList[2])
	result.HideTimer3 = MakeStructToJson(&deviceInfo.HideTimerList[3])

	return result
}

func LoadDeviceInfoFromDB(dbpool *sql.DB)  bool{
	rows, err := dbpool.Query("select w.IMEI, w.OwnerName, w.PhoneNumbers, w.ContactAvatar,  w.TimeZone, w.CountryCode, w.Avatar, d.SimID," +
		" w.ChildPowerOff, w.UseDST, w.SocketModeOff, w.Volume, w.Lang, w.VerifyCode, w.Fence1,w.Fence2, w.Fence3," +
		" w.Fence4,w.Fence5,w.Fence6,w.Fence7,w.Fence8,w.Fence9,w.Fence10, w.WatchAlarm0, w.WatchAlarm1, " +
		" w.WatchAlarm2,w.WatchAlarm3, w.WatchAlarm4,w.HideSelf,w.HideTimer0,w.HideTimer1,w.HideTimer2," +
		" w.HideTimer3, pm.model, c.name from watchinfo w join device d on w.recid=d.recid join productmodel pm  " +
		" on d.modelid=pm.recid join companies c on d.companyid=c.recid where pm.model='GT06' ")
	if err != nil {
		fmt.Println("LoadDeviceInfoFromDB failed,", err.Error())
		os.Exit(1)
	}

	var(
		IMEI      		,
		OwnerName  		,
		PhoneNumbers   	,
		ContactAvatar 	,
		TimeZone		,
		CountryCode 		,
		Avatar 			,
		SimID 			,
		ChildPowerOff		,
		UseDST		,
		SocketModeOff	,
		Volume		,
		Lang			,
		VerifyCode 	interface{}

		Fences		=	make([]interface{}, MAX_SAFE_ZONE_NUM)//[MAX_SAFE_ZONE_NUM]string{}
		WatchAlarms = make([]interface{}, MAX_WATCH_ALARM_NUM) //[MAX_WATCH_ALARM_NUM]string{}

		HideSelf		interface{}
		HideTimers = make([]interface{}, MAX_HIDE_TIMER_NUM) //[MAX_HIDE_TIMER_NUM]string{}

		Model 			interface{}
		Company 		 interface{}
	)

	tmpDeviceInfoList := &map[uint64]*DeviceInfo{}
	for rows.Next() {
		deviceInfo := &DeviceInfo{}
		err := rows.Scan(&IMEI, &OwnerName, &PhoneNumbers, &ContactAvatar, &TimeZone, &CountryCode, &Avatar, &SimID,
			&ChildPowerOff, &UseDST, &SocketModeOff, &Volume, &Lang, &VerifyCode, &Fences[0], &Fences[1], &Fences[2],
			&Fences[3], &Fences[4], &Fences[5], &Fences[6], &Fences[7], &Fences[8], &Fences[9], &WatchAlarms[0], &WatchAlarms[1],
			&WatchAlarms[2], &WatchAlarms[3], &WatchAlarms[4], &HideSelf, &HideTimers[0], &HideTimers[1], &HideTimers[2],
			&HideTimers[3], &Model, &Company)
		if err != nil {
			fmt.Println("row scan err: ", err.Error())
		}
		deviceInfo.Imei = Str2Num(parseUint8Array(IMEI), 10)
		deviceInfo.OwnerName = parseUint8Array(OwnerName)
		ParseFamilyMembers(parseUint8Array(PhoneNumbers), &deviceInfo.Family)
		ParseContactAvatars(parseUint8Array(ContactAvatar), &deviceInfo.Family)
		deviceInfo.TimeZone  = ParseTimeZone(parseUint8Array(TimeZone))
		deviceInfo.CountryCode = parseUint8Array(CountryCode)
		deviceInfo.Avatar = parseUint8Array(Avatar)
		deviceInfo.SimID = parseUint8Array(SimID)
		deviceInfo.ChildPowerOff = parseUint8Array(ChildPowerOff) == "1"
		deviceInfo.UseDST = parseUint8Array(UseDST) == "1"
		deviceInfo.SocketModeOff = parseUint8Array(SocketModeOff) == "1"
		deviceInfo.Volume = uint8(Str2Num(parseUint8Array(Volume), 10))
		deviceInfo.Lang = parseUint8Array(Lang)
		deviceInfo.VerifyCode = parseUint8Array(VerifyCode)
		ParseSafeZones(Fences, &deviceInfo.SafeZoneList)
		ParseWatchAlarms(WatchAlarms, &deviceInfo.WatchAlarmList)
		deviceInfo.HideTimerOn = parseUint8Array(HideSelf) == "1"
		ParseHideTimers(HideTimers, &deviceInfo.HideTimerList)
		deviceInfo.Model = ParseDeviceModel(parseUint8Array(Model))
		deviceInfo.Company = parseUint8Array(Company)

		(*tmpDeviceInfoList)[deviceInfo.Imei] = deviceInfo
	}

	fmt.Println("deviceinfo list len: ", len(*tmpDeviceInfoList))

	DeviceInfoListLock.Lock()
	DeviceInfoList = tmpDeviceInfoList
	DeviceInfoListLock.Unlock()
	return true
}

func ParseDeviceModel(model string) int {
	for i, modelName := range ModelNameList {
		if modelName == model {
			return i
		}
	}

	return -1
}

func ParseSafeZones(zones []interface{}, safeZoneList *[MAX_SAFE_ZONE_NUM]SafeZone)  {
//	{"Radius":200,"Name":"home","Center":"22.588015574341,113.91333157419001","On":"1","Wifi":{"SSID":"gatorgroup","BSSID":"d8:24:bd:77:4d:6e"}}
	for i, zone := range zones  {
		strZone := parseUint8Array(zone)
		if len(strZone) == 0 {
			continue
		}

		err:=json.Unmarshal([]byte(strZone), &safeZoneList[i])
		if err != nil {
			continue
		}
	}
}

func ParseWatchAlarms(alarms []interface{}, alarmList *[MAX_WATCH_ALARM_NUM]WatchAlarm)  {
	for i, alarm := range alarms  {
		strAlarm := parseUint8Array(alarm)
		if len(strAlarm) == 0 {
			continue
		}

		err:=json.Unmarshal([]byte(strAlarm), &alarmList[i])
		if err != nil {
			continue
		}
	}
}

func ParseHideTimers(timers []interface{}, hidetimerList *[MAX_HIDE_TIMER_NUM]HideiTimer)  {
	for i, timer := range timers  {
		strTimer := parseUint8Array(timer)
		if len(strTimer) == 0 {
			continue
		}

		err:=json.Unmarshal([]byte(strTimer), &hidetimerList[i])
		if err != nil {
			continue
		}
	}
}


func ParseSinglePhoneNumberString(phone string, i int)  FamilyMember{
	member := FamilyMember{}
	if len(phone) == 0 {
		return member
	}

	fields := strings.SplitN(phone, "|", 3)
	if len(fields) == 0 {
		return member
	}

	if len(fields) >= 1 {
		member.Phone = fields[0]
	}

	if len(fields) >= 2 {
		member.Type = int(Str2Num(fields[1], 10))
	}

	if len(fields) >= 3 {
		member.Name = fields[2]
	}

	member.Index = i

	return member
}


func ParseContactAvatars(contactAvatars string, familyMemberList *[MAX_FAMILY_MEMBER_NUM]FamilyMember)  {
	avatars := ContactAvatars{}
	err := json.Unmarshal([]byte(contactAvatars), &avatars)
	if err != nil {
		fmt.Println("parse contact avatars failed, ", err.Error())
		return
	}

	for i, m := range avatars.ContactAvatars {
		if i >= MAX_FAMILY_MEMBER_NUM  || m.Index > MAX_FAMILY_MEMBER_NUM {
			break
		}

		familyMemberList[i].Avatar = m.Avatar
	}
}

func ParseFamilyMembers(PhoneNumbers string, familyMemberList *[MAX_FAMILY_MEMBER_NUM]FamilyMember)  {
	members := strings.Split(PhoneNumbers, ",")
	for i, m := range members {
		if i >= MAX_FAMILY_MEMBER_NUM {
			break
		}

		if len(m) == 0 {
			continue
		}

		familyMemberList[i] = ParseSinglePhoneNumberString(m, i + 1)
	}
}

func CheckStrIsNumber(szInput string) bool {
	if len(szInput) == 0 {
		return false
	}

	for i := 0; i < len(szInput); i++ {
		if szInput[i] < '0' || szInput[i] > '9' {
			return false
		}
	}

	return true
}

func checkTimeZoneOffset(tz string) bool {
	if len(tz) == 5 {
		return (tz =="00:00")
	} else if len(tz) == 6 {
		sub := string(tz[1: 1 + 2])
		num, _ := strconv.Atoi(sub)

		if (tz[0] != '+' && tz[0] != '-') || (tz[3] != ':') ||  (tz[4] != '3' && tz[4] != '0') ||  (tz[5] != '0')  {
			return false
		}

		if !CheckStrIsNumber(sub)  || num > 11 {
			return false
		}

		return true
	} else {
		return false
	}
}


func ParseTimeZone(timezone string)  int {
	if checkTimeZoneOffset(timezone) == false {
		return INVALID_TIMEZONE
	}

	bSignal := true
	if (timezone[0] == '-'){
		bSignal = false
	}

	TimeZone := (int32(timezone[1]) - '0')*1000 + (int32(timezone[2]) - '0')*100 + (int32(timezone[4]) - '0') * 10

	if (!bSignal) {
		TimeZone = 0 - TimeZone
	}

	return int(TimeZone)
}

func Str2Float(str string)  float64 {
	value, _ :=strconv.ParseFloat(str, 0)
	return value
}

func Str2Num(str string, base int)  uint64 {
	if len(str) == 0 {
		return uint64(0)
	}

	value, _ :=strconv.ParseUint(str, base, 0)
	return value
}

func Num2Str(num uint64, base int)  string {
	if base == 10 {
		return fmt.Sprintf("%d", num)
	}else if base == 16{
		return fmt.Sprintf("%X", num)
	}else {
		return ""
	}
}

func IntCmd(cmd string) uint16 {
	for i, c := range commands {
		if c == cmd {
			return uint16(i)
		}
	}

	return 0
}

func StringCmd(cmd uint16) string {
	return commands[cmd]
}


func NewMsgID() uint64 {
	return uint64(time.Now().UnixNano() / int64(time.Millisecond )* 10)
}

func msgStructTotalSize(msg *MsgData) uint16 {
	if msg == nil {
		return 0
	}

	return uint16(binary.Size(msg.Header)) + msg.Header.Header.Size
}

func Encode(msg *MsgData) []byte {
	data := make([]byte, msgStructTotalSize(msg))
	offset, i :=uint8(0), uint8(0)

	binary.LittleEndian.PutUint16(data[offset: offset + offsets[i]], msg.Header.Header.Version)
	offset, i = offset + offsets[i], i + 1

	binary.LittleEndian.PutUint16(data[offset: offset + offsets[i]], msg.Header.Header.Size)
	offset, i = offset + offsets[i], i + 1

	data[offset] = msg.Header.Header.From
	offset, i = offset + offsets[i], i + 1

	data[offset] = msg.Header.Header.Status
	offset, i = offset + offsets[i], i + 1

	binary.LittleEndian.PutUint16(data[offset: offset + offsets[i]], msg.Header.Header.Cmd)
	offset, i = offset + offsets[i], i + 1

	binary.LittleEndian.PutUint64(data[offset: offset + offsets[i]], msg.Header.Header.ID)
	offset, i = offset + offsets[i], i + 1

	binary.LittleEndian.PutUint64(data[offset: offset + offsets[i]], msg.Header.Header.Imei)
	offset, i = offset + offsets[i], i + 1

	binary.LittleEndian.PutUint32(data[offset: offset + offsets[i]], msg.Header.ResumeHeader.TotalShardings)
	offset, i = offset + offsets[i], i + 1

	binary.LittleEndian.PutUint32(data[offset: offset + offsets[i]], msg.Header.ResumeHeader.TotalDataSize)
	offset, i = offset + offsets[i], i + 1

	binary.LittleEndian.PutUint32(data[offset: offset + offsets[i]], msg.Header.ResumeHeader.CurShardingIdx)
	offset, i = offset + offsets[i], i + 1

	binary.LittleEndian.PutUint32(data[offset: offset + offsets[i]], msg.Header.ResumeHeader.CurDataOffset)
	offset, i = offset + offsets[i], i + 1

	copy(data[offset: ], msg.Data)
	return data
}

func Decode(data []byte)  (*MsgData) {
	msg := &MsgData{}
	offset, i :=uint8(0), uint8(0)

	msg.Header.Header.Version = binary.LittleEndian.Uint16(data[offset: offset + offsets[i]])
	offset, i = offset + offsets[i], i + 1

	msg.Header.Header.Size = binary.LittleEndian.Uint16(data[offset: offset + offsets[i]])
	offset, i = offset + offsets[i], i + 1

	msg.Header.Header.From = data[offset]
	offset, i = offset + offsets[i], i + 1

	msg.Header.Header.Status = data[offset]
	offset, i = offset + offsets[i], i + 1

	msg.Header.Header.Cmd = binary.LittleEndian.Uint16(data[offset: offset + offsets[i]])
	offset, i = offset + offsets[i], i + 1

	msg.Header.Header.ID = binary.LittleEndian.Uint64(data[offset: offset + offsets[i]])
	offset, i = offset + offsets[i], i + 1

	msg.Header.Header.Imei = binary.LittleEndian.Uint64(data[offset: offset + offsets[i]])
	offset, i = offset + offsets[i], i + 1

	msg.Header.ResumeHeader.TotalShardings = binary.LittleEndian.Uint32(data[offset: offset + offsets[i]])
	offset, i = offset + offsets[i], i + 1

	msg.Header.ResumeHeader.TotalDataSize = binary.LittleEndian.Uint32(data[offset: offset + offsets[i]])
	offset, i = offset + offsets[i], i + 1

	msg.Header.ResumeHeader.CurShardingIdx = binary.LittleEndian.Uint32(data[offset: offset + offsets[i]])
	offset, i = offset + offsets[i], i + 1

	msg.Header.ResumeHeader.CurDataOffset = binary.LittleEndian.Uint32(data[offset: offset + offsets[i]])
	offset, i = offset + offsets[i], i + 1

	msg.Data = make([]byte, msg.Header.Header.Size)
	copy(msg.Data[0:], data[offset:])
	return msg
}

func IsDeviceInCompanyBlacklist(imei uint64) bool{
	DeviceInfoListLock.RLock()
	defer DeviceInfoListLock.RUnlock()

	deviceInfo, ok := (*DeviceInfoList)[imei]
	if ok {
		for i := 0; i < len(company_blacklist); i++ {
			if deviceInfo.Company == company_blacklist[i] {
				return true
			}
		}
	}

	return false
}

func GetSafeZoneSettings(imei uint64)  *[MAX_SAFE_ZONE_NUM]SafeZone{
	deviceInfo, ok := (*DeviceInfoList)[imei]
	if ok {
		return &deviceInfo.SafeZoneList
	}

	return nil
}

