package proto

import (
	"net/http"
	"io/ioutil"
	"strings"
	"../logging"
	"io"
)

type reportJSONData struct {
	FamilyNumber string `json:"familyNumber"`
	DeviceToken string `json:"deviceToken" binding:"required"`
	Platform  string `json:"platform"`
	Language string `json:"language"`
}

type reportJSON struct {
	Imei     uint64 `json:"imei" binding:"required"`
	UUID        string `json:"uuid" binding:"required"`
	Data  reportJSONData `json:"data" binding:"required"`
}

type pushJSON struct {
	Imei     uint64 `json:"imei" binding:"required"`
	FamilyNumber string `json:"familyNumber"`
	OwnerName string `json:"ownerName"`
	Time uint64  `json:"time"`
	AlarmType uint8  `json:"larmType"`
	ZoneName string `json:"zoneName"`
}


func ReportDevieeToken(apiBaseURL string, imei uint64, familyNumber, uuid, deviceToken, platform, lang  string )  {
	urlRequest := apiBaseURL + "/report"
	reportInfo := reportJSON{Imei: imei, UUID: uuid, Data: reportJSONData{familyNumber, deviceToken, platform, lang}}
	reader := strings.NewReader(MakeStructToJson(&reportInfo))
	requestAPNS(urlRequest, imei, reader)
}


func PushNotificationToApp(apiBaseURL string, imei uint64, familyNumber,  ownerName string, datatime uint64,
	alarmType uint8, zoneName string)  {
	urlRequest := apiBaseURL + "/push"
	pushInfo := pushJSON{imei, familyNumber, ownerName, datatime, alarmType, zoneName}
	reader := strings.NewReader(MakeStructToJson(&pushInfo))
	requestAPNS(urlRequest, imei, reader)
}

func requestAPNS(apiURL string,  imei uint64, bodyParams  io.Reader)  bool {
	resp, err := http.Post(apiURL, "application/json",  bodyParams)
	if err != nil {
		logging.Log(Num2Str(imei, 10) + " app  requestAPNS failed, " + err.Error())
		return false
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log(Num2Str(imei, 10) + " requestAPNS response has err, " + err.Error())
		return false
	}

	logging.Log(Num2Str(imei, 10) + " requestAPNS response body: " + string(body))
	return true
}