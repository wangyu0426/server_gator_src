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
	SHARDING_SIZE  = 700     //分片大小为700字节，这是一个比较合适的经验值。
)

const (
	DRT_MIN = iota

	DRT_SYNC_TIME       // 同BP00，手表请求对时
	DRT_SEND_LOCATION      // 同BP30，手表上报定位(报警)数据
	DRT_DEVICE_LOGIN    	   // 同BP31，手表登录服务器
	DRT_DEVICE_ACK	    	   // 同BP04，手表对服务器请求的应答
	DRT_MONITOR_ACK	   // 同BP05，手表对服务器请求电话监听的应答
	DRT_SEND_MINICHAT      // 同BP34，手表发送语音微聊
	DRT_FETCH_MINICHAT    // 同BP11，手表获取语音微聊
	DRT_FETCH_AGPS        	  // 同BP32，手表获取AGPS数据
	DRT_HEART_BEAT       	  // 同BP33，手表发送状态数据

	DRT_MAX
)
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
	 CMD_AP30
	 CMD_AP31
 )



var commands = []string{
	"",
	"BP00",
	"BP30",
	"BP31",
	"BP04",
	"BP05",
	"BP34",
	"BP11",
	"BP32",
	"BP33",
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
                                  0 - 表示完成并OK；如果是不做分片和断点续传的请求，则status应始终赋值为0
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
	ID uint64
	Cmd string
	Imei uint64
	Data []byte
	Conn interface{}
}


type IPInfo struct {
	StartIP uint32
	EndIP uint32
	TimeZone int32
}

type EPOInfo struct {
	EPOBuf[]byte
}

const (
	DM_MIN = -1

	DM_WH01 = iota
	DM_GT03
	DM_GTI3
	DM_GT06

	DM_MAX
)

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
	Phone string
	Name string
	Avatar string
	Type int
	Index int
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
	Name string
	Company string
	CountryCode string
	Lang string
	Volume uint8
	CanTurnOff bool
	UseDST bool
	SocketModeOff bool
	WatchAlarmList [MAX_WATCH_ALARM_NUM]WatchAlarm
	SafeZoneList [MAX_SAFE_ZONE_NUM]SafeZone
	Family [MAX_FAMILY_MEMBER_NUM]FamilyMember
	HideTimerOn bool
	HideTimerList [MAX_HIDE_TIMER_NUM]HideiTimer
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
var ipinfoListLock sync.RWMutex
var StartGPSHour uint32
var EPOInfoList []*EPOInfo
var EpoInfoListLock sync.RWMutex
var DeviceInfoList = &map[uint64]*DeviceInfo{}
var DeviceInfoListLock sync.RWMutex
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
	//os.Exit(0)

	LoadIPInfosFromFile()
	LoadEPOFromFile()
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

func LoadEPOFromFile()  {

	mtk30,err := os.Open("./EPO/MTK30.EPO")
	if err != nil {
		panic(err)
	}
	defer mtk30.Close()

	epo36h, err := os.OpenFile("./EPO/36H.EPO", os.O_CREATE | os.O_WRONLY | os.O_TRUNC, 0666)
	if err != nil {
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
}

func LoadDeviceInfoFromDB(dbpool *sql.DB)  bool{
	rows, err := dbpool.Query("select w.IMEI, w.OwnerName, w.PhoneNumbers, w.TimeZone, w.CountryCode, w.ChildPowerOff, w.UseDST, w.SocketModeOff, w.Volume, w.Lang, w.Fence1,w.Fence2, w.Fence3,w.Fence4,w.Fence5,w.Fence6,w.Fence7,w.Fence8,w.Fence9,w.Fence10, w.WatchAlarm0, w.WatchAlarm1, w.WatchAlarm2,w.WatchAlarm3, w.WatchAlarm4,w.HideSelf,w.HideTimer0,w.HideTimer1,w.HideTimer2,w.HideTimer3, pm.model, c.name from watchinfo w join device d on w.recid=d.recid join productmodel pm  on d.modelid=pm.recid join companies c on d.companyid=c.recid ")
	if err != nil {
		fmt.Println("LoadDeviceInfoFromDB failed,", err.Error())
		os.Exit(1)
	}

	var(
		IMEI      		string
		OwnerName  		string
		PhoneNumbers   	string
		TimeZone		string
		CountryCode 		string
		ChildPowerOff		uint8
		UseDST		uint8
		SocketModeOff	uint8
		Volume		uint8
		Lang			string

		Fences		=	[MAX_SAFE_ZONE_NUM]string{}
		WatchAlarms = [MAX_WATCH_ALARM_NUM]string{}

		HideSelf		uint8
		HideTimers = [MAX_HIDE_TIMER_NUM]string{}

		Model 			string
		Company 		 string
	)

	tmpDeviceInfoList := &map[uint64]*DeviceInfo{}
	for rows.Next() {
		deviceInfo := &DeviceInfo{}
		rows.Scan(&IMEI, &OwnerName, &PhoneNumbers, &TimeZone, &CountryCode, &ChildPowerOff, &UseDST, &SocketModeOff, &Volume, &Lang,&Fences[0], &Fences[1], &Fences[2], &Fences[3], &Fences[4], &Fences[5], &Fences[6], &Fences[7], &Fences[8], &Fences[9], &WatchAlarms[0], &WatchAlarms[1],&WatchAlarms[2], &WatchAlarms[3], &WatchAlarms[4], &HideSelf, &HideTimers[0], &HideTimers[1], &HideTimers[2], &HideTimers[3], &Model, &Company)
		deviceInfo.Imei = Str2Num(IMEI, 10)
		deviceInfo.Name = OwnerName
		ParseFamilyMembers(PhoneNumbers, &deviceInfo.Family)
		deviceInfo.TimeZone  = ParseTimeZone(TimeZone)
		deviceInfo.CountryCode = CountryCode
		deviceInfo.CanTurnOff = ChildPowerOff != 0
		deviceInfo.UseDST = UseDST != 0
		deviceInfo.SocketModeOff = SocketModeOff != 0
		deviceInfo.Volume = Volume
		deviceInfo.Lang = Lang
		ParseSafeZones(Fences, &deviceInfo.SafeZoneList)
		ParseWatchAlarms(WatchAlarms, &deviceInfo.WatchAlarmList)
		deviceInfo.HideTimerOn = HideSelf != 0
		ParseHideTimers(HideTimers, &deviceInfo.HideTimerList)
		deviceInfo.Model = ParseDeviceModel(Model)
		deviceInfo.Company = Company

		(*tmpDeviceInfoList)[deviceInfo.Imei] = deviceInfo
		//fmt.Println("deviceInfo: ", *deviceInfo)
	}

	DeviceInfoListLock.Lock()
	DeviceInfoList = tmpDeviceInfoList
	DeviceInfoListLock.Unlock()
	return true
}

func ParseDeviceModel(model string) int {
	switch model {
	case "WH01":
		return DM_WH01
	case "GT03":
		return DM_GT03
	case "GTI3":
		return DM_GTI3
	case "GT06":
		return DM_GT06
	default:
		return -1
	}

	return -1
}

func ParseSafeZones(zones [MAX_SAFE_ZONE_NUM]string, safeZoneList *[MAX_SAFE_ZONE_NUM]SafeZone)  {
//	{"Radius":200,"Name":"home","Center":"22.588015574341,113.91333157419001","On":"1","Wifi":{"SSID":"gatorgroup","BSSID":"d8:24:bd:77:4d:6e"}}
	for i, zone := range zones  {
		err:=json.Unmarshal([]byte(zone), &safeZoneList[i])
		if err != nil {
			continue
		}
	}
}

func ParseWatchAlarms(alarms [MAX_WATCH_ALARM_NUM]string, alarmList *[MAX_WATCH_ALARM_NUM]WatchAlarm)  {
	for i, alarm := range alarms  {
		err:=json.Unmarshal([]byte(alarm), &alarmList[i])
		if err != nil {
			continue
		}
	}
}

func ParseHideTimers(timers [MAX_HIDE_TIMER_NUM]string, hidetimerList *[MAX_HIDE_TIMER_NUM]HideiTimer)  {
	for i, timer := range timers  {
		err:=json.Unmarshal([]byte(timer), &hidetimerList[i])
		if err != nil {
			continue
		}
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

		fields := strings.SplitAfter(m, "|")
		//fmt.Println(m, len(fields), fields)
		if len(fields) >= 1 && len(fields[0]) > 0 && fields[0] != "|"{
			if fields[0][len(fields[0]) - 1] == '|' {
				familyMemberList[i].Phone = string(fields[0][0: len(fields[0]) - 2])
			}else {
				familyMemberList[i].Phone = fields[0]
			}
		}

		if len(fields) >= 2 && len(fields[1]) > 0 && fields[1] != "|" {
			if fields[1][len(fields[1]) - 1] == '|' {
				familyMemberList[i].Type = int(Str2Num(string(fields[1][0: len(fields[1]) - 2]), 10))
			}else {
				familyMemberList[i].Type = int(Str2Num(fields[1], 10))
			}
		}

		if len(fields) >= 3 && len(fields[2]) > 0 && fields[2] != "|" {
			//fmt.Println("fields2: ", fields, fields[2], len(fields[2]))
			if fields[2][len(fields[2]) - 1] == '|' {
				familyMemberList[i].Name = string(fields[2][0: len(fields[2]) - 2])
			}else {
				familyMemberList[i].Name = fields[2]
			}
		}

		familyMemberList[i].Index = i + 1
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

