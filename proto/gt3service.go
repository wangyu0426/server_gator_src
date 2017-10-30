package proto

/*
#cgo LDFLAGS: -L ./    gt3service.a -lstdc++
#include "gt3service.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"unsafe"
	"fmt"
)

func Gt3Test() {
	service := C.createGt3Service()
	cstr := C.CString("(357593060692350BP09,177,2,171030,113400,6)")//("(357593060153353BP09,6)") //("(357593060153353BP01,G,150728A2934.0133N10627.2544E000.0141830309.62,5)")//("(357593060153353BP00,WU01_GATOR_BRIAN_V15_150703,5,46000)")
	C.gt3ServiceHandleRequest(service, cstr,  C.uint(C.strlen(cstr)))
	gt3result := C.struct_Gt3ServiceResult{}
	C.gt3ServiceDo(service, (&gt3result))
	C.gt3ServiceHandleResponse(service)
	C.destoryGt3Service(service)
	C.free(unsafe.Pointer(cstr))
	fmt.Println(gt3result.m_cBattery_orig, gt3result.m_stDataInfo.m_iSpeed, gt3result.m_stDataInfo)
	gt3service := GT03Service{}
	gt3service.Test()
}

func parseMsgString(bufMsg []byte, out *GT03Service) {
	service := C.createGt3Service()
	cbuf, csize :=  GoArrayToCArray(bufMsg)// C.CString(strMsg)//("(357593060153353BP01,G,150728A2934.0133N10627.2544E000.0141830309.62,5)")//("(357593060153353BP00,WU01_GATOR_BRIAN_V15_150703,5,46000)")
	if cbuf == nil || csize == 0 {
		return
	}

	C.gt3ServiceHandleRequest(service, cbuf,  C.uint(csize))
	gt3result := C.struct_Gt3ServiceResult{}
	C.gt3ServiceDo(service, (&gt3result))
	//C.gt3ServiceHandleResponse(service)

	out.imei = uint64(gt3result.m_i64SrcDeviceID)
	out.needSendLocation = gt3result.m_bNeedSendLocation != 0
	out.reqChatInfoNum = int(gt3result.m_shReqChatInfoNum)
	out.cur.OrigBattery = uint8(gt3result.m_cBattery_orig)
	out.cur.AlarmType = uint8(gt3result.m_iAlarmStatu)
	out.cur.Steps = uint64(gt3result.m_stDataInfo.m_iSpeed)
	out.cur.DataTime = uint64(gt3result.m_stDataInfo.m_uiDataTime)
	out.cur.LocateType = uint8(gt3result.m_stDataInfo.m_cStationType)
	if out.cur.LocateType == LBS_GPS {
		out.cur.Lat = float64(gt3result.m_stDataInfo.m_iLatitude) / 1000000.0
		out.cur.Lng = float64(gt3result.m_stDataInfo.m_iLongtiTude) / 1000000.0
	}else{
		if gt3result.m_shWiFiNum > 0 {
			out.wiFiNum = uint16(gt3result.m_shWiFiNum)
			for i := 0; i < int(gt3result.m_shWiFiNum); i++ {
				out.wifiInfoList[i].WIFIName = C.GoString(&gt3result.m_astWifiInfo[i].m_szWIFIName[0])
				//macid := C.GoString(&gt3result.m_astWifiInfo[i].m_szMacID[0])
				//out.wifiInfoList[i].MacID = []byte(macid)
				//copy(out.wifiInfoList[i].MacID, macid[0: MAX_MACID_LEN])
				for c := 0; c < MAX_MACID_LEN; c++ {
					out.wifiInfoList[i].MacID[c] = byte(gt3result.m_astWifiInfo[i].m_szMacID[c])
				}
				out.wifiInfoList[i].Ratio = int16(gt3result.m_astWifiInfo[i].m_shRatio)
			}
		}

		if gt3result.m_shLBSNum > 0{
			out.lbsNum = uint16(gt3result.m_shLBSNum)
			for i := 0; i < int(gt3result.m_shLBSNum); i++ {
				out.lbsInfoList[i].Mcc = int32(gt3result.m_astLBSInfo[i].m_iMcc)
				out.lbsInfoList[i].Mnc = int32(gt3result.m_astLBSInfo[i].m_iMnc)
				out.lbsInfoList[i].Lac = int32(gt3result.m_astLBSInfo[i].m_iLac)
				out.lbsInfoList[i].CellID = int32(gt3result.m_astLBSInfo[i].m_iCellID)
				out.lbsInfoList[i].Signal = int32(gt3result.m_astLBSInfo[i].m_iSignal)
			}
		}
	}

	if gt3result.m_chatData != nil {
		out.chatData = CArrayToGoArray(unsafe.Pointer(gt3result.m_chatData), int(gt3result.m_chatDataSize))
		out.chat.Imei = out.imei
		out.chat.DateTime = uint64(gt3result.chatInfo.m_i64Time)
		out.chat.VoiceMilisecs = int(gt3result.chatInfo.m_ushSec)
	}

	C.destoryGt3Service(service)

	if cbuf != nil {
		C.free(unsafe.Pointer(cbuf))
		cbuf = nil
		csize = 0
	}
}

func CArrayToGoArray(cArray unsafe.Pointer, size int) (goArray []byte) {
	p := uintptr(cArray)
	for i :=0; i < size; i++ {
		j := *(*byte)(unsafe.Pointer(p))
		goArray = append(goArray, j)
		p += unsafe.Sizeof(j)
	}

	return
}

func GoArrayToCArray (goArray []byte) (cArray *C.char, size int){
	cArray = nil
	size = 0

	 if goArray == nil || len(goArray) == 0 {
		 return
	}

	size = len(goArray)
	cArray = (*C.char)(C.malloc(C.size_t(size)))
	p := uintptr(unsafe.Pointer(cArray))
	for i :=0; i < size; i++ {
		*(*byte)(unsafe.Pointer(p)) = goArray[i]
		p += 1
	}

	return
}

func Alloc(size int) *byte {
	return (*byte)(C.malloc((C.size_t(size))))
}

func Free(ptr *byte) {
	C.free(unsafe.Pointer(ptr))
}