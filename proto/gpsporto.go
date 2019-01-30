package proto

import (
	"../logging"
	"fmt"
	"time"
	"encoding/hex"
	"strings"
	"io/ioutil"
	"net/http"
	"bytes"
	"encoding/json"
)

type GPSService struct {
	imei uint64
	msgSize uint64
	cmd uint16
	isCmdForAck bool
	msgAckId uint64
	old LocationData
	cur LocationData
	fetchType string
	needSendLocation bool

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
	rspList []*ResponseItem
	reqCtx RequestContext
}

type GPSAPNInfo struct{
	Apn,
	UserName,
	PassWord string
}

func (service *GPSService) PreDorequest() bool {
	service.wifiZoneIndex = -1
	service.cur.ZoneIndex = -1
	service.old.ZoneIndex = -1
	service.cur.LastZoneIndex = -1
	service.old.LastZoneIndex = -1
	service.isCmdForAck = false

	return true
}

func (service *GPSService) GetWatchDataInfo(imei uint64)  {
	if service.reqCtx.GetDeviceDataFunc != nil {
		service.old = service.reqCtx.GetDeviceDataFunc(imei, service.reqCtx.Pgpool)
	}
}

/*服务器触发的下行消息*/
//设置立即定位,设置读取终端版本消息,设置清空所有者号码,控制设备重启消息,设置终端恢复出厂设置
func MakeReplyGPSMsg(imei uint64,protocl uint16,length uint16) ([]byte,uint64) {
	body := make([]byte,length)
	body[0] = 0x24		//起始位
	body[1] = 0x24
	body[2] = byte(length >> 8 & 0xFF)		//包长度
	body[3] = byte(length & 0xFF)
	strimei := Num2Str(imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}
	//协议号
	body[12] = uint8(protocl >> 8 & 0xFF)
	body[13] = uint8(protocl & 0xFF)
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}
	body[20] = 0x0D
	body[21] = 0x0A
	id := Str2Num(strtime[4:],16)

	return body,id
}

func MakeSetAPNReplyMsg(imei uint64,APNInfo *[]GPSAPNInfo) ([]byte,uint64) {
	body := make([]byte,1024)
	body[0] = 0x24		//起始位
	body[1] = 0x24
	strimei := Num2Str(imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}
	//协议号
	body[12] = 0x44
	body[13] = 0x02
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}
	//apn info
	buffoffset := 20
	for index,_ := range *APNInfo{
		lenApn := len((*APNInfo)[index].Apn)
		body[buffoffset] = byte(lenApn)
		buffoffset++
		copy(body[buffoffset:buffoffset + lenApn],(*APNInfo)[index].Apn)
		buffoffset += lenApn
		lenuser := len((*APNInfo)[index].UserName)
		body[buffoffset] = byte(lenuser)
		buffoffset++
		if lenuser > 0{
			copy(body[buffoffset:buffoffset + lenuser],(*APNInfo)[index].UserName)
		}
		buffoffset += lenuser
		lenPasswd := len((*APNInfo)[index].PassWord)
		body[buffoffset] = byte(lenPasswd)
		buffoffset++
		if lenPasswd > 0{
			copy(body[buffoffset:buffoffset + lenPasswd],(*APNInfo)[index].PassWord)
		}
		buffoffset += lenPasswd
	}

	result := make([]byte,buffoffset + 2)
	copy(result[0:],body)
	result[buffoffset] = 0x0D
	result[buffoffset + 1] = 0x0A
	id := Str2Num(strtime[4:],16)

	return result,id
}

func MakeSetPhoneNumbers(imei uint64) ([]byte,uint64){
	body := make([]byte,256)
	body[0] = 0x24		//起始位
	body[1] = 0x24
	strimei := Num2Str(imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}

	//协议号
	body[12] = 0x44
	body[13] = 0x06
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}

	bufoffset := 20
	DeviceInfoListLock.Lock()
	deviceinfo,ok := (*DeviceInfoList)[imei]
	if ok && deviceinfo != nil{
		model := deviceinfo.Model
		PhoneNumbers := MakeDeviceFamilyPhoneNumbersEx(&deviceinfo.Family,imei,model)
		PhoneList := strings.Split(PhoneNumbers,"#")
		for _,mem := range PhoneList{
			body[bufoffset] = byte(len(mem))
			bufoffset++
			if len(mem) == 0{
				continue
			}
			copy(body[bufoffset:bufoffset + len(mem)],mem)
			bufoffset += len(mem)
		}
	}
	DeviceInfoListLock.Unlock()

	body[bufoffset] = 0x0D
	bufoffset++
	body[bufoffset] = 0x0A

	result := make([]byte,bufoffset + 1)
	copy(result,body)
	result[2] = byte((bufoffset + 1) >> 8 & 0xFF)
	result[3] = byte((bufoffset + 1) & 0xFF)
	id := Str2Num(strtime[4:],16)

	return result,id
}

func MakeSetBuzzerMsg(imei uint64,setting string) ([]byte,uint64) {
	body := make([]byte,24)
	body[0] = 0x24		//起始位
	body[1] = 0x24
	body[2] = 0		//起始位
	body[3] = 0x18
	strimei := Num2Str(imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}
	//协议号
	body[12] = 0x44
	body[13] = 0x23
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}
	sets := strings.Split(setting,"|")
	if len(sets) == 2 {
		buzzer := sets[0]
		lamp := sets[1]
		if buzzer == "1" {
			body[20] = byte(Str2Num(buzzer, 10)) << 7
		}
		if lamp == "1" {
			body[21] = byte(Str2Num(lamp, 10)) << 7
		}
	}
	body[22] = 0x0D
	body[23] = 0x0A
	id := Str2Num(strtime[4:],16)
	return body,id
}

func (service *GPSService) MakeSetIPPortReplyMsg() ([]byte,uint64) {
	body := make([]byte,256)
	body[0] = 0x24		//起始位
	body[1] = 0x24

	strimei := Num2Str(service.imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}
	//协议号
	body[12] = 0x44
	body[13] = 0x01
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}
	var length int = 0
	DeviceInfoListLock.Lock()
	device, ok := (*DeviceInfoList)[service.imei]
	if ok{
		length = len(device.CompanyHost)
		body[20] = byte(length)
		copy(body[21:],device.CompanyHost)
		body[21 + length] = byte(device.CompanyPort >> 8 & 0xFF)
		body[21 + length + 1] = byte(device.CompanyPort & 0xFF)
	}
	DeviceInfoListLock.Unlock()

	result := make([]byte,23 + length + 2)
	copy(result,body)
	result[2] = byte((25 + length) >> 8 & 0xFF)
	result[3] = byte((25 + length) & 0xFF)
	result[23 + length] = 0x0D
	result[24 + length] = 0x0A
	id := Str2Num(strtime[4:],16)

	return result,id
}

//夏令时设置消息,固件升级设置
func  MakeSetUseDSTMsg(imei uint64,protcol uint16,flag string) ([]byte,uint64) {
	body := make([]byte,23)
	body[0] = 0x24		//起始位
	body[1] = 0x24
	body[2] = 0x00		//包长度
	body[3] = 0x17
	strimei := Num2Str(imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}
	//协议号
	body[12] = byte(protcol >> 8 & 0xff)
	body[13] = byte(protcol & 0xff)
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}
	body[20] = byte(Str2Num(flag,10))
	body[21] = 0x0D
	body[22] = 0x0A
	id := Str2Num(strtime[4:],16)

	return body,id
}

//设置监听命令
func MakeSetListenMsg(imei uint64,phone string) ([]byte,uint64){
	body := make([]byte,256)
	body[0] = 0x24		//起始位
	body[1] = 0x24

	strimei := Num2Str(imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}

	//协议号
	body[12] = 0x44
	body[13] = 0x05
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}
	body[20] = byte(len(phone))
	copy(body[21:],phone)
	result := make([]byte,23 + len(phone))
	copy(result,body)
	result[2] = byte((23 + len(phone)) >> 8 & 0xFF)
	result[3] = byte((23 + len(phone)) & 0xFF)
	result[21 + len(phone)] = 0x0D
	result[22 + len(phone)] = 0x0A
	id := Str2Num(strtime[4:],16)

	return result,id
}

//报警设置消息
func MakeSetAlarmMsg(imei uint64,setting string) ([]byte,uint64){
	body := make([]byte,24)
	body[0] = 0x24		//起始位
	body[1] = 0x24
	body[2] = 0x00		//包长度
	body[3] = 0x18
	strimei := Num2Str(imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}
	//协议号
	body[12] = 0x44
	body[13] = 0x21
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}
	//Bit0 0：超速报警关闭 1：超速报警开启
	strAlarmSet := strings.Split(setting,"|")
	if len(strAlarmSet) >= 2 {
		if strAlarmSet[0] == "0" {
			body[20] &= 0x00
		} else {
			body[20] |= 0x01
		}
		body[21] = byte(Str2Num(strAlarmSet[1],10))
	}
	body[22] = 0x0D
	body[23] = 0x0A
	id := Str2Num(strtime[4:],16)

	return body,id
}

// 修改时区、时间
func (service *GPSService) MakeSyncReplyMsg() ([]byte,uint64) {
	curtime := time.Now().UTC().Format("060102150411")
	timezone := 0
	DeviceInfoListLock.Lock()
	device, ok := (*DeviceInfoList)[service.imei]
	if ok && device != nil{
		timezone = device.TimeZone
	}
	DeviceInfoListLock.Unlock()
	
	if timezone == INVALID_TIMEZONE{
		timezone = int(GetTimeZone(service.reqCtx.IP))
		logging.Log(fmt.Sprintf("imei %d, IP %d  GetTimeZone: %d", service.imei, service.reqCtx.IP, timezone))
		service.UpdateDeviceTimeZone(service.imei,timezone)
	}
	bTz := true
	if timezone < 0 {
		timezone = -timezone
		bTz = false
	}

	body := make([]byte,30)
	body[0] = 0x24		//起始位
	body[1] = 0x24
	body[2] = 0x00		//包长度
	body[3] = 0x1E
	strimei := Num2Str(service.imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}
	//协议号
	body[12] = 0x44
	body[13] = 0x03
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}
	//UTC日期时间
	body[20] = byte(Str2Num(curtime[0:2],10))
	body[21] = byte(Str2Num(curtime[2:4],10))
	body[22] = byte(Str2Num(curtime[4:6],10))
	body[23] = byte(Str2Num(curtime[6:8],10))
	body[24] = byte(Str2Num(curtime[8:10],10))
	body[25] = byte(time.Now().Second())
	//时区
	_tzone := timezone << 4 & 0xFFFF
	if bTz == false {
		_tzone |= 0x08
	}

	body[26] = byte(_tzone >> 8 & 0xFF)
	body[27] = byte(_tzone & 0xFF)
	//停止位
	body[28] = 0x0D
	body[29] = 0x0A
	id := Str2Num(strtime[4:],16)

	return body,id
}

func  MakeSetTimeZoneMsg(imei uint64) ([]byte,uint64) {
	curtime := time.Now().UTC().Format("060102150411")
	timezone := 0
	DeviceInfoListLock.Lock()
	device, ok := (*DeviceInfoList)[imei]
	if ok && device != nil{
		timezone = device.TimeZone
	}
	DeviceInfoListLock.Unlock()

	bTz := true
	if timezone < 0 {
		timezone = -timezone
		bTz = false
	}

	body := make([]byte,30)
	body[0] = 0x24		//起始位
	body[1] = 0x24
	body[2] = 0x00		//包长度
	body[3] = 0x1E
	strimei := Num2Str(imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}
	//协议号
	body[12] = 0x44
	body[13] = 0x03
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}
	//UTC日期时间
	body[20] = byte(Str2Num(curtime[0:2],10))
	body[21] = byte(Str2Num(curtime[2:4],10))
	body[22] = byte(Str2Num(curtime[4:6],10))
	body[23] = byte(Str2Num(curtime[6:8],10))
	body[24] = byte(Str2Num(curtime[8:10],10))
	body[25] = byte(time.Now().Second())
	//时区
	_tzone := timezone << 4 & 0xFFFF
	if bTz == false {
		_tzone |= 0x08
	}

	body[26] = byte(_tzone >> 8 & 0xFF)
	body[27] = byte(_tzone & 0xFF)
	//停止位
	body[28] = 0x0D
	body[29] = 0x0A
	id := Str2Num(strtime[4:],16)

	return body,id
}

//定位数据设置消息
func (service *GPSService) makeSendLocationReplyMsg() ([]byte, uint64) {
	curtime := time.Now().UTC()
	body := make([]byte,256)
	body[0] = 0x24		//起始位
	body[1] = 0x24
	strimei := Num2Str(service.imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}
	//协议号
	body[12] = 0x44
	body[13] = 0x0E
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}
	strlocate := fmt.Sprintf("%f,%f,%d,%04d,%02d,%02d,%02d,%02d,%02d",
		service.cur.Lat,service.cur.Lng,180,curtime.Year(),curtime.Month(),curtime.Day(),curtime.Hour(),curtime.Minute(),curtime.Second())
	body[20] = byte(len(strlocate))
	copy(body[21:21 + len(strlocate)],strlocate)

	result := make([]byte,21 + len(strlocate) + 2)
	copy(result[:],body)
	result[23 + len(strlocate) - 2] = 0x0D
	result[23 + len(strlocate) - 1] = 0x0A
	result[2] = byte((23 + len(strlocate)) >> 8 & 0xFF)
	result[3] = byte((23 + len(strlocate)) & 0xFF)
	id := Str2Num(strtime[4:],16)

	return result,id
}

//设备定位模式设置消息
func  MakeSetLocatModelMsg(imei uint64,setting string) ([]byte,uint64) {
	body := make([]byte,23)
	body[0] = 0x24		//起始位
	body[1] = 0x24
	body[2] = 0x00		//包长度
	body[3] = 0x17

	strimei := Num2Str(imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}
	//协议号
	body[12] = 0x44
	body[13] = 0x22
	//服务器流水号
	strtime := fmt.Sprintf("%X",time.Now().UnixNano())
	if len(strtime) >= 16{
		body[14] = byte(Str2Num(strtime[4:6],16))
		body[15] = byte(Str2Num(strtime[6:8],16))
		body[16] = byte(Str2Num(strtime[8:10],16))
		body[17] = byte(Str2Num(strtime[10:12],16))
		body[18] = byte(Str2Num(strtime[12:14],16))
		body[19] = byte(Str2Num(strtime[14:],16))
	}
	//指令内容
	/*
	Bit7
	0：开启被动定位模式
	1：开启主动定位模式
	Bit6
	Bit6 ~ Bit0：主动定位模式，定位数据上传间隔，单位为秒*/
	strModel := strings.Split(setting,"|")
	if len(strModel) == 2 {
		body[20] = 0
		if strModel[0] == "1" {
			body[20] |= (1 << 7)
		}
		body[20] |= (byte(Str2Num(strModel[1],10)) & 0x7F)
	}
	body[21] = 0x0D
	body[22] = 0x0A
	id := Str2Num(strtime[4:],16)

	return body,id
}

func (service *GPSService) UpdateDeviceTimeZone(imei uint64, timezone int) bool {
	DeviceInfoListLock.Lock()
	_,ok := (*DeviceInfoList)[imei]
	if ok{
		(*DeviceInfoList)[service.imei].TimeZone = timezone
	}
	DeviceInfoListLock.Unlock()

	//时区写入数据库
	szTimeZone, signed,hour, min := "", "", int(timezone / 100), timezone % 100
	if timezone > 0{
		signed = "+"
	}else {
		signed = "-"
		if timezone == 0 {
			signed = ""
		}
		hour,min = int(-timezone / 100), -timezone % 100
	}

	szTimeZone = fmt.Sprintf("%s%02d:%02d",signed,hour,min)
	strSQL := fmt.Sprintf("UPDATE watchinfo SET TimeZone='%s'  where IMEI='%d'",szTimeZone,service.imei)
	logging.Log("SQL: " + strSQL)
	_,err := service.reqCtx.MysqlPool.Exec(strSQL)
	if err != nil{
		logging.Log(fmt.Sprintf("[%d] update time zone into db failed, %s", service.imei, err.Error()))
		return false
	}

	return true
}

func (service *GPSService)makeReplyMsg(requireAck bool, data []byte, id uint64) *MsgData{
	return MakeReplyMsg(service.imei, requireAck, data, id)
}

func (service *GPSService) MakeHeartReplyMsg() ([]byte,uint64) {
	curtime := time.Now().UTC().Format("060102150411")
	id := makeId()
	body := make([]byte,22)
	body[0] = 0x24		//起始位
	body[1] = 0x24
	body[2] = 0x00		//包长度
	body[3] = 0x16
	strimei := Num2Str(service.imei,10)
	if len(strimei) == 15{
		//终端ID
		body[4] = strimei[0] - '0'
		body[5] = byte(Str2Num(strimei[1:3],16))
		body[6] = byte(Str2Num(strimei[3:5],16))
		body[7] = byte(Str2Num(strimei[5:7],16))
		body[8] = byte(Str2Num(strimei[7:9],16))
		body[9] = byte(Str2Num(strimei[9:11],16))
		body[10] = byte(Str2Num(strimei[11:13],16))
		body[11] = byte(Str2Num(strimei[13:],16))
	}
	//协议号
	body[12] = 0x44
	body[13] = 0x3F
	//UTC日期时间
	body[14] = byte(Str2Num(curtime[0:2],16))
	body[15] = byte(Str2Num(curtime[2:4],16))
	body[16] = byte(Str2Num(curtime[4:6],16))
	body[17] = byte(Str2Num(curtime[6:8],16))
	body[18] = byte(Str2Num(curtime[8:10],16))
	body[19] = byte(Str2Num(curtime[10:],16))
	body[20] = 0x0D
	body[21] = 0x0A

	return body,id
}

func (service *GPSService) Dorequest(msg *MsgData) bool {
	ret := true
	buffoffset := 0
	service.msgSize = uint64(msg.Header.Header.Size)
	service.imei = msg.Header.Header.Imei
	service.cmd  = msg.Header.Header.Cmd
	service.cur.Imei = service.imei

	if IsDeviceInCompanyBlacklist(service.imei){
		logging.Log(fmt.Sprintf("gps device %d  is in the company black list", service.imei))
		return false
	}
	DeviceInfoListLock.Lock()
	deviceInfo,ok := (*DeviceInfoList)[msg.Header.Header.Imei]
	if ok == false  || deviceInfo == nil{
		DeviceInfoListLock.Unlock()
		logging.Log(fmt.Sprintf("gps invalid deivce, imei: %d cmd: %s", msg.Header.Header.Imei, StringGpsCmd(msg.Header.Header.Cmd)))
		return false
	}
	if (deviceInfo.RedirectIPPort || deviceInfo.RedirectServer) && (deviceInfo.CompanyPort != 0 && deviceInfo.CompanyHost != ""){
		madeData,id := service.MakeSetIPPortReplyMsg()
		resp := &ResponseItem{CMD_GPS_SETIPPORT,service.makeReplyMsg(false,madeData,id)}
		service.rspList = append(service.rspList,resp)
	}
	DeviceInfoListLock.Unlock()
	//当前的版本比设备上传的版本da,升级
	if deviceInfo.DevVer == "1"{
		madeData, id := MakeSetUseDSTMsg(service.imei, 0x4424, "1")
		logging.Log(fmt.Sprintf("[%d]gps now updating...%s",service.imei,deviceInfo.DevVer))
		resp := &ResponseItem{CMD_GPS_SETUPDATE, service.makeReplyMsg(true, madeData, id)}
		service.rspList = append(service.rspList, resp)
		deviceInfo.DevVer = "0"
		strSQL := fmt.Sprintf("update watchinfo set DevVer = '0' where IMEI = '%d'",service.imei)
		logging.Log("SQL: " + strSQL)
		_, err := service.reqCtx.MysqlPool.Exec(strSQL)
		if err != nil {
			logging.Log(fmt.Sprintf("[%d] update watchinfo set DevVer into db failed, %s", service.imei, err.Error()))
			return  false
		}
	}


	service.GetWatchDataInfo(service.imei)
	logging.Log(fmt.Sprintf("DoRequest gps msg.cmd,msg.Data:%d--%x",msg.Header.Header.Cmd,msg.Data))
	if service.cmd == CMD_GPS_SETIPPORT_ACK ||
		service.cmd == 	CMD_GPS_SETAPN_ACK ||
		service.cmd == 	CMD_GPS_SETTIMEZONE_ACK ||
		service.cmd == 	CMD_GPS_SETLISTEN_ACK ||
		service.cmd == 	CMD_GPS_SETPHONE_ACK ||
		service.cmd == 	CMD_GPS_CLEANPHONE_ACK ||
		service.cmd == 	CMD_GPS_SETDEVRESTART_ACK ||
		service.cmd == 	CMD_GPS_SETLOCATION_ACK ||
		service.cmd == 	CMD_GPS_SETSUMMER_ACK ||
		service.cmd == 	CMD_GPS_RECOVER_ACK ||
		service.cmd == 	CMD_GPS_SETALARM_ACK ||
		service.cmd == 	CMD_GPS_LOCATIONSTYLE_ACK ||
		service.cmd == CMD_GPS_SETBUZZER_ACK {
		if msg.Data[buffoffset + 8] == 0{
			//success,不同于GT06的msgid和服务器流水号相同,gpsgator的msgid取协议里面的
			strId := fmt.Sprintf("%02x%02x%02x%02x%02x%02x",msg.Data[2],msg.Data[3],msg.Data[4],msg.Data[5],msg.Data[6],msg.Data[7])
			service.msgAckId = Str2Num(strId,16)
			fmt.Printf("service.cmd:%d,%s\n",service.cmd,strId)
			resp := &ResponseItem{CMD_ACK,service.MakeAckParsedMsg(service.msgAckId)}
			service.rspList = append(service.rspList,resp)
		}else{
			//failure
		}

		if service.cmd == CMD_GPS_SETBUZZER_ACK{
			result := HttpAPIResult{
				ErrCode: 500,
				ErrMsg: "failed to set buzzer",
				Imei: Num2Str(service.imei, 10),
			}
			if msg.Data[buffoffset + 8] == 0{
				result.ErrCode = 0
				result.ErrMsg = ""
				service.reqCtx.AppNotifyChan <- &AppMsgData{Cmd:SetDevBuzzerAck,Imei:service.imei,Data:MakeStructToJson(result),ConnID:0}
			}else {
				service.reqCtx.AppNotifyChan <- &AppMsgData{Cmd:SetDevBuzzerAck,Imei:service.imei,Data:MakeStructToJson(result),ConnID:0}
			}
		}

	}else if service.cmd == CMD_GPS_READVER_ACK{
		strId := fmt.Sprintf("%02x%02x%02x%02x%02x%02x",msg.Data[2],msg.Data[3],msg.Data[4],msg.Data[5],msg.Data[6],msg.Data[7])
		service.msgAckId = Str2Num(strId,16)
		resp := &ResponseItem{CMD_ACK,service.MakeAckParsedMsg(service.msgAckId)}
		service.rspList = append(service.rspList,resp)
		lenVer := msg.Data[buffoffset + 8]
		var verInfo []byte
		if lenVer > 0 {
			verInfo = make([]byte, lenVer)
		}
		copy(verInfo,msg.Data[buffoffset + 9:buffoffset + 9 + int(lenVer)])
		result := ReadVersionResult{Imei:service.imei,DevVersion:string(verInfo)}
		service.reqCtx.AppNotifyChan <- &AppMsgData{Cmd:ReadVersionAck,Imei:service.imei,Data:MakeStructToJson(result),ConnID:0}
	} else if service.cmd == CMD_GPS_SYNC{
		madeData, id := service.MakeSyncReplyMsg()
		fmt.Printf("madeData:%x\n",madeData)
		resp := &ResponseItem{CMD_GPS_SET_SYNC,service.makeReplyMsg(true,madeData,id)}
		service.rspList = append(service.rspList,resp)
		//
		lenVer := msg.Data[buffoffset + 14]
		var verInfo []byte
		if lenVer > 0{
			verInfo = make([]byte,lenVer)
			copy(verInfo,msg.Data[buffoffset + 15:buffoffset + 15 + int(lenVer)])
			result := ReadVersionResult{Imei:service.imei,DevVersion:string(verInfo)}
			service.reqCtx.AppNotifyChan <- &AppMsgData{Cmd:ReadVersionAck,Imei:service.imei,Data:MakeStructToJson(result),ConnID:0}
		}
	}else if service.cmd == CMD_GPS_SETUPDATE_ACK{
		if msg.Data[buffoffset + 8] == 0 {
			strId := fmt.Sprintf("%02x%02x%02x%02x%02x%02x",msg.Data[2],msg.Data[3],msg.Data[4],msg.Data[5],msg.Data[6],msg.Data[7])
			service.msgAckId = Str2Num(strId,16)
			fmt.Printf("service.cmd:%d,%s\n",service.cmd,strId)
			resp := &ResponseItem{CMD_ACK,service.MakeAckParsedMsg(service.msgAckId)}
			service.rspList = append(service.rspList,resp)
		}
	}else if service.cmd == CMD_GPS_LOCATION{
		if !(msg.Data[buffoffset] == 0x55 && msg.Data[buffoffset + 1] == 0X3C){
			logging.Log("gps msg is not location data")
			ret = false
			return ret
		}

		buffoffset += 2
		year := msg.Data[buffoffset]
		buffoffset++
		month := msg.Data[buffoffset]
		buffoffset++
		day := msg.Data[buffoffset]
		buffoffset++
		hour := msg.Data[buffoffset]
		buffoffset++
		min := msg.Data[buffoffset]
		buffoffset++
		sec := msg.Data[buffoffset]
		strTime := fmt.Sprintf("%02d%02d%02d%02d%02d%02d",year,month,day,hour,min,sec)
		i64Time := Str2Num(strTime,10)
		service.cur.DataTime = i64Time

		/*终端状态信息
		位
		代码含义
		BYTE_1
		Bit7 0
		Bit6 0
		Bit5 0
		Bit4 0
		Bit3 0
		Bit2 0
		Bit1 0
		Bit0 0：夏令时关闭
		1：夏令时开启
		BYTE_2
		Bit7
		0：超速报警模式关闭
		1：超速报警模式开启
		Bit6
		0：被动定位模式开启
		1：主动定位模式开启
		Bit3～Bit5
		电压等级(0-7):
		000：低电关机
		001：呼叫限制
		010：低电报警
		011: 0格显示
		100: 1格显示
		101：2格显示
		110: 3格显示
		111: 4格显示
		Bit0～Bit2
	GSM信号强度:
		000：无信号
		001：信号极弱
		010：信号较弱
		011：信号良好
		100：信号强*/

		//battery
		buffoffset += 2
		service.cur.OrigBattery = msg.Data[buffoffset] >> 3 & 0x07
		model := msg.Data[buffoffset] >> 6 & 0x01
		DeviceInfoListLock.Lock()
		deviceInfo,ok := (*DeviceInfoList)[service.imei]
		if ok && deviceInfo != nil{
			service.cur.LocateModel = deviceInfo.LocateModel
		}
		DeviceInfoListLock.Unlock()
		fmt.Println("model:",model,service.cur.LocateModel)
		if len(service.cur.LocateModel) == 0{
			service.cur.LocateModel = fmt.Sprintf("%d|15",model)
		}else {
			pos := strings.Index(service.cur.LocateModel,"|")
			if pos != -1{
				style := fmt.Sprintf("%d",model)
				if len(service.cur.LocateModel) > pos + 1{
					timespec := "|" + service.cur.LocateModel[pos + 1:]
					service.cur.LocateModel = style + timespec
				}
			}else {
				//default 15 seconds
				service.cur.LocateModel = fmt.Sprintf("%d|15",model)
			}
		}
		buffoffset++
		//报警字段
		//去掉手表上报的低电，由服务器计算是否报警
		service.cur.AlarmType &= (^uint8(ALARM_BATTERYLOW))
		alarmBit := msg.Data[buffoffset]
		if (alarmBit & 0x01) != 0 {
			//low battery alarm 上次没有数据，这次电量低于2，报低电
			if service.old.DataTime == 0 && service.cur.OrigBattery <= 2 ||
				service.old.DataTime > 0 && service.old.OrigBattery >= 3 && service.cur.OrigBattery <= 2 {
				//上次有数据，上次电量大于等于3，而这次电量小于等于2，报低电
				service.cur.AlarmType |= ALARM_BATTERYLOW
			}
		}
		if (alarmBit & 0x02) != 0 {
			service.cur.AlarmType |= ALARM_SPEED
		}

		//定位数据构成
		/*定位数据构成
		BYTE
		Bit7 0
		Bit6 0
		Bit5 0
		Bit4 0
		Bit3 0：无操作  1：服务器下发定位数据(协议3.1.9)
		Bit2 0：不包含GPS数据  1：包含GPS数据
		Bit1 0：不包含WiFi数据 1：包含WiFi数据
		Bit0 0：不包含 LBS数据 1：包含LBS数据*/

		buffoffset++
		service.needSendLocation = msg.Data[buffoffset] >> 3 & 0x01 == 1
		fmt.Println("needSendLocation:",service.needSendLocation,msg.Data[buffoffset],service.cur.OrigBattery)
		clocateTag := msg.Data[buffoffset] & 0x07
		if (clocateTag & 4) != 0 {
			//GPS信息
			buffoffset += 2
			service.cur.LocateType = LBS_GPS
			lat := Str2Num(hex.EncodeToString(msg.Data[buffoffset:buffoffset+4]), 16)
			mainLat := ((float64)(lat) / 30000) / 60
			service.cur.Lat = float64(mainLat)

			buffoffset += 4
			longtitude := Str2Num(hex.EncodeToString(msg.Data[buffoffset:buffoffset+4]), 16)
			mainlnt := ((float64)(longtitude) / 30000) / 60
			service.cur.Lng = float64(mainlnt)
			buffoffset += 4
			service.cur.Speed = int32(msg.Data[buffoffset])
			//state bit3:0,east, 1,west   bit2:0,south, 1,north
			buffoffset++
			isEast := (msg.Data[buffoffset] >> 3) & 0x01 == 0
			if !isEast{
				service.cur.Lng = -(service.cur.Lng)
			}
			isSouth := (msg.Data[buffoffset] >> 2) & 0x01 == 0
			if isSouth{
				service.cur.Lat = -(service.cur.Lat)
			}
			buffoffset +=3
			//MNC
		}
		if (clocateTag & 1) != 0 {

			buffoffset++
			shBufLen := uint32(0)
			service.ParseLBSInfo(msg.Data[buffoffset:],&shBufLen)
			buffoffset += int(shBufLen)
			if service.lbsNum <= 1{
				if service.cur.OrigBattery >= 3{
					service.cur.Battery = service.cur.OrigBattery - 3
				}
				watchStatus := &WatchGPSStatus{}
				watchStatus.i64DeviceID = service.imei
				watchStatus.i64Time = service.cur.DataTime
				watchStatus.Battery = service.cur.Battery
				watchStatus.LocateModel = service.cur.LocateModel
				watchStatus.AlarmType = service.cur.AlarmType
				watchStatus.iLocateType = service.cur.LocateType
				ret := service.UpdateWathStatus(watchStatus)
				if ret == false{
					logging.Log("Update LBS gps Watch status failed")
					return false
				}

				service.getSameWifi = true
				return  true
			}
			fmt.Println(service.cur,service.lbsInfoList)
			ret = service.GetLocation()
			if ret == false {
				logging.Log("gps GetLocation of LBS failed")
				return false
			}
			service.cur.Accracy = service.accracy
			service.cur.LocateType = LBS_JIZHAN
		}
		if (clocateTag & 2) != 0 {

			//WiFi info
			shBufLen := uint32(0)
			service.ParseWifiInfo(msg.Data[buffoffset:],&shBufLen)
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
			service.cur.LocateType = LBS_WIFI
		}

		if ret == false || service.cur.LocateType == LBS_INVALID_LOCATION || service.cur.Lng == 0 && service.cur.Lat== 0{
			service.needSendLocation = false
			logging.Log(fmt.Sprintf("Error gps Locate(%d, %06f, %06f,%d,%d)", service.imei, service.cur.Lng, service.cur.Lat,ret,service.cur.LocateType))
			return ret
		}

		if service.getSameWifi && service.cur.AlarmType <= 0  {
			logging.Log(fmt.Sprintf("return with: m_bGetSameWifi && m_iAlarmStatu <= 0(%d, %d, %d)",
				service.imei, service.cur.Lat, service.cur.Lng))
			return ret
		}
		//同时，把数据写入内存服务器
		if service.cur.OrigBattery >= 3{
			service.cur.Battery = service.cur.OrigBattery - 3
		}

		//高德地图判定经纬度是否在大陆
		if service.cur.Lat <= 53.33 &&
			service.cur.Lat >= 3.51 &&
			service.cur.Lng >= 73.33 &&
			service.cur.Lng <= 135.05{

			addr := service.reqCtx.GetAddressFunc(service.cur.Lat,service.cur.Lng)
			service.cur.Address = addr
		}

		ret = service.WatchDataUpdateDB()
		if ret == false{
			logging.Log(fmt.Sprintf("Update gps WatchData into Database failed"))
			return false
		}

		ret  = service.UpdateWatchData()
		if ret == false  {
			logging.Log(fmt.Sprintf("gps UpdateWatchData failed"))
			return false
		}

		if service.cur.AlarmType >0 {
			//推送报警
			service.NotifyAlarmMsg()
		}

		service.NotifyAppWithNewLocation()


		timezone := 0
		DeviceInfoListLock.Lock()
		device, ok := (*DeviceInfoList)[service.imei]
		if ok && device != nil{
			timezone = device.TimeZone
		}
		DeviceInfoListLock.Unlock()
		if timezone == INVALID_TIMEZONE{
			timezone = int(GetTimeZone(service.reqCtx.IP))
			logging.Log(fmt.Sprintf("imei %d, IP %d  GetTimeZone: %d", service.imei, service.reqCtx.IP, timezone))
			service.UpdateDeviceTimeZone(service.imei,timezone)
		}
		//offset
		logging.Log(fmt.Sprintf("timezone:%d",timezone))
		bEast := true
		if timezone < 0{
			timezone = -timezone
			bEast = false
		}
		offhour := byte(timezone / 100)
		offmin := byte(timezone % 100)
		if bEast {
			min = min - offmin
			if min < 0{
				min += 60
				hour -= 1
			}
			hour = hour - offhour
			if hour < 0{
				hour += 24
				day -= 1
			}
		}else {
			min = min  + offmin
			if min  >= 60 {
				min -= 60
				hour += 1
			}
			hour = hour + offhour
			if hour  >= 24 {
				hour -= 24
				day += 1
			}
		}
		strUTC := time.Now().UTC().Format("060102150411")
		utcYear := byte(Str2Num(strUTC[0:2],10))
		utcMonth := byte(Str2Num(strUTC[2:4],10))
		utcDay := byte(Str2Num(strUTC[4:6],10))
		utcHour := byte(Str2Num(strUTC[6:8],10))
		utcMin := byte(Str2Num(strUTC[8:10],10))
		bNeed := utcYear == year && utcMonth == month && utcDay == day && utcHour == hour && utcMin == min
		if service.needSendLocation && bNeed{
			madeData, id := service.makeSendLocationReplyMsg()
			resp := &ResponseItem{CMD_GPS_LOCACTION_ACK,service.makeReplyMsg(false,madeData,id)}
			service.rspList = append(service.rspList,resp)
		}

	}else if service.cmd == CMD_GPS_ACTIVE{
		//通知app把设置推送到车载机器
		service.reqCtx.AppNotifyChan <- &AppMsgData{Cmd:DevActiveAck,Imei:service.imei,ConnID:0}
		madeData,id := MakeReplyGPSMsg(service.imei,0x443D,0x16)
		resp := &ResponseItem{CMD_GPS_SET_ACTIVE_ACK,service.makeReplyMsg(false,madeData,id)}
		service.rspList = append(service.rspList,resp)
	}else if service.cmd == CMD_GPS_HEART{
		madeData,id := service.MakeHeartReplyMsg()
		resp := &ResponseItem{CMD_GPS_HEART_ACK,service.makeReplyMsg(false,madeData,id)}
		service.rspList = append(service.rspList,resp)
	}

	return ret
}

func (service *GPSService) UpdateWatchData() bool {
	if service.reqCtx.SetDeviceDataFunc != nil {
		service.reqCtx.SetDeviceDataFunc(service.imei, DEVICE_DATA_LOCATION, service.cur)
		return true
	}else{
		return false
	}
}

func (service *GPSService) WatchDataUpdateDB() bool {
	suffix := service.cur.DataTime / 100000000
	suffixYear := suffix / 100
	suffixMonth := suffix % 100
	strTableName := fmt.Sprintf("gator3_device_location_20%02d_%02d",suffixYear,suffixMonth)
	//strTableName := "gator3_device_location_2019_01"
	strSQL := fmt.Sprintf("INSERT INTO %s VALUES(%d, %d, %06f, %06f, '%s'::jsonb)",
		strTableName,service.imei,20000000000000 + service.cur.DataTime,service.cur.Lat, service.cur.Lng,MakeStructToJson(service.cur))
	logging.Log(fmt.Sprintf("SQL: %s---%d", strSQL,service.cur.DataTime))

	_,err := service.reqCtx.Pgpool.Exec(strSQL)
	if err != nil{
		logging.Log("pg pool exec sql failed, " + err.Error())
		return false
	}

	return true
}

func (service *GPSService) UpdateWathStatus(watchstatus *WatchGPSStatus) bool {
	deviceData := LocationData{LocateType:watchstatus.iLocateType,
	Battery:watchstatus.Battery,
	AlarmType:watchstatus.AlarmType,
	Imei:watchstatus.i64DeviceID,
	DataTime:watchstatus.i64Time,
	LocateModel:watchstatus.LocateModel}

	if service.reqCtx.SetDeviceDataFunc != nil {
		service.reqCtx.SetDeviceDataFunc(watchstatus.i64DeviceID,DEVICE_DATA_STATUS,deviceData)

		return true
	}
	return false
}

func (service *GPSService) MakeAckParsedMsg(id uint64) *MsgData{
	msg := &MsgData{}
	msg.Header.Header.ID = id
	msg.Header.Header.Version = MSG_HEADER_ACK_PARSED
	msg.Header.Header.Imei = service.imei
	msg.Header.Header.Cmd = service.cmd

	return msg
}

func (service *GPSService) NotifyAlarmMsg() bool {
	ownerName := Num2Str(service.imei,10)
	DeviceInfoListLock.Lock()
	deviceInfo, ok := (*DeviceInfoList)[service.imei]
	if ok && deviceInfo != nil{
		if deviceInfo.OwnerName != ""{
			ownerName = deviceInfo.OwnerName
		}
	}
	DeviceInfoListLock.Unlock()

	PushNotificationToApp(service.reqCtx.APNSServerBaseURL,service.imei,
		"",ownerName,service.cur.DataTime,service.cur.AlarmType,"")
	return true
}

func (service *GPSService) NotifyAppWithNewLocation() bool {
	result := HeartbeatResult{Timestamp:time.Now().Format("20060102150405")}
	result.Locations = append(result.Locations,service.cur)
	fmt.Println("gps NotifyAppWithNewLocation heartbeat-ack: ", MakeStructToJson(result))

	service.reqCtx.AppNotifyChan <- &AppMsgData{Cmd:HearbeatAckCmdName,
		Imei:service.imei,
		Data:MakeStructToJson(result),ConnID:0}
	return true
}

func  (service *GPSService) GetLocation()  bool{
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

//for amap(高德) api
func  (service *GPSService) GetLocationByAmap() bool  {
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

	logging.Log(fmt.Sprintf("amap TransForm location Is:%06f, %06f,locateType = %d", service.cur.Lat, service.cur.Lng,service.cur.LocateType))

	return true
}

func  (service *GPSService) GetLocationByGoogle() bool  {
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

func (service *GPSService) ParseWifiInfo(pszMsgBuf []byte,shBufLen *uint32) bool {
	*shBufLen = 0
	buffoffset := 0
	shWifiNum := int(pszMsgBuf[buffoffset])
	fmt.Println("shWifiNum:",shWifiNum)
	buffoffset++
	service.wiFiNum = uint16(shWifiNum)
	service.wifiInfoList = [MAX_WIFI_NUM + 1]WIFIInfo{}
	for i := 0;i < shWifiNum;i++{
		k := 0
		for j := 0;j < 6;j++{

			copy(service.wifiInfoList[i].MacID[k:k+2],fmt.Sprintf("%02X",pszMsgBuf[buffoffset]))
			buffoffset++
			k += 2
			if j < 5 {
				service.wifiInfoList[i].MacID[k] = ':'
				k++
			}
		}

		ssidLen := int(pszMsgBuf[buffoffset])
		fmt.Printf("MacID:%s,%d\n",string(service.wifiInfoList[i].MacID[0:]),ssidLen)
		buffoffset++
		service.wifiInfoList[i].WIFIName = string(pszMsgBuf[buffoffset:buffoffset + ssidLen])
		fmt.Println("WIFIName:",service.wifiInfoList[i].WIFIName)
		buffoffset += ssidLen
		service.wifiInfoList[i].Ratio = int16(int8(pszMsgBuf[buffoffset]))
		buffoffset++
	}
	*shBufLen = uint32(buffoffset)

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

func (service *GPSService) ParseLBSInfo(pszMsgBuf []byte,shBufLen *uint32) bool {
	*shBufLen = 0
	buffoffset := 0
	fmt.Println("pszMsgBuf:",pszMsgBuf[buffoffset])
	shMcc1 := int32(pszMsgBuf[buffoffset]) << 8
	buffoffset++
	shMcc2 := int32(pszMsgBuf[buffoffset])
	shMcc := shMcc1 + shMcc2
	buffoffset++
	shMnc := int32(pszMsgBuf[buffoffset])
	buffoffset++ //MCC,NMC,CellNum
	shLBSNum := int(pszMsgBuf[buffoffset])
	buffoffset++
	service.lbsNum = uint16(shLBSNum)
	fmt.Println("lbsinfo:",service.lbsNum,shMcc1,shMcc2,shMnc)
	service.lbsInfoList = [MAX_LBS_NUM + 1]LBSINFO{}
	for i :=0; i < shLBSNum; i++{
		service.lbsInfoList[i].Mcc = shMcc
		service.lbsInfoList[i].Mnc = shMnc
		//lac 2个字节
		lac1 := int32(pszMsgBuf[buffoffset]) << 8
		buffoffset++
		lac2 := int32(pszMsgBuf[buffoffset])
		buffoffset++
		fmt.Println("Lac:",lac1,lac2)
		service.lbsInfoList[i].Lac = lac1 + lac2
		//CellId 4个字节
		cellid1 := int32(pszMsgBuf[buffoffset]) << 24
		buffoffset++
		cellid2 := int32(pszMsgBuf[buffoffset]) << 16
		buffoffset++
		cellid3 := int32(pszMsgBuf[buffoffset]) << 8
		buffoffset++
		cellid4 := int32(pszMsgBuf[buffoffset])
		buffoffset++
		service.lbsInfoList[i].CellID = cellid1 + cellid2 + cellid3 +cellid4
		//RSSI 1个字节
		service.lbsInfoList[i].Signal = int32(int8(pszMsgBuf[buffoffset]))
		buffoffset++
		//service.lbsInfoList[i].Signal = service.lbsInfoList[i].Signal * 2 - 113
	}

	if service.lbsNum >= MAX_LBS_NUM {
		service.lbsNum = MAX_LBS_NUM
	}

	*shBufLen = uint32(buffoffset)

	return true
}

func (service *GPSService) Doresponse() []*MsgData {
	if len(service.rspList) > 0{
		msgList := []*MsgData{}
		for _,respCmd := range service.rspList{
			msg := MsgData{}
			msg = *respCmd.msg
			msg.Header.Header.Cmd = respCmd.rspCmdType
			msg.Data = make([]byte,len(respCmd.msg.Data))
			copy(msg.Data[0:], respCmd.msg.Data[0:])
			msgList = append(msgList,&msg)
		}

		return msgList
	}else {
		return nil
	}
}
func HandleGpsRequest(reqCtx RequestContext) bool {
	service := &GPSService{reqCtx:reqCtx}
	ret := service.PreDorequest()
	if ret == false{
		return false
	}
	ret = service.Dorequest(reqCtx.Msg)

	msgReplyList := service.Doresponse()
	if msgReplyList != nil && len(msgReplyList) > 0 {
		for _,msg := range msgReplyList{
			reqCtx.WritebackChan <- msg
		}
	}

	//通知ManagerLoop, 将上次缓存的未发送的数据发送给手表
	msgNotify := &MsgData{}
	msgNotify.Header.Header.Version = MSG_HEADER_PUSH_CACHE
	msgNotify.Header.Header.Imei = service.imei
	reqCtx.WritebackChan <- msgNotify
	return  true
}