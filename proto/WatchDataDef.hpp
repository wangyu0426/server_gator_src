#ifndef __WATCH_DEF_HPP__
#define __WATCH_DEF_HPP__

#include <sys/types.h>

typedef struct tag_WatchDeviceKey
{
	int64_t m_i64DeviceID;
	void Initialize()
	{
		m_i64DeviceID = 0;
	}
}TWatchDeviceKey;

const int MAX_ZONENAME_LEN = 16;
const int WIFI_MACID_LEN = 18;

typedef struct tag_WatchDataInfo
{
	int64_t m_uiDataTime;
	int m_iLongtiTude;
	int m_iLatitude;
	int m_iSpeed;
	char m_ucAlarmStatu;
	char m_ucReadFlag;
	char m_cBattery;
	unsigned char m_cStationType;
	unsigned int m_iReserve;
	char m_szZoneName[MAX_ZONENAME_LEN];
	char m_szWifiMacID[WIFI_MACID_LEN];
	char m_szChatExPhoneNo[32];
	int64_t m_uiDeviceId;
}TWatchDataInfo;

const int MAX_ALARMINFO_NUM = 64;

typedef struct tag_WatchAlarmInfo
{
	short m_shAlarmInfoNum;
	TWatchDataInfo m_astWatchDataIno[MAX_ALARMINFO_NUM];
}TWatchAlarmInfo;

typedef struct tag_WatchDetail
{
	int64_t m_i64DeviceID;
	TWatchDataInfo m_stDataInfo;
}TWatchDetail;

const int MAX_WATCHINFO_NUM = 64;
typedef struct tag_MutilWatchInfo
{
	short m_shWatchNum;
	TWatchDetail m_astDetailInfo[MAX_WATCHINFO_NUM];
}TMutilDetailInfo;

const int MAX_NAME_LEN = 32;
const int MAX_CHATINFO_NUM = 512;
typedef struct
{
	int64_t m_i64Time;
	unsigned char m_ucSource;
	unsigned char m_shSendFlag;
	unsigned short m_ushSec;
	unsigned char m_szName[MAX_NAME_LEN];
	unsigned char m_szSenderFamilyNo[MAX_NAME_LEN];
}TChatInfo;

typedef struct
{
	short m_shChatInfoNum;
	TChatInfo m_astChatInfo[MAX_CHATINFO_NUM];
}TMulChatInfo;

typedef struct
{
	int64_t m_i64IMEI;
    short m_shTimeZone;
    unsigned short m_ushUseDst;
    unsigned short m_ushSocketModeOff;
    unsigned long m_ulPhoneNo;
}TWatchSettings;

struct WatchStatus
{
	int64_t m_i64DeviceID;
	int m_iLocateType;
	int64_t m_i64Time;
	int m_iStep;
};

enum enQuerySource
{
	QUERY_APP = 1,
	QUERY_WATCH = 2,
};

enum enDataSource
{
	DATA_APP = 1,
	DATA_WATCH = 2,
};

enum enUpdateFlag
{
	FLAG_UPDATE = 0,
	FLAG_DEL = 1,
};

enum enWatchCfgType
{
	WATCHCFG_PHONE = 0,
	WATCHCFG_ALARM = 1,
};


#define SETWATCHCFG_MAKECALL_SET    "makecallset"

#endif

