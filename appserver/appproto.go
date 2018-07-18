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
	"github.com/garyburd/redigo/redis"
	"crypto/tls"
	"image"
	"os"
	"path/filepath"
	"image/jpeg"
	"image/png"
	"image/gif"
)

const (
	base64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
)

const MAX_CHAN_NUM = 16

var coder = base64.NewEncoding(base64Table)

func base64Encode(src []byte) []byte {
	return []byte(coder.EncodeToString(src))
}

func base64Decode(src []byte) ([]byte, error) {
	return coder.DecodeString(string(src))
}

func init()  {
}

func IsAccessTokenValid(accessToken string) (bool, []string) {
	var imeiList []string
	valid := false
	//需要进行更多验证
	if valid, imeiList = ValidAccessTokenFromService(accessToken); valid {
		AccessTokenTableLock.Lock()
		AccessTokenTable[accessToken] = true
		AccessTokenTableLock.Unlock()
		valid = true
	}
	/*AccessTokenTableLock.Lock()
	logined, ok := AccessTokenTable[accessToken]
	if ok && logined {
		//验证通过
		valid = true
	}else{
		//需要进行更多验证
		if valid, imeiList = ValidAccessTokenFromService(accessToken); valid {
			AccessTokenTable[accessToken] = true
			valid = true
		}
	}
	AccessTokenTableLock.Unlock()*/

	return valid, imeiList
}

func HandleAppRequest(connid uint64, appserverChan chan *proto.AppMsgData, data []byte) bool {
	//logging.Log(fmt.Sprintf("HandleAppRequest: %s\n",string(data)))

	var itf interface{}
	header := data[:6]
	proto.DevConnidenc[connid] = false
	if string(header) == "GTS01:" {
		//logging.Log(fmt.Sprintf("handle connid:%d\n\n",connid))
		proto.DevConnidenc[connid] = true
		ens := data[6:len(data)]
		//logging.Log("ens: " + string(ens))
		desc := proto.AppserDecrypt(string(ens))

		err :=json.Unmarshal([]byte(desc), &itf)
		if err != nil {
			logging.Log("GTS01:parse recved json data failed, " + err.Error())
			return false
		}

	} else  {
		return false
		/*err :=json.Unmarshal(data[0:], &itf)
		if err != nil {
			logging.Log("AppConnReadLoop,parse recved json data failed, " + err.Error())
			return false
		}*/
	}

	msg:= itf.(map[string]interface{})
	cmd :=  msg["cmd"]

	if cmd == nil {
		return false
	}

	//chenqw,20171124
	//default false ,no encrypt
	/*encData := msg["data"].(map[string]interface{})
	if encData["encrypt"] != nil {
		proto.DevConnidenc[connid] = true
		desdata, err := proto.AppserDecrypt(encData["encrypt"].(string))
		logging.Log("HandleAppRequest:desdata:" + string(desdata))
		var itfdec interface{}
		err = json.Unmarshal([]byte(desdata), &itfdec)
		if err != nil {
			logging.Log("desdata to json failed, " + err.Error())
			return false
		}
		newMsg := itfdec.(map[string]interface{})
		//now msg map struct is same as before encrypted......
		msg["data"] = newMsg["data"]
	}*/

	var imeiList []string
	valid := false
	params := msg["data"].(map[string]interface{})
	//logging.Log(fmt.Sprintf("handle cmd:%s,accessToken:%s\n\n",cmd,params["accessToken"].(string)))
	if ( params["accessToken"] == nil ||  params["accessToken"].(string) == "") { //没有accesstoken
		//同时又不是注册和登录，那么认为是非法请求
		if cmd.(string) != proto.LoginCmdName && cmd.(string) != proto.RegisterCmdName  &&
			cmd.(string) != proto.ResetPasswordCmdName {
			logging.Log("access token is bad, not login and register")
			return false
		}
	}else{
		/*有accesstoken，那么需要对token进行验证,if accesstoken's imei belongs to one of the imeilist,
			then accesskoken verify is passed!
		*/
		accessToken := params["accessToken"].(string)
		valid, imeiList = IsAccessTokenValid(accessToken)
		logging.Log(fmt.Sprintf( "cmd :%s imeiList 0 :",cmd, params["username"], params["accessToken"], imeiList))
		if valid == false{
			logging.Log("access token is not found")
			return false
		}
	}


	switch cmd{
	case proto.LoginCmdName:
		//logging.Log(fmt.Sprintf("Login username:%s,password:%s\n\n",params["username"].(string), params["password"].(string)))
		return login(connid, params["username"].(string), params["password"].(string), false)
	case proto.LoginOutCmdName:
		logging.Log(fmt.Sprintf("Logout imei:%s,uuid:%s\n\n",params["imei"].([]interface{}), params["uuid"].(string)))
		var _imeiList []string
		for _,mem := range params["imei"].([]interface{}){
			_imeiList = append(_imeiList,mem.(string))
		}
		return LoginOut(connid,_imeiList,params["uuid"].(string))
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
		//logging.Log(fmt.Sprintf("Hearbeat username:%s,accessToken:%s\n\n",params["username"].(string), params["accessToken"].(string)))
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.HeartbeatParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("heartbeat parse json failed, " + err.Error())
			return false
		}

		//logging.Log(fmt.Sprint( "imeiList 1 :", params.UserName, params.AccessToken, imeiList))
		return handleHeartBeat(imeiList, connid, &params)

	case proto.VerifyCodeCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceBaseParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string)}

		/*if InStringArray(params.Imei, imeiList) == false {
			return false
		}*/

		return getDeviceVerifyCode(connid, &params)
	case proto.RefreshDeviceCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceBaseParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string)}

		if InStringArray(params.Imei, imeiList) == false {
			return false
		}

		return refreshDevice(connid, &params)
	case proto.SetDeviceCmdName:
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.DeviceSettingParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("set-device parse json failed, " + err.Error())
			return false
		}

		if InStringArray(params.Imei, imeiList) == false {
			logging.Log("imei is not in StringArray")
			return false
		}

		return AppUpdateDeviceSetting(connid, &params, false, "")
	case proto.GetDeviceByImeiCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceAddParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string)}

		/*if InStringArray(params.Imei, imeiList) == false {
			return false
		}*/

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

		/*if InStringArray(params.Imei, imeiList) == false {
			return false
		}*/
		logging.Log(fmt.Sprintf("AddDeviceCmdName:%s---%d",string(jsonString),params.AccountType))
		addDeviceManagerChan <- &AddDeviceChanCtx{cmd: proto.AddDeviceCmdName, connid: connid, params: &params}
	case proto.DeleteDeviceCmdName:
		jsonString, _ := json.Marshal(msg["data"])
		params := proto.DeviceAddParams{}
		err:=json.Unmarshal(jsonString, &params)
		if err != nil {
			logging.Log("delete-device parse json failed, " + err.Error())
			return false
		}

		if InStringArray(params.Imei, imeiList) == false {
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

		if InStringArray(params.Imei, imeiList) == false {
			return false
		}

		return AppDeleteVoices(connid, &params)
	case proto.DeviceLocateNowCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceActiveParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string),
			Phone: datas["phone"].(string)}

		if InStringArray(params.Imei, imeiList) == false {
			return false
		}

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

		if InStringArray(params.Imei, imeiList) == false {
			return false
		}

		return AppActiveDeviceSos(connid, &params)
	case proto.SetDeviceVoiceMonitorCmdName:
		datas := msg["data"].(map[string]interface{})
		params := proto.DeviceActiveParams{Imei: datas["imei"].(string),
			UserName: datas["username"].(string),
			AccessToken: datas["accessToken"].(string),
			Phone: datas["phone"].(string)}

		if InStringArray(params.Imei, imeiList) == false {
			return false
		}

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

		if InStringArray(params.Imei, imeiList) == false {
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

		if InStringArray(params.Imei, imeiList) == false {
			return false
		}

		return AppDeleteAlarms(connid, &params)
	case proto.CmdLocation:
		jsonString,_ := json.Marshal(msg["data"])
		params := proto.LocateIMEIParams{}
		err := json.Unmarshal(jsonString,&params)
		if err != nil{
			logging.Log(cmd.(string) + " parse json LocateIMEIParams failed, " + err.Error())
			return false
		}
		if InStringArray(params.Imei,imeiList) == false{
			return false
		}
	default:
		break
	}
	
	return true
}

func handleHeartBeat(imeiList []string, connid uint64, params *proto.HeartbeatParams) bool {
	if params.AccessToken == ""{
		logging.Log("no AccessToken," + params.UserName)
		return true
	}
	logging.Log(fmt.Sprintf("handleHeartBeat params:UserName:%s,SelectedDevice:%s,familynumber:%s,Devices:%s--%s--%s--%s--%s",
		params.UserName,params.SelectedDevice,params.FamilyNumber,params.Devices,params.UUID,params.DeviceToken, params.Platform, params.Language))

	proto.MapAccessToLang[params.AccessToken] = params.Language
	proto.AccessTokenMap[params.AccessToken] = params.UserName
	for i, imei := range params.Devices {
		if InStringArray(imei, imeiList) == false {
			continue
		}

		imeiUint64 := proto.Str2Num(imei, 10)
		if params.FamilyNumbers == nil || len(params.FamilyNumbers) < (i + 1) {
			if params.UUID != "" &&  params.DeviceToken != "" && params.Platform != "android"{
				proto.ReportDevieeToken(svrctx.Get().APNSServerApiBase, imeiUint64, "",
					params.UUID, params.DeviceToken, params.Platform, params.Language)
			}else if params.UUID != "" &&  params.AccessToken != "" && params.Platform == "android"{
				proto.ReportDevieeToken(svrctx.Get().APNSServerApiBase, imeiUint64, "",
					params.UUID, params.AccessToken, params.Platform, params.Language)
			}
		}else{
			if params.UUID != "" &&  params.DeviceToken != "" && params.Platform != "android"{
				proto.ReportDevieeToken(svrctx.Get().APNSServerApiBase, imeiUint64, params.FamilyNumbers[i],
					params.UUID, params.DeviceToken, params.Platform, params.Language)
			}else if params.UUID != "" &&  params.AccessToken != "" && params.Platform == "android"{
				proto.ReportDevieeToken(svrctx.Get().APNSServerApiBase, imeiUint64, params.FamilyNumbers[i],
					params.UUID, params.AccessToken, params.Platform, params.Language)
			}
		}

		_,ok1 := proto.ConnidUserName[imeiUint64]
		if !ok1{
			proto.ConnidUserName[imeiUint64] = map[string]string{}
		}
		_,ok2 := proto.ConnidUserName[imeiUint64][params.FamilyNumber]
		if !ok2 {
			proto.ConnidUserName[imeiUint64][params.FamilyNumber] = params.UserName
		}
	}

	tmpMinichat := []proto.ChatInfo{}
	result := proto.HeartbeatResult{Timestamp: time.Now().Format("20060102150405"),ImeiAddFriend:map[uint64]proto.AddFriend{}}
	if params.SelectedDevice == "" {
		for _, imei := range params.Devices {
			if InStringArray(imei, imeiList) == false {
				continue
			}

			imeiUint64 := proto.Str2Num(imei, 10)
			result.Locations = append(result.Locations, svrctx.GetDeviceData(imeiUint64, svrctx.Get().PGPool))
			result.Minichat = append(result.Minichat, proto.GetChatListForApp(imeiUint64, params.UserName)...)

			for k, _ := range result.Minichat {
				/*if result.Minichat[k].Receiver == deviceInfo.Family[j].Phone && len(result.Minichat[k].Receiver) > 1 ||
					len(result.Minichat[k].Receiver) == 0 {
				//Receiver为空表示是从手机APP端发送至手表 {
					if proto.ConnidUserName[params.UserName] == deviceInfo.Family[j].Username {
						logging.Log("handleHeartBeat responseChan")
						tmpMinichat = append(tmpMinichat,result.Minichat[k])
						break
					}
				}*/
				if (result.Minichat[k].Receiver == params.FamilyNumber && len(result.Minichat[k].Receiver) > 1) ||
					len(result.Minichat[k].Receiver) == 0{
					tmpMinichat = append(tmpMinichat,result.Minichat[k])
					logging.Log("1 handleHeartBeat responseChan")
					continue
				}

				if result.Minichat[k].Receiver == "0" {
					tmpMinichat = append(tmpMinichat,result.Minichat[k])
					logging.Log("1 handleHeartBeat responseChan 000")
					continue
				}
			}

			//add friend flag

			proto.DeviceInfoListLock.Lock()
			deviceInfo, ok := (*proto.DeviceInfoList)[imeiUint64]
			if ok{
				for index,_ := range deviceInfo.Family {
					if deviceInfo.Family[index].IsAddFriend == 1{
						var addfriend = proto.AddFriend{}
						addfriend.FriendPhone = deviceInfo.Family[index].Phone
						addfriend.FriendDevName = deviceInfo.Family[index].FriendDevName
						if deviceInfo.Family[index].FriendAvatar != "" {
							if svrctx.Get().UseHttps {
								addfriend.FriendAvatar = fmt.Sprintf("%s:%d", svrctx.Get().HttpServerName, svrctx.Get().WSSPort) +
									deviceInfo.Family[index].FriendAvatar
							} else {
								addfriend.FriendAvatar = fmt.Sprintf("%s:%d", svrctx.Get().HttpServerName, svrctx.Get().WSPort) +
									deviceInfo.Family[index].FriendAvatar
							}
						}

						proto.MapPhone2IMEILock.Lock()
						_imei,ok1 := (*proto.MapPhone2IMEI)[deviceInfo.Family[index].Phone]
						if ok1{
							addfriend.FriendIMEI = _imei
						}
						proto.MapPhone2IMEILock.Unlock()

						result.ImeiAddFriend[imeiUint64] = addfriend
					}
				}
			}
			proto.DeviceInfoListLock.Unlock()
		}
	}else{
		if InStringArray(params.SelectedDevice, imeiList) {
			imeiUint64 := proto.Str2Num(params.SelectedDevice, 10)
			endTime := uint64(params.Timestamp)
			beginTime := endTime - endTime % 1000000
			result.Locations = append(result.Locations, svrctx.GetDeviceData(imeiUint64, svrctx.Get().PGPool))
			result.Minichat = append(result.Minichat, proto.GetChatListForApp(imeiUint64, params.UserName)...)

			alarms := svrctx.QueryLocations(imeiUint64, svrctx.Get().PGPool, beginTime, endTime, false, true)
			if alarms != nil {
				result.Alarms = append(result.Alarms, (*alarms)...)
			}

			for k, _ := range result.Minichat {
				/*if (result.Minichat[k].Receiver == deviceInfo.Family[j].Phone && len(result.Minichat[k].Receiver) > 1)  ||
					len(result.Minichat[k].Receiver) == 0{
					//旧的模式没有username,兼容之
					if proto.ConnidUserName[params.UserName] == deviceInfo.Family[j].Username {
						logging.Log("handleHeartBeat responseChan")
						tmpMinichat = append(tmpMinichat,result.Minichat[k])
						//break
						continue
					}
				}*/
				if (result.Minichat[k].Receiver == params.FamilyNumber && len(result.Minichat[k].Receiver) > 1) ||
					len(result.Minichat[k].Receiver) == 0{
					tmpMinichat = append(tmpMinichat,result.Minichat[k])
					logging.Log("2 handleHeartBeat responseChan")
					continue
				}
				//Receiver == "0"表示群发消息至关注该手表的人;len(Receiver) == 0表示仅仅手机端发送和接收
				if result.Minichat[k].Receiver == "0"{
					tmpMinichat = append(tmpMinichat,result.Minichat[k])
					//break
					continue
				}
			}

			//add friend tag
			proto.DeviceInfoListLock.Lock()
			deviceInfo, ok := (*proto.DeviceInfoList)[imeiUint64]
			if ok{
				for index,_ := range deviceInfo.Family {
					fmt.Println("AAAA",deviceInfo.Family[index].IsAddFriend)
					if deviceInfo.Family[index].IsAddFriend == 1{
						var addfriend = proto.AddFriend{}
						//对应好友的
						addfriend.FriendPhone = deviceInfo.Family[index].Phone
						addfriend.FriendDevName = deviceInfo.Family[index].FriendDevName
						if deviceInfo.Family[index].FriendAvatar != "" {
							if svrctx.Get().UseHttps {
								addfriend.FriendAvatar = fmt.Sprintf("%s:%d", svrctx.Get().HttpServerName, svrctx.Get().WSSPort) +
									deviceInfo.Family[index].FriendAvatar
							} else {
								addfriend.FriendAvatar = fmt.Sprintf("%s:%d", svrctx.Get().HttpServerName, svrctx.Get().WSPort) +
									deviceInfo.Family[index].FriendAvatar
							}
						}
						proto.MapPhone2IMEILock.Lock()
						_imei,ok1 := (*proto.MapPhone2IMEI)[deviceInfo.Family[index].Phone]
						if ok1{
							addfriend.FriendIMEI = _imei
						}
						proto.MapPhone2IMEILock.Unlock()

						result.ImeiAddFriend[imeiUint64] = addfriend
					}
				}
			}
			proto.DeviceInfoListLock.Unlock()
		}
	}
	fmt.Println("MapPhone2IMEI:",*proto.MapPhone2IMEI)
	result.Minichat = tmpMinichat
	proto.QuickSort(result.Minichat,0,len(result.Minichat) - 1)
	//logging.Log(fmt.Sprintf("atfer sort...%s:", proto.MakeStructToJson(result)))

	appServerChan <- (&proto.AppMsgData{Cmd: proto.HearbeatAckCmdName,
		UserName: params.UserName,
		AccessToken: params.AccessToken,
		Data: proto.MakeStructToJson(result), ConnID: connid})

	return true
}

func login(connid uint64, username, password string, isRegister bool) bool {
	var urlRequest string
	if !svrctx.Get().IsUseAliYun {
		urlRequest = "https://watch.gatorcn.com/web/index.php?r=app/auth/"
	}else {
		urlRequest = "http://120.25.214.188/tracker/web/index.php?r=app/auth/"

	}
	reqType := "login"
	if isRegister {
		reqType = "register"
	}

	urlRequest += reqType

	tr := &http.Transport{
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}


	resp, err := client.PostForm(urlRequest, url.Values{"username": {username}, "password": {password}})
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
	//re,_ := json.Marshal(itf)
	logging.Log(fmt.Sprintf("parse login format :%s", string(body)))
	if err != nil {
		logging.Log("login parse  " + reqType + " response as json failed, " + err.Error())
		return false
	}

	loginData := itf.(map[string]interface{})
	if loginData == nil {
		return false
	}

	status :=  loginData["status"]
	accessToken := loginData["accessToken"]
	devices := loginData["devices"]

	//handle wrong login password
	if accessToken == nil {

		proto.ConnidLogin[connid] += 1
		proto.LoginTimeOut[connid] = time.Now().Unix()

	}

	logging.Log("status: " + fmt.Sprint(status))
	logging.Log("accessToken: " + fmt.Sprint(accessToken))
	//logging.Log("devices: " + fmt.Sprint(devices))
	if status != nil && accessToken != nil {
		proto.AccessTokenMap[fmt.Sprint(accessToken)] = username

		AddAccessToken(accessToken.(string))
		if isRegister == false {
			if devices != nil {
				locations := []proto.LocationData{}
				for i, d := range devices.([]interface{}) {
					device := d.(map[string]interface{})
					if device == nil || device["IMEI"] == nil {
						continue
					}

					proto.ConnidLogin[connid] = 0

					imei, _ := strconv.ParseUint(device["IMEI"].(string), 0, 0)

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

						for k, ava := range deviceInfoResult.ContactAvatar{
							if len(ava) > 0 {
								deviceInfoResult.ContactAvatar[k] = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName,
									svrctx.Get().WSPort, svrctx.Get().HttpStaticURL + ava)
							}
						}

						//deviceInfoResult.FamilyNumber = loginData["devices"].([]interface{})[i].(map[string]interface{})["FamilyNumber"].(string)
						//deviceInfoResult.Name = loginData["devices"].([]interface{})[i].(map[string]interface{})["Name"].(string)

						k := 0
						//default not root manager
						deviceInfoResult.AccountType = 1
						for k = 0; k < len(deviceInfo.Family);k++ {
							if username == deviceInfo.Family[k].Username{
								deviceInfoResult.Name = deviceInfo.Family[k].Name
								deviceInfoResult.FamilyNumber = deviceInfo.Family[k].Phone
								deviceInfoResult.AccountType = deviceInfo.Family[k].IsAdmin
								//var tag proto.TagUserName
								//tag.Username = username
								//tag.Phone = deviceInfo.Family[k].Phone
								//proto.ConnidtagUserName[username] = tag
								break
							}
						}

						//chenqw 20171228
						//deviceInfoResult.PhoneNumbers = proto.SplitPhone(deviceInfoResult.PhoneNumbers)
						logging.Log(fmt.Sprintf("the deviceInfoResult.PhoneNumbers is :%s",deviceInfoResult.PhoneNumbers))
						if k != len(deviceInfo.Family) {
							logging.Log(fmt.Sprintf("len devices = %d i = %d,k = %d",len(loginData["devices"].([]interface{})),i,k))
							loginData["devices"].([]interface{})[i] = deviceInfoResult
							fmt.Println("deviceinfoResult: ", proto.MakeStructToJson(&deviceInfoResult))
						} else {
							deviceInfoResult.FamilyNumber = deviceInfo.Family[0].Phone
							deviceInfoResult.Name = loginData["devices"].([]interface{})[i].(map[string]interface{})["Name"].(string)
							deviceInfoResult.AccountType = 0
							loginData["devices"].([]interface{})[i] = deviceInfoResult
							logging.Log(fmt.Sprintf("new loginData devices: %s\n",proto.MakeStructToJson(deviceInfoResult)))
						}
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
			if accessToken == nil {
				if proto.ConnidLogin[connid] == 5 {
					proto.ConnidLogin[connid] = 0
					appServerChan <- (&proto.AppMsgData{Cmd:proto.LoginAckCmdName,
						UserName:username,
						Data:"{\"status\": -3}", ConnID: connid})
				}else {
					appServerChan <- (&proto.AppMsgData{Cmd:proto.LoginAckCmdName,
						UserName:username,
						Data:"{\"status\": -2}", ConnID: connid})
				}

			}else {
				appServerChan <- (&proto.AppMsgData{Cmd: proto.LoginAckCmdName,
					UserName: username,
					Data: "{\"status\": -1}", ConnID: connid})
			}
		}
	}

	return true
}

func LoginOut(connid uint64,IMEI []string,UUID string) bool {
	for _,imei := range IMEI {
		proto.DeleteDevieeToken(svrctx.Get().APNSServerApiBase,proto.Str2Num(imei,10),UUID)
	}
	return true
}

//忘记密码，通过服务器重设秘密
func resetPassword(connid uint64, username string) bool {
	var urlRequest string
	if !svrctx.Get().IsUseAliYun {
		urlRequest = "https://watch.gatorcn.com/web/index.php?r=app/auth/"
	}else {
		urlRequest = "http://120.25.214.188/tracker/web/index.php?r=app/auth/"
	}
	//reqType := "login"
	reqType := "reset"
	urlRequest += reqType


	tr := &http.Transport{
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	resp, err := client.PostForm(urlRequest, url.Values{"username": {username}})
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
	var requesetURL string
	if !svrctx.Get().IsUseAliYun {
		requesetURL = "https://watch.gatorcn.com/web/index.php?r=app/service/modpwd&access-token=" + accessToken
	}else {
		requesetURL = "http://120.25.214.188/tracker/web/index.php?r=app/service/modpwd&access-token=" + accessToken

	}


	tr := &http.Transport{
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}


	logging.Log("modify password url: " + requesetURL)
	resp, err := client.PostForm(requesetURL, url.Values{"oldpwd": {oldPasswd}, "newpwd": {newPasswd}})
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
	var requesetURL string
	if !svrctx.Get().IsUseAliYun {
		requesetURL = "https://watch.gatorcn.com/web/index.php?r=app/service/feedback&access-token=" + accessToken
	}else {
		requesetURL = "http://120.25.214.188/tracker/web/index.php?r=app/service/feedback&access-token=" + accessToken

	}

	tr := &http.Transport{
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	logging.Log("url: " + requesetURL)
	resp, err := client.PostForm(requesetURL, url.Values{"feedback": {feedback}})
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
	//isAdmin, _,_,_ := queryIsAdmin(params.Imei, params.UserName)

	isAdmin := false
	bFoundAdmin := false
	proto.DeviceInfoListLock.Lock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	if ok {
		//新的管理员判断方式,新的模式下如果是管理员则肯定在新的模式下判断出来,
		for ii,_ := range deviceInfo.Family{
			if len(deviceInfo.Family[ii].Phone) == 0{
				continue
			}
			//only one root user
			if deviceInfo.Family[ii].Username == params.UserName {
				if deviceInfo.Family[ii].IsAdmin == 0 {
					isAdmin = true
					goto LABEL
				}
			}
		}

		//兼容旧的判断管理员模式,因为旧的模式username为0,旧的成员都是管理员
		for k,_ := range deviceInfo.Family {
			//新的管理员在旧的判断管理员模式判断
			if len(deviceInfo.Family[k].Username) > 1 &&
				deviceInfo.Family[k].Username == params.UserName &&
				deviceInfo.Family[k].IsAdmin != 0 {
				bFoundAdmin = true
				break
			}
		}
		//如果username都是0则都是管理员
		if !bFoundAdmin{
			isAdmin = true
		}
	}
LABEL:
	if isAdmin {
		deviceInfo, ok := (*proto.DeviceInfoList)[imei]
		if ok && deviceInfo != nil {
			verifyCode = deviceInfo.VerifyCode
		}
	}
	proto.DeviceInfoListLock.Unlock()

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

func  getDeviceInfoByImei(connid uint64, params *proto.DeviceAddParams) bool {
	found := false
	deviceInfoResult := proto.DeviceInfo{}
	imei := proto.Str2Num(params.Imei, 10)
	proto.DeviceInfoListLock.Lock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	if ok && deviceInfo != nil {
		//deviceInfoResult = *deviceInfo
		deviceInfoResult.Imei = imei
		deviceInfoResult.Model = deviceInfo.Model
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

	isAdmin := true		//true表示是管理员
	var err error
	if found {
		if deviceInfo.Model == proto.DM_GT02 {
			isAdmin, _, _, err = queryIsAdmin(params.Imei, params.UserName)
			if err != nil {
				return false
			}
		}else {

			for i, _ := range deviceInfo.Family {
				if deviceInfo.Family[i].Phone != "" {
					if deviceInfo.Family[i].IsAdmin == 0 {
						isAdmin = false
						break
					}
				}
			}
		}
		//
		if isAdmin {
			deviceInfoResult.IsAdmin = 1		//deviceInfoResult.IsAdmin = 1表示这个手表首次被关注
		}
	}

	if found == false {
		deviceInfoResult.Imei = 0
	}

	if deviceInfoResult.IsAdmin == 0 && found == true {
		deviceInfoResult.SimID = deviceInfo.SimID
		deviceInfoResult.CountryCode = deviceInfo.CountryCode
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

		//refresh.刷新时也要保存
		//proto.ConnidUserName[params.UserName] = params.UserName
		proto.AccessTokenMap[params.AccessToken] = params.UserName

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

	//chenqw,20171228
	/*strSQL := fmt.Sprintf("select veh.FamilyNumber from vehiclesinuser veh join watchinfo w on veh.VehId = w.recid where w.IMEI= '%s' limit 1", params.Imei)
	logging.Log("strSQL: " + strSQL)
	rows, err := svrctx.Get().MySQLPool.Query(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d]query FamilyNumber in db failed",imei))
		return  false
	}
	defer rows.Close()

	for rows.Next() {

		rows.Scan(&phone)
		fmt.Println(phone)
	}
	if phone != "" {
		deviceInfoResult.FamilyNumber = phone
	}else {
		//chenqw,get the first number if sql query return null
		devinfo,ok := (*proto.DeviceInfoList)[proto.Str2Num(params.Imei,10)]
		if ok {
			phone = devinfo.Family[0].Phone
		}
		deviceInfoResult.FamilyNumber = phone
	}*/
	phone := ""
	devinfo,ok := (*proto.DeviceInfoList)[proto.Str2Num(params.Imei,10)]
	if ok {
		for i := 0; i< len(deviceInfo.Family);i++{
			if deviceInfo.Family[i].Username == params.UserName {
				//客户端每次发送语音时都会调用refresh-device，要把正确的familynumber传过去
				phone = devinfo.Family[i].Phone
				deviceInfoResult.FamilyNumber = phone
				deviceInfoResult.AccountType = deviceInfo.Family[i].IsAdmin
			}
			if deviceInfo.Family[i].IsAddFriend == 1{
				deviceInfoResult.OwnerName = deviceInfo.OwnerName
			}
		}
	}
	if ok && phone == ""{
		//如果familynumber为空,则把管理员号码发送过去
		phone = devinfo.Family[0].Phone
		deviceInfoResult.FamilyNumber = phone
	}

	fmt.Printf("deviceInfoResult.FamilyNumber" + deviceInfoResult.FamilyNumber + "\n")
	//deviceInfoResult.PhoneNumbers = proto.SplitPhone(deviceInfoResult.PhoneNumbers)
	fmt.Printf("deviceInfoResult.PhoneNumbers" + deviceInfoResult.PhoneNumbers + "\n")
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

	//chenqw,another method to check if username is admin
	isAdmin := params.AccountType == 0
	isFound := false
	proto.DeviceInfoListLock.Lock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	if ok {
		for ii,_ := range deviceInfo.Family{
			if len(deviceInfo.Family[ii].Phone) == 0{
				continue
			}
			//only one root user
			if deviceInfo.Family[ii].IsAdmin == 0{
				isAdmin = false
			}

			if deviceInfo.Family[ii].Username == params.UserName{
				//这个账号已经添加过手表了
				isFound = true
			}
		}
	}
	proto.DeviceInfoListLock.Unlock()

	// 手表换了号码后，要把以前的好友关系清除
	setPhone := ""
	if deviceInfo.SimID != params.DeviceSimID{
		for k,_:= range deviceInfo.Family{
			if deviceInfo.Family[k].IsAddFriend == 1 {
				deviceInfo.Family[k] = proto.FamilyMember{}
			}
			setPhone += fmt.Sprintf("#%s#%s#%d", deviceInfo.Family[k].Phone, deviceInfo.Family[k].Name, deviceInfo.Family[k].Type)
		}
		phoneNumbers = proto.MakeFamilyPhoneNumbersEx(&deviceInfo.Family)
		strSQL := fmt.Sprintf("update watchinfo set PhoneNumbers='%s' where IMEI = '%s'",phoneNumbers,params.Imei)
		logging.Log("delete friend SQL :" + strSQL)
		_,err := svrctx.Get().MySQLPool.Exec(strSQL)
		if err != nil{
			logging.Log("add friend server failed to update db" + err.Error())
			return false
		}
		msg := proto.MsgData{}
		msg.Header.Header.Imei = imei
		msg.Header.Header.ID = proto.NewMsgID()
		msg.Header.Header.Status = 0

		body := fmt.Sprintf("%015dAP06%s,%016X)", imei,
			setPhone, msg.Header.Header.ID)
		msg.Data = []byte(fmt.Sprintf("(%04X", 5+len(body)) + body)
		svrctx.Get().TcpServerChan <- &msg
	}
	//
	_, _, deviceRecId, err := queryIsAdmin(params.Imei, params.UserName)
	if err == nil {
		if isAdmin == true {
			verifyCodeWrong = false
		} else if isAdmin == false {
			if checkVerifyCode(params.Imei, params.VerifyCode) == false {
				verifyCodeWrong = true
			}
		}

		if verifyCodeWrong {
			// error code:  -1 表示验证码不正确
			result.ErrCode = -1
			result.ErrMsg = "verification code invalid"
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
					deviceInfoResult.AccountType = 1
					resultJson, _ := json.Marshal(&deviceInfoResult)
					result.Data = string([]byte(resultJson))
				}
				proto.DeviceInfoListLock.Unlock()
			}else{
				logging.Log(fmt.Sprintf("addDeviceByUser add new phone imei %d:",imei))
				proto.Mapimei2PhoneLock.Lock()
				 _,ok := proto.Mapimei2Phone[imei]
				if ok {
					if len(proto.Mapimei2Phone[imei]) == 0 {
						logging.Log(fmt.Sprintf("addDeviceByUser add new phone :%s", params.MySimID))
						proto.Mapimei2Phone[imei] = append(proto.Mapimei2Phone[imei], params.MySimID)
					} else {
						bFoundphone := false
						for i := 0; i < len(proto.Mapimei2Phone[imei]); i++ {
							if proto.Mapimei2Phone[imei][i] == params.MySimID {
								logging.Log(fmt.Sprintf("addDeviceByUser add new phone 2 :%s", params.MySimID))
								bFoundphone = true
								break
							}
						}
						if !bFoundphone {
							logging.Log(fmt.Sprintf("addDeviceByUser add new phone 1 :%s", params.MySimID))
							proto.Mapimei2Phone[imei] = append(proto.Mapimei2Phone[imei], params.MySimID)
						}
					}
				}else {
					logging.Log(fmt.Sprintf("addDeviceByUser add new phone 3 :%s", params.MySimID))
					proto.Mapimei2Phone[imei] = append(proto.Mapimei2Phone[imei], params.MySimID)
				}
				proto.Mapimei2PhoneLock.Unlock()

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

							//chenqw,add
							deviceInfo.Family[i].IsAdmin = int(proto.Bool2UInt8(!isAdmin))
							deviceInfo.Family[i].Username = params.UserName
							//chenqw

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
					phoneNumbers = proto.MakeFamilyPhoneNumbersEx(&family)
					if isAdmin {
						//如果是管理员，则更新手表对应的字段数据，非管理员仅更新关注列表和对应的亲情号
						strSqlUpdateDeviceInfo = fmt.Sprintf("update watchinfo set OwnerName='%s', CountryCode='%s', " +
							"PhoneNumbers='%s', TimeZone='%s', VerifyCode='%s'  where IMEI='%s' ", params.OwnerName, params.DeviceSimCountryCode,
							phoneNumbers, params.TimeZone, newVerifycode, params.Imei)
						strSQLUpdateSimID := fmt.Sprintf("UPDATE device SET SimID=%s  where systemNo=%d",
							params.DeviceSimID, imei % 100000000000)
						fmt.Printf("params.DeviceSimID is %s\n",params.DeviceSimID)
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
						logging.Log(fmt.Sprintf("[%d] update  watchinfo db failed, %s", imei, err.Error()))
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
							logging.Log(fmt.Sprintf("[%d] insert into vehiclesinuser db failed, %s", imei, err.Error()))
							result.ErrCode = 500
							result.ErrMsg = "server failed to update db"
						}

						admin, success := GetAdminByDeviceId(imei, deviceRecId)
						if success {
							strSqlUpdateUserCompany := fmt.Sprintf("update users set companyid='%s', bossid='%s'  " +
								"where loginname='%s' ", admin.CompanyId, admin.UserId, params.UserName)

							logging.Log("strSqlUpdateUserCompany: " + strSqlUpdateUserCompany)
							_, err := svrctx.Get().MySQLPool.Exec(strSqlUpdateUserCompany)
							if err != nil {
								logging.Log(fmt.Sprintf("[%s] update user %s company and boss id in db failed, %s",
									imei, params.UserName, err.Error()))
							}
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
		for i := 0;i < len(newSettings.Settings) ;i++{
			logging.Log("newSettings.Settings:" +
				fmt.Sprintf("%d FiledName:%s,CurValue:%s,NewValue:%s,Index:%d",
					i,
					newSettings.Settings[i].FieldName,
					newSettings.Settings[i].CurValue,
					newSettings.Settings[i].NewValue,
					newSettings.Settings[i].Index))
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

	strSqlDeleteDeviceUser := fmt.Sprintf("delete v  from vehiclesinuser as v left join watchinfo as w on v.VehId=w.recid "+
		" where w.imei='%s' and v.UserID='%s' ", params.Imei, params.UserId)
	logging.Log("SQL: " + strSqlDeleteDeviceUser)
	_, err := svrctx.Get().MySQLPool.Exec(strSqlDeleteDeviceUser)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] delete  from vehiclesinuser db failed, %s", imei, err.Error()))
		result.ErrCode = 500
		result.ErrMsg = "server failed to update db"
		resultData, _ := json.Marshal(&result)
		appServerChan <- (&proto.AppMsgData{Cmd: proto.DeleteDeviceAckCmdName, Imei: imei,
			UserName: params.UserName, AccessToken:params.AccessToken,
			Data: string(resultData), ConnID: connid})
		return  false
	}

	/*strSqlDeleteDeviceUser = fmt.Sprintf("update users u join vehiclesinuser viu on u.recid = viu.UserID" +
		" join watchinfo w on w.recid = viu.VehId set u.LoginName = '' WHERE w.IMEI = '%s' and viu.UserID='%s'",params.Imei,params.UserId)
	logging.Log("SQL: " + strSqlDeleteDeviceUser)
	_, err = svrctx.Get().MySQLPool.Exec(strSqlDeleteDeviceUser)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] delete  from vehiclesinuser db failed, %s", imei, err.Error()))
		result.ErrCode = 500
		result.ErrMsg = "server failed to update db"
		resultData,_ := json.Marshal(&result)
		appServerChan <- (&proto.AppMsgData{Cmd: proto.DeleteDeviceAckCmdName, Imei: imei,
			UserName: params.UserName, AccessToken:params.AccessToken,
			Data: string(resultData), ConnID: connid})
		return  false
	}*/


	//cqw,20180108
	proto.DeviceInfoListLock.Lock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	var i int
	bIsAdminCancel := 0
	i = 0
	if ok {
		if params.AccountType == 0{
			inDex := 0
			for i = 0;i < len(deviceInfo.Family);i++{
				//兼容以前老的，username="0"
				logging.Log(fmt.Sprintf("delete phonenumber:%s---%s---%d",params.MySimID,deviceInfo.Family[i].Phone,deviceInfo.Family[i].IsAdmin))
				if (params.UserName == deviceInfo.Family[i].Username || params.MySimID == deviceInfo.Family[i].Phone) &&
					deviceInfo.Family[i].Phone != ""{
					//chenqw,20180118,删除该号码对应的图片,兼容老版本
					msg := proto.MsgData{}
					msg.Header.Header.Imei = imei
					msg.Header.Header.ID = proto.NewMsgID()
					msg.Header.Header.Status = 0
					body := fmt.Sprintf("%015dAP25,%s,%016X)", imei,
						deviceInfo.Family[i].Phone,   msg.Header.Header.ID)
					msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
					svrctx.Get().TcpServerChan <- &msg

					newPhone := proto.ParseSinglePhoneNumberString("",-1)
					deviceInfo.Family[i] = newPhone

					bIsAdminCancel |= 0x01
				}

				if len(deviceInfo.Family[i].Username) > 1 && deviceInfo.Family[i].IsAdmin != 0 && (bIsAdminCancel & 0x02 == 0){
					logging.Log(fmt.Sprintf("bIsAdminCancel: i = %d",i))
					inDex = i
					bIsAdminCancel |= 0x02
					deviceInfo.Family[inDex].IsAdmin = 0
				}

				if bIsAdminCancel == 0x03 {
					break
				}
			}

			//旧的模式管理员取消关注时,此时新增关注的账号必须成为管理员
			for kk,_ := range deviceInfo.Family{
				if len(deviceInfo.Family[kk].Username) <= 1 &&
					len(deviceInfo.Family[kk].Phone) > 1 &&
					deviceInfo.Family[kk].IsAdmin == 0{

					deviceInfo.Family[kk].IsAdmin = 1
				}
			}

		}else {
			for i = 0;i < len(deviceInfo.Family);i++{
				logging.Log(fmt.Sprintf("the delete phonenumber:%s---%s---",params.MySimID,deviceInfo.Family[i].Phone))
				if (params.UserName == deviceInfo.Family[i].Username || params.MySimID == deviceInfo.Family[i].Phone) &&
					deviceInfo.Family[i].Phone != ""{
					//chenqw,20180118,删除该号码对应的图片,兼容老版本
					msg := proto.MsgData{}
					msg.Header.Header.Imei = imei
					msg.Header.Header.ID = proto.NewMsgID()
					msg.Header.Header.Status = 0
					body := fmt.Sprintf("%015dAP25,%s,%016X)", imei,
						deviceInfo.Family[i].Phone,   msg.Header.Header.ID)
					msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
					svrctx.Get().TcpServerChan <- &msg

					newPhone := proto.ParseSinglePhoneNumberString("",-1)
					deviceInfo.Family[i] = newPhone
					break
				}
			}
		}


		//管理员取消关注手表，判断phonenumbers里的username 是不是非0,
		// 如果都是0下次不管哪个账号关注这个手表就是管理员,如果非0就把na个非0的设置成管理员
		/*bCheckAdmin := false
		inDex := 0
		if bIsAdminCancel {
			for i := 0; i < len(deviceInfo.Family); i++ {
				if len(deviceInfo.Family[i].Username) > 1 {
					logging.Log(fmt.Sprintf("bIsAdminCancel: i = %d",i))
					inDex = i
					bCheckAdmin = true
					break
				}
			}
			for idx,_ := range deviceInfo.Family{
				deviceInfo.Family[idx].IsAdmin = 1
			}
			if bCheckAdmin{
				//IsAdmin = 0 means root manager
				//把管理员移到首位
				//modify:不用移到首位
				logging.Log(fmt.Sprintf("bCheckAdmin:%d,%s i = %d",deviceInfo.Family[inDex].IsAdmin,deviceInfo.Family[i].Username,inDex))
				deviceInfo.Family[inDex].IsAdmin = 0
				/*tmp := deviceInfo.Family[0]
				deviceInfo.Family[0] = deviceInfo.Family[inDex]
				deviceInfo.Family[inDex] = tmp
				logging.Log(fmt.Sprintf("###bCheckAdmin:%d,%s",deviceInfo.Family[0].IsAdmin,deviceInfo.Family[0].Username))*/
		//	}

		//}else {
			//deviceInfo.Family[0].IsAdmin = 0
		//}

		//管理员取消关注但添加了几个亲情号码的情况,并且还有其他用户关注
		//不需要移动
		/*if bCheckAdmin {
			newContacts := [proto.MAX_FAMILY_MEMBER_NUM]proto.FamilyMember{}
			count := 0
			for i := 0; i < 3; i++ {
				if deviceInfo.Family[i].Phone != "" {
					newContacts[count] = deviceInfo.Family[i]
					count++
				}
			}
			//白名单位置只能从第四个位置开始
			count = 3
			for i := 3; i < len(deviceInfo.Family); i++ {
				if deviceInfo.Family[i].Phone != "" {
					newContacts[count] = deviceInfo.Family[i]
					count++
				}
			}
			deviceInfo.Family = newContacts
		}*/

	}
	proto.DeviceInfoListLock.Unlock()

	//把发送AP06消息提前,解决管理员取消关注后第二个管理员手表端图片被删除问题
	msg := proto.MsgData{}
	msg.Header.Header.Imei = imei
	msg.Header.Header.ID = proto.NewMsgID()
	msg.Header.Header.Status = 0
	phoneNumbers := ""
	bAll := false
	proto.Mapimei2PhoneLock.Lock()
	proto.Mapimei2Phone[imei] = []string{}
	for kk := 0; kk < len(deviceInfo.Family); kk++ {
		phone := deviceInfo.Family[kk].Phone
		if phone == ""{
			phone = "0"
		}else {
			bAll = true
		}
		phoneNumbers += fmt.Sprintf("#%s#%s#%d", phone, deviceInfo.Family[kk].Name, deviceInfo.Family[kk].Type)

		if deviceInfo.Family[kk].Phone == ""{
			continue
		}
		proto.Mapimei2Phone[imei] = append(proto.Mapimei2Phone[imei],deviceInfo.Family[kk].Phone)
	}
	proto.Mapimei2PhoneLock.Unlock()

	if bAll {
		body := fmt.Sprintf("%015dAP06%s,%016X)", imei,
			phoneNumbers, msg.Header.Header.ID)
		msg.Data = []byte(fmt.Sprintf("(%04X", 5+len(body)) + body)
		/*body := fmt.Sprintf("%015dAP25,%s,%016X)", imei,
		deviceInfo.Family[i].Phone,   msg.Header.Header.ID)
	msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)*/
		svrctx.Get().TcpServerChan <- &msg
	}else {
		body := fmt.Sprintf("%015dAP07,%016X)", imei,msg.Header.Header.ID)
		msg.Data = []byte(fmt.Sprintf("(%04X", 5+len(body)) + body)

		svrctx.Get().TcpServerChan <- &msg
	}

	newPhone := proto.MakeFamilyPhoneNumbersEx(&deviceInfo.Family)
	ContactAvatar := makeContactAvatars(&deviceInfo.Family)

	logging.Log(fmt.Sprintf("newPhone Number: %s--%s--%s",newPhone,params.DeviceToken,params.AccessToken))
	strSQL := fmt.Sprintf("UPDATE watchinfo SET PhoneNumbers = '%s' where IMEI='%d'", newPhone, imei)
	logging.Log("SQL: " + strSQL)
	_,err = svrctx.Get().MySQLPool.Exec(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] UPDATE watchinfo SET PhoneNumbers db failed, %s", imei, err.Error()))
		result.ErrCode = 500
		result.ErrMsg = "server failed to update db"
		resultData, _ := json.Marshal(&result)
		appServerChan <- (&proto.AppMsgData{Cmd: proto.DeleteDeviceAckCmdName, Imei: imei,
			UserName: params.UserName, AccessToken:params.AccessToken,
			Data: string(resultData), ConnID: connid})
		return  false
	}

	strSQL = fmt.Sprintf("UPDATE watchinfo SET ContactAvatar = '{\"ContactAvatars\": %s}' where IMEI='%d'", ContactAvatar, imei)
	logging.Log("SQL: " + strSQL)
	_,err = svrctx.Get().MySQLPool.Exec(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] UPDATE watchinfo SET ContactAvatar db failed, %s", imei, err.Error()))
		result.ErrCode = 500
		result.ErrMsg = "server failed to update db"
		resultData, _ := json.Marshal(&result)
		appServerChan <- (&proto.AppMsgData{Cmd: proto.DeleteDeviceAckCmdName, Imei: imei,
			UserName: params.UserName, AccessToken:params.AccessToken,
			Data: string(resultData), ConnID: connid})
		return  false
	}
	//end cqw

	//通过devices IMEI删除通知数据(报警和微聊),不要推送至手机端,chenqw,20180105
	var conn redis.Conn
	conn = redisPool.Get()
	if conn == nil {
		return false
	}
	defer conn.Close()

	//redis 分布式锁
	Clock := svrctx.RedisLock{LockKey: "xxxxx"}
LABEL_REDIS:
	err = Clock.Lock(&conn,5)
	if err != nil{
		goto LABEL_REDIS
	}
	logging.Log("redis distribute lock success 1")
	_,err = conn.Do("del",fmt.Sprintf("ntfy:%d", imei))
	if err != nil {
		logging.Log("deleteDeviceByUser:" + err.Error())
		Clock.Unlock(&conn)
		return false
	}
	Clock.Unlock(&conn)
	//end chenqw

	resultData, _ := json.Marshal(&result)
	appServerChan <- (&proto.AppMsgData{Cmd: proto.DeleteDeviceAckCmdName, Imei: imei,
		UserName: params.UserName, AccessToken:params.AccessToken,
		Data: string(resultData), ConnID: connid})

	//ios:deviceToken,android:accessToken
	//if params.UUID != "" {
	//	proto.DeleteDevieeToken(svrctx.Get().APNSServerApiBase, imei, params.UUID)
	//}
	if params.DeviceToken == ""{
		//android,delete
		proto.DeleteDevieeToken(svrctx.Get().APNSServerApiBase,imei,params.AccessToken)
	}else {
		//ios,delete
		proto.DeleteDevieeToken(svrctx.Get().APNSServerApiBase,imei,params.DeviceToken)
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

func SaveDeviceSettings(imei uint64, settings []proto.SettingParam, valulesIsString []bool)  (bool,[]proto.SettingParam) {
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
						logging.Log(fmt.Sprintf("[%d] bad data for fence setting %d, err(%s) for %s",
							imei, setting.Index, err.Error(), setting.NewValue))
						proto.DeviceInfoListLock.Unlock()
						return false,nil
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
				logging.Log(fmt.Sprintf("setting.CurValue :%s,setting.NewValue:%s,setting.Index:%d",
					setting.CurValue,setting.NewValue,setting.Index))
				//if setting.Index == 0 {
				//	break
				//}
				if setting.NewValue == "delete"{
					//chenqw,要把用户名对应的userID传过来,否则不好删除vehiclesinuser,下次添加手表时会出错
					//要设置成只有管理员才可以删除，非管理员可以修改
					//proto.Mapimei2PhoneLock.Lock()
					proto.Mapimei2Phone[imei] = []string{}
					for i := 0; i < len(deviceInfo.Family); i++ {
						if i + 1 == setting.Index {
							//newPhone := proto.ParseSinglePhoneNumberString("",-1)
							//deviceInfo.Family[i] = newPhone
							if len(deviceInfo.Family[i].Username) > 1{
								//说明此时有账号删除关注手表
								strSqlUserID := fmt.Sprintf("select recid from users where LoginName = '%s'", deviceInfo.Family[i].Username)
								UserId := ""
								logging.Log("SQLL: " + strSqlUserID)
								rows, err := svrctx.Get().MySQLPool.Query(strSqlUserID)
								if err != nil {
									logging.Log(fmt.Sprintf("[%d] query recid in db failed, %s", imei, err.Error()))
									proto.DeviceInfoListLock.Unlock()
									return false, nil
								}
								defer rows.Close()
								for rows.Next() {
									err = rows.Scan(&UserId)
									if err != nil {
										logging.Log(fmt.Sprintf("[%d] query UserId users in db failed, %s", imei, err.Error()))
										proto.DeviceInfoListLock.Unlock()
										return false, nil
									}
									break
								}

								strSqlDeleteDeviceUser := fmt.Sprintf("delete v  from vehiclesinuser as v left join watchinfo as w on v.VehId=w.recid "+
									" where w.imei='%d' and v.UserID='%s' ", imei, UserId)
								_, err = svrctx.Get().MySQLPool.Exec(strSqlDeleteDeviceUser)
								logging.Log("SQLL: " + strSqlDeleteDeviceUser)
								if err != nil {
									logging.Log(fmt.Sprintf("[%d] delete rvehiclesinuser failed, %s", imei, err.Error()))
									proto.DeviceInfoListLock.Unlock()
									return false, nil
								}
							}

							//chenqw,20180118,删除该号码对应的图片
							msg := proto.MsgData{}
							msg.Header.Header.Imei = imei
							msg.Header.Header.ID = proto.NewMsgID()
							msg.Header.Header.Status = 0
							body := fmt.Sprintf("%015dAP25,%s,%016X)", imei,
								deviceInfo.Family[i].Phone,   msg.Header.Header.ID)
							msg.Data = []byte(fmt.Sprintf("(%04X", 5 + len(body)) + body)
							svrctx.Get().TcpServerChan <- &msg

							newPhone := proto.ParseSinglePhoneNumberString("",-1)
							deviceInfo.Family[setting.Index - 1] = newPhone
						}

						deviceInfo.Family[i].Index = i + 1
						if deviceInfo.Family[i].Phone == ""{
							continue
						}
						proto.Mapimei2Phone[imei] = append(proto.Mapimei2Phone[imei],deviceInfo.Family[i].Phone)
					}
					//proto.Mapimei2PhoneLock.Unlock()

					/*newContacts := [proto.MAX_FAMILY_MEMBER_NUM]proto.FamilyMember{}
					count := 0
					for i := 0;i < len(deviceInfo.Family);i++ {
						//前面3个phone不用往前移,
						if setting.Index > 3 {
							if i + 1 == setting.Index {
								newPhone := proto.ParseSinglePhoneNumberString("",-1)
								deviceInfo.Family[i] = newPhone
							}
							if i + 1 != setting.Index {
								newContacts[count] = deviceInfo.Family[i]
								count++
							}
						} else {
							if i + 1 == setting.Index {
								newPhone := proto.ParseSinglePhoneNumberString("", -1)
								deviceInfo.Family[i] = newPhone
							}
						}
					}

					if setting.Index > 3 {
						deviceInfo.Family = newContacts
					}*/
				}else{
					//只有管理员才可以添加,delete亲情号码
					logging.Log(fmt.Sprintf("enter add new phone......"))
					var newPhone proto.FamilyMember

					newPhone = proto.ParseSinglePhoneNumberString(setting.NewValue, setting.Index)
					//chenqw,20180119,新增白名单index=0xff,其他传正确的index
					if setting.Index == 0xff {
						for i := 3; i < len(deviceInfo.Family); i++ {
							//bkAvatar := deviceInfo.Family[i].Avatar
							if len(deviceInfo.Family[i].Phone) == 0 {
								logging.Log(fmt.Sprintf(" 0xff add new phone......"))
								deviceInfo.Family[i] = newPhone
								deviceInfo.Family[i].Index = i + 1
								settings[0].Index = i + 1
								break
							}
						}
					}else if setting.Index == 0{
						//setting.Index  == 0表示前面3个新增号码
						for i := 0; i < 3; i++ {
							if len(deviceInfo.Family[i].Phone) == 0 {
								logging.Log(fmt.Sprintf(" 0x00 add new phone......"))
								deviceInfo.Family[i] = newPhone
								deviceInfo.Family[i].Index = i + 1
								settings[0].Index = i + 1
								break
							}
						}
					} else{
						bkAvatar := deviceInfo.Family[setting.Index - 1].Avatar
						deviceInfo.Family[setting.Index - 1] = newPhone
						deviceInfo.Family[setting.Index - 1].Avatar = bkAvatar

						phone2users,ok1 := proto.ConnidUserName[imei]
						if ok1 {
							user, okname := phone2users[deviceInfo.Family[setting.Index-1].Phone]
							if okname && user != "" {
								delete(proto.ConnidUserName[imei], deviceInfo.Family[setting.Index-1].Phone)
								if len(proto.ConnidUserName[imei]) == 0{
									delete(proto.ConnidUserName,imei)
								}
								_,ok2 := proto.ConnidUserName[imei]
								if !ok2 {
									proto.ConnidUserName[imei] = map[string]string{}
								}
								proto.ConnidUserName[imei][deviceInfo.Family[setting.Index-1].Phone] = user
							}
						}

						//chenqw,20180116,更新下发图片和语音发送对应的亲情号码
						proto.AppNewPhotoPendingListLock.Lock()
						photoList, ok := proto.AppNewPhotoPendingList[imei]
						if ok {
							for photoidx, _ := range *photoList {
								logging.Log(fmt.Sprintf("AppNewPhotoPendingList[%d],%s",
									imei, (*proto.AppNewPhotoPendingList[imei])[photoidx].Info.Member.Phone))
								//号码要做相应的改变
								if (*photoList)[photoidx].Info.Index == setting.Index {
									(*photoList)[photoidx].Info.Member.Phone = newPhone.Phone
								}
								logging.Log(fmt.Sprintf("AppNewPhotoPendingList[%d],%s",
									imei, (*proto.AppNewPhotoPendingList[imei])[photoidx].Info.Member.Phone))
							}
						}
						proto.AppNewPhotoPendingListLock.Unlock()

						proto.AppSendChatListLock.Lock()
						chatList, ok := proto.AppSendChatList[imei]
						if ok {
							for chatidx, _ := range *chatList {
								logging.Log(fmt.Sprintf("AppSendChatList[%d],%s",
									imei, (*proto.AppSendChatList[imei])[chatidx].Info.Sender))
								//是当前用户登录并且号码已修改了
								if (*chatList)[chatidx].Info.SenderUser == newPhone.Username && (*chatList)[chatidx].Info.Sender != newPhone.Phone {
									(*chatList)[chatidx].Info.Sender = newPhone.Phone
								}
								logging.Log(fmt.Sprintf("AppSendChatList[%d],%s",
									imei, (*proto.AppSendChatList[imei])[chatidx].Info.Sender))
							}
						}
						proto.AppSendChatListLock.Unlock()

					}


					/*for i := 0; i < len(deviceInfo.Family); i++ {
						curPhone := proto.ParseSinglePhoneNumberString(setting.CurValue, setting.Index)
						newPhone = proto.ParseSinglePhoneNumberString(setting.NewValue, setting.Index)
						//fullPhoneNnumber := "00" + deviceInfo.Family[i].CountryCode + deviceInfo.Family[i].Phone
						if setting.Index != 0xff {
							if len(curPhone.Phone) == 0 { //之前没有号码，直接寻找一个空位就可以了
								if len(deviceInfo.Family[i].Phone) == 0 && i+1 == setting.Index {
									bkAvatar := deviceInfo.Family[i].Avatar
									deviceInfo.Family[i] = newPhone
									deviceInfo.Family[i].Avatar = bkAvatar
									//非管理员不保存username
									//if deviceInfo.Family[i].IsAdmin == 1{
									//	deviceInfo.Family[i].Username = "0"
									//}
									if deviceInfo.Family[i].Username == "undefined" {
										deviceInfo.Family[i].Username = "0"
									}

									//chenqw,20180116,更新下发图片和语音发送对应的亲情号码
									proto.AppNewPhotoPendingListLock.Lock()
									photoList, ok := proto.AppNewPhotoPendingList[imei]
									if ok {
										for photoidx, _ := range *photoList {
											logging.Log(fmt.Sprintf("##AppNewPhotoPendingList[%d],%s",
												imei, (*proto.AppNewPhotoPendingList[imei])[photoidx].Info.Member.Phone))
											//号码要做相应的改变
											if (*photoList)[photoidx].Info.Index == setting.Index {
												(*photoList)[photoidx].Info.Member.Phone = newPhone.Phone
											}
											logging.Log(fmt.Sprintf("##AppNewPhotoPendingList[%d],%s",
												imei, (*proto.AppNewPhotoPendingList[imei])[photoidx].Info.Member.Phone))
										}
									}
									proto.AppNewPhotoPendingListLock.Unlock()

									break
								}
							} else { //之前有号码，那么这里是修改号码，需要匹配之前的号码
								if deviceInfo.Family[i].Phone == curPhone.Phone &&
									deviceInfo.Family[i].Index == curPhone.Index {
									bkAvatar := deviceInfo.Family[i].Avatar
									deviceInfo.Family[i] = newPhone
									deviceInfo.Family[i].Avatar = bkAvatar
									//非管理员不保存username
									//if deviceInfo.Family[i].IsAdmin == 1{
									//	deviceInfo.Family[i].Username = "0"
									//}
									if deviceInfo.Family[i].Username == "undefined" {
										deviceInfo.Family[i].Username = "0"
									}

									//chenqw,20180116,更新下发图片和语音发送对应的亲情号码
									proto.AppNewPhotoPendingListLock.Lock()
									photoList, ok := proto.AppNewPhotoPendingList[imei]
									if ok {
										for photoidx, _ := range *photoList {
											logging.Log(fmt.Sprintf("AppNewPhotoPendingList[%d],%s",
												imei, (*proto.AppNewPhotoPendingList[imei])[photoidx].Info.Member.Phone))
											//号码要做相应的改变
											if (*photoList)[photoidx].Info.Member.Phone == curPhone.Phone {
												(*photoList)[photoidx].Info.Member.Phone = newPhone.Phone
											}
											logging.Log(fmt.Sprintf("AppNewPhotoPendingList[%d],%s",
												imei, (*proto.AppNewPhotoPendingList[imei])[photoidx].Info.Member.Phone))
										}
									}
									proto.AppNewPhotoPendingListLock.Unlock()

									proto.AppSendChatListLock.Lock()
									chatList, ok := proto.AppSendChatList[imei]
									if ok {
										for chatidx, _ := range *chatList {
											logging.Log(fmt.Sprintf("AppSendChatList[%d],%s",
												imei, (*proto.AppSendChatList[imei])[chatidx].Info.Sender))
											if (*chatList)[chatidx].Info.Sender != newPhone.Phone {
												(*chatList)[chatidx].Info.Sender = newPhone.Phone
											}
											logging.Log(fmt.Sprintf("AppSendChatList[%d],%s",
												imei, (*proto.AppSendChatList[imei])[chatidx].Info.Sender))
										}
									}
									proto.AppSendChatListLock.Unlock()

									break
								}
							}
						} else if setting.Index == 0xff {
							if i >= 3{

								if len(curPhone.Phone) == 0 { //之前没有号码，直接寻找一个空位就可以了
									if len(deviceInfo.Family[i].Phone) == 0 {
										bkAvatar := deviceInfo.Family[i].Avatar
										deviceInfo.Family[i] = newPhone
										deviceInfo.Family[i].Avatar = bkAvatar
										deviceInfo.Family[i].Index = i + 1
										setting.Index = i + 1
										//非管理员不保存username
										//if deviceInfo.Family[i].IsAdmin == 1{
										//	deviceInfo.Family[i].Username = "0"
										//}
										if deviceInfo.Family[i].Username == "undefined" {
											deviceInfo.Family[i].Username = "0"
										}

										//chenqw,20180116,更新下发图片和语音发送对应的亲情号码
										proto.AppNewPhotoPendingListLock.Lock()
										photoList, ok := proto.AppNewPhotoPendingList[imei]
										if ok {
											for photoidx, _ := range *photoList {
												logging.Log(fmt.Sprintf("##AppNewPhotoPendingList[%d],%s",
													imei, (*proto.AppNewPhotoPendingList[imei])[photoidx].Info.Member.Phone))
												//号码要做相应的改变
												if (*photoList)[photoidx].Info.Index == setting.Index {
													(*photoList)[photoidx].Info.Member.Phone = newPhone.Phone
												}
												logging.Log(fmt.Sprintf("##AppNewPhotoPendingList[%d],%s",
													imei, (*proto.AppNewPhotoPendingList[imei])[photoidx].Info.Member.Phone))
											}
										}
										proto.AppNewPhotoPendingListLock.Unlock()

										break
									}
								} else { //之前有号码，那么这里是修改号码，需要匹配之前的号码
									if deviceInfo.Family[i].Phone == curPhone.Phone {
										bkAvatar := deviceInfo.Family[i].Avatar
										deviceInfo.Family[i] = newPhone
										deviceInfo.Family[i].Avatar = bkAvatar
										deviceInfo.Family[i].Index = i + 1
										setting.Index = i + 1
										//非管理员不保存username
										//if deviceInfo.Family[i].IsAdmin == 1{
										//	deviceInfo.Family[i].Username = "0"
										//}
										if deviceInfo.Family[i].Username == "undefined" {
											deviceInfo.Family[i].Username = "0"
										}

										//chenqw,20180116,更新下发图片和语音发送对应的亲情号码
										proto.AppNewPhotoPendingListLock.Lock()
										photoList, ok := proto.AppNewPhotoPendingList[imei]
										if ok {
											for photoidx, _ := range *photoList {
												logging.Log(fmt.Sprintf("AppNewPhotoPendingList[%d],%s",
													imei, (*proto.AppNewPhotoPendingList[imei])[photoidx].Info.Member.Phone))
												//号码要做相应的改变
												if (*photoList)[photoidx].Info.Member.Phone == curPhone.Phone {
													(*photoList)[photoidx].Info.Member.Phone = newPhone.Phone
												}
												logging.Log(fmt.Sprintf("AppNewPhotoPendingList[%d],%s",
													imei, (*proto.AppNewPhotoPendingList[imei])[photoidx].Info.Member.Phone))
											}
										}
										proto.AppNewPhotoPendingListLock.Unlock()

										proto.AppSendChatListLock.Lock()
										chatList, ok := proto.AppSendChatList[imei]
										if ok {
											for chatidx, _ := range *chatList {
												logging.Log(fmt.Sprintf("AppSendChatList[%d],%s",
													imei, (*proto.AppSendChatList[imei])[chatidx].Info.Sender))
												if (*chatList)[chatidx].Info.Sender != newPhone.Phone {
													(*chatList)[chatidx].Info.Sender = newPhone.Phone
												}
												logging.Log(fmt.Sprintf("AppSendChatList[%d],%s",
													imei, (*proto.AppSendChatList[imei])[chatidx].Info.Sender))
											}
										}
										proto.AppSendChatListLock.Unlock()

										break
									}
								}


							}
						}
					}*/


					proto.Mapimei2PhoneLock.Lock()
					/*if len(proto.Mapimei2Phone[imei]) == 0 {
						logging.Log(fmt.Sprintf("add new phone :%s", newPhone.Phone))
						proto.Mapimei2Phone[imei] = append(proto.Mapimei2Phone[imei], newPhone.Phone)
					}else {
						bFoundphone := false
						for i := 0; i < len(proto.Mapimei2Phone[imei]); i++ {
							if proto.Mapimei2Phone[imei][i] == newPhone.Phone {
								logging.Log(fmt.Sprintf("addDeviceByUser add new phone 2 :%s", newPhone.Phone))
								bFoundphone = true
								break
							}
						}
						if !bFoundphone {
							if setting.Index <= len(proto.Mapimei2Phone[imei]) {
								logging.Log(fmt.Sprintf("addDeviceByUser add new phone 4 :%s", newPhone.Phone))
								proto.Mapimei2Phone[imei][setting.Index - 1] = ""
								proto.Mapimei2Phone[imei][setting.Index - 1] = newPhone.Phone
							}else {
								logging.Log(fmt.Sprintf("addDeviceByUser add new phone 1 :%s", newPhone.Phone))
								proto.Mapimei2Phone[imei] = append(proto.Mapimei2Phone[imei], newPhone.Phone)
							}
						}
					}*/

					proto.Mapimei2Phone[imei] = []string{}
					for i := 0;i < len(deviceInfo.Family);i++ {
						if deviceInfo.Family[i].Phone == ""{
							continue
						}
						proto.Mapimei2Phone[imei] = append(proto.Mapimei2Phone[imei],deviceInfo.Family[i].Phone)
					}

					proto.Mapimei2PhoneLock.Unlock()

					/*for i,_ :=range proto.Mapimei2Phone[imei]{
						logging.Log(fmt.Sprintf("Mapimei2Phone:%d,%s",imei,proto.Mapimei2Phone[imei][i]))
					}*/
				}

				phoneNumbers = proto.MakeFamilyPhoneNumbersEx(&deviceInfo.Family)
				settings[index].NewValue = phoneNumbers

			case proto.ContactAvatarsFieldName:
			//	for i, m := range deviceInfo.Family {
					logging.Log(fmt.Sprintf("ContactAvatarsFieldName %d,%s",setting.Index - 1,deviceInfo.Family[setting.Index - 1].Avatar))
					//if m.Index == setting.Index {
						deviceInfo.Family[setting.Index - 1].Avatar = setting.NewValue
						logging.Log(fmt.Sprintf("ContactAvatarsFieldName %d,%s",setting.Index - 1,deviceInfo.Family[setting.Index - 1].Avatar))
						//break
					//}
				//}

				settings[index].NewValue = fmt.Sprintf("{\"ContactAvatars\": %s}", makeContactAvatars(&deviceInfo.Family))
				logging.Log(fmt.Sprintf("settings[index].NewValue index = %d,ContactAvatars = %s",index,settings[index].NewValue))
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

							return false, nil
						}
					}

					settings[index].FieldName += proto.Num2Str(uint64(setting.Index),10)
				}else{
					proto.DeviceInfoListLock.Unlock()
					logging.Log(fmt.Sprintf("[%d] bad index %d for delete watch alarm", imei, setting.Index))
					return false,nil
				}
			case proto.HideSelfFieldName:
				deviceInfo.HideTimerOn = (proto.Str2Num(setting.NewValue, 10)) == 1
			case proto.DisableWiFiFieldName:
				deviceInfo.DisableWiFi = (proto.Str2Num(setting.NewValue, 10)) == 1
			case proto.DisableLBSFieldName:
				deviceInfo.DisableLBS =  (proto.Str2Num(setting.NewValue, 10)) == 1
			case proto.RedirectIPPortFieldName:
				deviceInfo.RedirectIPPort =  (proto.Str2Num(setting.NewValue, 10)) == 1
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

					return false,nil
				}
			default:
			}
		}


	}
	proto.DeviceInfoListLock.Unlock()

	//更新数据库,添加亲情号码时,username设置成管理员的username,但数据库存储不变
	/*returnSettings := settings
	for index, setting := range settings {
		if setting.FieldName ==  proto.PhoneNumbersFieldName && ok{
			phoneNumbers = proto.MakeFamilyPhoneNumbersEx(&deviceInfo.Family)
			settings[index].NewValue = phoneNumbers
		}
	}*/
	ret := UpdateDeviceSettingInDB(imei, settings, valulesIsString)
	fmt.Println("UpdateDeviceSettingInDB:",settings,"\n")
	return ret,settings
}

func AppUpdateDeviceSetting(connid uint64, params *proto.DeviceSettingParams, isAddDevice bool,
	familyNumber string) bool {
	logging.Log(fmt.Sprintf("AppUpdateDeviceSetting settings FieldName :%d",len(params.Settings)))
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
			if model == proto.DM_GT06 || model == proto.DM_GT05{
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
	//var settings []proto.SettingParam
	if isAddDevice == false {
		ret,_ = SaveDeviceSettings(imei, params.Settings, valulesIsString)
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
				//phoneNumbers := proto.MakeFamilyPhoneNumbersEx(&deviceInfo.Family)
				//deviceInfoResult.PhoneNumbers = phoneNumbers
				//logging.Log(fmt.Sprintf("params.Settings len = %d",len(params.Settings)))
				if len(params.Settings) > 1 {
					//ROOT
					deviceInfoResult.AccountType = 0
				}else {
					//not root
					deviceInfoResult.AccountType = 1
				}
				resultJson, _ := json.Marshal(&deviceInfoResult)
				result.Data = string([]byte(resultJson))
				logging.Log(fmt.Sprintf("deviceInfoResult Settings:" + string(resultJson)))
			}
			proto.DeviceInfoListLock.Unlock()
		}else{
			settingResult := proto.DeviceSettingResult{Settings: params.Settings,MsgId:params.MsgId}
			settingResultJson, _ := json.Marshal(settingResult)
			fmt.Printf("Settings:%s---%s",params.Settings[0].NewValue,settingResultJson)
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
		}else if setting.FieldName == "PhoneNumbers" {
			//chenqw,20180118,删除亲情号码时，要把对应的图片删掉
			if len(concatValues) == 0 {
				concatValues += fmt.Sprintf(" %s=%s ", setting.FieldName, newValue)
			}else{
				concatValues += fmt.Sprintf(", %s=%s ", setting.FieldName, newValue)
			}
			proto.DeviceInfoListLock.Lock()
			deviceInfo, ok := (*proto.DeviceInfoList)[imei]
			newContactAvatar := makeContactAvatars(&deviceInfo.Family)
			proto.DeviceInfoListLock.Unlock()
			if ok && deviceInfo != nil{
				strSQL = fmt.Sprintf("UPDATE watchinfo SET ContactAvatar = '{\"ContactAvatars\": %s}' where IMEI='%d'",
					newContactAvatar, imei)
				logging.Log("SQL: " + strSQL)
				_,err := svrctx.Get().MySQLPool.Exec(strSQL)
				if err != nil {
					logging.Log(fmt.Sprintf("[%d] UPDATE watchinfo SET ContactAvatar db failed, %s", imei, err.Error()))
					return  false
				}

			}


		}else{
			if len(concatValues) == 0 {
				concatValues += fmt.Sprintf(" %s=%s ", setting.FieldName, newValue)
			}else{
				concatValues += fmt.Sprintf(", %s=%s ", setting.FieldName, newValue)
			}

			//好友修改ownername时要及时更新name
			proto.DeviceInfoListLock.Lock()
			deviceInfo, ok := (*proto.DeviceInfoList)[imei]
			proto.DeviceInfoListLock.Unlock()
			if ok{
				for k,_ := range deviceInfo.Family{
					if deviceInfo.Family[k].IsAddFriend == 1{
						fmt.Println("MapPhone2IMEI:",*proto.MapPhone2IMEI)
						Imei := (*proto.MapPhone2IMEI)[deviceInfo.Family[k].Phone]
						proto.DeviceInfoListLock.Lock()
						dev, ok1 := (*proto.DeviceInfoList)[Imei]
						proto.DeviceInfoListLock.Unlock()
						if ok1{
							if dev.Family[k].FriendDevName != deviceInfo.OwnerName {
								dev.Family[k].FriendDevName = deviceInfo.OwnerName
								dev.Family[k].Name = deviceInfo.OwnerName
								newPhone := proto.MakeFamilyPhoneNumbersEx(&dev.Family)
								strSQL = fmt.Sprintf("update watchinfo set PhoneNumbers = '%s' where IMEI = '%s'",newPhone,proto.Num2Str(Imei,10))
								logging.Log("SQL update new: " + strSQL)
								_,err := svrctx.Get().MySQLPool.Exec(strSQL)
								if err != nil{
									logging.Log(fmt.Sprintf("[%d] UPDATE watchinfo SET PhoneNumbers db failed, %s", Imei, err.Error()))
									return  false
								}

								//AP06 TO devinfo set
								msg := &proto.MsgData{}
								msg.Header.Header.Imei = Imei
								msg.Header.Header.ID = proto.NewMsgID()
								msg.Header.Header.Status = 0
								setPhone := ""
								for k,_:= range dev.Family{
									setPhone += fmt.Sprintf("#%s#%s#%d", dev.Family[k].Phone, dev.Family[k].Name, dev.Family[k].Type)
								}
								body := fmt.Sprintf("%015dAP06%s,%016X)", Imei, setPhone, msg.Header.Header.ID)
								msg.Data = []byte(fmt.Sprintf("(%04X", 5+len(body)) + body)

								svrctx.Get().TcpServerChan <- msg
							}
						}
					}
				}
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
	canRequest := false
	//首先判断是否支持语音监听
	proto.DeviceInfoListLock.Lock()
	deviceInfo, ok := (*proto.DeviceInfoList)[imei]
	if ok && deviceInfo != nil {
		canRequest = deviceInfo.HideVoiceMonitor == false
	}
	proto.DeviceInfoListLock.Unlock()

	if canRequest == false {
		return false
	}
	isGT06 := proto.GetDeviceModel(imei) == proto.DM_GT06
	id := proto.NewMsgID()
	svrctx.Get().TcpServerChan <- proto.MakeReplyMsg(imei, true,
		proto.MakeVoiceMonitorReplyMsg(imei, id, params.Phone, isGT06), id)

	return true
}

func  AppQueryLocations(connid uint64, params *proto.QueryLocationsParams) bool {
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

	//
	for index,data := range result.Locations{
		if data.Lat <= 53.33 && data.Lat >= 3.51 && data.Lng <= 135.05 && data.Lng >= 73.33 {
			addr := GetAddress(data.Lat,data.Lng,imei)
			result.Locations[index].Address = addr
		}
	}

	appServerChan <- (&proto.AppMsgData{Cmd: cmdAck,
		UserName: params.UserName,
		AccessToken: params.AccessToken,
		Data: proto.MakeStructToJson(result), ConnID: connid})

	return true
}

func GetAddress(Lat,Lng float64,imei  uint64) string {
	var address string
	var itf interface{}
	var itfconv interface{}
	var weburl= "http://restapi.amap.com/v3/geocode/regeo"
	var index uint64
	var key,amapPos string
	var lng string
	var conn redis.Conn
	conn = redisPool.Get()
	if conn == nil {
		return ""
	}
	defer conn.Close()
	lat := strconv.FormatFloat(Lat, 'f', -1, 64)
	lng = strconv.FormatFloat(Lng, 'f', -1, 64)
	lng += ","
	lng += lat
	reply,errall := conn.Do("hget","latlngconv",lng)
	if errall != nil {
		logging.Log(fmt.Sprintf("hget latlngconv:%s",errall.Error()))
		return ""
	}
	if reply != nil{
		newlng := fmt.Sprintf("%s",reply)
		logging.Log("lng conv:" + newlng)
		reply,err := conn.Do("hget","imeipos",newlng)
		if err != nil {
			logging.Log(fmt.Sprintf("hget imeipos:%s",err.Error()))
			return ""
		}
		if reply != nil{
			address = fmt.Sprintf("%s",reply)
		}
		logging.Log("address conv:" + address)
	}else {
		var converturl = "http://restapi.amap.com/v3/assistant/coordinate/convert?"
		var locations = "locations="

		locations += lng
		converturl += locations
		converturl += "&coordsys=gps&output=json&key="
		rand.Seed(time.Now().UnixNano())
		index = rand.Uint64() % uint64(len(proto.MapKey))
		key = proto.MapKey[index]
		converturl += key
		logging.Log("converturl" + converturl)
		respconv, err := http.Get(converturl)
		bodyconv, err := ioutil.ReadAll(respconv.Body)
		if err != nil {
			fmt.Println("ioutil.ReadAll errr: " + err.Error())
			return ""
		}
		json.Unmarshal(bodyconv, &itfconv)
		convdata := itfconv.(map[string]interface{})
		if convdata == nil {
			return ""
		}
		amapPos = convdata["locations"].(string)

		_, err = conn.Do("hset", "latlngconv", lng,amapPos)
		if err != nil {
			logging.Log(fmt.Sprintf("sadd %d failed:%s", imei, err.Error()))
			return ""
		}

		rand.Seed(time.Now().UnixNano())
		index = rand.Uint64() % uint64(len(proto.MapKey))
		key = proto.MapKey[index]
		resp, err := http.PostForm(weburl, url.Values{"output": {"json"}, "location": {amapPos}, "batch": {"true"},
			"key":{key}, "radius": {"100000"}, "extensions": {"all"}})
		defer resp.Body.Close()
		if err != nil {
			logging.Log("errr: " + err.Error())
			return ""
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logging.Log("ioutil.ReadAll errr: " + err.Error())
			return ""
		}
		json.Unmarshal(body, &itf)
		mapdata := itf.(map[string]interface{})
		if mapdata == nil {
			return ""
		}
		logging.Log(fmt.Sprintf("mapdata:%s",mapdata["regeocodes"]))
		if mapdata["regeocodes"] == nil{
			return ""
		}
		for _, data := range mapdata["regeocodes"].([]interface{}) {
			location := data.(map[string]interface{})
			address = location["formatted_address"].(string)
		}

		_,err = conn.Do("hset","imeipos",amapPos,address)
		if err != nil{
			logging.Log(fmt.Sprintf("hset imeipos failed:%s",err.Error()))
			return ""
		}
	}
	return address
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

func AppLocateImei(connid uint64,msgCmd uint16,params *proto.LocateIMEIParams) bool {
	reqParams := proto.AppRequestLocationParams{}
	reqParams.ConnID = connid
	reqParams.Params = *params

	msg := proto.MsgData{Data:[]byte(proto.MakeStructToJson(&reqParams))}
	msg.Header.Header.ID = proto.NewMsgID()
	msg.Header.Header.Imei = proto.Str2Num(params.Imei, 10)
	msg.Header.Header.Cmd = msgCmd
	msg.Header.Header.Version = proto.MSG_HEADER_VER
	msg.Header.Header.From = proto.MsgFromAppServerToTcpServer

	svrctx.Get().TcpServerChan <- &msg

	return true
}

func GetAdminByDeviceId(imei uint64, recid string) (proto.UserInfo, bool) {
	info := proto.UserInfo{}
	strSQL := fmt.Sprintf("select c.name, c.recid from device d join companies c on d.companyid=c.recid where d.recid='%s' ", recid)
	logging.Log("SQL: " + strSQL)
	rows, err := svrctx.Get().MySQLPool.Query(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%s] query is admin in db failed, %s, %s", imei, recid, err.Error()))
		return  info, false
	}

	defer rows.Close()

	count := 0
	for rows.Next() {
		var Company interface{}
		var CompanyId interface{}
		err := rows.Scan(&Company, &CompanyId)
		if err != nil {
			fmt.Println(fmt.Sprintf("row %d scan err: %s", count, err.Error()))
			return info, false
		}

		cname := parseUint8Array(Company)
		logging.Log(fmt.Sprintf("[%d] company is %s", imei, cname))
		admin, adminId, ok := GetAdminByCompany(cname)
		if ok {
			logging.Log(fmt.Sprintf("[%d] company admin is %s, %s, %s", imei, cname, admin, adminId))
			info.CompanyId = parseUint8Array(CompanyId)
			info.Company = cname
			info.Email = admin
			info.UserId = adminId
		}else{
			return info, false
		}
	}

	return info, true
}

func GetAdminByCompany(companyName string) (string, string, bool) {
	strSQL := fmt.Sprintf("select u.loginname, u.recid from users u join companies c on u.companyid=c.recid where u.grade=1 and c.name='%s' ", companyName)
	logging.Log("SQL: " + strSQL)
	rows, err := svrctx.Get().MySQLPool.Query(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("query admin by company %s in db failed, %s", companyName, err.Error()))
		return  "", "", false
	}

	defer rows.Close()

	count := 0
	for rows.Next() {
		var admin interface{}
		var adminId interface{}
		err := rows.Scan(&admin, &adminId)
		if err != nil {
			fmt.Println(fmt.Sprintf("row %d scan err: %s", count, err.Error()))
			return  "", "", false
		}

		return parseUint8Array(admin), parseUint8Array(adminId), true
	}

	return "", "", false
}

func parseUint8Array(data interface{}) string {
	if data == nil {
		return ""
	}

	return string([]byte(data.([]uint8)))
}

func checkOwnedDevices(AccessToken string)  []string {
	var url string
	if !svrctx.Get().IsUseAliYun {
		url = "https://watch.gatorcn.com/web/index.php?r=app/service/devices&access-token=" + AccessToken
		if svrctx.Get().IsDebugLocal {
			url = "https://watch.gatorcn.com/web/index.php?r=app/service/devices&access-token=" + AccessToken
		}
	}else {
		url = "http://120.25.214.188/tracker/web/index.php?r=app/service/devices&access-token=" + AccessToken
		if svrctx.Get().IsDebugLocal {
			url = "http://120.25.214.188/tracker/web/index.php?r=app/service/devices&access-token=" + AccessToken
		}
	}
	//logging.Log("url: " + url)
	tr := &http.Transport{
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Get(url)
	if err != nil {
		logging.Log("get user devices failed, " + err.Error())
		return nil
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log("response has err, " + err.Error())
		return nil
	}

	var itf interface{}
	err = json.Unmarshal(body, &itf)
	if err != nil {
		logging.Log("parse login response as json failed, " + err.Error())
		return nil
	}

	userDevicesData := itf.(map[string]interface{})
	if userDevicesData == nil {
		return nil
	}

	status := userDevicesData["status"].(float64)
	if status != 0  {
		return nil
	}

	var imeiList []string
	devices := userDevicesData["devices"]
	if devices != nil {
		for _, d := range devices.([]interface{}) {
			device := d.(map[string]interface{})
			if device == nil || device["IMEI"] == nil {
				continue
			}

			imeiList = append(imeiList, device["IMEI"].(string))
		}
	}

	return imeiList
}

func SaveImages(p string,src image.Image) error{
	f,err := os.OpenFile(p,os.O_SYNC | os.O_RDWR | os.O_CREATE,0666)
	if err != nil{
		return err
	}
	defer f.Close()
	ext := filepath.Ext(p)

	if strings.EqualFold(ext,".jpg") || strings.EqualFold(ext,".jpg") {
		err = jpeg.Encode(f, src, &jpeg.Options{Quality: 80})
	}else if strings.EqualFold(ext, ".png"){
		err = png.Encode(f,src)
	}else if strings.EqualFold(ext,"gif"){
		err = gif.Encode(f,src, &gif.Options{NumColors: 256})
	}

	return err
}
