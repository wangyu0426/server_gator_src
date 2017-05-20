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
)

const (
	ImeiLen = 15
	CmdLen = 4
	MTKEPO_DATA_ONETIME =12
	MTKEPO_SV_NUMBER   = 32
	MTKEPO_RECORD_SIZE    =  72
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
	SHARDING_SIZE  = 640     //分片大小为640字节，这是一个比较合适的经验值。
)

const (
	DRT_MIN = iota

	DRT_SYNC_TIME       // 同BP00，手表请求对时
	DRT_SEND_LOCATION      // 同BP30，手表上报定位(报警)数据
	DRT_SEND_MINICHAT      // 同BP34，手表发送语音微聊
	DRT_FETCH_MINICHAT     // 同BP07，手表获取语音微聊
	DRT_FETCH_AGPS         // 同BP08，手表获取AGPS数据

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
	 CMD_AP30
 )



var commands = []string{
	"",
	"BP00",
	"BP30",
	"BP02",
	"BP34",
	"BP07",
	"BP08",
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

type SafeZone struct {
	ZoneID int32
	ZoneName string
	LatiTude float64
	LongTitude float64
	Radiu uint32
	WifiMACID string
	Flags int32
}

type FamilyMember struct {
	Phone string
	Name string
	Avatar string
	Type int
	Index int
}

type DeviceInfo struct {
	Imei uint64
	Model uint8
	TimeZone int
	Name string
	Company string
	Lang string
	Volume int
	CanTurnOff bool
	SafeZoneList []*SafeZone
	Family []*FamilyMember
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
var DeviceInfoList = map[uint64]*DeviceInfo{}
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
	//os.Exit(1)

	LoadIPInfosFromFile()
	LoadEPOFromFile()
	LoadDeviceInfoFromDB()
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

	return -100
}

func LoadEPOFromFile()  {

	mtk30,err := os.Open("./EPO/MTK30.EPO")
	if err != nil {
		panic(err)
	}
	defer mtk30.Close()

	epo36h, err := os.OpenFile("./EPO/36H.EPO", os.O_APPEND | os.O_CREATE | os.O_WRONLY, 0666)
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

func LoadDeviceInfoFromDB()  {
	deviceInfo := &DeviceInfo{}
	deviceInfo.Imei = 357593060571398
	deviceInfo.Model = DM_GT06
	deviceInfo.Company = "Caref Watch Co,.Ltd"

	tmpDeviceInfoList := map[uint64]*DeviceInfo{}
	tmpDeviceInfoList[deviceInfo.Imei] = deviceInfo

	DeviceInfoListLock.Lock()
	DeviceInfoList = tmpDeviceInfoList
	DeviceInfoListLock.Unlock()
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

	deviceInfo, ok := DeviceInfoList[imei]
	if ok {
		for i := 0; i < len(company_blacklist); i++ {
			if deviceInfo.Company == company_blacklist[i] {
				return true
			}
		}
	}

	return false
}

func GetSafeZoneSettings(imei uint64)  []*SafeZone{
	deviceInfo, ok := DeviceInfoList[imei]
	if ok {
		return deviceInfo.SafeZoneList
	}

	return nil
}

