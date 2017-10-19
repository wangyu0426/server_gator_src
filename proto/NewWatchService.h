#pragma once

#include "BusinessService.h"
#include "WatchDataDef.hpp"
#include "DeviceDef.hpp"

enum RspType
{
	CMD_AP00 = 0,
	CMD_AP01,
	CMD_AP02,
	CMD_AP03,
	CMD_AP04,
	CMD_AP05,
	CMD_AP06,
	CMD_AP07,
	CMD_AP08,
	CMD_AP09,
};


enum LBS_TYPE
{
	LBS_NORMAL = 0,
	LBS_GPS = 1, //gps
	LBS_JIZHAN = 2, //base station (LBS)
	LBS_WIFI = 3, //WIFI
	LBS_SMARTLOCATION = 4, //smart locate (device not moving)
	LBS_INVALID_LOCATION, //invalid locate
};


const int MAX_EPO_LEN = 72;
#define MTKEPO_SV_NUMBER        32
#define MTKEPO_RECORD_SIZE      72
#define MTKEPO_SEGMENT_NUM      (30 * 4)
#define MTKEPO_MAXSEG_ONETIME 8
#define MTKEPO_DATA_ONETIME 12

typedef struct tag_EPOInfo
{
	unsigned char m_ucEPOBuf[MTKEPO_SV_NUMBER*MTKEPO_RECORD_SIZE];
}TEPOInfo;


class NewWatchService : public BusinessService
{
public:
	NewWatchService();
	virtual ~NewWatchService();

public:
	virtual int HandleRequest(const char *msg, unsigned int msg_len);
	virtual int Do();
	virtual int HandleResponse();

protected:
	int ProcessLocate(unsigned char* pszMsg, char cLocateTag);
	int ProcessGPSInfo(unsigned char* pszMsgBuf);
	int GetIntValue(unsigned char* pszMsgBuf, short shLen);
	float GetFloatValue(unsigned char* pszMsgBuf, short shLen);
	int GetValue(unsigned char* pszMsgBuf, short& shMaxLen);
	int ProcessMicChat(unsigned char* pszMsgBuf, short shMsgLen);
	int ProcessRecordSMSInfo(unsigned char* pszMsg);
	int ProcessRspAGPSInfo();
	int utc_to_gps_hour (int iYr, int iMo, int iDay, int iHr);
	int ProcessUpdateWatchStatus(unsigned char* pszMsgBuf);
	int ProcessRspChat();
	int ProcessWifiInfo(unsigned char* pszMsgBuf);
	int ParseWifiInfo(unsigned char* pszMsgBuf, short& shBufLen);
	int ProcessLBSInfo(unsigned char* pszMsgBuf);
	int ParseLBSInfo(unsigned char* pszMsgBuf, short& shBufLen);
	int hex_to_decimal(char * szHex,  int  len);
	int hex_char_value( char  c);
	int ProcessMutilLocateInfo(unsigned char* pszMsgBuf);
	int GetDisTance(TPoint stPoint1, TPoint stPoint2);
	double rad(double d);

public:
	void copyOutData(Gt3ServiceResult &result);

private:
	unsigned int m_uiDeviceUin;
	int64_t m_i64DeviceID;
	int64_t m_i64SrcDeviceID;
	int m_iAlarmStatu;
	short m_shReqChatInfoNum;
	short m_shRspCmdType;
	int m_iTimeZone;
	bool m_bNeedSetTimeZone;
	TWatchDataInfo m_stDataInfo;
	TWatchDataInfo m_stOldDataInfo;
	unsigned char m_cBattery_orig;
	unsigned char m_cBattery_orig_old;

	bool m_bNeedSendChat;
	bool m_bNeedSendChatNum;
	bool m_bIsChatEx;
	char m_szChatExPhoneNo[32];
	int m_iAccracy;
	bool m_bNeedSendLocation;
	bool m_bGetWatchDataInfo;
	bool m_bGetSameWifi;
	bool m_bNewWifiStruct;
	short m_shWiFiNum;
	TWIFIInfo m_astWifiInfo[MAX_WIFI_NUM];
	short m_shLBSNum;
	TLBSInfo m_astLBSInfo[MAX_LBS_NUM];
	TMulChatInfo m_stMulChatInfo;
	TChatInfo m_stChatInfo;
	char *m_chatData;
	int m_chatDataSize;

	//below is the AGPS database, need to get it from Gator Server...
	int m_iStartGPSHour;
	TEPOInfo m_astEPOInfo[MTKEPO_DATA_ONETIME];
};

