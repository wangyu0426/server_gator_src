#ifndef __DEVICECONFIG_DEF_HPP__
#define __DEVICECONFIG_DEF_HPP__

#include <sys/types.h>
#include <string.h>

#define  MAX_POINT_NUM 12
#define  MAX_ALARM_ZONE 32
#define  MAX_ZONE_NAMELEN 32
#define MAX_UPDATE_ONETIME 64

typedef struct tag_Fatigue
{
	short m_shHoldTime;
	short m_shBuff;
}TFatigue;

typedef struct tag_Point
{
	int m_iLatitude;
	int m_iLongtiTude;
}TPoint;

typedef struct tag_DriveZone
{
	unsigned char m_ucPointNum;
	TPoint m_astPoint[MAX_POINT_NUM];
	char m_szZoneName[MAX_ZONE_NAMELEN];
	unsigned int m_iZoneID;
	unsigned char m_ucAlarmType;
	unsigned short m_shStarTime;
	unsigned short m_shEndTime;
	unsigned int m_iStartDate;
	unsigned int m_iEndDate;
	unsigned char m_ucAlarmWeek;
	unsigned char m_ucEMailNum;
	unsigned char m_ucSMSNum;
}TAlarmZone;

typedef struct tag_DeviceConfig
{
	unsigned int m_iAlarmStatu;
	TFatigue m_stFatigueCfg;
	unsigned char m_ucAlarmZoneNum;
	TAlarmZone m_astAlarmZone[MAX_ALARM_ZONE];
	unsigned char m_ucEMailNum;
	unsigned char m_ucSMSNum;
}TDeviceConfig;

const int MAX_TRANSBUF_LEN = 10240;

const int MAX_ALARM_TYPE_NUM = 8;

const int MAX_WATCH_PHONE_NUM = 13;
const int MAX_CFGDATA_LEN = 1024;

typedef struct tag_WatchConfig
{
	short m_shCfgLen;
	unsigned char m_szCfgData[MAX_CFGDATA_LEN];
}TWatchConfig;

const int MAX_WIFI_NAME_LEN = 64;
const int MAX_MACID_LEN = 18;
const int MAX_WIFI_NUM = 6;
const int MAX_LBS_NUM = 8;


#endif

