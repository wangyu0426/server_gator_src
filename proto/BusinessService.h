#pragma once

const int DEVICE_CMD_LENGTH = 4;
const int TIME_LEN = 6;


#define PI 3.1415926
#define BASE_TITUDE 1000000
const double EARTH_RADIUS = 6378.137;

enum DeviceModelType
{
	DM_MIN = -1,

	DM_WH01,
	DM_GT03,
	DM_GTI3,
	DM_GT06,

	DM_MAX,
};

struct BusinessService
{
public:
	virtual int HandleRequest(const char *msg, unsigned int msg_len) = 0;
	virtual int Do() = 0;
	virtual int HandleResponse() = 0;

protected:
	char* m_pszMsgBuf;
	short m_shBufLen;
};

