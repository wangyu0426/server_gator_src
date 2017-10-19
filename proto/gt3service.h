#ifdef __cplusplus
extern "C"{
#endif

#include <stdint.h>

#define _MAX_ZONENAME_LEN  16
#define _MAX_WIFI_NAME_LEN   64
#define _MAX_MACID_LEN  18
#define _MAX_WIFI_NUM 6
#define _MAX_LBS_NUM 8
#define _MAX_NAME_LEN  32

typedef struct tag_WIFIInfo
{
	char m_szWIFIName[_MAX_WIFI_NAME_LEN];
	char m_szMacID[_MAX_MACID_LEN];
	short m_shRatio;
}TWIFIInfo;

typedef struct tag_LBSINFO
{
	int m_iMcc;
	int m_iMnc;
	int m_iLac;
	int m_iCellID;
	int m_iSignal;
}TLBSInfo;


typedef struct tag_Gt3WatchData{
	int64_t m_uiDataTime;
	int m_iLongtiTude;
	int m_iLatitude;
	int m_iSpeed;
	char m_ucAlarmStatu;
	char m_ucReadFlag;
	char m_cBattery;
	unsigned char m_cStationType;
	unsigned int m_iReserve;
	char m_szZoneName[_MAX_ZONENAME_LEN];
	char m_szWifiMacID[_MAX_MACID_LEN];
	char m_szChatExPhoneNo[_MAX_NAME_LEN];
	int64_t m_uiDeviceId;
}Gt3WatchData;

typedef struct tag_Gt3ChatInfo
{
	int64_t m_i64Time;
	unsigned char m_ucSource;
	unsigned char m_shSendFlag;
	unsigned short m_ushSec;
	unsigned char m_szName[_MAX_NAME_LEN];
	unsigned char m_szSenderFamilyNo[_MAX_NAME_LEN];
}Gt3ChatInfo;

typedef struct Gt3ServiceResult{
    int lastErr;
    unsigned int m_uiDeviceUin;
    int64_t m_i64DeviceID;
    int64_t m_i64SrcDeviceID;
    int m_iAlarmStatu;
    short m_shReqChatInfoNum;
    short m_shRspCmdType;
    int m_iTimeZone;
    unsigned char m_bNeedSetTimeZone;
    Gt3WatchData m_stDataInfo;
    Gt3WatchData m_stOldDataInfo;
    unsigned char m_cBattery_orig;
    unsigned char m_cBattery_orig_old;

    unsigned char m_bNeedSendChat;
    unsigned char m_bNeedSendChatNum;
    unsigned char m_bIsChatEx;
    char m_szChatExPhoneNo[32];
    int m_iAccracy;
    unsigned char m_bNeedSendLocation;
    unsigned char m_bGetWatchDataInfo;
    unsigned char m_bGetSameWifi;
    unsigned char m_bNewWifiStruct;
    short m_shWiFiNum;
    TWIFIInfo m_astWifiInfo[_MAX_WIFI_NUM];
    short m_shLBSNum;
    TLBSInfo m_astLBSInfo[_MAX_LBS_NUM];
    unsigned char *data;
    	//TMulChatInfo m_stMulChatInfo;
    Gt3ChatInfo chatInfo;
    char *m_chatData;
    int m_chatDataSize;
}Gt3ServiceResult;

extern void *createGt3Service();
extern void destoryGt3Service(void *service);
extern int gt3ServiceHandleRequest(void *service, const char *msg, unsigned int msg_len);
extern void gt3ServiceDo(void *service, Gt3ServiceResult *result);
extern int gt3ServiceHandleResponse(void *service);
#ifdef __cplusplus
}
#endif

//g++ -c -o NewWatchService.o NewWatchService.cpp && ar -rs gt3service.a NewWatchService.o