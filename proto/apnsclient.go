package proto

import (
	"net/http"
	"io/ioutil"
	"strings"
	"../logging"
	"io"
	"fmt"
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

type deleteJSON struct {
	Imei     uint64 `json:"imei" binding:"required"`
	UUID        string `json:"uuid" binding:"required"`
}

type pushJSON struct {
	Imei     uint64 `json:"imei" binding:"required"`
	FamilyNumber string `json:"familyNumber"`
	OwnerName string `json:"ownerName"`
	Time uint64  `json:"time"`
	AlarmType uint8  `json:"alarmType"`
	ZoneName string `json:"zoneName"`
}


func ReportDevieeToken(apiBaseURL string, imei uint64, familyNumber, uuid, deviceToken, platform, lang  string )  {
	urlRequest := apiBaseURL + "/report"
	reportInfo := reportJSON{Imei: imei, UUID: uuid, Data: reportJSONData{familyNumber, deviceToken, platform, lang}}
	logging.Log(fmt.Sprintf("%d report device token: %s", imei, MakeStructToJson(&reportInfo)))
	reader := strings.NewReader(MakeStructToJson(&reportInfo))
	requestAPNS(urlRequest, imei, reader)
}

func DeleteDevieeToken(apiBaseURL string, imei uint64, uuid string )  {
	urlRequest := apiBaseURL + "/delete"
	deleteInfo := deleteJSON{Imei: imei, UUID: uuid}
	logging.Log(fmt.Sprintf("%d delete device token: %s", imei, uuid))
	reader := strings.NewReader(MakeStructToJson(&deleteInfo))
	requestAPNS(urlRequest, imei, reader)
}

func PushNotificationToApp(apiBaseURL string, imei uint64, familyNumber,  ownerName string, datatime uint64,
	alarmType uint8, zoneName string)  {
	urlRequest := apiBaseURL + "/push"
	pushInfo := pushJSON{imei, familyNumber, ownerName, datatime, alarmType, zoneName}
	logging.Log(fmt.Sprintf("%d push notification: %s", imei, MakeStructToJson(&pushInfo)))
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