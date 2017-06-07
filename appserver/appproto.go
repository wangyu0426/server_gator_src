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
)

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
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceSettingParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string),
			FieldName: datas["fieldname"].(string),
			CurValue: datas["curvalue"].(string),
			NewValue: datas["newvalue"].(string),
		}
		return updateDeviceSetting(c, &params)

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


func updateDeviceSetting(c *AppConnection, params *proto.DeviceSettingParams) bool {
	isNeedNotifyDevice := true
	imei := proto.Str2Num(params.Imei, 10)
	fieldName :=  params.FieldName
	newValue :=  params.NewValue
	proto.DeviceInfoListLock.Lock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	if ok && deviceInfo != nil {
		switch params.FieldName {
		case "avatar":
			isNeedNotifyDevice = false
			deviceInfo.Avatar = newValue
		}
	}
	proto.DeviceInfoListLock.Unlock()

	//更新数据库
	ret := UpdateDeviceSettingInDB(imei, fieldName, newValue)
	if ret == false {

	}else{

	}

	if isNeedNotifyDevice {

	}

	//svrctx.Get().TcpServerChan <-
	//appServerChan <- &proto.AppMsgData{Cmd: "verify-code-ack", Imei: imei,
	//	UserName: params.UserName, AccessToken:params.AccessToken,
	//	Data: fmt.Sprintf("{\"VerifyCode\": \"%s\"}", verifyCode), Conn: c}

	return true
}

func UpdateDeviceSettingInDB(imei uint64, fieldName, newValue string) bool {
	strSQL := fmt.Sprintf("UPDATE watchinfo SET %s=%s  where IMEI='%d'", fieldName, newValue, imei)

	logging.Log("SQL: " + strSQL)
	_, err := svrctx.Get().MySQLPool.Exec(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] update time zone into db failed, %s", imei, err.Error()))
		return false
	}

	return true
}
