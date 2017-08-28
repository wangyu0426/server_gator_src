package appserver

import (
	"../logging"
	"../svrctx"
	"../models"
	"../proto"

	"gopkg.in/kataras/iris.v6"
	"gopkg.in/kataras/iris.v6/adaptors/httprouter"
	"gopkg.in/kataras/iris.v6/adaptors/websocket"
	"fmt"
	"os"
	"encoding/json"
	"strconv"
	"net/http"
	"io/ioutil"
	"strings"
)

var addConnChan chan *AppConnection
var delConnChan chan *AppConnection
var AppClientTable = map[uint64]map[string]map[string]*AppConnection{}
var FenceIndex = uint64(0)

var appServerChan chan *proto.AppMsgData
var addDeviceManagerChan = make(chan *AddDeviceChanCtx, 1024 * 2)

type AddDeviceChanCtx struct {
	cmd string
	params *proto.DeviceAddParams
	c *AppConnection
}

func init() {
	logging.Log("appserver init")
	models.PrintSelf()

	addConnChan = make(chan *AppConnection, 1024)
	delConnChan = make(chan *AppConnection, 1024)

	//a := StructTest2{"test",StructTest{"123", "456", "789", "abc"}}
	//b, _ := json.Marshal(a)
	//fmt.Println(string(b))

	//a := "{\"Username\":\"123\",\"Password\":\"456\"}"
	//var test StructTest
	//json.Unmarshal([]byte(a), &test)
	//fmt.Println(test)
	//b, _ := json.Marshal(test)
	//fmt.Println(string(b))
	//a := []byte("abcde")
	//for i, letter := range a  {
	//	fmt.Println(i, letter)
	//}
	//fmt.Println("rand verify code: ", makerRandomVerifyCode())
	//os.Exit(0)

}

func LocalAPIServerRunLoop(serverCtx *svrctx.ServerContext) {
	defer logging.PanicLogAndExit("LocalAPIServerRunLoop")

	app := iris.New()
	//app.Adapt(iris.DevLogger())
	app.Adapt(httprouter.New())

	app.Get("/api/get-locations", GetLocationsByURL)

	app.Get("GetWatchData", GetLocationsByURL)
	//GetWatchData?systemno={systemno}&datatype=2&callback={callback}&end={end}&start={start}

	addr := fmt.Sprintf("%s:%d", serverCtx.LocalAPIBindAddr, serverCtx.LocalAPIPort)
	logging.Log("LocalAPIServer listen:  " + addr)
	app.Listen(addr)

}

func AppServerRunLoop(serverCtx *svrctx.ServerContext)  {
	defer logging.PanicLogAndExit("AppServerRunLoop")

	appServerChan = serverCtx.AppServerChan
	app := iris.New()
	//app.Adapt(iris.DevLogger())
	app.Adapt(httprouter.New())
	ws := websocket.New(websocket.Config{Endpoint: "/wsapi"})
	app.Adapt(ws)
	ws.OnConnection(OnClientConnected)

	app.StaticWeb(svrctx.Get().HttpStaticURL, svrctx.Get().HttpStaticDir)

	app.Post("/api/gator3-version", GetAppVersionOnline)
	app.Post(svrctx.Get().HttpUploadURL, func(ctx *iris.Context) {
		result := proto.HttpAPIResult{
			ErrCode: 0,
			ErrMsg: "",
			Imei: "0",
		}

		imei := ctx.FormValue("imei")
		username := ctx.FormValue("username")
		fieldname := ctx.FormValue("fieldname")
		uploadType := ctx.FormValue("type")
		fmt.Println(uploadType)

		result.Imei = imei

		//_,fileInfo, err1 := ctx.FormFile(uploadType)
		//if err1 != nil {
		//	result.ErrCode = 500
		//	ctx.JSON(500, result)
		//	return
		//}
		//
		//file,err2 :=fileInfo.Open()
		//if err2 != nil {
		//	result.ErrCode = 500
		//	result.ErrMsg = "server failed to open the uploaded  file"
		//	ctx.JSON(500, result)
		//	return
		//}
		//
		//defer file.Close()
		//fileData, err3 :=ioutil.ReadAll(file)
		//if err3 != nil {
		//	result.ErrCode = 500
		//	result.ErrMsg = "server failed to read the data of  the uploaded  file"
		//	ctx.JSON(500, result)
		//	return
		//}

		if uploadType != "minichat" {
			fileData, err := base64Decode([]byte(ctx.FormValue(uploadType)))
			if err != nil {
				result.ErrCode = 500
				result.ErrMsg = "upload bad data"
				ctx.JSON(500, result)
				return
			}

			os.MkdirAll(svrctx.Get().HttpStaticDir + svrctx.Get().HttpStaticAvatarDir + imei, 0755)
			fileName, timestampString := "", proto.MakeTimestampIdString()

			if uploadType == "contactAvatar" {
				contactIndex := ctx.FormValue("index")
				fileName += "contact_" + contactIndex + "_"
			}

			fileName += timestampString + ".jpg"

			uploadTypeDir := svrctx.Get().HttpStaticAvatarDir

			err4 := ioutil.WriteFile(svrctx.Get().HttpStaticDir + uploadTypeDir + imei + "/" + fileName, fileData, 0666)
			if err4 != nil {
				result.ErrCode = 500
				result.ErrMsg = "server failed to save the uploaded  file"
				ctx.JSON(500, result)
				return
			}

			settings := make([]proto.SettingParam, 1)
			if uploadType == "contactAvatar" {
				settings[0].Index = int(proto.Str2Num(ctx.FormValue("index"), 10))
			}
			settings[0].FieldName = fieldname
			settings[0].NewValue = svrctx.Get().HttpStaticAvatarDir +  imei + "/"  +  fileName

			ret := SaveDeviceSettings(proto.Str2Num(imei, 10), settings, nil)
			if ret {
				if uploadType == "contactAvatar" {
					photoInfo := proto.PhotoSettingInfo{}
					photoInfo.CreateTime = proto.NewMsgID()
					photoInfo.Member.Phone = ctx.FormValue("phone")
					photoInfo.ContentType = proto.ChatContentPhoto
					photoInfo.Content = fileName//proto.MakeTimestampIdString()
					photoInfo.MsgId = proto.Str2Num(ctx.FormValue("msgId"), 10)
					svrctx.AddPendingPhotoData(proto.Str2Num(imei, 10), photoInfo)
				}

				result.Data = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName, svrctx.Get().WSPort,svrctx.Get().HttpStaticURL +
					svrctx.Get().HttpStaticAvatarDir +  imei + "/" +  fileName)
				fmt.Println(fileName)
				ctx.JSON(200, result)
			}else{
				result.ErrCode = 500
				result.ErrMsg = "server failed to update the device setting in db"
				ctx.JSON(500, result)
				return
			}
		}else if uploadType == "minichat" {
			imeiUint64 := proto.Str2Num(imei, 10)
			phone := ctx.FormValue("phone")

			if svrctx.IsPhoneNumberInFamilyList(imeiUint64, phone) == false {
				result.ErrCode = 500
				result.ErrMsg = fmt.Sprintf("phone number %s is not in the family phone list of %d", phone, imeiUint64)
				ctx.JSON(500, result)
				return
			}

			_,fileInfo, err5 := ctx.FormFile(uploadType)
			if err5 != nil {
				result.ErrCode = 500
				result.ErrMsg = "the uploaded type is not a file"
				ctx.JSON(500, result)
				return
			}

			file,err6 :=fileInfo.Open()
			if err6 != nil {
				result.ErrCode = 500
				result.ErrMsg = "server failed to open the uploaded  file"
				ctx.JSON(500, result)
				return
			}

			defer file.Close()
			fileData, err7 :=ioutil.ReadAll(file)
			if err7 != nil {
				result.ErrCode = 500
				result.ErrMsg = "server failed to read the data of  the uploaded  file"
				ctx.JSON(500, result)
				return
			}

			if len(fileData) == 0 {
				result.ErrCode = 500
				result.ErrMsg = "no content in the uploaded  file (size is 0)"
				ctx.JSON(500, result)
				return
			}

			os.MkdirAll(svrctx.Get().HttpStaticDir + svrctx.Get().HttpStaticMinichatDir + imei, 0755)
			fileName, timestampString := "", proto.MakeTimestampIdString()
			fileName += timestampString + ".aac"

			uploadTypeDir := svrctx.Get().HttpStaticMinichatDir
			filePath := svrctx.Get().HttpStaticDir + uploadTypeDir +  imei + "/" + fileName
			fileAmrPath := svrctx.Get().HttpStaticDir + uploadTypeDir +  imei + "/" +  timestampString + ".amr"

				err8 := ioutil.WriteFile(filePath, fileData, 0666)
			if err8 != nil {
				result.ErrCode = 500
				result.ErrMsg = "server failed to save the uploaded  file"
				ctx.JSON(500, result)
				return
			}

			chat := proto.ChatInfo{}
			chat.CreateTime = proto.NewMsgID()
			chat.Imei = imeiUint64
			chat.Sender = phone
			chat.SenderType = 1
			chat.SenderUser = username
			chat.VoiceMilisecs = int(proto.Str2Num(ctx.FormValue("duration"), 10))
			chat.ContentType = proto.ChatContentVoice
			chat.Content = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName, svrctx.Get().WSPort,svrctx.Get().HttpStaticURL +
				svrctx.Get().HttpStaticMinichatDir +  imei + "/" +  fileName)//timestampString
			chat.FileID = proto.Str2Num(timestampString, 10)
			if len(ctx.FormValue("timestamp")) == 14 {
				chat.DateTime = proto.Str2Num( ctx.FormValue("timestamp")[2:14], 10)
			}else{
				chat.DateTime = proto.Str2Num(timestampString[0:12], 10)
			}


			args := fmt.Sprintf("-i %s -acodec amr_nb -ab 3.2k -ar 8000 %s", filePath, fileAmrPath)
			err9, _ := proto.ExecCmd("ffmpeg",  strings.Split(args, " ")...)
			if err9 != nil {
				logging.Log(fmt.Sprintf("[%d] ffmpeg %s failed, %s", imeiUint64, args, err9.Error()))
			}

			logging.Log(fmt.Sprintf("[%d] app upload chat: %s", imeiUint64, proto.MakeStructToJson(chat)))
			svrctx.AddChatData(imeiUint64, chat)
			proto.AddChatForApp(chat)

			//这里应该通知APP，微聊列表有新的项
			proto.NotifyAppWithNewMinichat("", imeiUint64, appServerChan, chat)
			//result.Data = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName, svrctx.Get().WSPort,svrctx.Get().HttpStaticURL +
				//svrctx.Get().HttpStaticMinichatDir +  imei + "/" +  fileName)

			fmt.Println(fileName)
			ctx.JSON(200, result)
			return
		}else{
		}
	})

	go AddDeviceManagerLoop()

	//负责管理连接、并且回发数据到app端
	go func() {
		defer logging.PanicLogAndExit("appserver.go: 274")

		for  {
			select {
			case c := <- delConnChan:
				if c == nil {
					logging.Log("delete a nil app connection, exit")
					os.Exit(1)
				}

				for _, imei := range c.imeis {
					subTable, ok := AppClientTable[imei]
					if ok {
						connList, ok := subTable[c.user.GetAccessToken()]
						if ok {
							if connList != nil && len(connList) > 0 {
								for connID, connItem := range connList {
									if c == connItem {
										delete(connList, connID)
										logging.Log("connection deleted from AppClientTable," + connID + ", " + (*c.conn).ID())
									}
								}
							}

							if len(connList) == 0{
								delete(subTable, c.user.GetAccessToken())
							}
						} else {
							logging.Log("will delete connection from appClientTable, but connection not found")
						}

						if len(subTable) == 0 {
							delete(AppClientTable, imei)
						}
					}
				}
				logging.Log("after delete conn:" + fmt.Sprint(AppClientTable))
			case c := <- addConnChan:
				if c == nil {
					logging.Log("add a nil app connection, exit")
					os.Exit(1)
				}

				for _, imei := range c.imeis {
					_, ok := AppClientTable[imei]
					if !ok {   //不存在，则首先创建新表，然后加入
						AppClientTable[imei] = map[string]map[string]*AppConnection{}
						AppClientTable[imei][c.user.GetAccessToken()] = map[string]*AppConnection{}
					}else{
						_, ok2 := AppClientTable[imei][c.user.GetAccessToken()]
						if !ok2 {
							AppClientTable[imei][c.user.GetAccessToken()] = map[string]*AppConnection{}
						}
					}

					AppClientTable[imei][c.user.GetAccessToken()][(*c.conn).ID()] = c
				}

				logging.Log("after add conn:" + fmt.Sprint(AppClientTable))
			case msg := <- appServerChan:  //回发数据到APP
				if msg == nil {
					logging.Log("send  a nil msg data to app , exit")
					os.Exit(1)
				}

				logging.Log("send data to app:" + fmt.Sprint(string(msg.Cmd)))
				//如果是login请求，则与设备无关,无须查表，直接发送数据到APP客户端
				if msg.Cmd == proto.LoginAckCmdName ||
					msg.Cmd == proto.RegisterAckCmdName ||
					msg.Cmd == proto.ResetPasswordAckCmdName ||
					msg.Cmd == proto.ModifyPasswordAckCmdName ||
					msg.Cmd == proto.FeedbackAckCmdName ||
					(msg.Cmd == proto.HearbeatAckCmdName && msg.Conn != nil) ||
					msg.Conn == proto.VerifyCodeAckCmdName ||
					msg.Cmd == proto.GetDeviceByImeiAckCmdName ||
					msg.Cmd == proto.AddDeviceAckCmdName ||
					msg.Cmd == proto.DeleteDeviceAckCmdName ||
					msg.Cmd == proto.GetLocationsAckCmdName ||
					msg.Cmd == proto.GetAlarmsAckCmdName ||
					msg.Cmd == proto.DeleteAlarmsAckCmdName {

					if msg.Cmd == proto.HearbeatAckCmdName {
						getAppClientsByImei(msg)
					}
					c := msg.Conn.(*AppConnection)
					data, err := json.Marshal(&msg)
					err = (*c.conn).EmitMessage(data)
					if err != nil {
						logging.Log("send msg to app failed, " + err.Error())
					}else{
						logging.Log("send msg: " + fmt.Sprint(msg))
					}
					break
				}else if msg.Cmd == proto.DeviceLocateNowAckCmdName {
					params := proto.AppRequestTcpConnParams{}
					err := json.Unmarshal([]byte(msg.Data), &params)
					if err != nil {
						logging.Log("parse json data for locate now ack failed, " + err.Error() + ", " + msg.Data)
						break
					}

					connList := getAppClientsByAccessToken(msg.Imei,  params.Params.AccessToken)
					if connList != nil && len(connList) > 0 {
						for _, c := range connList {
							if c != nil {
								result := proto.HttpAPIResult{ErrCode: 1, Data: proto.DeviceLocateNowSms}
								appMsg := proto.AppMsgData{Cmd: proto.DeviceLocateNowAckCmdName,
									Imei: msg.Imei, Data: proto.MakeStructToJson(&result)}
								err =(*c.conn).EmitMessage([]byte(proto.MakeStructToJson(&appMsg)))
								if err != nil {
									logging.Log("send msg to app failed, " + err.Error())
								}else{
									logging.Log("send msg: " + fmt.Sprint(msg, c))
								}
							}
						}
					}

					break
				}

				//从表中找出所有跟此IMEI关联的APP客户端，并将数据发送至每一个APP客户端
				//如果APP客户端已经登陆过，但服务器上没有该客户端关注的手表的数据，
				//那么收到该APP客户端的第一个请求，应该首先读取该客户端关注的手表数据

				subTable := getAppClientsByImei(msg)
				if subTable != nil {
					logging.Log(fmt.Sprint(msg.Imei, "app clients: ", subTable))
					for _, connList := range subTable{
						logging.Log(fmt.Sprint(msg.Imei, "app connlist: ", connList))
						if connList != nil && len(connList) > 0 {
							logging.Log(fmt.Sprint(msg.Imei, "app connlist len: ", len(connList)))
							for _, c := range connList {
								logging.Log(fmt.Sprint(msg.Imei, "app conn: ", c))
								if c != nil {
									data, err := json.Marshal(&msg)
									err = (*c.conn).EmitMessage(data)
									if err != nil {
										logging.Log("send msg to app failed, " + err.Error())
									} else {
										logging.Log("send msg: " + fmt.Sprint(msg, c))
									}
								}
							}
						}
					}
				}
			}
		}
	}()

	app.Listen(fmt.Sprintf("%s:%d", serverCtx.BindAddr, serverCtx.WSPort))
}

func OnClientConnected(conn websocket.Connection)  {
	FenceIndex++
	logging.Log("websocket connected: " + conn.Context().RemoteAddr() + "Client ID: " + conn.ID())
	connection := newAppConn(&conn, FenceIndex)
	conn.SetValue("ctx", connection)

	//for reading, 负责处理APP请求的业务逻辑
	go func(c *AppConnection) {
		defer logging.PanicLogAndExit("appserver.go: 407")

		for  {
			select {
			case <- c.closeChan:
				return
			case data := <- c.requestChan:
				HandleAppRequest(c, appServerChan, data)
			}
		}
	}(connection)

	conn.OnMessage(func(data []byte) {
		logging.Log("recv from client: " + string(data))
		c := conn.GetValue("ctx").(*AppConnection)
		c.requestChan <- data

		//c.EmitMessage([]byte("Message from: " + c.ID() + "-> " + message)) // broadcast to all clients except this
		//c.EmitMessage([]byte("Me: " + message))                                                    // writes to itself
	})

	conn.OnDisconnect(func() {
		logging.Log("websocket disconnected: " + conn.Context().RemoteAddr() + "Client ID: " + conn.ID())
		c := conn.GetValue("ctx").(*AppConnection)
		c.SetClosed()
		close(c.requestChan)
		close(c.responseChan)
		close(c.closeChan)
		(*c.conn).Disconnect()

		FenceIndex++
		delConnChan <- c
	})
}

func getAppClientsByImei(msg *proto.AppMsgData)  map[string]map[string]*AppConnection {
	subTable, ok := AppClientTable[msg.Imei]
	if ok {
		return subTable
	}else {
		if msg.Conn == nil || len(msg.AccessToken) == 0 {
			return nil
		}

		url := "http://127.0.0.1/web/index.php?r=app/service/devices&access-token=" + msg.AccessToken
		if svrctx.Get().IsDebugLocal {
			url = "http://120.25.214.188/web/index.php?r=app/service/devices&access-token=" + msg.AccessToken
		}

		logging.Log("url: " + url)
		resp, err := http.Get(url)
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

		//logging.Log("userDevicesData: " + fmt.Sprint(userDevicesData))
		status := userDevicesData["status"].(float64)
		devices := userDevicesData["devices"]

		//logging.Log("status: " + fmt.Sprint(status))
		//logging.Log("accessToken: " + fmt.Sprint(accessToken))
		//logging.Log("devices: " + fmt.Sprint(devices))
		if status == 0 && devices != nil {
			c := msg.Conn.(*AppConnection)
			c.user.AccessToken = msg.AccessToken
			c.user.Logined = true
			c.user.Name = msg.UserName

			c.imeis = []uint64{}

			for _, d := range devices.([]interface{}) {
				device := d.(map[string]interface{})
				imei, _ := strconv.ParseUint(device["IMEI"].(string), 0, 0)
				logging.Log("device: " + fmt.Sprint(imei))
				c.imeis = append(c.imeis, imei)
			}

			if msg.Cmd == proto.AddDeviceOKAckCmdName {
				c.imeis = append(c.imeis, msg.Imei)
			}

			for _, imei := range c.imeis {
				_, ok := AppClientTable[imei]
				if !ok {   //不存在，则首先创建新表，然后加入
					AppClientTable[imei] = map[string]map[string]*AppConnection{}
					AppClientTable[imei][c.user.GetAccessToken()] = map[string]*AppConnection{}
				}else{
					_, ok2 := AppClientTable[imei][c.user.GetAccessToken()]
					if !ok2 {
						AppClientTable[imei][c.user.GetAccessToken()] = map[string]*AppConnection{}
					}
				}

				AppClientTable[imei][c.user.GetAccessToken()][(*c.conn).ID()] = c
			}

			logging.Log("after add conn:" + fmt.Sprint(AppClientTable))

			subTable, ok := AppClientTable[msg.Imei]
			if ok {
				return subTable
			}else {
				return nil
			}
		}

		return nil
	}
}

func getAppClientsByAccessToken(imei uint64, accessToken string)  map[string]*AppConnection {
	if imei == 0 || len(accessToken) == 0 {
		logging.Log(fmt.Sprintf("bad input params for getAppClientsByAccessToken: %d, %s", imei, accessToken))
		return nil
	}

	subTable, ok := AppClientTable[imei]
	if ok && subTable != nil {
		accessTokenConn, ok2 := subTable[accessToken]
		if ok2 && accessTokenConn != nil {
			return accessTokenConn
		}
	}

	logging.Log(fmt.Sprintf("getAppClientsByAccessToken not found app connection: %d, %s", imei, accessToken))
	return nil
}

func GetLocationsByURL(ctx *iris.Context) {
	systemno := proto.Str2Num(ctx.FormValue("systemno"), 10)
	proto.SystemNo2ImeiMapLock.Lock()
	imei, ok := proto.SystemNo2ImeiMap[systemno]
	proto.SystemNo2ImeiMapLock.Unlock()
	if ok == false || imei == 0 {
		logging.Log(fmt.Sprintf("bad imei for systemno %d ", systemno))
		dataResult := proto.PhpQueryLocationsResult{Result: -1, ResultStr: "",  Systemno: systemno}
		ctx.JSON(200, dataResult)
		return
	}

	datatype := ctx.FormValue("datatype")
	var locations *[]proto.LocationData
	if datatype == "1"{
		dataResult := proto.PhpQueryLocationsResult{Result: 0, ResultStr: "",  Systemno: systemno}
		data := svrctx.GetDeviceData(imei, svrctx.Get().PGPool)
		dataResult.Data = append(dataResult.Data, data.DataTime)
		dataResult.Data = append(dataResult.Data, uint64(data.Lat * 1000000))
		dataResult.Data = append(dataResult.Data, uint64(data.Lng * 1000000))
		dataResult.Data = append(dataResult.Data, data.Steps)
		dataResult.Data = append(dataResult.Data, data.Battery)
		dataResult.Data = append(dataResult.Data, data.AlarmType)
		dataResult.Data = append(dataResult.Data, data.ReadFlag)
		dataResult.Data = append(dataResult.Data, data.LocateType)
		dataResult.Data = append(dataResult.Data, data.ZoneName)
		dataResult.Data = append(dataResult.Data, data.Accracy)
		ctx.JSON(200, dataResult)
	}else if(datatype == "2"){
		dataResult := proto.PhpQueryLocationsResult{Result: 0, ResultStr: "",  Systemno: systemno}
		beginTime := proto.Str2Num("20" + ctx.FormValue("start"), 10)
		endTime := proto.Str2Num("20" + ctx.FormValue("end"), 10)

		locations = svrctx.QueryLocations(imei, svrctx.Get().PGPool, beginTime, endTime, true, false)
		if locations != nil && len(*locations) > 0 {
			for _, data := range *locations {
				dataItem := []interface{}{}
				dataItem = append(dataItem, data.DataTime)
				dataItem = append(dataItem, uint64(data.Lat * 1000000))
				dataItem = append(dataItem, uint64(data.Lng * 1000000))
				dataItem = append(dataItem, data.Steps)
				dataItem = append(dataItem, data.Battery)
				dataItem = append(dataItem, data.AlarmType)
				dataItem = append(dataItem, data.ReadFlag)
				dataItem = append(dataItem, data.LocateType)
				dataItem = append(dataItem, data.ZoneName)
				dataItem = append(dataItem, data.Accracy)

				dataResult.Data = append(dataResult.Data, dataItem)
			}
		}else{
			dataResult.Data = []interface{}{}
		}

		ctx.JSON(200, dataResult)
	}
}

func GetAppVersionOnline(ctx *iris.Context)  {
	result := proto.HttpQueryAppVersionResult{}
	result.Status = -1

	platform := ctx.FormValue("platform")
	reqUrl := ""
	if platform=="android" {
		reqUrl = svrctx.Get().AndroidAppURL
	}else if platform=="ios" {
		reqUrl = svrctx.Get().IOSAppURL
	}else{
		logging.Log("get app verion from online store failed, " + "bad params")
		ctx.JSON(500, proto.MakeStructToJson(&result))
		return
	}

	resp, err := http.Get(reqUrl)
	if err != nil {
		logging.Log("get app verion from online store failed, " + err.Error() + " , " + reqUrl)
		ctx.JSON(500, proto.MakeStructToJson(&result))
		return
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log("get app verion from online store response has err, " + err.Error() + " , " + reqUrl)
		ctx.JSON(500, proto.MakeStructToJson(&result))
		return
	}

	if platform=="android" {
		pos := strings.Index(string(body), "softwareVersion")
		if pos < 0 {
			logging.Log("get app verion from online store response has err, " + err.Error() + " , " + reqUrl)
			ctx.JSON(500, proto.MakeStructToJson(&result))
			return
		}

		if len(body[pos:]) < 30 {
			logging.Log("get app verion from online store response has err, " + err.Error() + " , " + reqUrl)
			ctx.JSON(500, proto.MakeStructToJson(&result))
			return
		}

		strVer := string(body[pos: pos + 30])
		arr := strings.Split(strVer, " ")
		if len(arr) < 2 {
			logging.Log("get app verion from online store response has err, " + err.Error() + " , " + reqUrl)
			ctx.JSON(500, proto.MakeStructToJson(&result))
			return
		}

		result.Version = strings.Split(arr[1], ".")
		result.AppUrl = svrctx.Get().AndroidAppURL
	}else{
		info := proto.IOSAppInfo{}
		err = json.Unmarshal(body, &info)
		if err != nil {
			logging.Log("parse  response as json failed, " + err.Error() +
				", response: " + string(body))
			ctx.JSON(500, proto.MakeStructToJson(&result))
			return
		}

		if len(info.Results) < 3 {
			logging.Log("get app verion from online store response has err, " + err.Error() + " , " + reqUrl)
			ctx.JSON(500, proto.MakeStructToJson(&result))
			return
		}

		result.Version = strings.Split(info.Results[2].Version, ".")
		result.AppUrl = svrctx.Get().IOSAppURL
	}

	ctx.JSON(200, proto.MakeStructToJson(&result))
}