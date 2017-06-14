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
	case proto.LoginCmdName:
		params := msg["data"].(map[string]interface{})
		return login(c, params["username"].(string), params["password"].(string))

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

		return handleHeartBeat(c, &params)

	case proto.VerifyCodeCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceBaseParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string)}
		return getDeviceVerifyCode(c, &params)
	case proto.SetDeviceCmdName:
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.DeviceSettingParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("set-device parse json failed, " + err.Error())
			return false
		}
		return AppUpdateDeviceSetting(c, &params, false)
	case proto.GetDeviceByImeiCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceAddParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string)}
		//return getDeviceInfoByImei(c, &params)
		addDeviceManagerChan <- &AddDeviceChanCtx{cmd: proto.GetDeviceByImeiCmdName, c: c, params: &params}
	case proto.AddDeviceCmdName:
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.DeviceAddParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("add-device parse json failed, " + err.Error())
			return false
		}

		addDeviceManagerChan <- &AddDeviceChanCtx{cmd: proto.AddDeviceCmdName, c: c, params: &params}
	case proto.DeleteDeviceCmdName:
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.DeviceAddParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("delete-device parse json failed, " + err.Error())
			return false
		}

		addDeviceManagerChan <- &AddDeviceChanCtx{cmd: proto.DeleteDeviceCmdName, c: c, params: &params}
	default:
		break
	}
	
	return true
}

func handleHeartBeat(c *AppConnection, params *proto.HeartbeatParams) bool {
	result := proto.HeartbeatResult{Timestamp: time.Now().Format("20060102150405")}
	for _, imei := range params.Devices {
		result.Locations = append(result.Locations, svrctx.GetDeviceData(proto.Str2Num(imei, 10), svrctx.Get().PGPool))
	}

	appServerChan <- &proto.AppMsgData{Cmd: proto.HearbeatAckCmdName,
		UserName: params.UserName,
		AccessToken: params.AccessToken,
		Data: proto.MakeStructToJson(result), Conn: c}

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

		appServerChan <- &proto.AppMsgData{Cmd: proto.LoginAckCmdName,
			Data: (fmt.Sprintf("{\"user\": %s, \"location\": %s}", string(body), string(jsonLocations))), Conn: c}
	}

	return true
}

func getDeviceVerifyCode(c *AppConnection, params *proto.DeviceBaseParams) bool {
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
	isAdmin, _,_,_ := queryIsAdmin(params.Imei, params.UserName)
	if isAdmin {
		proto.DeviceInfoListLock.RLock()
		deviceInfo, ok := (*proto.DeviceInfoList)[imei]
		if ok && deviceInfo != nil {
			verifyCode = deviceInfo.VerifyCode
		}
		proto.DeviceInfoListLock.RUnlock()
	}

	appServerChan <- &proto.AppMsgData{Cmd: proto.VerifyCodeAckCmdName, Imei: imei,
		UserName: params.UserName, AccessToken:params.AccessToken,
			Data: fmt.Sprintf("{\"VerifyCode\": \"%s\", \"isAdmin\": %d}", verifyCode, proto.Bool2UInt8(isAdmin)), Conn: c}

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
		return true, matched, deviceRecId, nil
	}else {
		return false, matched, deviceRecId, nil
	}
}

func getDeviceInfoByImei(c *AppConnection, params *proto.DeviceAddParams) bool {
	deviceInfoResult := proto.DeviceInfo{}
	imei := proto.Str2Num(params.Imei, 10)
	proto.DeviceInfoListLock.RLock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	if ok && deviceInfo != nil {
		deviceInfoResult = *deviceInfo
	}else{
		proto.DeviceInfoListLock.RUnlock()
		logging.Log(params.Imei + "  imei not found")
		return false
	}
	proto.DeviceInfoListLock.RUnlock()

	deviceInfoResult.VerifyCode = ""

	isAdmin, _, _, err := queryIsAdmin(params.Imei, params.UserName)
	if err != nil {
		return false
	}

	if isAdmin {
		deviceInfoResult.IsAdmin = 1
	}

	resultData, _ := json.Marshal(&deviceInfoResult)

	appServerChan <- &proto.AppMsgData{Cmd: proto.GetDeviceByImeiAckCmdName, Imei: imei,
		UserName: params.UserName, AccessToken:params.AccessToken,
		Data: string(resultData), Conn: c}

	return true
}

func checkVerifyCode(imei, code string) bool {
	matched := false
	proto.DeviceInfoListLock.RLock()
	deviceInfo, ok := (*proto.DeviceInfoList)[proto.Str2Num(imei, 10)]
	if ok && deviceInfo != nil {
		matched = deviceInfo.VerifyCode == code
	}else{
		matched = true
	}
	proto.DeviceInfoListLock.RUnlock()

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

func makeFamilyPhoneNumbers(family *[proto.MAX_FAMILY_MEMBER_NUM]proto.FamilyMember) string {
	phoneNumbers := ""
	for i := 0; i < len(family); i++ {
		if  i > 0 {
			phoneNumbers += ","
		}

		phoneNumbers += fmt.Sprintf("%s|%d|%s",  family[i].Phone, family[i].Type, family[i].Name)
	}

	return phoneNumbers
}

func addDeviceByUser(c *AppConnection, params *proto.DeviceAddParams) bool {
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
			}else{
				strSqlUpdateDeviceInfo := ""
				isFound := false
				family := [proto.MAX_FAMILY_MEMBER_NUM]proto.FamilyMember{}
				newVerifycode := makerRandomVerifyCode()
				proto.DeviceInfoListLock.Lock()
				deviceInfo, ok := (*proto.DeviceInfoList)[imei]
				if ok && deviceInfo != nil {
					deviceInfo.VerifyCode = newVerifycode
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
				}
				proto.DeviceInfoListLock.Unlock()
				if isFound {
					phoneNumbers = makeFamilyPhoneNumbers(&family)
					if isAdmin {
						//如果是管理员，则更新手表对应的字段数据，非管理员仅更新关注列表和对应的亲情号
						strSqlUpdateDeviceInfo = fmt.Sprintf("update watchinfo set OwnerName='%s', CountryCode='%s', " +
							"PhoneNumbers='%s', TimeZone='%s', VerifyCode='%s'  where IMEI='%s' ", params.OwnerName, params.DeviceSimCountryCode,
							phoneNumbers, params.TimeZone, newVerifycode, params.Imei)
					} else {
						strSqlUpdateDeviceInfo = fmt.Sprintf("update watchinfo set PhoneNumbers='%s', VerifyCode='%s' ",
							phoneNumbers, newVerifycode)
					}

					logging.Log("SQL: " + strSqlUpdateDeviceInfo)
					_, err := svrctx.Get().MySQLPool.Exec(strSqlUpdateDeviceInfo)
					if err != nil {
						logging.Log(fmt.Sprintf("[%s] update  watchinfo db failed, %s", imei, err.Error()))
						result.ErrCode = 500
						result.ErrMsg = "server failed to update db"
					}else{
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

		setting := proto.SettingParam{proto.PhoneNumbersFieldName, "", phoneNumbers,
			//fmt.Sprintf("%s|%d|%s",  params.MySimID, params.PhoneType, params.MyName),
		}

		newSettings.Settings = append(newSettings.Settings, setting)

		if isAdmin {
			setting = proto.SettingParam{proto.OwnerNameFieldName, "", params.OwnerName}
			newSettings.Settings = append(newSettings.Settings, setting)

			setting = proto.SettingParam{proto.CountryCodeFieldName, "", params.DeviceSimCountryCode}
			newSettings.Settings = append(newSettings.Settings, setting)

			setting = proto.SettingParam{proto.SimIDFieldName, "", params.DeviceSimID}
			newSettings.Settings = append(newSettings.Settings, setting)

			setting = proto.SettingParam{proto.TimeZoneFieldName, "", params.TimeZone}
			newSettings.Settings = append(newSettings.Settings, setting)
		}

		return AppUpdateDeviceSetting(c, &newSettings, true)
	}else{
		resultData, _ := json.Marshal(&result)
		appServerChan <- &proto.AppMsgData{Cmd: proto.AddDeviceAckCmdName, Imei: imei,
			UserName: params.UserName, AccessToken:params.AccessToken,
			Data: string(resultData), Conn: c}
	}

	return true
}

func deleteDeviceByUser(c *AppConnection, params *proto.DeviceAddParams) bool {
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
	appServerChan <- &proto.AppMsgData{Cmd: proto.DeleteDeviceAckCmdName, Imei: imei,
		UserName: params.UserName, AccessToken:params.AccessToken,
		Data: string(resultData), Conn: c}

	return true
}

func AddDeviceManagerLoop()  {
	for {
		select {
		case msg := <- addDeviceManagerChan:
			if msg == nil {
				return
			}

			if msg.cmd == proto.GetDeviceByImeiCmdName {
				getDeviceInfoByImei(msg.c, msg.params)
			}else if  msg.cmd == proto.AddDeviceCmdName {
				addDeviceByUser(msg.c, msg.params)
			}else if  msg.cmd == proto.DeleteDeviceCmdName {
				deleteDeviceByUser(msg.c, msg.params)
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
			case proto.CountryCodeFieldName:
				deviceInfo.CountryCode = setting.NewValue
			case proto.PhoneNumbersFieldName:
				phoneNumbers = makeFamilyPhoneNumbers(&deviceInfo.Family)
				for i := 0; i < len(deviceInfo.Family); i++ {
					curPhone := proto.ParseSinglePhoneNumberString(setting.CurValue, i)
					newPhone := proto.ParseSinglePhoneNumberString(setting.NewValue, i)
					//fullPhoneNnumber := "00" + deviceInfo.Family[i].CountryCode + deviceInfo.Family[i].Phone
					if len(curPhone.Phone) == 0 { //之前没有号码，直接寻找一个空位就可以了
						if len(deviceInfo.Family[i].Phone) == 0 {
							deviceInfo.Family[i] = newPhone
							break
						}
					}else{ //之前有号码，那么这里是修改号码，需要匹配之前的号码
						if deviceInfo.Family[i].Phone == curPhone.Phone {
							deviceInfo.Family[i] = newPhone
							break
						}
					}
				}

				settings[index].NewValue = phoneNumbers
			default:
			}
		}
	}
	proto.DeviceInfoListLock.Unlock()

	//更新数据库
	return UpdateDeviceSettingInDB(imei, settings, valulesIsString)
}

func AppUpdateDeviceSetting(c *AppConnection, params *proto.DeviceSettingParams, isAddDevice bool) bool {
	isNeedNotifyDevice := make([]bool, len(params.Settings))
	valulesIsString := make([]bool, len(params.Settings))
	imei := proto.Str2Num(params.Imei, 10)
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
		case proto.PhoneNumbersFieldName:
			//上面都是需要通知手表更新设置的

		case proto.CountryCodeFieldName:
		case proto.SimIDFieldName:
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
			proto.DeviceInfoListLock.RLock()
			deviceInfo, ok := (*proto.DeviceInfoList)[imei]
			if ok && deviceInfo != nil {
				resultJson, _ := json.Marshal(proto.MakeDeviceInfoResult(deviceInfo))
				result.Data = string([]byte(resultJson))
			}
			proto.DeviceInfoListLock.RUnlock()
		}else{
			settingResult := proto.DeviceSettingResult{Settings: params.Settings}
			settingResultJson, _ := json.Marshal(settingResult)
			result.Data = string([]byte(settingResultJson))
		}

		jsonData, _ := json.Marshal(&result)

		appServerChan <- &proto.AppMsgData{Cmd: cmdAck, Imei: imei,
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
