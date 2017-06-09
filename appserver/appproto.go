package appserver

import (
	"encoding/json"
	"../logging"
	"net/http"
	"net/url"
	"io/ioutil"
	"../proto"
	"../svrctx"
	"fmt"
	"time"
	"strconv"
	//"github.com/jackc/pgx"
	//"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"strings"
	"encoding/base64"
)

const (
	base64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
)

var coder = base64.NewEncoding(base64Table)

func base64Encode(src []byte) []byte {
	return []byte(coder.EncodeToString(src))
}

func base64Decode(src []byte) ([]byte, error) {
	return coder.DecodeString(string(src))
}

func HandleAppRequest(c *AppConnection, appserverChan chan *proto.AppMsgData, data []byte) bool {
	var itf interface{}
	err:=json.Unmarshal(data, &itf)
	if err != nil {
		return false
	}

	msg:= itf.(map[string]interface{})
	cmd :=  msg["cmd"]
	if cmd == nil {
		return false
	}

	switch cmd{
	case "login":
		params := msg["data"].(map[string]interface{})
		return login(c, params["username"].(string), params["password"].(string))

	case "heartbeat":
		datas := msg["data"].(map[string]interface{})
		//params := proto.HeartBeatParams{TimeStamp: datas["timestamp"].(string),
		//	UserName: datas["username"].(string),
		//	AccessToken: datas["accessToken"].(string)}

		appServerChan <- &proto.AppMsgData{Cmd: "heartbeat-ack",
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string),
			Data: (fmt.Sprintf("{\"timestamp\": \"%s\"}", time.Now().String())), Conn: c}
		break

	case "verify-code":
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceVerifyCodeParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string)}
		return getDeviceVerifyCode(c, &params)
	case "set-device":
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.DeviceSettingParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("set-device parse json failed, " + err.Error())
			return false
		}
		return AppUpdateDeviceSetting(c, &params)

	default:
		break
	}
	
	return true
}

func login(c *AppConnection, username, password string) bool {
	resp, err := http.PostForm("http://service.gatorcn.com/tracker/web/index.php?r=app/auth/login",
		url.Values{"username": {username}, "password": {password}})
	if err != nil {
		logging.Log("app login failed, " + err.Error())
		return false
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log("response has err, " + err.Error())
		return false
	}

	var itf interface{}
	err = json.Unmarshal(body, &itf)
	if err != nil {
		logging.Log("parse login response as json failed, " + err.Error())
		return false
	}

	loginData := itf.(map[string]interface{})
	if loginData == nil {
		return false
	}

	status :=  loginData["status"]
	accessToken := loginData["accessToken"]
	devices := loginData["devices"]

	//logging.Log("status: " + fmt.Sprint(status))
	//logging.Log("accessToken: " + fmt.Sprint(accessToken))
	//logging.Log("devices: " + fmt.Sprint(devices))
	if status != nil && accessToken != nil && devices != nil {
		c.user.AccessToken = accessToken.(string)
		c.user.Logined = true
		c.user.Name = username
		c.user.PasswordMD5 = password

		addConnChan <- c

		//devicesLocationURL := "http://184.107.50.180:8012/GetMultiWatchData?systemno="
		locations := []proto.LocationData{}
		for _, d := range devices.([]interface{}) {
			device := d.(map[string]interface {})
			imei, _ := strconv.ParseUint(device["IMEI"].(string), 0, 0)
			logging.Log("device: " + fmt.Sprint(imei))
			c.imeis = append(c.imeis, imei)
			locations = append(locations, svrctx.GetDeviceData(imei, svrctx.Get().PGPool))

			//devicesLocationURL += device["IMEI"].(string)[4:]
			//
			//if i < len(devices.([]interface{})) - 1 {
			//	devicesLocationURL += "|"
			//}
		}

		jsonLocations , _ := json.Marshal(locations)

		//logging.Log("devicesLocationURL: " + devicesLocationURL)
		//
		//respLocation, err := http.Get(devicesLocationURL)
		//if err != nil {
		//	logging.Log("get devicesLocationURL failed" + err.Error())
		//}
		//
		//defer respLocation.Body.Close()

		//bodyLocation, err := ioutil.ReadAll(respLocation.Body)
		//if err != nil {
		//	logging.Log("response has err, " + err.Error())
		//}
		//
		//logging.Log("bodyLocation: " +  string(bodyLocation))

		appServerChan <- &proto.AppMsgData{Cmd: "login-ack",
			Data: (fmt.Sprintf("{\"user\": %s, \"location\": %s}", string(body), string(jsonLocations))), Conn: c}
	}

	return true
}

func getDeviceVerifyCode(c *AppConnection, params *proto.DeviceVerifyCodeParams) bool {
	//url := fmt.Sprintf("http://service.gatorcn.com/tracker/web/index.php?r=app/service/verify-code&SystemNo=%s&access-token=%s",
	//	string(params.Imei[4: ]), params.AccessToken)
	//logging.Log("get verify code url: " + url)
	//resp, err := http.Get(url)
	//if err != nil {
	//	logging.Log(fmt.Sprintf("[%s] get device verify code  failed, ", params.Imei, err.Error()))
	//	return false
	//}
	//
	//defer resp.Body.Close()
	//
	//body, err := ioutil.ReadAll(resp.Body)
	//if err != nil {
	//	logging.Log("response has err, " + err.Error())
	//	return false
	//}
	//
	//logging.Log("get verify code body: " + string(body))


	imei, verifyCode := proto.Str2Num(params.Imei, 10), ""
	proto.DeviceInfoListLock.RLock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	if ok && deviceInfo != nil {
		verifyCode = deviceInfo.VerifyCode
	}
	proto.DeviceInfoListLock.RUnlock()

	appServerChan <- &proto.AppMsgData{Cmd: "verify-code-ack", Imei: imei,
		UserName: params.UserName, AccessToken:params.AccessToken,
			Data: fmt.Sprintf("{\"VerifyCode\": \"%s\"}", verifyCode), Conn: c}

	return true
}

func SaveDeviceSettings(imei uint64, settings []proto.SettingParam, valulesIsString []bool)  bool {
	proto.DeviceInfoListLock.Lock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	if ok && deviceInfo != nil {
		for _, setting := range settings {
			switch setting.FieldName {
			case "Avatar":
				deviceInfo.Avatar = setting.NewValue
			case "OwnerName":
				deviceInfo.Name = setting.NewValue
			case "TimeZone":
				deviceInfo.TimeZone = proto.DeviceTimeZoneInt(setting.NewValue)
			case "SimID":
				deviceInfo.SimID = setting.NewValue
			case "Volume":
				deviceInfo.Volume = uint8(proto.Str2Num(setting.NewValue, 10))
			case "Lang":
				deviceInfo.Lang = setting.NewValue
			case "UseDST":
				deviceInfo.UseDST = (proto.Str2Num(setting.NewValue, 10)) != 0
			case "ChildPowerOff":
				deviceInfo.CanTurnOff = (proto.Str2Num(setting.NewValue, 10)) != 0
			default:
			}
		}
	}
	proto.DeviceInfoListLock.Unlock()

	//更新数据库
	return UpdateDeviceSettingInDB(imei, settings, valulesIsString)
}

func AppUpdateDeviceSetting(c *AppConnection, params *proto.DeviceSettingParams) bool {
	isNeedNotifyDevice := make([]bool, len(params.Settings))
	valulesIsString := make([]bool, len(params.Settings))
	imei := proto.Str2Num(params.Imei, 10)

	for i, setting := range params.Settings {
		fieldName :=  setting.FieldName
		//newValue := setting.NewValue
		isNeedNotifyDevice[i] = true
		valulesIsString[i] = true

		switch fieldName {
		case "OwnerName":
		case "TimeZone":
		case "Volume":
		case "Lang":
		case "UseDST":
		case "ChildPowerOff":
		case "SimID":
			isNeedNotifyDevice[i] = false
		default:
			return false
		}
	}

	ret := SaveDeviceSettings(imei, params.Settings, valulesIsString)
	settingResult := proto.DeviceSettingResult{Settings: params.Settings}
	result := proto.HttpAPIResult{
		ErrCode: 0,
		ErrMsg: "",
		Imei: proto.Num2Str(imei, 10),
	}

	for _, isNeed := range isNeedNotifyDevice {
		if isNeed {
			msgNotify := &proto.MsgData{}
			msgNotify.Header.Header.Version = proto.MSG_HEADER_VER
			msgNotify.Header.Header.Imei = imei
			msgNotify.Data = proto.MakeSetDeviceConfigReplyMsg(imei, params)
			svrctx.Get().TcpServerChan <- msgNotify
			break
		}
	}

	if ret == false {
		result.ErrCode = 500
		result.ErrMsg = "save device setting in db failed"
	}else {
		//concatStr := ""
		//for i, setting := range settingResult.Settings {
		//	strJson, _ := json.Marshal(&setting)
		//	if i == 0 {
		//		concatStr += fmt.Sprintf("\"v%d\": %s ", i, strJson)
		//	}else{
		//		concatStr += fmt.Sprintf(" , \"v%d\": %s ", i, strJson)
		//	}
		//}
		//settingResultJson := fmt.Sprintf("{\"count\": \"%d\", %s}", len(settingResult.Settings), concatStr)
		//result.Data = string(base64Encode([]byte(settingResultJson)))
		settingResultJson, _ := json.Marshal(settingResult)
		result.Data = string([]byte(settingResultJson))
		jsonData, _ := json.Marshal(&result)

		appServerChan <- &proto.AppMsgData{Cmd: "set-device-ack", Imei: imei,
			UserName: params.UserName, AccessToken:params.AccessToken,
			Data: string(jsonData), Conn: c}
	}

	return ret
}

func UpdateDeviceSettingInDB(imei uint64,settings []proto.SettingParam, valulesIsString []bool) bool {
	strSQL := ""
	concatValues := ""
	FieldNames := ""
	for i, setting := range settings {
		FieldNames += setting.FieldName + ","
		newValue := setting.NewValue
		if valulesIsString == nil || valulesIsString[i] {
			newValue = strings.Replace(newValue, "'", "\\'", -1)
			newValue = "'" + setting.NewValue + "'"
		}

		if setting.FieldName == "SimID" {
			strSQLUpdateSimID := fmt.Sprintf("UPDATE device SET %s=%s  where systemNo=%d", setting.FieldName, newValue, imei % 100000000000)
			logging.Log("SQL: " + strSQLUpdateSimID)
			_, err := svrctx.Get().MySQLPool.Exec(strSQLUpdateSimID)
			if err != nil {
				logging.Log(fmt.Sprintf("[%d] update %s into db failed, %s", imei, FieldNames, err.Error()))
				return false
			}
		}else{
			if len(concatValues) == 0 {
				concatValues += fmt.Sprintf(" %s=%s ", setting.FieldName, newValue)
			}else{
				concatValues += fmt.Sprintf(", %s=%s ", setting.FieldName, newValue)
			}
		}
	}

	if len(concatValues) > 0 {
		strSQL = fmt.Sprintf("UPDATE watchinfo SET %s where IMEI='%d'", concatValues, imei)
		logging.Log("SQL: " + strSQL)
		_, err := svrctx.Get().MySQLPool.Exec(strSQL)
		if err != nil {
			logging.Log(fmt.Sprintf("[%d] update %s into db failed, %s", imei, FieldNames, err.Error()))
			return false
		}
	}

	return true
}
