#include <netinet/in.h>
#include <arpa/inet.h>
#include <math.h>
#include <sys/time.h>
#include <time.h>
#include <ctype.h>
#include <stdio.h>
 #include <stdlib.h>
#include <string>

#include "gt3service.h"
#include "NewWatchService.h"

using namespace std;

static const int DEVICEID_BIT_NUM = 15;

NewWatchService::NewWatchService()
{
	m_bIsChatEx = false;
	m_cBattery_orig = 0;
	m_cBattery_orig_old = 0;
	m_shLBSNum = 0;
	m_shWiFiNum = 0;
	m_chatData = NULL;
	m_chatDataSize = 0;

	memset(m_szChatExPhoneNo, 0, sizeof(m_szChatExPhoneNo));
}

NewWatchService::~NewWatchService()
{
    m_chatDataSize = 0;

    if(m_chatData){
        delete []m_chatData;
        m_chatData = NULL;
    }
}

int NewWatchService::HandleRequest(const char *msg, unsigned int msg_len)
{
	if(!msg || !msg[0] || !msg_len)
	{
		return -1;
	}

	int ret = 0;
	m_pszMsgBuf = (char *)msg;
	m_shBufLen = msg_len;

	m_cBattery_orig = 0;
	m_cBattery_orig_old = 0;
	m_iAlarmStatu = 0;
	m_shRspCmdType = -1;
	m_iTimeZone = -100;
	m_bNeedSendChat = false;
	m_bNeedSendChatNum = true;
	m_bGetWatchDataInfo = false;
	m_bNewWifiStruct = false;

	memset(&m_stDataInfo, 0, sizeof(m_stDataInfo));

	return ret;
}

int NewWatchService::Do()
{
	int ret = 0;

	unsigned char* pszMsgBuf = (unsigned char* )m_pszMsgBuf;
	unsigned short shMsgLen = m_shBufLen;

	m_bNeedSetTimeZone = true;
	m_bNeedSendChat = false;
	m_bNeedSendChatNum = true;
	m_bIsChatEx = false;
	m_bNeedSendLocation = false;
	m_bGetSameWifi = false;
	m_iAccracy = 0;
	memset(&m_stOldDataInfo, 0, sizeof(m_stOldDataInfo));

	unsigned char ucBegin = *(unsigned char*)pszMsgBuf;
	if (ucBegin != 0x28)
	{
		return -1;
	}

	pszMsgBuf++;
	shMsgLen--;

	char szDeviceID[DEVICEID_BIT_NUM + 1];
	memset(szDeviceID, 0, DEVICEID_BIT_NUM + 1);
	memcpy(szDeviceID, pszMsgBuf, DEVICEID_BIT_NUM);
	pszMsgBuf += DEVICEID_BIT_NUM;
	shMsgLen -= DEVICEID_BIT_NUM;

	m_i64SrcDeviceID = atol(szDeviceID);
	char szCmd[DEVICE_CMD_LENGTH + 1];
	memset(szCmd, 0, sizeof(szCmd));
	memcpy(szCmd, pszMsgBuf, DEVICE_CMD_LENGTH);
	pszMsgBuf += sizeof(int);
	shMsgLen -= sizeof(int);
	int iRet = 0;
	m_i64DeviceID = m_i64SrcDeviceID % 100000000000;

	m_shRspCmdType = CMD_AP09;
    if (!strncmp(szCmd, "BP00", sizeof(int))) //report version
	{
		m_shRspCmdType = CMD_AP03;
		pszMsgBuf++;
		char szVersion[64];
		memset(szVersion, 0, sizeof(szVersion));
		int index = 0;
		while(pszMsgBuf && *pszMsgBuf != ',' && index < 64)
		{
			szVersion[index++] = *pszMsgBuf;
			pszMsgBuf++;
		}

		//result data is: szVersion
		printf("version: %s\n", szVersion);
	}
    else if (!strncmp(szCmd, "BP01", sizeof(int)) || !strncmp(szCmd, "BPW1", sizeof(int)) ||
    		!strncmp(szCmd, "BPM1", sizeof(int)))  //tracking data (location)
	{
    	//BPW1 -- wifi location;  BPM1 -- LBS+WiFi mixed location
        if (!strncmp(szCmd, "BPW1", sizeof(int)) || !strncmp(szCmd, "BPM1", sizeof(int)))
            m_bNewWifiStruct = true;

		pszMsgBuf++;
		char cLocateTag = *(char*)pszMsgBuf;
		pszMsgBuf++;
		ret = ProcessLocate(pszMsgBuf, cLocateTag);
	}
    else if (!strncmp(szCmd, "BP02", sizeof(int)) || !strncmp(szCmd, "BPW2", sizeof(int)) ||
    		!strncmp(szCmd, "BPM2", sizeof(int))) //tracking data with alert
    {
        if (!strncmp(szCmd, "BPW2", sizeof(int)) || !strncmp(szCmd, "BPM2", sizeof(int)))
            m_bNewWifiStruct = true;

		pszMsgBuf++;
		char cLocateTag = *(char*)pszMsgBuf;
		pszMsgBuf += sizeof(short);
		m_iAlarmStatu = *(unsigned char*)pszMsgBuf - '0';
		pszMsgBuf++;

		ret = ProcessLocate(pszMsgBuf, cLocateTag);
	}
	else if (!strncmp(szCmd, "BP03", sizeof(int)))
	{
		pszMsgBuf++;
		ret = ProcessRecordSMSInfo(pszMsgBuf);
	}
	else if (!strncmp(szCmd, "BP04", sizeof(int)))
	{
		if(strstr((char *)pszMsgBuf, "AP15"))
			m_bNeedSendChatNum = false;
	}
	else if (!strncmp(szCmd, "BP05", sizeof(int)))
	{ //not use
	}
	else if (!strncmp(szCmd, "BP06", sizeof(int)) || !strncmp(szCmd, "BP11", sizeof(int)))
	{
		m_bIsChatEx = !strncmp(szCmd, "BP11", sizeof(int));
		ret = ProcessMicChat(pszMsgBuf, shMsgLen);
	}
	else if (!strncmp(szCmd, "BP07", sizeof(int)))
	{
		m_bNeedSendChat = true;
		m_shReqChatInfoNum = 0;
		for (short i = 0; i < sizeof(short); i++)
		{
			m_shReqChatInfoNum = m_shReqChatInfoNum * 10 + *(char*)pszMsgBuf - '0';
			pszMsgBuf++;
			shMsgLen--;
		}
	}
	else if (!strncmp(szCmd, "BP08", sizeof(int)))
	{
			ret = ProcessRspAGPSInfo();
	}
	else if (!strncmp(szCmd, "BP09", sizeof(int)))
	{
		// heart break
		//if (*pszMsgBuf != ')' && shMsgLen > sizeof(int) * 3)
		{
			pszMsgBuf++;
			ret = ProcessUpdateWatchStatus(pszMsgBuf);
		}
	}
	else
	{
		ret = -1;
	}

	return ret;
}

int NewWatchService::HandleResponse()
{
	int ret = 0;

	if (m_shRspCmdType == CMD_AP03 || m_bNeedSendLocation) // not need response
	{
		unsigned char ucRspMsgBuf[256] = {0};
		short shMsgLen = 0;

		unsigned char* pszTemp = ucRspMsgBuf;
		*(unsigned char*)pszTemp = 0x28;
		pszTemp++;
		shMsgLen++;

		sprintf((char*)pszTemp, "%015lld", m_i64SrcDeviceID);
		pszTemp += DEVICEID_BIT_NUM;
		shMsgLen += DEVICEID_BIT_NUM;
		if (m_bNeedSendLocation)
		{
			memcpy(pszTemp, "AP18", sizeof(int));
			shMsgLen += sizeof(int);
			pszTemp += sizeof(int);
			time_t tNow = time(NULL);
			struct tm utc_tm;
			struct tm* pTempTm = gmtime_r( &tNow, &utc_tm ); //获取格林威治时间
			if (NULL != pszTemp)
			{
				float dLongtitude = m_stDataInfo.m_iLongtiTude / 1000000.0;
				float dLatiTude = m_stDataInfo.m_iLatitude / 1000000.0;
				if (m_iAccracy <= 0)
				{
					m_iAccracy = 100;
				}

				int iLen = sprintf((char*)pszTemp, "%f,%f,%d,%04d,%02d,%02d,%02d,%02d,%02d",
					dLatiTude, dLongtitude, m_iAccracy, utc_tm.tm_year+1900,utc_tm.tm_mon + 1,
					utc_tm.tm_mday,utc_tm.tm_hour, utc_tm.tm_min, utc_tm.tm_sec);
				shMsgLen += iLen;
				pszTemp += iLen;
			}
		}
		else
		{
			m_bNeedSetTimeZone = false;
            short shTimeZone = 0;  //should set a correct time zone
			m_bNeedSetTimeZone = true;
			m_iTimeZone = shTimeZone;
        	
			memcpy(pszTemp, "AP03", sizeof(int));
			shMsgLen += sizeof(int);
			pszTemp += sizeof(int);
			time_t tNow = time(NULL);
			struct tm utc_tm;
			struct tm* pTempTm = gmtime_r( &tNow, &utc_tm ); //获取格林威治时间
			if (NULL != pTempTm)
			{
				*(char*)pszTemp = ',';
				pszTemp++;
				shMsgLen++;
				int iYearTime = (utc_tm.tm_year - 100)*10000
					+ (utc_tm.tm_mon + 1)*100 + utc_tm.tm_mday;
				sprintf((char*)pszTemp, "%06d", iYearTime);
				pszTemp += sizeof(int) + sizeof(short);
				shMsgLen += sizeof(int) + sizeof(short);
				*(char*)pszTemp = ',';
				pszTemp++;
				shMsgLen++;
				int iSecTime = utc_tm.tm_hour*10000 +
					utc_tm.tm_min * 100 + utc_tm.tm_sec;
				sprintf((char*)pszTemp, "%06d", iSecTime);
				pszTemp += sizeof(int) + sizeof(short);
				shMsgLen += sizeof(int) + sizeof(short);

				if (m_bNeedSetTimeZone)
				{
					*(char*)pszTemp = ',';
					pszTemp++;
					shMsgLen++;
					if (m_iTimeZone >= 0)
					{
						sprintf((char*)pszTemp, "e%04d", m_iTimeZone);
					}
					else
					{
						m_iTimeZone = -m_iTimeZone;
						sprintf((char*)pszTemp, "w%04d", m_iTimeZone);
					}

					pszTemp += sizeof(int) + sizeof(char);
					shMsgLen += sizeof(int) + sizeof(char);
				}
			}
		}		

		*(unsigned char*)pszTemp = 0x29;
		pszTemp++;
		shMsgLen++;

		//here send ucRspMsgBuf to Watch,  total size is  shMsgLen
		printf("send to watch: %s\n", ucRspMsgBuf);
	}

	if (m_bNeedSendChat)
	{
		ProcessRspChat();
	}

	if(m_bNeedSendChatNum == false)
		return ret;

	memset(&m_stMulChatInfo, 0, sizeof(m_stMulChatInfo));

	//we can do the logic of pushing chat data here...
	int iChatInfoNum = m_stMulChatInfo.m_shChatInfoNum;

	if (iChatInfoNum > 0)
	{
		unsigned char ucRspMsgBuf[256] = {0};
		ucRspMsgBuf[0] = '\0';
		short shMsgLen = 0;
		//同时 通知终端有聊天信息
		memset(ucRspMsgBuf, 0, sizeof(ucRspMsgBuf));
		unsigned char* pszTemp = ucRspMsgBuf;

		*(unsigned char*)pszTemp = 0x28;
		pszTemp++;
		shMsgLen++;
		sprintf((char*)pszTemp, "%015lld", m_i64SrcDeviceID);
		pszTemp += 15;
		shMsgLen += 15;

		memcpy(pszTemp, "AP15", sizeof(int));
		shMsgLen += sizeof(int);
		pszTemp += sizeof(int);

		char szChatNum[2];
		memset(szChatNum, 0, sizeof(szChatNum));
		sprintf(szChatNum, "%02d", iChatInfoNum);

		memcpy(pszTemp, szChatNum, sizeof(short));
		pszTemp += sizeof(short);
		shMsgLen += sizeof(short);

		*(unsigned char*)pszTemp = 0x29;
		pszTemp++;
		shMsgLen++;

		//send chat data(buffer: ucRspMsgBuf, size: shMsgLen) to watch here ....
	}

	return ret;
}

int NewWatchService::ProcessLocate(unsigned char* pszMsg, char cLocateTag)
{
	int iRet = 0;

	if (cLocateTag == 'G' || cLocateTag == 'g')
	{
		iRet = ProcessGPSInfo(pszMsg);
	}
	else if (cLocateTag == 'W' || cLocateTag == 'w')
	{
		iRet = ProcessWifiInfo(pszMsg);
		if(m_shWiFiNum <= 1)
		{
			m_bNeedSendLocation = false;
			return -1;
		}
	}
	else if (cLocateTag == 'L' || cLocateTag == 'l')
	{
		iRet = ProcessLBSInfo(pszMsg);
	}
	else if (cLocateTag == 'M' || cLocateTag == 'm')
	{
		iRet = ProcessMutilLocateInfo(pszMsg);
	}
	else
	{
		return -1;
	}

	if (iRet != 0 || (m_stDataInfo.m_iLatitude == 0 && m_stDataInfo.m_iLongtiTude == 0))
	{
		return iRet;
	}

	return 0;
}


int NewWatchService::ProcessGPSInfo(unsigned char* pszMsgBuf)
{
	int ret = 0;
	if (NULL == pszMsgBuf)
	{
		return -1;
	}

	pszMsgBuf++;
	bool bTrueTime = true;
	int iTimeDay = 0;
	if (*pszMsgBuf == '-')
	{
		pszMsgBuf++;
		bTrueTime = false;
		pszMsgBuf += sizeof(int) * 2;
	}
	else
	{
		iTimeDay = GetIntValue(pszMsgBuf, TIME_LEN);
		pszMsgBuf += TIME_LEN;
	}

	char cTag = *(char*)pszMsgBuf;
	if (cTag != 'A' && cTag != 'V')
	{
		return -1;
	}
	pszMsgBuf++;

	short shMaxTitudeLen = 10;
	int iLatitude = GetValue(pszMsgBuf, shMaxTitudeLen);
	pszMsgBuf += shMaxTitudeLen;

	int iTemp = iLatitude % 1000000;
	iLatitude = iLatitude - iTemp;

	double dLatitude = iTemp / 60.0;
	int iSubLatitude = (int)(dLatitude * 100);
	iLatitude += iSubLatitude;

	cTag = *(char*)pszMsgBuf;
	if (cTag == 'S')
	{
		iLatitude = 0 - iLatitude;
	}
	pszMsgBuf++;

	shMaxTitudeLen = 10;
	int iLongtitude = GetValue(pszMsgBuf, shMaxTitudeLen);

	iTemp = iLongtitude % 1000000;
	iLongtitude = iLongtitude - iTemp;

	double dLontitude = iTemp / 60.0;
	int iSubLontitude = (int)(dLontitude * 100);

	iLongtitude += iSubLontitude;
	pszMsgBuf += shMaxTitudeLen;
	cTag = *(char*)pszMsgBuf;
	if (cTag == 'W')
	{
		iLongtitude = 0 - iLongtitude;
	}
	pszMsgBuf++;

	float fSpeed = GetFloatValue(pszMsgBuf, sizeof(int) + sizeof(char));
	int iSpeed = (int)fSpeed * 10;
	pszMsgBuf += sizeof(int) + sizeof(char);

	int iTimeSec = GetIntValue(pszMsgBuf, TIME_LEN);
	pszMsgBuf += TIME_LEN;

	while(*pszMsgBuf && *pszMsgBuf != ',')
	{
		pszMsgBuf++;
	}
	pszMsgBuf++;
	m_cBattery_orig = *(char*)pszMsgBuf - '0';
	int iIOStatu = *(char*)pszMsgBuf - '0' - 2;
	if (iIOStatu < 1)
	{
		iIOStatu = 1;
	}

	int64_t i64Time =  0;
	if (bTrueTime)
	{
		i64Time = (unsigned long)iTimeDay * 1000000 + iTimeSec;
	}
	else
	{
		time_t tNow = time(NULL);
		struct tm utc_tm;
		struct tm* pTempTm = gmtime_r( &tNow, &utc_tm ); //获取格林威治时间
		if (NULL != pTempTm)
		{
			int iMonTime = 1602;
			int iDayTime = utc_tm.tm_mday;
			int iHTime = utc_tm.tm_hour;
			int iTemp = iHTime - 6;
			if (iTemp < 0)
			{
				iDayTime -= 1;
				iHTime = iTemp + 24;
			}
			else
			{
				iHTime = iTemp;
			}

			int iSec =utc_tm.tm_min * 100 + utc_tm.tm_sec;

			i64Time = (unsigned long)iMonTime * 100000000 + iDayTime * 1000000 + iHTime*10000
				+ iSec;
		}
	}

	m_stDataInfo.m_uiDataTime = i64Time;
	m_stDataInfo.m_iLongtiTude = iLongtitude;
	m_stDataInfo.m_iLatitude = iLatitude;
	m_stDataInfo.m_iSpeed = iSpeed;
	m_stDataInfo.m_ucReadFlag = 0;
	m_stDataInfo.m_cBattery = iIOStatu;
	m_stDataInfo.m_cStationType = LBS_GPS;

	printf("%d, %d, %d\n", iLatitude, iLongtitude, iIOStatu);

	return 0;
}


int NewWatchService::GetIntValue(unsigned char* pszMsgBuf, short shLen)
{
	char szValue[9];
	memset(szValue, 0, sizeof(szValue));
	if (shLen > 9)
	{
		shLen = 9;
	}

	memcpy(szValue, pszMsgBuf, shLen);

	return atoi(szValue);
}

float NewWatchService::GetFloatValue(unsigned char* pszMsgBuf, short shLen)
{
	char szValue[12];
	memset(szValue, 0, sizeof(szValue));
	if (shLen > 12)
	{
		shLen = 12;
	}

	memcpy(szValue, pszMsgBuf, shLen);

	return atof(szValue);
}

int NewWatchService::GetValue(unsigned char* pszMsgBuf, short& shMaxLen)
{
	short shTempLen = shMaxLen;
	shMaxLen = 0;
	if (NULL == pszMsgBuf)
	{
		return -1;
	}

	int iRetValue = 0;
	char* p = (char*)pszMsgBuf;
	while (*p)
	{
		if (*p >= '0' && *p <= '9')
		{
			iRetValue = iRetValue * 10 + (*p - '0');
			p++;
			shMaxLen++;
		}
		else if (*p == '.')
		{
			p++;
			shMaxLen++;
		}
		else
		{
			break;
		}

		if (shMaxLen >= shTempLen)
		{
			break;
		}
	}

	return iRetValue;
}


int NewWatchService::ProcessMicChat(unsigned char* pszMsgBuf, short shMsgLen)
{
	int ret = 0;

	if(this->m_bIsChatEx)
	{
		pszMsgBuf++;
		shMsgLen--;

		const char *strAll = "ALL";
		unsigned short str_len = strlen(strAll);
		if(!strncmp((char *)pszMsgBuf, strAll, str_len))
		{
			pszMsgBuf += str_len + sizeof(char);
			shMsgLen -= str_len + sizeof(char);
			memset(m_szChatExPhoneNo, 0, sizeof(m_szChatExPhoneNo));
			strcpy(m_szChatExPhoneNo, strAll);
		}
		else
		{
			unsigned int end_pos = 0;
			for(int i = 0; i < shMsgLen; i++)
			{
				if(pszMsgBuf[i] == ',')
				{
					end_pos = i;
					break;
				}
			}

			if(end_pos == 0)
			{
				ret = -1;
				return ret;
			}

			memset(m_szChatExPhoneNo, 0, sizeof(m_szChatExPhoneNo));
			memcpy(m_szChatExPhoneNo, pszMsgBuf, end_pos);
			pszMsgBuf += end_pos + sizeof(char);
			shMsgLen -= end_pos + sizeof(char);
		}
	}

	pszMsgBuf += sizeof(int) + sizeof(char);
	shMsgLen -= sizeof(int) + sizeof(char);

	char szTime[4];
	memset(szTime, 0, sizeof(szTime));
	memcpy(szTime, pszMsgBuf, sizeof(int));
	int iRecordTime = 0;
	int t = 0;
	for(short i=0;i < sizeof(int);i++)
	{
		if(szTime[i]<='9')
		{
			t=szTime[i]-'0';
		}
		else
		{
			t=szTime[i]-'A'+10;
		}

		iRecordTime = iRecordTime*16 + t;
	}

//	int iTemp = iRecordTime % 1000;
//	iRecordTime = iRecordTime /1000;
//	if (iTemp >= 500)
//	{
//		iRecordTime += 1;
//	}
//
//	if (iRecordTime < 1)
//	{
//		iRecordTime = 1;
//	}

	pszMsgBuf += sizeof(int) + sizeof(char);
	shMsgLen -= sizeof(int) + sizeof(char);
	int iDayTime = GetIntValue(pszMsgBuf, TIME_LEN);
	iDayTime = iDayTime % 1000000;
	pszMsgBuf += TIME_LEN;
	shMsgLen -= TIME_LEN;

	int iSecTime = GetIntValue(pszMsgBuf, TIME_LEN);
	int iIndex = GetIntValue(pszMsgBuf + TIME_LEN, 2) * 100;
	unsigned long i64Time_orig = (unsigned long)iDayTime * 1000000 + iSecTime;
	unsigned long i64Time = 0;

    timeval tv;
    gettimeofday(&tv, NULL);
    unsigned int usec = tv.tv_usec;
    while(usec >= 10000) usec /= 10;

    //char strtime[256]  = {0}; //datetime max len is not more than 25;
    struct tm local;
    localtime_r(&tv.tv_sec,&local);
	DeviceModelType device_model = DM_GTI3;
	if(iIndex || (!iIndex && device_model >= DM_GTI3 && device_model < DM_MAX))
		i64Time = i64Time_orig * 10000 + iIndex;
	else
	{
	    //sprintf(strtime,"%02d%02d%02d%02d%02d%02d%04d",(local.tm_year+1900)%100,local.tm_mon+1,local.tm_mday,local.tm_hour, local.tm_min, local.tm_sec, usec);
	    i64Time = (i64Time_orig + local.tm_sec) * 10000 + usec;//atoll(strtime);
	}

	pszMsgBuf += TIME_LEN + sizeof(short);
	shMsgLen -= TIME_LEN + sizeof(short);

	char szDirPath[256];
	memset(szDirPath, 0, sizeof(szDirPath));
	bool bSendAll = !(m_bIsChatEx && strcmp(m_szChatExPhoneNo, "ALL"));

    memset(&m_stChatInfo, 0, sizeof(m_stChatInfo));
    m_stChatInfo.m_i64Time = i64Time;
    m_stChatInfo.m_shSendFlag = 0;
    m_stChatInfo.m_ucSource = DATA_WATCH;
    m_stChatInfo.m_ushSec = iRecordTime;
    sprintf((char*)m_stChatInfo.m_szName, "%lld", m_i64DeviceID);

    if(!bSendAll)
    {
        sprintf((char*)m_stChatInfo.m_szSenderFamilyNo, "%s", m_szChatExPhoneNo);
    }

    m_chatDataSize = shMsgLen - 1;
    m_chatData = new char[m_chatDataSize];
    memcpy(m_chatData, pszMsgBuf, m_chatDataSize);

	//here we can do the logic of save voice chat data with .amr format...
	return 0;
}


int NewWatchService::ProcessRecordSMSInfo(unsigned char* pszMsg)
{
	int ret = 0;
	return ret;
}

int NewWatchService::ProcessRspAGPSInfo()
{
	int ret = 0;
	static unsigned char ucRspMsgBuf[30*1024] = {0};
	ucRspMsgBuf[0] = '\0';
	short shMsgLen = 0;

	unsigned char* pszTemp = ucRspMsgBuf;
	*(unsigned char*)pszTemp = 0x28;
	pszTemp++;
	shMsgLen++;

	sprintf((char*)pszTemp, "%015lld", m_i64SrcDeviceID);
	pszTemp += DEVICEID_BIT_NUM;
	shMsgLen += DEVICEID_BIT_NUM;

	memcpy(pszTemp, "AP17", sizeof(int));
	shMsgLen += sizeof(int);
	pszTemp += sizeof(int);

	char* pszLen = (char*)pszTemp;
	sprintf((char*)pszTemp, "%04x", shMsgLen);
	shMsgLen += sizeof(int);
	pszTemp += sizeof(int);

	time_t tNow = time(NULL);
	struct tm utc_tm;
	struct tm* pTempTm = gmtime_r( &tNow, &utc_tm ); //获取格林威治时间
	int iCurrentGPSHour = utc_to_gps_hour(pTempTm->tm_year + 1900, pTempTm->tm_mon + 1, pTempTm->tm_mday, pTempTm->tm_hour);

	int segment = (iCurrentGPSHour - m_iStartGPSHour) / 6;
	if ((segment < 0) || (segment >= MTKEPO_SEGMENT_NUM))
	{
		ret = -1;
		return ret;
	}

	for (short j = 0; j < MTKEPO_DATA_ONETIME; j++)
	{
		memcpy(pszTemp, m_astEPOInfo[j].m_ucEPOBuf, MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER);
		shMsgLen += MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER;
		pszTemp += MTKEPO_RECORD_SIZE * MTKEPO_SV_NUMBER;
	}

	*(unsigned char*)pszTemp = 0x29;
	pszTemp++;
	shMsgLen++;

	char szLen[4];
	memset(szLen, 0, sizeof(int));
	sprintf(szLen, "%04x", shMsgLen);
	memcpy(pszLen, szLen, sizeof(int));

	//should send AGPS data to Watch here....
	return ret;
}

int NewWatchService::utc_to_gps_hour (int iYr, int iMo, int iDay, int iHr)
{
	int iYearsElapsed;     // Years since 1980
	int iDaysElapsed;      // Days elapsed since Jan 6, 1980
	int iLeapDays;         // Leap days since Jan 6, 1980
	int i;

	// Number of days into the year at the start of each month (ignoring leap years)
	const unsigned short doy[12] = {0,31,59,90,120,151,181,212,243,273,304,334};

	iYearsElapsed = iYr - 1980;
	i = 0;
	iLeapDays = 0;

	while (i <= iYearsElapsed)
	{
		if ((i % 100) == 20)
		{
			if ((i % 400) == 20)
			{
				iLeapDays++;
			}
		}
		else if ((i % 4) == 0)
		{
			iLeapDays++;
		}
		i++;
	}
	if ((iYearsElapsed % 100) == 20)
	{
		if (((iYearsElapsed % 400) == 20) && (iMo <= 2))
		{
			iLeapDays--;
		}
	}
	else if (((iYearsElapsed % 4) == 0) && (iMo <= 2))
	{
		iLeapDays--;
	}

	iDaysElapsed = iYearsElapsed * 365 + (int)doy[iMo - 1] + iDay + iLeapDays - 6;

	return  (iDaysElapsed * 24 + iHr);
}


int NewWatchService::ProcessUpdateWatchStatus(unsigned char* pszMsgBuf)
{
	if (NULL == pszMsgBuf || *pszMsgBuf < '0' || *pszMsgBuf > '9')
	{
		return -1;
	}

	int nStep = 0, nField = 0;
	unsigned char ucBattery = 0;
	while (*pszMsgBuf >= '0'  && *pszMsgBuf <= '9')
	{
		nField = nField * 10 + *pszMsgBuf - '0';
		pszMsgBuf++;
	}

	if(*pszMsgBuf == ')' && nField < 10 && nField > 0) //nField is battery value
	{
		m_cBattery_orig = nField;
		ucBattery = (nField > 2) ? (nField -2) : 1;
		//Here ucBattery is the current battery value shown on Watch
		// 1 -- 20%, 5 -- 100%

		return 0;
	}

	nStep = nField;  //nField is step value
	if (nStep >= 0x7fff - 1 || nStep < 0)
	{
		nStep = 0;
	}

	pszMsgBuf++;
	int nStatu = *pszMsgBuf - '0';
	pszMsgBuf += sizeof(short);

	int iLocateType = LBS_NORMAL;
	if (nStatu != 0)
	{
		iLocateType = LBS_SMARTLOCATION;
	}

	int iDayTime = GetIntValue(pszMsgBuf, TIME_LEN);
	iDayTime = iDayTime % 1000000;
	pszMsgBuf += TIME_LEN + sizeof(char);

	int iSecTime = GetIntValue(pszMsgBuf, TIME_LEN);
	unsigned long i64Time = (unsigned long)iDayTime * 1000000 + iSecTime;
	pszMsgBuf += TIME_LEN + sizeof(char);
	ucBattery = *pszMsgBuf - '0';
	m_cBattery_orig = *pszMsgBuf - '0';

	WatchStatus stWatchStatus;
	stWatchStatus.m_i64DeviceID = m_i64DeviceID;
	stWatchStatus.m_iLocateType = iLocateType;
	stWatchStatus.m_i64Time = i64Time;
	stWatchStatus.m_iStep = nStep;

	m_stDataInfo.m_uiDataTime = i64Time;
	m_stDataInfo.m_iSpeed = nStep;
	m_stDataInfo.m_cStationType = iLocateType;

	//stWatchStatus is the parsed result.
	return 0;
}

int NewWatchService::ProcessRspChat()
{
	TMulChatInfo stChatInfo;

	m_shReqChatInfoNum =  m_shReqChatInfoNum <= 3 ? m_shReqChatInfoNum : 3;

	memset(&stChatInfo, 0, sizeof(stChatInfo));
	int iRet = 0;

	//Here put the logic to get voice chat data which will be sent to Watch...
	for (short i = 0; i < stChatInfo.m_shChatInfoNum; i++)
	{
		TChatInfo& stInfo = stChatInfo.m_astChatInfo[i];
		unsigned char ucRspMsgBuf[20*1024] = {0};
		memset(ucRspMsgBuf, 0, sizeof(ucRspMsgBuf));
		short shMsgLen = 0;

		unsigned char* pszTemp = ucRspMsgBuf;
		*(unsigned char*)pszTemp = 0x28;
		pszTemp++;
		shMsgLen++;

		sprintf((char*)pszTemp, "%015lld", m_i64SrcDeviceID);
		pszTemp += DEVICEID_BIT_NUM;
		shMsgLen += DEVICEID_BIT_NUM;

		memcpy(pszTemp, "AP16", sizeof(int));
		shMsgLen += sizeof(int);
		pszTemp += sizeof(int);

		char* pszLen = (char*)pszTemp;
		sprintf((char*)pszTemp, "%04x", shMsgLen);
		shMsgLen += sizeof(int);
		pszTemp += sizeof(int);

		//then copy buffer data of voice into pszTemp...
		const int MAX_FILE_LEN = 20240;
		int iReadLen = 0; //fread(pszTemp, 1, MAX_FILE_LEN, pstFile);

		shMsgLen += iReadLen;
		pszTemp += iReadLen;

		*(unsigned char*)pszTemp = 0x29;
		pszTemp++;
		shMsgLen++;

		char szLen[5];
		memset(szLen, 0, 5);
		sprintf(szLen, "%04x", shMsgLen);
		memcpy(pszLen, szLen, sizeof(int));

		//Here send data to Watch...
	}

	return 0;
}

int NewWatchService::ProcessWifiInfo(unsigned char* pszMsgBuf)
{
	int ret = 0;
	if (NULL == pszMsgBuf)
	{
		return -1;
	}

	pszMsgBuf++;
	int iDayTime = GetIntValue(pszMsgBuf, TIME_LEN);
	iDayTime = iDayTime % 1000000;
	pszMsgBuf += TIME_LEN + sizeof(char);

	int iSecTime = GetIntValue(pszMsgBuf, TIME_LEN);
	unsigned long i64Time = (unsigned long)iDayTime * 1000000 + iSecTime;
	pszMsgBuf += TIME_LEN + sizeof(char);

	unsigned char* pszTemp = pszMsgBuf;
	short shBufLen = 0;
	ParseWifiInfo(pszTemp, shBufLen);
	pszMsgBuf += shBufLen;

	m_stDataInfo.m_uiDataTime = i64Time;
	m_cBattery_orig = *(char*)pszMsgBuf - '0';
	m_stDataInfo.m_cBattery = *(char*)pszMsgBuf - '0' - 2;
	if (m_stDataInfo.m_cBattery < 1)
	{
		m_stDataInfo.m_cBattery = 1;
	}

	pszMsgBuf++;
	if (*pszMsgBuf == ',')
	{
		pszMsgBuf++;
		int iValue = *(char*)pszMsgBuf - '0';
		if (iValue == 1)
		{
			m_bNeedSendLocation = true;
		}
	}

	int iLongTitude = 0;
	int iLatitude =0;
	int iType = 0;

	//Here should parse Wifi data by google geolocation API or others to get the iLatitude and iLongTitude...

	m_stDataInfo.m_iLongtiTude = iLongTitude;
	m_stDataInfo.m_iLatitude = iLatitude;
	m_stDataInfo.m_iSpeed = 0;
	m_stDataInfo.m_ucReadFlag = 0;
	m_stDataInfo.m_cStationType = LBS_WIFI;
	m_stDataInfo.m_iReserve = 0;

	//m_stDataInfo is the parsed result.
	return 0;
}

int NewWatchService::ParseWifiInfo(unsigned char* pszMsgBuf, short& shBufLen)
{
	shBufLen = 0;
	if (NULL == pszMsgBuf)
	{
		return -1;
	}

	m_shWiFiNum = 0;
	memset(m_astWifiInfo, 0, sizeof(m_astWifiInfo));

	short shWifiNum = *(char*)pszMsgBuf - '0';
	pszMsgBuf += sizeof(short);
	shBufLen += sizeof(short);

	if (shWifiNum > MAX_WIFI_NUM)
	{
		shWifiNum = MAX_WIFI_NUM;
	}

	m_shWiFiNum = 0;
	short shIndex = 0;
	short shWifiNameLen = 0;
	short shMacIDLen = 0;
	while(*pszMsgBuf && shWifiNum > 0)
	{
		TWIFIInfo& stWifiInfo = m_astWifiInfo[m_shWiFiNum];
        if (m_bNewWifiStruct && shIndex == 0)
        {
            unsigned char hi = ((*pszMsgBuf) >= 'A' && (*pszMsgBuf) <= 'F') ? ((*pszMsgBuf) - 'A' + 10) : ((*pszMsgBuf) - '0');
            unsigned char low = ((*(pszMsgBuf + 1)) >= 'A' && (*(pszMsgBuf + 1)) <= 'F') ? ((*(pszMsgBuf + 1)) - 'A' + 10) : ((*(pszMsgBuf + 1)) - '0');
            short len = 16 * hi + low;
            if (len < MAX_WIFI_NAME_LEN)
            {
                memcpy(stWifiInfo.m_szWIFIName, pszMsgBuf + 3, len);
                pszMsgBuf += 4 + len;
                shBufLen += 4 + len;
                shIndex++;
            }

        }
		if (*pszMsgBuf == ',')
		{
			shIndex++;
			if (shIndex == 3)
			{
				string strWifiName = stWifiInfo.m_szWIFIName;
				size_t szPos = strWifiName.find("iPhone");
				size_t szPos1 = strWifiName.find("Android");
				if (szPos != std::string::npos || szPos1 != std::string::npos)
				{
					//DONOTING
				}
				else
				{
					if (m_shWiFiNum < MAX_WIFI_NUM)
					{
						m_shWiFiNum++;
					}
				}

				shIndex = 0;
				shWifiNameLen = 0;
				shMacIDLen = 0;
				if (stWifiInfo.m_szWIFIName[0] == '\0')
				{
					memcpy(stWifiInfo.m_szWIFIName, "mark", sizeof(int));
				}

				shWifiNum--;
			}
		}
		else
		{
			switch(shIndex)
			{
			case 0:
				{
					if (shWifiNameLen < MAX_WIFI_NAME_LEN && (isdigit(*pszMsgBuf) || isalpha(*pszMsgBuf)))
					{
						stWifiInfo.m_szWIFIName[shWifiNameLen++] = *pszMsgBuf;
					}
				}
				break;
			case 1:
				{
					stWifiInfo.m_szMacID[shMacIDLen++] = *pszMsgBuf;
				}
				break;
			case 2:
				{
					if (*pszMsgBuf != '-')
					{
						stWifiInfo.m_shRatio = stWifiInfo.m_shRatio * 10 + *pszMsgBuf - '0';
					}
				}
				break;
			default:
				break;
			}
		}

		pszMsgBuf++;
		shBufLen++;
	}

	if (m_shWiFiNum > MAX_WIFI_NUM)
	{
		m_shWiFiNum = MAX_WIFI_NUM;
	}

	return 0;
}


int NewWatchService::ProcessLBSInfo(unsigned char* pszMsgBuf)
{
	if (NULL == pszMsgBuf)
	{
		return -1;
	}

	pszMsgBuf++;
	unsigned char* pszTemp = pszMsgBuf;

	short shLBSLen = 0;
	ParseLBSInfo(pszTemp,  shLBSLen);
	pszMsgBuf += shLBSLen;

	if (!isdigit(*pszMsgBuf))
	{
		pszMsgBuf++;
	}

    int iDayTime = GetIntValue(pszMsgBuf, TIME_LEN);
	iDayTime = iDayTime % 1000000;
	pszMsgBuf += TIME_LEN + sizeof(char);

	int iSecTime = GetIntValue(pszMsgBuf, TIME_LEN);
	unsigned long i64Time = (unsigned long)iDayTime * 1000000 + iSecTime;

	pszMsgBuf += TIME_LEN + sizeof(char);
	m_cBattery_orig = *(char*)pszMsgBuf - '0';
	m_stDataInfo.m_cBattery = *(char*)pszMsgBuf - '0' - 2;
	if (m_stDataInfo.m_cBattery < 1)
	{
		m_stDataInfo.m_cBattery = 1;
	}

	pszMsgBuf++;
	if (*pszMsgBuf == ',')
	{
		pszMsgBuf++;
		int iValue = *(char*)pszMsgBuf - '0';
		if (iValue == 1)
		{
			m_bNeedSendLocation = true;
		}
	}

	if (m_shLBSNum <= 1)
	{
		WatchStatus stWatchStatus;
		stWatchStatus.m_i64DeviceID = m_i64DeviceID;
		stWatchStatus.m_i64Time = i64Time;
		stWatchStatus.m_iLocateType = LBS_SMARTLOCATION;
		stWatchStatus.m_iStep = 0;

		m_bGetSameWifi = true;
		return 0;
	}

	int iLongTitude = 0;
	int iLatitude =0;
	int iType = 0;

	//to parse LBS data by google geolocation API or others...

	m_stDataInfo.m_uiDataTime = i64Time;
	m_stDataInfo.m_iLongtiTude = iLongTitude;
	m_stDataInfo.m_iLatitude = iLatitude;
	m_stDataInfo.m_iSpeed = 0;
	m_stDataInfo.m_ucReadFlag = 0;
	m_stDataInfo.m_cStationType = LBS_JIZHAN;

	return 0;
}

int NewWatchService::ParseLBSInfo(unsigned char* pszMsgBuf, short& shBufLen)
{
	if (NULL == pszMsgBuf)
	{
		return -1;
	}

	m_shLBSNum = 0;
	shBufLen = 0;
	memset(m_astLBSInfo, 0, sizeof(m_astLBSInfo));

	short shLBSNum = *(unsigned char*)pszMsgBuf - '0';
	if (shLBSNum > MAX_LBS_NUM)
	{
		shLBSNum = MAX_LBS_NUM;
	}
	pszMsgBuf += sizeof(short);
	shBufLen += sizeof(short);
	char szLBSInfo[32];
	memset(szLBSInfo, 0, sizeof(szLBSInfo));
	short shIndex = 0;
	char szLac[4] = {0};
	char szCellID[4] = {0};
	while(*pszMsgBuf && shLBSNum > 0)
	{
		if (*pszMsgBuf == ',')
		{
			TLBSInfo& stLBSInfo = m_astLBSInfo[m_shLBSNum];
			memset(szLac, 0, sizeof(szLac));
			memset(szCellID, 0, sizeof(szCellID));
			char* pStart = szLBSInfo;
			stLBSInfo.m_iMcc = GetIntValue((unsigned char*)pStart, 3);
			pStart += 3;
			if (shIndex >= 16)
			{
				stLBSInfo.m_iMnc = GetIntValue((unsigned char*)pStart, 3);
				pStart += 3;
			}
			else
			{
				stLBSInfo.m_iMnc = GetIntValue((unsigned char*)pStart, sizeof(short));
				pStart += sizeof(short);
			}

			stLBSInfo.m_iLac = hex_to_decimal(pStart, sizeof(int));
			pStart += sizeof(int);
			stLBSInfo.m_iCellID = hex_to_decimal(pStart, sizeof(int));
			pStart += sizeof(int);
			stLBSInfo.m_iSignal = hex_to_decimal(pStart, sizeof(short))*2 - 113;
			if (stLBSInfo.m_iSignal > 0)
			{
				stLBSInfo.m_iSignal = 0;
			}

			m_shLBSNum++;
			shLBSNum--;
			shIndex = 0;
			memset(szLBSInfo, 0, sizeof(szLBSInfo));
		}
		else
		{
			szLBSInfo[shIndex++] = *pszMsgBuf;
		}

		pszMsgBuf++;
		shBufLen++;
	}

	if (m_shLBSNum >= MAX_LBS_NUM)
	{
		m_shLBSNum = MAX_LBS_NUM;
	}

	return 0;
}

int NewWatchService::hex_char_value( char  c)
{
	if (c >=  '0'  && c <=  '9' )
		return  c -  '0' ;
	else   if (c >=  'a'  && c <=  'f' )
		return  (c -  'a'  + 10);
	else   if (c >=  'A'  && c <=  'F' )
		return  (c -  'A'  + 10);
	return  0;
}

int NewWatchService::hex_to_decimal(char * szHex,  int  len)
{
	int  result = 0;
	for ( int  i = 0; i < len; i++)
	{
		result += (int )pow(( float )16, ( int )len-i-1) * hex_char_value(szHex[i]);
	}
	return  result;
}


int NewWatchService::ProcessMutilLocateInfo(unsigned char* pszMsgBuf)
{
	if (NULL == pszMsgBuf)
	{
		return -1;
	}

	pszMsgBuf++;
	int iDayTime = GetIntValue(pszMsgBuf, TIME_LEN);
	iDayTime = iDayTime % 1000000;
	pszMsgBuf += TIME_LEN + sizeof(char);

	int iSecTime = GetIntValue(pszMsgBuf, TIME_LEN);
	unsigned long i64Time = (unsigned long)iDayTime * 1000000 + iSecTime;
	pszMsgBuf += TIME_LEN + sizeof(char);

	unsigned char* pszTemp = pszMsgBuf;
	short shBufLen = 0;
	ParseLBSInfo(pszTemp, shBufLen);
	pszMsgBuf += shBufLen;

	shBufLen = 0;
	if (!isdigit(*pszMsgBuf))
	{
		pszMsgBuf++;
	}

	pszTemp = pszMsgBuf;
	ParseWifiInfo(pszTemp, shBufLen);
	pszMsgBuf += shBufLen;
	if (!isdigit(*pszMsgBuf))
	{
		pszMsgBuf++;
	}

	m_stDataInfo.m_uiDataTime = i64Time;
	m_cBattery_orig = *(char*)pszMsgBuf - '0';
	m_stDataInfo.m_cBattery = *(char*)pszMsgBuf - '0' - 2;
	if (m_stDataInfo.m_cBattery < 1)
	{
		m_stDataInfo.m_cBattery = 1;
	}

	pszMsgBuf++;
	if (*pszMsgBuf == ',')
	{
		pszMsgBuf++;
		int iValue = *(char*)pszMsgBuf - '0';
		if (iValue == 1)
		{
			m_bNeedSendLocation = true;
		}
	}

	int iLongTitude = 0;
	int iLatitude =0;
	int iType = 0;

	//to parse LBS+Wifi mixed data by google geolocation API or others...

	m_stDataInfo.m_iLongtiTude = iLongTitude;
	m_stDataInfo.m_iLatitude = iLatitude;
	m_stDataInfo.m_iSpeed = 0;
	m_stDataInfo.m_ucReadFlag = 0;
	m_stDataInfo.m_cStationType = LBS_WIFI;

	return 0;
}

struct timestruct {
    short y;
    short m;
    short d;
    short h;
    short mi;
    short s;
};

timestruct getTimeStruct(long long time){
    timestruct t;
    t.y = time / 10000000000;
    t.m = (time - t.y * 10000000000) / 100000000;
    t.d = (time - t.y * 10000000000 - t.m * 100000000) / 1000000;
    t.h = (time - t.y * 10000000000 - t.m * 100000000 - t.d * 1000000) / 10000;
    t.mi = (time - t.y * 10000000000 - t.m * 100000000 - t.d * 1000000 - t.h * 10000) / 100;
    t.s = 0;
    return t;
}

static unsigned short daysForMonth[] = {
    //1  2   3   4   5   6   7   8   9  10  11  12
    31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31
};

bool isLeapYear(unsigned int year){
    return (year % 4 == 0 && year % 100 != 0 || year % 400 == 0);
}

int deltaMinutes(long long time1, long long time2){
	timestruct t1 = getTimeStruct(time1);
    timestruct t2 = getTimeStruct(time2);
    unsigned int days1 = 0, days2 = 0;

    for(int i = 0; i < t1.m - 1; i++){
        unsigned char leapYear = (i == 1 && isLeapYear(2000 + t1.y)) ? 1 : 0;
        days1 += daysForMonth[i] + leapYear;
    }
    
    days1 += t1.d;
    
    for (int i = 0; i < (t2.y - t1.y); i++) {
        for (int j = 0; j < 12; j++) {
            unsigned char leapYear = (j == 1 && isLeapYear(2000 + t1.y + i)) ? 1 : 0;
            days2 += daysForMonth[j] + leapYear;
        }
    }
    for(int i = 0; i < t2.m - 1; i++){
        unsigned char leapYear = (i == 1 && isLeapYear(2000 + t2.y)) ? 1 : 0;
        days2 += daysForMonth[i] + leapYear;
    }
    
    days2 += t2.d;
    
    return (days2 - days1) * 24 * 60 + (t2.h - t1.h) * 60 + (t2.mi - t1.mi);
}

double NewWatchService::rad(double d)
{
	return d * PI / 180.0;
}

int NewWatchService::GetDisTance(TPoint stPoint1, TPoint stPoint2)
{
	double lng1 = (double(stPoint1.m_iLongtiTude)) / BASE_TITUDE;
	double lat1 = (double(stPoint1.m_iLatitude)) / BASE_TITUDE;
	double lng2 = (double(stPoint2.m_iLongtiTude)) / BASE_TITUDE;
	double lat2 = (double(stPoint2.m_iLatitude)) / BASE_TITUDE;

	double radLat1 = rad(lat1);
	double radLat2 = rad(lat2);
	double a = radLat1 - radLat2;
	double b = rad(lng1) - rad(lng2);
	double dDistance = 2 * sin(sqrt(pow(sin(a / 2), 2) +
		cos(radLat1) * cos(radLat2) * pow(sin(b / 2), 2)));

	dDistance = dDistance * EARTH_RADIUS;
	dDistance = dDistance * 1000;

	return (int)dDistance;
}

void NewWatchService::copyOutData(Gt3ServiceResult &result){
    result.m_uiDeviceUin = m_uiDeviceUin;
    result.m_i64DeviceID = m_i64DeviceID;
    result.m_i64SrcDeviceID = m_i64SrcDeviceID;
    result.m_iAlarmStatu = m_iAlarmStatu;
    result.m_shReqChatInfoNum = m_shReqChatInfoNum;
    result.m_shRspCmdType= m_shRspCmdType;
    result.m_iTimeZone = m_iTimeZone;
    result.m_bNeedSetTimeZone = m_bNeedSetTimeZone;
    memcpy(&result.m_stDataInfo, &m_stDataInfo, sizeof(result.m_stDataInfo));
    memcpy(&result.m_stOldDataInfo, &m_stOldDataInfo, sizeof(result.m_stOldDataInfo));
    result.m_cBattery_orig = m_cBattery_orig;
    result.m_cBattery_orig_old = m_cBattery_orig_old;
    result.m_bNeedSendChat = m_bNeedSendChat;
    result.m_bNeedSendChatNum = m_bNeedSendChatNum;
    result.m_bIsChatEx = m_bIsChatEx;
    strcpy(result.m_szChatExPhoneNo,m_szChatExPhoneNo);
    result.m_iAccracy = m_iAccracy;
    result.m_bNeedSendLocation = m_bNeedSendLocation;
    result.m_bGetWatchDataInfo = m_bGetWatchDataInfo;
    result.m_bGetSameWifi = m_bGetSameWifi;
    result.m_bNewWifiStruct = m_bNewWifiStruct;
    result.m_shWiFiNum = m_shWiFiNum;
    memcpy(result.m_astWifiInfo, m_astWifiInfo, sizeof(TWIFIInfo) * MAX_WIFI_NUM);
    result.m_shLBSNum = m_shLBSNum;
    memcpy(result.m_astLBSInfo, m_astLBSInfo, sizeof(TLBSInfo) * MAX_LBS_NUM);
    memcpy(&result.chatInfo, &m_stChatInfo, sizeof(m_stChatInfo));
    result.m_chatData = m_chatData;
    result.m_chatDataSize = m_chatDataSize;
}

void *createGt3Service(){
	return new NewWatchService;
}

void destoryGt3Service(void *service){
	if(service){
		delete service;
		service = NULL;
	}
}

int gt3ServiceHandleRequest(void *service, const char *msg, unsigned int msg_len){
	if(service){
		return ((NewWatchService*)service)->HandleRequest(msg, msg_len);
	}

	return -1;
}

void gt3ServiceDo(void *service, Gt3ServiceResult *result){
    if(result == NULL){
        return;
    }

    memset(result, 0, sizeof(*result));
    result->lastErr = -1;
	if(service){
		result->lastErr =  ((NewWatchService*)service)->Do();
		if(result->lastErr == 0){
		    ((NewWatchService*)service)->copyOutData(*result);
		}
	}
}

int gt3ServiceHandleResponse(void *service){
	if(service){
		return ((NewWatchService*)service)->HandleResponse();
	}

	return -1;
}