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
	"math/rand"
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

func init()  {
}

func HandleAppRequest(connid uint64, appserverChan chan *proto.AppMsgData, data []byte) bool {
	var itf interface{}
	err:=json.Unmarshal(data, &itf)
	if err != nil {
		logging.Log("parse recved json data failed, " + err.Error())
		return false
	}

	msg:= itf.(map[string]interface{})
	cmd :=  msg["cmd"]
	if cmd == nil {
		return false
	}

	params := msg["data"].(map[string]interface{})

	if ( params["accessToken"] == nil ||  params["accessToken"].(string) == "") { //没有accesstoken
		//同时又不是注册和登录，那么认为是非法请求
		if cmd.(string) != proto.LoginCmdName && cmd.(string) != proto.RegisterCmdName  &&
			cmd.(string) != proto.ResetPasswordCmdName {
			logging.Log("access token is bad, not login and register")
			return false
		}
	}else{//有accesstoken，那么需要对token进行验证
		accessToken := params["accessToken"].(string)
		valid := false
		AccessTokenTableLock.Lock()
		logined, ok := AccessTokenTable[accessToken]
		if ok && logined {
			//验证通过
			valid = true
		}else{
			//需要进行更多验证
			if ValidAccessTokenFromService(accessToken) {
				AccessTokenTable[accessToken] = true
				valid = true
			}
		}
		AccessTokenTableLock.Unlock()

		if valid == false{
			logging.Log("access token is not found")
			return false
		}
	}

	switch cmd{
	case proto.LoginCmdName:
		return login(connid, params["username"].(string), params["password"].(string), false)

	case proto.RegisterCmdName:
		return login(connid, params["username"].(string), params["password"].(string), true)

	case proto.ResetPasswordCmdName:
		return resetPassword(connid, params["username"].(string))

	case proto.ModifyPasswordCmdName:
		return modifyPassword(connid, params["username"].(string), params["accessToken"].(string),
			params["oldPassword"].(string), params["newPassword"].(string))

	case proto.FeedbackCmdName:
		return handleFeedback(connid, params["username"].(string), params["accessToken"].(string),params["feedback"].(string))

	case proto.HearbeatCmdName:
		//datas := msg["data"].(map[string]interface{})
		//params := proto.HeartBeatParams{TimeStamp: datas["timestamp"].(string),
		//	UserName: datas["username"].(string),
		//	AccessToken: datas["accessToken"].(string)}

		jsonString, _ := json.Marshal(msg["data"])
		params := proto.HeartbeatParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("heartbeat parse json failed, " + err.Error())
			return false
		}

		return handleHeartBeat(connid, &params)

	case proto.VerifyCodeCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceBaseParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string)}
		return getDeviceVerifyCode(connid, &params)
	case proto.RefreshDeviceCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceBaseParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string)}
		return refreshDevice(connid, &params)
	case proto.SetDeviceCmdName:
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.DeviceSettingParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("set-device parse json failed, " + err.Error())
			return false
		}
		return AppUpdateDeviceSetting(connid, &params, false, "")
	case proto.GetDeviceByImeiCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceAddParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string)}
		//return getDeviceInfoByImei(connid, &params)
		addCtx := AddDeviceChanCtx{cmd: proto.GetDeviceByImeiCmdName, connid: connid, params: &params}
		addDeviceManagerChan <- &addCtx
	case proto.AddDeviceCmdName:
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.DeviceAddParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("add-device parse json failed, " + err.Error())
			return false
		}

		addDeviceManagerChan <- &AddDeviceChanCtx{cmd: proto.AddDeviceCmdName, connid: connid, params: &params}
	case proto.DeleteDeviceCmdName:
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.DeviceAddParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("delete-device parse json failed, " + err.Error())
			return false
		}

		addDeviceManagerChan <- &AddDeviceChanCtx{cmd: proto.DeleteDeviceCmdName, connid: connid, params: &params}
	case proto.DeleteVoicesCmdName:
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.DeleteVoicesParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("delete-voices parse json failed, " + err.Error())
			return false
		}

		return AppDeleteVoices(connid, &params)
	case proto.DeviceLocateNowCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceActiveParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string),
			Phone: datas["phone"].(string)}

		return AppActiveDeviceToLocateNow(connid, &params)
	case proto.ActiveDeviceCmdName:
		//datas := msg["data"].(map[string]interface{})
		//params := proto.DeviceActiveParams{Imei: datas["imei"].(string),
		//	UserName: datas["username"].(string),
		//	AccessToken: datas["accessToken"].(string),
		//	Phone: datas["phone"].(string)}
		//
		//return AppActiveDeviceToConnectServer(connid, &params)
	case proto.ActiveDeviceSosCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceActiveParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string),
			Phone: datas["phone"].(string)}

		return AppActiveDeviceSos(connid, &params)
	case proto.SetDeviceVoiceMonitorCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceActiveParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string),
			Phone: datas["phone"].(string)}

		return AppSetDeviceVoiceMonitor(connid, &params)
	case proto.GetAlarmsCmdName:
		fallthrough
	case proto.GetLocationsCmdName:
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.QueryLocationsParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log(cmd.(string) + " parse json failed, " + err.Error())
			return false
		}

		return AppQueryLocations(connid, &params)
	case proto.DeleteAlarmsCmdName:
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.QueryLocationsParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log(cmd.(string) + " parse json failed, " + err.Error())
			return false
		}

		return AppDeleteAlarms(connid, &params)
	default:
		break
	}
	
	return true
}

func handleHeartBeat(connid uint64, params *proto.HeartbeatParams) bool {
	if params.AccessToken == ""{
		return true
	}

	for i, imei := range params.Devices {
		imeiUint64 := proto.Str2Num(imei, 10)
		if params.FamilyNumbers == nil || len(params.FamilyNumbers) < (i + 1) {
			if params.UUID != "" &&  params.DeviceToken != "" {
				proto.ReportDevieeToken(svrctx.Get().APNSServerApiBase, imeiUint64, "",
					params.UUID, params.DeviceToken, params.Platform, params.Language)
			}
		}else{
			if params.UUID != "" &&  params.DeviceToken != "" {
				proto.ReportDevieeToken(svrctx.Get().APNSServerApiBase, imeiUint64, params.FamilyNumbers[i],
					params.UUID, params.DeviceToken, params.Platform, params.Language)
			}
		}
	}

	result := proto.HeartbeatResult{Timestamp: time.Now().Format("20060102150405")}
	if params.SelectedDevice == "" {
		for _, imei := range params.Devices {
			imeiUint64 := proto.Str2Num(imei, 10)
			result.Locations = append(result.Locations, svrctx.GetDeviceData(imeiUint64, svrctx.Get().PGPool))
			result.Minichat = append(result.Minichat, proto.GetChatListForApp(imeiUint64, params.UserName)...)
		}
	}else{
		imeiUint64 := proto.Str2Num(params.SelectedDevice, 10)
		endTime := uint64(params.Timestamp)
		beginTime := endTime - endTime %1000000
		result.Locations = append(result.Locations, svrctx.GetDeviceData(imeiUint64, svrctx.Get().PGPool))
		result.Minichat = append(result.Minichat, proto.GetChatListForApp(imeiUint64, params.UserName)...)

		alarms := svrctx.QueryLocations(imeiUint64, svrctx.Get().PGPool, beginTime, endTime, false, true)
		if alarms != nil {
			result.Alarms = append(result.Alarms, (*alarms)...)
		}
	}

	//chat := proto.ChatInfo{}
	//DeviceMinichatBaseUrl := fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName,
	//	svrctx.Get().WSPort, svrctx.Get().HttpStaticURL + svrctx.Get().HttpStaticMinichatDir)
	//
	//chat.Content = fmt.Sprintf("%swatch/%d/%d.mp3", DeviceMinichatBaseUrl,
	//	357593060571398, 1706201641546398)
	//chat.FileID = 1706201641546398
	////chat.Sender = proto.Num2Str()
	//chat.SenderType = 0
	//chat.VoiceMilisecs = 3000
	//chat.Sender = "357593060571398"
	//chat.DateTime = 170620164154
	//chat.Imei = 357593060571398
	//result.Minichat = append(result.Minichat, chat)

	fmt.Println("heartbeat-ack: ", proto.MakeStructToJson(result))

	appServerChan <- (&proto.AppMsgData{Cmd: proto.HearbeatAckCmdName,
		UserName: params.UserName,
		AccessToken: params.AccessToken,
		Data: proto.MakeStructToJson(result), ConnID: connid})

	return true
}

func login(connid uint64, username, password string, isRegister bool) bool {
	urlRequest := "http://127.0.0.1/web/index.php?r=app/auth/"
	if svrctx.Get().IsDebugLocal {
		urlRequest = "http://watch.gatorcn.com/web/index.php?r=app/auth/"
	}
	reqType := "login"
	if isRegister {
		reqType = "register"
	}

	urlRequest += reqType

	resp, err := http.PostForm(urlRequest, url.Values{"username": {username}, "password": {password}})
	if err != nil {
		logging.Log("app " + reqType + "failed, " + err.Error())
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
		logging.Log("parse  " + reqType + " response as json failed, " + err.Error())
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
	if status != nil && accessToken != nil {
		AddAccessToken(accessToken.(string))
		if isRegister == false {
			if devices != nil {
				locations := []proto.LocationData{}
				for i, d := range devices.([]interface{}) {
					device := d.(map[string]interface{})
					imei, _ := strconv.ParseUint(device["IMEI"].(string), 0, 0)
					logging.Log("device: " + fmt.Sprint(imei))
					locations = append(locations, svrctx.GetDeviceData(imei, svrctx.Get().PGPool))

					deviceInfoResult := proto.DeviceInfoResult{}
					//json.Unmarshal([]byte(proto.MakeStructToJson(d)), &deviceInfoResult)
					//fmt.Println("deviceinfoResult: ", deviceInfoResult)

					proto.DeviceInfoListLock.Lock()
					deviceInfo, ok := (*proto.DeviceInfoList)[imei]
					if ok && deviceInfo != nil {
						deviceInfoResult = proto.MakeDeviceInfoResult(deviceInfo)
						if len(deviceInfoResult.Avatar) > 0 {
							deviceInfoResult.Avatar = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName, svrctx.Get().WSPort, svrctx.Get().HttpStaticURL +
								deviceInfoResult.Avatar)
						}

						for i, ava := range deviceInfoResult.ContactAvatar{
							if len(ava) > 0 {
								deviceInfoResult.ContactAvatar[i] = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName,
									svrctx.Get().WSPort, svrctx.Get().HttpStaticURL + ava)
							}
						}

						deviceInfoResult.FamilyNumber = loginData["devices"].([]interface{})[i].(map[string]interface{})["FamilyNumber"].(string)
						deviceInfoResult.Name = loginData["devices"].([]interface{})[i].(map[string]interface{})["Name"].(string)
						loginData["devices"].([]interface{})[i] = deviceInfoResult
						fmt.Println("deviceinfoResult: ", proto.MakeStructToJson(&deviceInfoResult))
					}
					proto.DeviceInfoListLock.Unlock()
				}

				jsonLocations, _ := json.Marshal(locations)
				appServerChan <- ( &proto.AppMsgData{Cmd: proto.LoginAckCmdName,
					UserName: username,
					Data: (fmt.Sprintf("{\"user\": %s, \"location\": %s}",
						proto.MakeStructToJson(&loginData), string(jsonLocations))), ConnID: connid})

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
			} else {
				appServerChan <- (&proto.AppMsgData{Cmd: proto.LoginAckCmdName,
					UserName: username,
					Data: (fmt.Sprintf("{\"user\": %s, \"location\": []}", string(body))), ConnID: connid})
			}
		} else {
			appServerChan <- (&proto.AppMsgData{Cmd: proto.RegisterAckCmdName,
				UserName: username,
				Data: (fmt.Sprintf("{\"user\": %s, \"location\": []}", string(body))), ConnID: connid})
		}
	}else{
		if isRegister{
			appServerChan <- (&proto.AppMsgData{Cmd: proto.RegisterAckCmdName,
				UserName: username,
				Data: "{\"status\": -1}", ConnID: connid})
		}else{
			appServerChan <- (&proto.AppMsgData{Cmd: proto.LoginAckCmdName,
				UserName: username,
				Data: "{\"status\": -1}", ConnID: connid})
		}
	}

	return true
}

//忘记密码，通过服务器重设秘密
func resetPassword(connid uint64, username string) bool {
	urlRequest := "http://127.0.0.1/web/index.php?r=app/auth/"
	if svrctx.Get().IsDebugLocal {
		urlRequest = "http://watch.gatorcn.com/web/index.php?r=app/auth/"
	}

	reqType := "reset"
	urlRequest += reqType

	resp, err := http.PostForm(urlRequest, url.Values{"username": {username}})
	if err != nil {
		logging.Log("app " + reqType + "failed, " + err.Error())
		return false
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log("response has err, " + err.Error())
		return false
	}

	ioutil.WriteFile("error.html",  body, 0666)

	appServerChan <- (&proto.AppMsgData{Cmd: proto.ResetPasswordAckCmdName,
		UserName: username,
		Data: string(body), ConnID: connid})

	return true
}

//有旧密码，重设新密码
func modifyPassword(connid uint64, username, accessToken, oldPasswd, newPasswd  string) bool {
	requesetURL := "http://127.0.0.1/web/index.php?r=app/service/modpwd&access-token=" + accessToken
	if svrctx.Get().IsDebugLocal {
		requesetURL = "http://watch.gatorcn.com/web/index.php?r=app/service/modpwd&access-token=" + accessToken
	}

	logging.Log("url: " + requesetURL)
	resp, err := http.PostForm(requesetURL, url.Values{"oldpwd": {oldPasswd}, "newpwd": {newPasswd}})
	if err != nil {
		logging.Log(fmt.Sprintf("user %s modify password  failed, ", username, err.Error()))
		return false
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log("response has err, " + err.Error())
		return false
	}

	appServerChan <- (&proto.AppMsgData{Cmd: proto.ModifyPasswordAckCmdName,
		UserName: username,
		Data: (string(body)), ConnID: connid})

	return true
}


func handleFeedback(connid uint64, username, accessToken, feedback string) bool {
	requesetURL := "http://127.0.0.1/web/index.php?r=app/service/feedback&access-token=" + accessToken
	if svrctx.Get().IsDebugLocal {
		requesetURL = "http://watch.gatorcn.com/web/index.php?r=app/service/feedback&access-token=" + accessToken
	}

	logging.Log("url: " + requesetURL)
	resp, err := http.PostForm(requesetURL, url.Values{"feedback": {feedback}})
	if err != nil {
		logging.Log(fmt.Sprintf("user %s send feedback failed, ", username, err.Error()))
		return false
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log("response has err, " + err.Error())
		return false
	}

	appServerChan <- (&proto.AppMsgData{Cmd: proto.FeedbackAckCmdName,
		UserName: username,
		Data: (string(body)), ConnID: connid})

	return true
}


func getDeviceVerifyCode(connid uint64, params *proto.DeviceBaseParams) bool {
	//url := fmt.Sprintf("http://service.gatorcn.com/web/index.php?r=app/service/verify-code&SystemNo=%s&access-token=%s",
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
	isAdmin, _,_,_ := queryIsAdmin(params.Imei, params.UserName)
	if isAdmin {
		proto.DeviceInfoListLock.Lock()
		deviceInfo, ok := (*proto.DeviceInfoList)[imei]
		if ok && deviceInfo != nil {
			verifyCode = deviceInfo.VerifyCode
		}
		proto.DeviceInfoListLock.Unlock()
	}

	appServerChan <- (&proto.AppMsgData{Cmd: proto.VerifyCodeAckCmdName, Imei: imei,
		UserName: params.UserName, AccessToken:params.AccessToken,
			Data: fmt.Sprintf("{\"VerifyCode\": \"%s\", \"isAdmin\": %d}", verifyCode, proto.Bool2UInt8(isAdmin)), ConnID: connid})

	return true
}

func queryIsAdmin(imei, userName string) (bool, bool, string, error) {
	strSQL := fmt.Sprintf("SELECT  u.loginname,  viu.VehId  from users u JOIN vehiclesinuser viu on u.recid = viu.UserID JOIN " +
		"watchinfo w on w.recid = viu.VehId where w.imei='%s' order by viu.CreateTime asc", imei)
	logging.Log("SQL: " + strSQL)
	rows, err := svrctx.Get().MySQLPool.Query(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%s] query is admin in db failed, %s", imei, err.Error()))
		return false, false, "", err
	}

	defer rows.Close()

	usersCount := 0
	matched := false
	deviceRecId := ""
	for rows.Next() {
		userNameInDB, deviceRecIdInDB := "", ""
		rows.Scan(&userNameInDB, &deviceRecIdInDB)
		fmt.Println(userNameInDB, deviceRecIdInDB)
		deviceRecId = deviceRecIdInDB
		if userNameInDB == userName {
			matched = true
			break
		}
		usersCount++
	}

	if usersCount == 0 {
		if deviceRecId == "" {
			//没有人添加过，此时要查询出设备recid
			strSQL := fmt.Sprintf("select recid from watchinfo where imei='%s' ", imei)
			logging.Log("SQL: " + strSQL)
			rowsDeviceRecid, err := svrctx.Get().MySQLPool.Query(strSQL)
			if err != nil {
				logging.Log(fmt.Sprintf("[%s] query watchinfo recid  in db failed, %s", imei, err.Error()))
				return false, false, "", err
			}

			defer rowsDeviceRecid.Close()

			for rowsDeviceRecid.Next(){
				err = rowsDeviceRecid.Scan(&deviceRecId)
				if err != nil {
					logging.Log(fmt.Sprintf("[%s] query watchinfo recid  scan result failed, %s", imei, err.Error()))
					return false, false, "", err
				}
				break
			}
		}

		return true, matched, deviceRecId, nil
	}else {
		return false, matched, deviceRecId, nil
	}
}

func getDeviceInfoByImei(connid uint64, params *proto.DeviceAddParams) bool {
	found := false
	deviceInfoResult := proto.DeviceInfo{}
	imei := proto.Str2Num(params.Imei, 10)
	proto.DeviceInfoListLock.Lock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	if ok && deviceInfo != nil {
		deviceInfoResult = *deviceInfo
		found = true
	}else{
		//logging.Log(params.Imei + "  imei not found")
	}
	proto.DeviceInfoListLock.Unlock()

	deviceInfoResult.Imei = imei
	deviceInfoResult.VerifyCode = ""

	if found == false {
		logging.Log(params.Imei + "  imei not found")
	}

	if found {
		isAdmin, _, _, err := queryIsAdmin(params.Imei, params.UserName)
		if err != nil {
			return false
		}

		if isAdmin {
			deviceInfoResult.IsAdmin = 1
		}
	}

	if found == false {
		deviceInfoResult.Imei = 0
	}

	if deviceInfoResult.IsAdmin == 0 {
		deviceInfoResult.SimID = ""
	}

	resultData, _ := json.Marshal(&deviceInfoResult)

	appServerChan <- (&proto.AppMsgData{Cmd: proto.GetDeviceByImeiAckCmdName, Imei: imei,
		UserName: params.UserName, AccessToken:params.AccessToken,
		Data: string(resultData), ConnID: connid})

	return true
}

func refreshDevice(connid uint64, params *proto.DeviceBaseParams) bool {
	found := false
	deviceInfoResult := proto.DeviceInfoResult{}
	imei := proto.Str2Num(params.Imei, 10)
	proto.DeviceInfoListLock.Lock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	if ok && deviceInfo != nil {
		found = true
		deviceInfoResult = proto.MakeDeviceInfoResult(deviceInfo)
		if len(deviceInfo.Avatar) == 0 || (strings.Contains(deviceInfo.Avatar, ".jpg") == false &&
			strings.Contains(deviceInfo.Avatar, ".JPG") == false) {
			deviceInfoResult.Avatar = ""
		}else{
			if deviceInfo.Avatar[0] == '/'{
				deviceInfoResult.Avatar = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName, svrctx.Get().WSPort, svrctx.Get().HttpStaticURL +
					deviceInfoResult.Avatar)
			}
		}

		for i, ava := range deviceInfoResult.ContactAvatar{
			if len(ava) > 0 {
				deviceInfoResult.ContactAvatar[i] = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName,
					svrctx.Get().WSPort, svrctx.Get().HttpStaticURL + ava)
			}
		}
	}
	proto.DeviceInfoListLock.Unlock()

	if found == false {
		logging.Log(params.Imei + "  imei not found.")
		deviceInfoResult.IMEI = ""
		return false
	}

	resultData, _ := json.Marshal(&deviceInfoResult)

	appServerChan <- (&proto.AppMsgData{Cmd: proto.RefreshDeviceAckCmdName, Imei: imei,
		UserName: params.UserName, AccessToken:params.AccessToken,
		Data: string(resultData), ConnID: connid})

	return true
}

func checkVerifyCode(imei, code string) bool {
	matched := false
	proto.DeviceInfoListLock.Lock()
	deviceInfo, ok := (*proto.DeviceInfoList)[proto.Str2Num(imei, 10)]
	if ok && deviceInfo != nil {
		matched = deviceInfo.VerifyCode == code
	}else{
		matched = true
	}
	proto.DeviceInfoListLock.Unlock()

	return matched
}

const CHAR_LIST = "1234567890abcdefghijklmnopqrstuvwxyz_"

func makerRandomVerifyCode() string {
	rand.Seed(time.Now().UnixNano())
	randChars := [4]byte{}
	for i := 0; i < 4; i++{
		index := rand.Uint64() %  uint64(len(CHAR_LIST))
		randChars[i] = CHAR_LIST[int(index)]
	}

	return string(randChars[0:4])
}

func makeContactAvatars(family *[proto.MAX_FAMILY_MEMBER_NUM]proto.FamilyMember) string {
	return proto.MakeStructToJson(family)
}

func addDeviceByUser(connid uint64, params *proto.DeviceAddParams) bool {
	// error code:  -1 表示验证码不正确
	// error code:  -2 表示验该用户已经关注了手表
	// error code:  -3 表示手表的亲情号码满了
	// error code:  500 表示服务器内部错误
	result := proto.HttpAPIResult{0, "", params.Imei, ""}
	imei := proto.Str2Num(params.Imei, 10)

	verifyCodeWrong := false
	phoneNumbers := "" //这里需要用到DeviceInfoList缓存来寻找空闲的亲情号列表，并且更新到缓存中去

	isAdmin, isFound, deviceRecId, err := queryIsAdmin(params.Imei, params.UserName)
	if err == nil {
		if params.IsAdmin == 1 && isAdmin == false {
			verifyCodeWrong = true
		} else if params.IsAdmin == 0 && isAdmin == false {
			if checkVerifyCode(params.Imei, params.VerifyCode) == false {
				verifyCodeWrong = true
			}
		}

		if verifyCodeWrong {
			// error code:  -1 表示验证码不正确
			result.ErrCode = -1
			result.ErrMsg = "the verify code is incorrect"
		}else{
			//从数据库查询是否已经关注了该手表
			if isFound {
				// error code:  -2 表示验该用户已经关注了手表
				result.ErrCode = -2
				result.ErrMsg = "the device was added duplicately"

				proto.DeviceInfoListLock.Lock()
				deviceInfo, ok := (*proto.DeviceInfoList)[imei]
				if ok && deviceInfo != nil {
					deviceInfoResult := proto.MakeDeviceInfoResult(deviceInfo)
					if len(deviceInfo.Avatar) == 0 || (strings.Contains(deviceInfo.Avatar, ".jpg") == false &&
						strings.Contains(deviceInfo.Avatar, ".JPG") == false) {
						deviceInfoResult.Avatar = ""
					}else{
						if deviceInfo.Avatar[0] == '/'{
							deviceInfoResult.Avatar = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName, svrctx.Get().WSPort, svrctx.Get().HttpStaticURL +
								deviceInfoResult.Avatar)
						}
					}

					for i, ava := range deviceInfoResult.ContactAvatar{
						if len(ava) > 0 {
							deviceInfoResult.ContactAvatar[i] = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName,
								svrctx.Get().WSPort, svrctx.Get().HttpStaticURL + ava)
						}
					}

					deviceInfoResult.FamilyNumber = params.MySimID
					resultJson, _ := json.Marshal(&deviceInfoResult)
					result.Data = string([]byte(resultJson))
				}
				proto.DeviceInfoListLock.Unlock()
			}else{
				strSqlUpdateDeviceInfo := ""
				isFound := false
				family := [proto.MAX_FAMILY_MEMBER_NUM]proto.FamilyMember{}
				newVerifycode := makerRandomVerifyCode()
				proto.DeviceInfoListLock.Lock()
				deviceInfo, ok := (*proto.DeviceInfoList)[imei]
				if ok && deviceInfo != nil {
					for i := 0; i < len(deviceInfo.Family); i++ {
						if len(deviceInfo.Family[i].Phone) == 0 {//空号码，没有被使用
							deviceInfo.Family[i].Phone = params.MySimID
							deviceInfo.Family[i].CountryCode = params.MySimCountryCode
							deviceInfo.Family[i].Name = params.MyName
							deviceInfo.Family[i].Type = params.PhoneType
							deviceInfo.Family[i].Index = i + 1
							isFound = true
							break
						}
					}

					family = deviceInfo.Family

					if isAdmin{
						deviceInfo.OwnerName = params.OwnerName
						deviceInfo.CountryCode = params.DeviceSimCountryCode
						deviceInfo.SimID = params.DeviceSimID
						deviceInfo.TimeZone = proto.DeviceTimeZoneInt(params.TimeZone)
					}
				}
				proto.DeviceInfoListLock.Unlock()
				if isFound {
					phoneNumbers = proto.MakeFamilyPhoneNumbers(&family)
					if isAdmin {
						//如果是管理员，则更新手表对应的字段数据，非管理员仅更新关注列表和对应的亲情号
						strSqlUpdateDeviceInfo = fmt.Sprintf("update watchinfo set OwnerName='%s', CountryCode='%s', " +
							"PhoneNumbers='%s', TimeZone='%s', VerifyCode='%s'  where IMEI='%s' ", params.OwnerName, params.DeviceSimCountryCode,
							phoneNumbers, params.TimeZone, newVerifycode, params.Imei)
						strSQLUpdateSimID := fmt.Sprintf("UPDATE device SET SimID=%s  where systemNo=%d",
							params.DeviceSimID, imei % 100000000000)
						logging.Log("SQL: " + strSQLUpdateSimID)
						_, err := svrctx.Get().MySQLPool.Exec(strSQLUpdateSimID)
						if err != nil {
							logging.Log(fmt.Sprintf("[%d] update %s into db failed, %s", imei, "SimID", err.Error()))
							result.ErrCode = 500
							result.ErrMsg = "server failed to update db"
						}
					} else {
						strSqlUpdateDeviceInfo = fmt.Sprintf("update watchinfo set PhoneNumbers='%s', VerifyCode='%s'   where IMEI='%s' ",
							phoneNumbers, newVerifycode, params.Imei)
					}

					logging.Log("SQL: " + strSqlUpdateDeviceInfo)
					_, err := svrctx.Get().MySQLPool.Exec(strSqlUpdateDeviceInfo)
					if err != nil {
						logging.Log(fmt.Sprintf("[%s] update  watchinfo db failed, %s", imei, err.Error()))
						result.ErrCode = 500
						result.ErrMsg = "server failed to update db"
					}else{
						proto.DeviceInfoListLock.Lock()
						deviceInfo, ok := (*proto.DeviceInfoList)[imei]
						if ok && deviceInfo != nil {
							deviceInfo.VerifyCode = newVerifycode
						}
						proto.DeviceInfoListLock.Unlock()

						strSqlInsertDeviceUser :=  fmt.Sprintf("insert into vehiclesinuser (userid, vehid, FamilyNumber, Name, " +
							"CountryCode)  values('%s', '%s', '%s', '%s', '%s') ", params.UserId, deviceRecId, params.MySimID,
							params.MyName, params.MySimCountryCode )
						logging.Log("SQL: " + strSqlInsertDeviceUser)
						_, err := svrctx.Get().MySQLPool.Exec(strSqlInsertDeviceUser)
						if err != nil {
							logging.Log(fmt.Sprintf("[%s] insert into vehiclesinuser db failed, %s", imei, err.Error()))
							result.ErrCode = 500
							result.ErrMsg = "server failed to update db"
						}
					}
				}else{
					// error code:  -3 表示手表的亲情号码满了
					result.ErrCode = -3
					result.ErrMsg = "the device family numbers are used up"
				}
			}
		}
	}else{
		result.ErrCode = 500
		result.ErrMsg = "server failed to query db"
	}

	if result.ErrCode == 0 {
		newSettings := proto.DeviceSettingParams{Imei: params.Imei,
			UserName: params.UserName,
			AccessToken: params.AccessToken,
		}

		setting := proto.SettingParam{proto.PhoneNumbersFieldName, "", phoneNumbers, 0,
			//fmt.Sprintf("%s|%d|%s",  params.MySimID, params.PhoneType, params.MyName),
		}

		newSettings.Settings = append(newSettings.Settings, setting)

		if isAdmin {
			setting = proto.SettingParam{proto.OwnerNameFieldName, "", params.OwnerName, 0}
			newSettings.Settings = append(newSettings.Settings, setting)

			setting = proto.SettingParam{proto.CountryCodeFieldName, "", params.DeviceSimCountryCode, 0}
			newSettings.Settings = append(newSettings.Settings, setting)

			setting = proto.SettingParam{proto.SimIDFieldName, "", params.DeviceSimID, 0}
			newSettings.Settings = append(newSettings.Settings, setting)

			setting = proto.SettingParam{proto.TimeZoneFieldName, "", params.TimeZone, 0}
			newSettings.Settings = append(newSettings.Settings, setting)
		}

		return AppUpdateDeviceSetting(connid, &newSettings, true, params.MySimID)
	}else{
		resultData, _ := json.Marshal(&result)
		appServerChan <- (&proto.AppMsgData{Cmd: proto.AddDeviceAckCmdName, Imei: imei,
			UserName: params.UserName, AccessToken:params.AccessToken,
			Data: string(resultData), ConnID: connid})
	}

	return true
}

func deleteDeviceByUser(connid uint64, params *proto.DeviceAddParams) bool {
	// error code:  500 表示服务器内部错误
	result := proto.HttpAPIResult{0, "", params.Imei, ""}
	imei := proto.Str2Num(params.Imei, 10)

	strSqlDeleteDeviceUser := fmt.Sprintf("delete v  from vehiclesinuser as v left join watchinfo as w on v.VehId=w.recid " +
		" where w.imei='%s' and v.UserID='%s' ", params.Imei,  params.UserId)
	logging.Log("SQL: " + strSqlDeleteDeviceUser)
	_, err := svrctx.Get().MySQLPool.Exec(strSqlDeleteDeviceUser)
	if err != nil {
		logging.Log(fmt.Sprintf("[%s] delete  from vehiclesinuser db failed, %s", imei, err.Error()))
		result.ErrCode = 500
		result.ErrMsg = "server failed to update db"
	}

	resultData, _ := json.Marshal(&result)
	appServerChan <- (&proto.AppMsgData{Cmd: proto.DeleteDeviceAckCmdName, Imei: imei,
		UserName: params.UserName, AccessToken:params.AccessToken,
		Data: string(resultData), ConnID: connid})

	if params.UUID != "" {
		proto.DeleteDevieeToken(svrctx.Get().APNSServerApiBase, imei, params.UUID)
	}

	return true
}

func AddDeviceManagerLoop()  {
	defer logging.PanicLogAndExit("AddDeviceManagerLoop")
	for {
		select {
		case msg := <- addDeviceManagerChan:
			if msg == nil {
				return
			}

			if msg.cmd == proto.GetDeviceByImeiCmdName {
				getDeviceInfoByImei(msg.connid, msg.params)
			}else if  msg.cmd == proto.AddDeviceCmdName {
				addDeviceByUser(msg.connid, msg.params)
			}else if  msg.cmd == proto.DeleteDeviceCmdName {
				deleteDeviceByUser(msg.connid, msg.params)
			}
		}
	}
}

func SaveDeviceSettings(imei uint64, settings []proto.SettingParam, valulesIsString []bool)  bool {
	phoneNumbers := ""
	proto.DeviceInfoListLock.Lock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	if ok && deviceInfo != nil {
		for index, setting := range settings {
			switch {
			case strings.Contains(setting.FieldName, proto.FenceFieldName):
				logging.Log("fence setting")
				if setting.NewValue == "delete"{
					deviceInfo.SafeZoneList[setting.Index - 1] = proto.SafeZone{}
					valulesIsString[index] = false
					settings[index].NewValue = "null"
				}else {
					err := json.Unmarshal([]byte(setting.NewValue), &deviceInfo.SafeZoneList[setting.Index - 1])
					if err != nil {
						proto.DeviceInfoListLock.Unlock()
						logging.Log(fmt.Sprintf("[%d] bad data for fence setting %d, err(%s) for %s",
							imei, setting.Index, err.Error(), setting.NewValue))
						return false
					}
				}
				settings[index].FieldName += proto.Num2Str(uint64(setting.Index),10)
				continue
			}

			logging.Log("non-fence setting")

			switch setting.FieldName {
			case proto.AvatarFieldName:
				deviceInfo.Avatar = setting.NewValue
			case proto.OwnerNameFieldName:
				deviceInfo.OwnerName = setting.NewValue
			case proto.TimeZoneFieldName:
				deviceInfo.TimeZone = proto.DeviceTimeZoneInt(setting.NewValue)
			case proto.SimIDFieldName:
				deviceInfo.SimID = setting.NewValue
			case proto.VolumeFieldName:
				deviceInfo.Volume = uint8(proto.Str2Num(setting.NewValue, 10))
			case proto.LangFieldName:
				deviceInfo.Lang = setting.NewValue
			case proto.UseDSTFieldName:
				deviceInfo.UseDST = (proto.Str2Num(setting.NewValue, 10)) != 0
			case proto.ChildPowerOffFieldName:
				deviceInfo.ChildPowerOff = (proto.Str2Num(setting.NewValue, 10)) != 0
			case proto.SocketModeOffFieldName:
				deviceInfo.SocketModeOff = (proto.Str2Num(setting.NewValue, 10)) != 0
			case proto.CountryCodeFieldName:
				deviceInfo.CountryCode = setting.NewValue
			case proto.PhoneNumbersFieldName:
				if setting.NewValue == "delete"{
					newContacts := [proto.MAX_FAMILY_MEMBER_NUM]proto.FamilyMember{}
					count := 0
					for i := 0; i < len(deviceInfo.Family); i++ {
						if i + 1 != setting.Index {
							newContacts[count] = deviceInfo.Family[i]
							count++
						}
					}

					deviceInfo.Family = newContacts
				}else{
					for i := 0; i < len(deviceInfo.Family); i++ {
						curPhone := proto.ParseSinglePhoneNumberString(setting.CurValue, setting.Index)
						newPhone := proto.ParseSinglePhoneNumberString(setting.NewValue, setting.Index)
						//fullPhoneNnumber := "00" + deviceInfo.Family[i].CountryCode + deviceInfo.Family[i].Phone
						if len(curPhone.Phone) == 0 { //之前没有号码，直接寻找一个空位就可以了
							if len(deviceInfo.Family[i].Phone) == 0 {
								bkAvatar := deviceInfo.Family[i].Avatar
								deviceInfo.Family[i] = newPhone
								deviceInfo.Family[i].Avatar = bkAvatar
								break
							}
						}else{ //之前有号码，那么这里是修改号码，需要匹配之前的号码
							if deviceInfo.Family[i].Phone == curPhone.Phone || deviceInfo.Family[i].Index == curPhone.Index {
								bkAvatar := deviceInfo.Family[i].Avatar
								deviceInfo.Family[i] = newPhone
								deviceInfo.Family[i].Avatar = bkAvatar
								break
							}
						}
					}
				}

				phoneNumbers = proto.MakeFamilyPhoneNumbers(&deviceInfo.Family)
				settings[index].NewValue = phoneNumbers

			case proto.ContactAvatarsFieldName:
				for i, m := range deviceInfo.Family {
					if m.Index == setting.Index {
						deviceInfo.Family[i].Avatar = setting.NewValue
						break
					}
				}

				settings[index].NewValue = fmt.Sprintf("{\"ContactAvatars\": %s}", makeContactAvatars(&deviceInfo.Family))
			case proto.WatchAlarmFieldName:
				if setting.Index >=0 && setting.Index < proto.MAX_WATCH_ALARM_NUM {
					if setting.NewValue == "delete"{
						deviceInfo.WatchAlarmList[setting.Index] = proto.WatchAlarm{}
						valulesIsString[index] = false
						settings[index].NewValue = "null"
					}else{
						err := json.Unmarshal([]byte(setting.NewValue), &deviceInfo.WatchAlarmList[setting.Index])
						if err != nil {
							proto.DeviceInfoListLock.Unlock()
							logging.Log(fmt.Sprintf("[%d] bad data for watch alarm setting %d, err(%s) for %s",
								imei, setting.Index, err.Error(), setting.NewValue))
							return false
						}
					}

					settings[index].FieldName += proto.Num2Str(uint64(setting.Index),10)
				}else{
					proto.DeviceInfoListLock.Unlock()
					logging.Log(fmt.Sprintf("[%d] bad index %d for delete watch alarm", imei, setting.Index))
					return false
				}
			case proto.HideSelfFieldName:
				deviceInfo.HideTimerOn = (proto.Str2Num(setting.NewValue, 10)) == 1
			case proto.DisableWiFiFieldName:
				deviceInfo.DisableWiFi = (proto.Str2Num(setting.NewValue, 10)) == 1
			case proto.DisableLBSFieldName:
				deviceInfo.DisableLBS =  (proto.Str2Num(setting.NewValue, 10)) == 1
			case proto.HideTimer0FieldName:
				fallthrough
			case proto.HideTimer1FieldName:
				fallthrough
			case proto.HideTimer2FieldName:
				fallthrough
			case proto.HideTimer3FieldName:
				err := json.Unmarshal([]byte(setting.NewValue), &deviceInfo.HideTimerList[setting.Index])
				if err != nil {
					proto.DeviceInfoListLock.Unlock()
					logging.Log(fmt.Sprintf("[%d] bad data for hide timer setting %d, err(%s) for %s",
						imei, setting.Index, err.Error(), setting.NewValue))
					return false
				}
			default:
			}
		}
	}
	proto.DeviceInfoListLock.Unlock()

	//更新数据库
	return UpdateDeviceSettingInDB(imei, settings, valulesIsString)
}

func AppUpdateDeviceSetting(connid uint64, params *proto.DeviceSettingParams, isAddDevice bool,
	familyNumber string) bool {

	extraMsgNotifyDataList := []*proto.MsgData{}
	isNeedNotifyDevice := make([]bool, len(params.Settings))
	valulesIsString := make([]bool, len(params.Settings))
	imei := proto.Str2Num(params.Imei, 10)
	model := proto.GetDeviceModel(imei)
	cmdAck := proto.SetDeviceAckCmdName
	if isAddDevice {
		cmdAck = proto.AddDeviceOKAckCmdName
	}

	for i, setting := range params.Settings {
		fieldName :=  setting.FieldName
		//newValue := setting.NewValue
		isNeedNotifyDevice[i] = true
		valulesIsString[i] = true

		switch fieldName {
		case proto.OwnerNameFieldName:
		case proto.TimeZoneFieldName:
		case proto.VolumeFieldName:
		case proto.LangFieldName:
		case proto.UseDSTFieldName:
		case proto.ChildPowerOffFieldName:
		case proto.SocketModeOffFieldName:
		case proto.PhoneNumbersFieldName:
		case proto.WatchAlarmFieldName:
		case proto.HideSelfFieldName:
		case proto.HideTimer0FieldName:
		case proto.HideTimer1FieldName:
		case proto.HideTimer2FieldName:
		case proto.HideTimer3FieldName:

		//上面都是需要通知手表更新设置的
		case proto.FenceFieldName:
			if model == proto.DM_GT06{
				isNeedNotifyDevice[i] = false
				if setting.Index == 1 || setting.Index == 2{
					proto.DeviceInfoListLock.Lock()
					info, ok := (*proto.DeviceInfoList)[imei]
					if ok && info != nil {
						newFence := proto.SafeZone{}
						err := json.Unmarshal([]byte(setting.NewValue), &newFence)
						if err == nil {
							if info.SafeZoneList[setting.Index - 1].Wifi.BSSID != newFence.Wifi.BSSID{
								//isNeedNotifyDevice[i] = true
								//(0035357593060571398AP27,3C:46:D8:27:2E:63,48:3C:0C:F5:56:48,0000000000000021)
								msg := proto.MsgData{}
								msg.Header.Header.Imei = imei
								msg.Header.Header.ID = proto.NewMsgID()
								msg.Header.Header.Status = 1

								newBSSID0, newBSSID1 := info.SafeZoneList[0].Wifi.BSSID, info.SafeZoneList[1].Wifi.BSSID
								if setting.Index == 1 {
									newBSSID0 = newFence.Wifi.BSSID
								}else if setting.Index == 2{
									newBSSID1 = newFence.Wifi.BSSID
								}

								body := fmt.Sprintf("%015dAP27,%s,%s,%016X)", imei,
									newBSSID0, newBSSID1,
									msg.Header.Header.ID)
								msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
								extraMsgNotifyDataList = append(extraMsgNotifyDataList, &msg)
								logging.Log("update home or school wifi to device, " + string(msg.Data))
							}
						}else{
							logging.Log(fmt.Sprintf("%d new fence bad json data %s", imei, setting.NewValue))
						}
					}
					proto.DeviceInfoListLock.Unlock()
				}
			}

		case proto.CountryCodeFieldName:
		case proto.SimIDFieldName:
		case proto.AvatarFieldName:
		case proto.DisableWiFiFieldName:
		case proto.DisableLBSFieldName:
			isNeedNotifyDevice[i] = false
		default:
			return false
		}
	}

	ret := true
	if isAddDevice == false {
		ret = SaveDeviceSettings(imei, params.Settings, valulesIsString)
	}

	result := proto.HttpAPIResult{
		ErrCode: 0,
		ErrMsg: "",
		Imei: proto.Num2Str(imei, 10),
	}

	if ret {
		for _, isNeed := range isNeedNotifyDevice {
			if isNeed {
				//msgNotify := &proto.MsgData{}
				//msgNotify.Header.Header.Version = proto.MSG_HEADER_VER
				//msgNotify.Header.Header.Imei = imei
				//msgNotify.Data = proto.MakeSetDeviceConfigReplyMsg(imei, params)
				msgNotifyDataList := proto.MakeSetDeviceConfigReplyMsg(imei, params)
				for _, msgNotify := range msgNotifyDataList  {
					svrctx.Get().TcpServerChan <- msgNotify
				}

				break
			}
		}

		for _, extraMsgNotify := range extraMsgNotifyDataList  {
			svrctx.Get().TcpServerChan <- extraMsgNotify
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
		if isAddDevice{
			proto.DeviceInfoListLock.Lock()
			deviceInfo, ok := (*proto.DeviceInfoList)[imei]
			if ok && deviceInfo != nil {
				deviceInfoResult := proto.MakeDeviceInfoResult(deviceInfo)
				if len(deviceInfo.Avatar) == 0 || (strings.Contains(deviceInfo.Avatar, ".jpg") == false &&
					strings.Contains(deviceInfo.Avatar, ".JPG") == false) {
					deviceInfoResult.Avatar = ""
				}else{
					if deviceInfo.Avatar[0] == '/'{
						deviceInfoResult.Avatar = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName, svrctx.Get().WSPort, svrctx.Get().HttpStaticURL +
							deviceInfoResult.Avatar)
					}
				}

				for i, ava := range deviceInfoResult.ContactAvatar{
					if len(ava) > 0 {
						deviceInfoResult.ContactAvatar[i] = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName,
							svrctx.Get().WSPort, svrctx.Get().HttpStaticURL + ava)
					}
				}

				deviceInfoResult.FamilyNumber = familyNumber
				resultJson, _ := json.Marshal(&deviceInfoResult)
				result.Data = string([]byte(resultJson))
			}
			proto.DeviceInfoListLock.Unlock()
		}else{
			settingResult := proto.DeviceSettingResult{Settings: params.Settings}
			settingResultJson, _ := json.Marshal(settingResult)
			result.Data = string([]byte(settingResultJson))
		}
	}

	jsonData, _ := json.Marshal(&result)

	appServerChan <- (&proto.AppMsgData{Cmd: cmdAck, Imei: imei,
		UserName: params.UserName, AccessToken:params.AccessToken,
		Data: string(jsonData), ConnID: connid})

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


func AppDeleteVoices(connid uint64, params *proto.DeleteVoicesParams) bool {
	return proto.DeleteVoicesForApp(proto.Str2Num(params.Imei, 10), params.DeleteVoices)
}

func AppActiveDeviceToLocateNow(connid uint64, params *proto.DeviceActiveParams) bool {
	return AppActiveDevice(connid, proto.DeviceLocateNowCmdName, proto.CMD_AP00, params)
}

//func AppActiveDeviceToConnectServer(connid uint64, params *proto.DeviceActiveParams) bool {
//	return AppActiveDevice(proto.ActiveDeviceCmdName, proto.CMD_ACTIVE_DEVICE, params)
//}

func AppActiveDeviceSos(connid uint64, params *proto.DeviceActiveParams) bool {
	//return AppActiveDevice(proto.ActiveDeviceSosCmdName, proto.CMD_AP16, params)
	imei := proto.Str2Num(params.Imei, 10)
	id := proto.NewMsgID()
	svrctx.Get().TcpServerChan <- proto.MakeReplyMsg(imei, true, proto.MakeSosReplyMsg(imei, id), id)

	return true
}

func AppSetDeviceVoiceMonitor(connid uint64, params *proto.DeviceActiveParams) bool {
	//return AppActiveDevice(proto.ActiveDeviceSosCmdName, proto.CMD_AP16, params)
	imei := proto.Str2Num(params.Imei, 10)
	isGT06 := proto.GetDeviceModel(imei) == proto.DM_GT06
	id := proto.NewMsgID()
	svrctx.Get().TcpServerChan <- proto.MakeReplyMsg(imei, true,
		proto.MakeVoiceMonitorReplyMsg(imei, id, params.Phone, isGT06), id)

	return true
}

func AppQueryLocations(connid uint64, params *proto.QueryLocationsParams) bool {
	cmdAck := proto.GetLocationsAckCmdName
	if params.AlarmOnly {
		cmdAck = proto.GetAlarmsAckCmdName
	}

	imei := proto.Str2Num(params.Imei, 10)
	locations := svrctx.QueryLocations(imei, svrctx.Get().PGPool, params.BeginTime, params.EndTime, params.Lbs, params.AlarmOnly)
	if locations == nil || len(*locations) == 0 {
		locations = &[]proto.LocationData{}
	}

	result := proto.QueryLocationsResult{Imei: params.Imei, BeginTime: params.BeginTime, EndTime: params.EndTime, Locations: *locations}

	appServerChan <- (&proto.AppMsgData{Cmd: cmdAck,
		UserName: params.UserName,
		AccessToken: params.AccessToken,
		Data: proto.MakeStructToJson(result), ConnID: connid})

	return true
}

func AppDeleteAlarms(connid uint64, params *proto.QueryLocationsParams) bool {
	cmdAck := proto.DeleteAlarmsAckCmdName
	imei := proto.Str2Num(params.Imei, 10)
	ret := svrctx.DeleteAlarms(imei, svrctx.Get().PGPool, params.BeginTime, params.EndTime)

	result := proto.DeleteAlarmsResult{Imei: params.Imei, BeginTime: params.BeginTime, EndTime: params.EndTime,
		ErrorCode: 0}
	if ret == false {
		result.ErrorCode = 500
	}

	appServerChan <- (&proto.AppMsgData{Cmd: cmdAck,
		UserName: params.UserName,
		AccessToken: params.AccessToken,
		Data: proto.MakeStructToJson(result), ConnID: connid})

	return true
}

func AppActiveDevice(connid uint64, reqCmd string, msgCmd uint16, params *proto.DeviceActiveParams)  bool {
	reqParams := proto.AppRequestTcpConnParams{}
	reqParams.ConnID = connid
	reqParams.ReqCmd = reqCmd
	reqParams.Params = *params

	msg := proto.MsgData{Data: []byte(proto.MakeStructToJson(&reqParams))}
	msg.Header.Header.ID = proto.NewMsgID()
	msg.Header.Header.Imei = proto.Str2Num(params.Imei, 10)
	msg.Header.Header.Cmd = msgCmd
	msg.Header.Header.Version = proto.MSG_HEADER_VER_EX
	msg.Header.Header.From = proto.MsgFromAppServerToTcpServer

	svrctx.Get().TcpServerChan <- &msg

	return true
}