package appserver

import (
	"../logging"
	"../svrctx"
	"../models"
	"../proto"
	"fmt"
	"os"
	"encoding/json"
	"net/http"
	"golang.org/x/net/websocket"
	"io/ioutil"
	"strings"
	"sync"
	"time"
)

// go get github.com/golang/net
//ln -s  /home/work/go/src/github.com/golang/net   /home/work/go/src/golang.org/x/net
//go install golang.org/x/net/websocket

type DeviceConnInfo struct {
	imei uint64
	username string
}

type ConnAccessTokenInfo struct {
	connID uint64
	username string
	accessToken string
}

type UpdateAppConnInfo struct {
	connID uint64
	usernameOld string
	usernameNew string
	conn *AppConnection
}

var addDeviceConnChan chan DeviceConnInfo
var delDeviceConnChan chan DeviceConnInfo

var addConnAccessTokenChan chan ConnAccessTokenInfo

var addConnChan chan *AppConnection
var updateConnChan chan *UpdateAppConnInfo
var delConnChan chan *AppConnection

var AppDeviceTable = map[uint64]map[string]bool{}
var AppConnTable = map[string]map[uint64]*AppConnection{}

var AccessTokenTableLock = &sync.RWMutex{}
var AccessTokenTable = map[string]bool{}

var appServerChan chan *proto.AppMsgData
var addDeviceManagerChan = make(chan *AddDeviceChanCtx, 1024 * 20)

type AddDeviceChanCtx struct {
	cmd string
	params *proto.DeviceAddParams
	connid uint64
}

func init() {
	logging.Log("appserver init")
	models.PrintSelf()

	addConnChan = make(chan *AppConnection, 10240)
	updateConnChan = make(chan *UpdateAppConnInfo, 10240)
	delConnChan = make(chan *AppConnection, 10240)

	addDeviceConnChan = make(chan DeviceConnInfo, 10240)
	delDeviceConnChan = make(chan DeviceConnInfo, 10240)

	addConnAccessTokenChan  = make(chan ConnAccessTokenInfo, 10240)
}

func LocalAPIServerRunLoop(serverCtx *svrctx.ServerContext) {
	defer logging.PanicLogAndExit("LocalAPIServerRunLoop")

	http.HandleFunc("/api/get-locations", GetLocationsByURL)
	http.HandleFunc("/GetWatchData", GetLocationsByURL)
	addr := fmt.Sprintf("%s:%d", serverCtx.LocalAPIBindAddr, serverCtx.LocalAPIPort)
	logging.Log("LocalAPIServer listen:  " + addr)
	http.ListenAndServe(addr, nil)
}

func AppServerRunLoop(serverCtx *svrctx.ServerContext)  {
	defer logging.PanicLogAndExit("AppServerRunLoop")

	appServerChan = serverCtx.AppServerChan

	//for managing connection, 对内负责管理APP连接对象，对外为TCP server提供通信接口
	go AppConnManagerLoop()

	go AddDeviceManagerLoop()

	http.Handle("/",  http.FileServer(http.Dir(svrctx.Get().HttpStaticDir)))
	http.HandleFunc("/api/gator3-version", GetAppVersionOnline)
	http.HandleFunc(svrctx.Get().HttpUploadURL, HandleUploadFile)
	http.Handle("/wsapi", websocket.Handler(OnClientConnected))

	//http.HandleFunc("/api/cmd", HandleApiCmd)
	//http.Handle(svrctx.Get().HttpStaticURL, http.FileServer(http.Dir(svrctx.Get().HttpStaticDir)))

	mux := http.NewServeMux()
	mux.Handle("/",  http.FileServer(http.Dir(svrctx.Get().HttpStaticDir)))
	mux.HandleFunc("/api/gator3-version", GetAppVersionOnline)

	go http.ListenAndServe(fmt.Sprintf("%s:%d", serverCtx.BindAddr, serverCtx.WSPort), mux)

	//go http.ListenAndServe(fmt.Sprintf("%s:%d", serverCtx.BindAddr, serverCtx.WSPort), nil)

	fmt.Println(http.ListenAndServeTLS(fmt.Sprintf("%s:%d", serverCtx.BindAddr, serverCtx.WSSPort),
		"/home/ec2-user/work/codes/https_test/watch.gatorcn.com/watch.gatorcn.com.cer",
		"/home/ec2-user/work/codes/https_test/watch.gatorcn.com/watch.gatorcn.com.key",nil))
}

//for managing connection, 对内负责管理APP连接对象，对外为TCP server提供通信接口
func AppConnManagerLoop() {
	defer logging.PanicLogAndExit("AppConnManagerLoop")

	for  {
		select {
		case c := <- addConnChan:
			if c == nil {
				logging.Log("add a nil app connection")
				os.Exit(-1)
			}

			AddNewAppConn(c)

		case info := <- updateConnChan:
			logging.Log(fmt.Sprintf("126: update an app connection:%v", (*info)))

			if info.conn == nil || info.connID == 0 || info.usernameOld == "" || info.usernameNew == ""{
				logging.Log("update a nil app connection")
				os.Exit(-1)
			}

			UpdateAppConn(info)

		case c := <- delConnChan:
			if c == nil {
				logging.Log("delete a nil app connection")
				os.Exit(-1)
			}

			RemoveAppConn(c)

		case info := <- addConnAccessTokenChan:
			if info.connID == 0  || info.username == "" ||   info.accessToken == ""{
				logging.Log("add a nil app connection access token, exit")
				os.Exit(-1)
			}

			AddAppConnAccessToken(info)

		case info := <- addDeviceConnChan:
			if info.imei == 0  ||  info.username == ""{
				logging.Log("add a nil app device connection, exit")
				os.Exit(-1)
			}

			AddAppDevice(info)

		case info := <- delDeviceConnChan:
			if info.imei == 0  ||  info.username == ""{
				logging.Log("delete a nil app device connection")
				os.Exit(-1)
			}

			RemoveAppDevice(info)

		case msg := <- appServerChan:  //转发数据到APP
			if msg == nil {
				logging.Log("send  a nil msg data to app , exit")
				os.Exit(-1)
			}

			//logging.Log("send data to app:" + fmt.Sprint(string(msg.Cmd)))

			if msg.Cmd == proto.DeviceLocateNowAckCmdName {
				params := proto.AppRequestTcpConnParams{}
				err := json.Unmarshal([]byte(msg.Data), &params)
				if err != nil {
					logging.Log("parse json data for locate now ack failed, " + err.Error() + ", " + msg.Data)
					break
				}

				conn := getAppConnByConnID(params.Params.UserName, params.ConnID)
				if conn != nil  {
					result := proto.HttpAPIResult{ErrCode: 1, Data: proto.DeviceLocateNowSms}
					appMsg := proto.AppMsgData{Cmd: proto.DeviceLocateNowAckCmdName,
						Imei: msg.Imei, Data: proto.MakeStructToJson(&result)}
					conn.responseChan <- &appMsg
				}

				break
			}

			//如果connid不为0，则直接发送到这个单独的连接
			if msg.ConnID != 0 {
				conn := getAppConnByConnID(msg.UserName, msg.ConnID)
				if conn != nil  && conn.responseChan != nil {
					conn.responseChan <- msg
				}

				break
			}

			//connid为0，表示群发给所有关注此IMEI的APP客户端
			//从表中找出所有跟此IMEI关联的APP客户端，并将数据发送至每一个APP客户端
			imeiAppUsers, _ := AppDeviceTable[msg.Imei]
			if imeiAppUsers == nil {
				AppDeviceTable[msg.Imei] = getDeviceUserTable(msg.Imei)
				if AppDeviceTable[msg.Imei] == nil {
					AppDeviceTable[msg.Imei] = map[string]bool{}
				}

				imeiAppUsers, _ = AppDeviceTable[msg.Imei]
			}

			if len(imeiAppUsers) >  0 {
				for username, _ := range imeiAppUsers {
					userConnTable := getAppConnsByUserName(username)
					if userConnTable != nil {
						for _, c := range userConnTable {
							if c != nil  && c.responseChan != nil {
								c.responseChan <- msg
							}
						}
					}
				}
			}
		}
	}
}

func HandleUploadFile(w http.ResponseWriter, r *http.Request) {
	result := proto.HttpAPIResult{
		ErrCode: 0,
		ErrMsg: "",
		Imei: "0",
	}

	w.Header().Set("Content-Type", "application/json")

	imei := r.FormValue("imei")
	username := r.FormValue("username")
	fieldname := r.FormValue("fieldname")
	uploadType := r.FormValue("type")
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
		fileData, err := base64Decode([]byte(r.FormValue(uploadType)))
		if err != nil {
			result.ErrCode = 500
			result.ErrMsg = "upload bad data"
			JSON(w, 500, &result)
			return
		}

		os.MkdirAll(svrctx.Get().HttpStaticDir + svrctx.Get().HttpStaticAvatarDir + imei, 0755)
		fileName, timestampString := "", proto.MakeTimestampIdString()

		if uploadType == "contactAvatar" {
			contactIndex := r.FormValue("index")
			fileName += "contact_" + contactIndex + "_"
		}

		fileName += timestampString + ".jpg"

		uploadTypeDir := svrctx.Get().HttpStaticAvatarDir

		err4 := ioutil.WriteFile(svrctx.Get().HttpStaticDir + uploadTypeDir + imei + "/" + fileName, fileData, 0666)
		if err4 != nil {
			result.ErrCode = 500
			result.ErrMsg = "server failed to save the uploaded  file"
			JSON(w, 500, &result)
			return
		}

		settings := make([]proto.SettingParam, 1)
		if uploadType == "contactAvatar" {
			settings[0].Index = int(proto.Str2Num(r.FormValue("index"), 10))
		}
		settings[0].FieldName = fieldname
		settings[0].NewValue = svrctx.Get().HttpStaticAvatarDir + imei + "/" + fileName

		ret := SaveDeviceSettings(proto.Str2Num(imei, 10), settings, nil)
		if ret {
			if uploadType == "contactAvatar" {
				photoInfo := proto.PhotoSettingInfo{}
				photoInfo.CreateTime = proto.NewMsgID()
				photoInfo.Member.Phone = r.FormValue("phone")
				photoInfo.ContentType = proto.ChatContentPhoto
				photoInfo.Content = fileName//proto.MakeTimestampIdString()
				photoInfo.MsgId = proto.Str2Num(r.FormValue("msgId"), 10)
				svrctx.AddPendingPhotoData(proto.Str2Num(imei, 10), photoInfo)
			}

			result.Data = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName, svrctx.Get().WSPort, svrctx.Get().HttpStaticURL +
				svrctx.Get().HttpStaticAvatarDir + imei + "/" + fileName)
			fmt.Println(fileName)
			JSON(w, 200, &result)
		} else {
			result.ErrCode = 500
			result.ErrMsg = "server failed to update the device setting in db"
			JSON(w, 500, &result)
			return
		}
	} else if uploadType == "minichat" {
		imeiUint64 := proto.Str2Num(imei, 10)
		phone := r.FormValue("phone")

		//if svrctx.IsPhoneNumberInFamilyList(imeiUint64, phone) == false {
		//	result.ErrCode = 500
		//	result.ErrMsg = fmt.Sprintf("phone number %s is not in the family phone list", phone)
		//	result.Data = ""
		//	JSON(w, 500, &result)
		//	return
		//}

		_, fileInfo, err5 := r.FormFile(uploadType)
		if err5 != nil {
			result.ErrCode = 500
			result.ErrMsg = "the uploaded type is not a file"
			JSON(w, 500, &result)
			return
		}

		file, err6 := fileInfo.Open()
		if err6 != nil {
			result.ErrCode = 500
			result.ErrMsg = "server failed to open the uploaded  file"
			JSON(w, 500, &result)
			return
		}

		defer file.Close()
		fileData, err7 := ioutil.ReadAll(file)
		if err7 != nil {
			result.ErrCode = 500
			result.ErrMsg = "server failed to read the data of  the uploaded  file"
			JSON(w, 500, &result)
			return
		}

		if len(fileData) == 0 {
			result.ErrCode = 500
			result.ErrMsg = "no content in the uploaded  file (size is 0)"
			JSON(w, 500, &result)
			return
		}

		os.MkdirAll(svrctx.Get().HttpStaticDir + svrctx.Get().HttpStaticMinichatDir + imei, 0755)
		fileName, timestampString := "", proto.MakeTimestampIdString()
		fileName += timestampString + ".aac"

		uploadTypeDir := svrctx.Get().HttpStaticMinichatDir
		filePath := svrctx.Get().HttpStaticDir + uploadTypeDir + imei + "/" + fileName
		fileAmrPath := svrctx.Get().HttpStaticDir + uploadTypeDir + imei + "/" + timestampString + ".amr"

		err8 := ioutil.WriteFile(filePath, fileData, 0666)
		if err8 != nil {
			result.ErrCode = 500
			result.ErrMsg = "server failed to save the uploaded  file"
			JSON(w, 500, &result)
			return
		}

		chat := proto.ChatInfo{}
		chat.CreateTime = proto.NewMsgID()
		chat.Imei = imeiUint64
		chat.Sender = phone
		chat.SenderType = 1
		chat.SenderUser = username
		chat.VoiceMilisecs = int(proto.Str2Num(r.FormValue("duration"), 10))
		chat.ContentType = proto.ChatContentVoice
		chat.Content = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName, svrctx.Get().WSPort, svrctx.Get().HttpStaticURL +
			svrctx.Get().HttpStaticMinichatDir + imei + "/" + fileName)//timestampString
		chat.FileID = proto.Str2Num(timestampString, 10)
		if len(r.FormValue("timestamp")) == 14 {
			chat.DateTime = proto.Str2Num(r.FormValue("timestamp")[2:14], 10)
		} else {
			chat.DateTime = proto.Str2Num(timestampString[0:12], 10)
		}

		args := fmt.Sprintf("-i %s -acodec amr_nb -ab 3.2k -ar 8000 %s", filePath, fileAmrPath)
		err9, _ := proto.ExecCmd("ffmpeg", strings.Split(args, " ")...)
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
		JSON(w, 200, &result)
		return
	} else {
	}
}

func OnClientConnected(conn *websocket.Conn) {
	connection := newAppConn(conn)
	logging.Log(fmt.Sprintf("websocket connected,  IP: %s, ID: %d ", conn.RemoteAddr(), connection.ID))

	//for writing, 写协程等待一个channel的数据，将channel收到的数据发送至客户端
	go AppConnWriteLoop(connection)

	//for business handler，业务处理的协程
	go AppBusinessHandleLoop(connection)

	//for reading
	 AppConnReadLoop(connection)
}

//for reading
func AppConnReadLoop(c *AppConnection) {
	//for reading, 负责处理APP请求的业务逻辑
	defer logging.PanicLogAndExit("AppConnReadLoop")

	defer func() {
		logging.Log(fmt.Sprintf("client %s, %d closed", c.user.Name, c.ID))
		delConnChan <- c
	}()

	for  {
		n := 0
		var err error
		buf := make([]byte,  16  * 1024)
		if svrctx.Get().IsDebug == false {
			c.conn.SetReadDeadline(time.Now().Add(time.Second * svrctx.Get().RecvTimeout))
		}

		if n, err = c.conn.Read(buf); err != nil {
			logging.Log(fmt.Sprintf("websocket recv  failed, recv size = %d, err = %s", n, err.Error()))
			return
		}

		logging.Log("recv: " + string(buf[0: n]))

		var itf interface{}

		err =json.Unmarshal(buf[0: n], &itf)
		if err != nil {
			logging.Log("parse recved json data failed, " + err.Error())
			return
		}

		msg:= itf.(map[string]interface{})
		data := msg["data"]
		if data == nil || proto.MakeStructToJson(&data) == "" {
			logging.Log("parse recved json data is empty ")
			return
		}

		params := msg["data"].(map[string]interface{})

		if ( params["username"] == nil ||  params["username"].(string) == "") {
			//没有username
			if msg["cmd"] == nil || msg["cmd"].(string) != proto.HearbeatCmdName {
				logging.Log("parse recved json username is empty ")
				return
			}else{
				continue
			}
		}

		if c.saved == false {
			//c.imeis = getUserDevicesImei(params["username"].(string))
			c.user.Name = params["username"].(string)
			c.saved = true
			addConnChan <- c
		}else{
			logging.Log(fmt.Sprintf("connid=%d, old user=%s, new user=%s", c.ID, c.user.Name, params["username"].(string)))
			if c.user.Name != "" && c.ID != 0 && params["username"].(string) != "" && params["username"].(string) != c.user.Name {
				info := &UpdateAppConnInfo{}
				info.connID = c.ID
				info.usernameOld = c.user.Name
				info.usernameNew = params["username"].(string)
				info.conn = c
				logging.Log(fmt.Sprintf("504: update an app connection:%v", (*info)))
				updateConnChan <- info
			}
		}

		c.requestChan <- buf[0: n]
	}
}

//for business handler，业务处理的协程
func AppBusinessHandleLoop(c *AppConnection) {
	defer logging.PanicLogAndExit("AppBusinessHandleLoop")

	for {
		select {
		case <-c.closeChan:
			logging.Log("app business goroutine exit")
			return
		case data := <-c.requestChan:
			if data == nil {
				logging.Log("connection closed, business goroutine exit")
				return
			}

			HandleAppRequest(c.ID, appServerChan, data)
		}
	}
}

//for writing, 写协程等待一个channel的数据，将channel收到的数据发送至客户端
func AppConnWriteLoop(c *AppConnection) {
	defer logging.PanicLogAndExit("AppConnWriteLoop")

	for   {
		select {
		case <-c.closeChan:
			logging.Log("write goroutine exit")
			return
		case data := <-c.responseChan:
			if data == nil ||  c.IsClosed() {
				logging.Log("connection closed, write goroutine exit")
				return
			}

			sendData := proto.MakeStructToJson(data)
			if n, err := c.conn.Write([]byte(sendData)); err != nil {
				logging.Log(fmt.Sprintf("send data to client failed: %s,  %d bytes sent",  err.Error(), n))
			}else{
				logging.Log(fmt.Sprintf("send data to client: %s,  %d bytes sent", sendData, n))
			}
		}
	}
}

func getAppConnByConnID(username string, connid uint64)  *AppConnection {
	if username == "" || connid == 0 {
		logging.Log(fmt.Sprintf("bad input params for getAppConnByConnID: %d, %s", connid, username))
		return nil
	}

	userConnTable, ok := AppConnTable[username]
	if ok && userConnTable != nil && len(userConnTable) > 0 {
		conn, ok2 := userConnTable[connid]
		if ok2 && conn != nil && connid == conn.ID {
			return conn
		}
	}

	logging.Log(fmt.Sprintf("getAppConnByConnID not found app connection: %d, %s", connid, username))
	return nil
}

func getAppConnsByUserName(username string) map[uint64]*AppConnection {
	if username == ""{
		logging.Log(fmt.Sprintf("bad input params for getAppConnByConnID:  %s",  username))
		return nil
	}

	userConnTable, ok := AppConnTable[username]
	if ok && userConnTable != nil && len(userConnTable) > 0 {
		return userConnTable
	}

	logging.Log(fmt.Sprintf("getAppConnByConnID not found app connection:  %s",  username))
	return nil
}


func JSON2(w http.ResponseWriter, ret int,  data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(ret)
	buf := proto.MakeStructToJson(proto.MakeStructToJson(&data))
	//
	////andHex := []byte("\\u0026")
	////and    := []byte("&")
	//
	////result = bytes.Replace(buf, ltHex, lt, -1)
	////result = bytes.Replace(result, gtHex, gt, -1)
	////unescapedString := bytes.Replace([]byte(buf), andHex, and, -1)
	//
	encodedString := buf //strings.Replace(buf, "\\", "\\\\", -1)
	fmt.Println(encodedString)
	w.Write([]byte(encodedString))
}


func JSON(w http.ResponseWriter, ret int,  data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(ret)
	buf := (proto.MakeStructToJson(&data))
	//
	////andHex := []byte("\\u0026")
	////and    := []byte("&")
	//
	////result = bytes.Replace(buf, ltHex, lt, -1)
	////result = bytes.Replace(result, gtHex, gt, -1)
	////unescapedString := bytes.Replace([]byte(buf), andHex, and, -1)
	//
	encodedString := buf //strings.Replace(buf, "\\", "\\\\", -1)
	fmt.Println(encodedString)
	w.Write([]byte(encodedString))
}

func GetLocationsByURL(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	systemno := proto.Str2Num(r.FormValue("systemno"), 10)
	proto.SystemNo2ImeiMapLock.Lock()
	imei, ok := proto.SystemNo2ImeiMap[systemno]
	proto.SystemNo2ImeiMapLock.Unlock()
	if ok == false || imei == 0 {
		logging.Log(fmt.Sprintf("bad imei for systemno %d ", systemno))
		dataResult := proto.PhpQueryLocationsResult{Result: -1, ResultStr: "",  Systemno: systemno}
		JSON(w, 200, &dataResult)
		return
	}

	datatype := r.FormValue("datatype")
	var locations *[]proto.LocationData
	if datatype == "1"{
		dataResult := proto.PhpQueryLocationsResult{Result: 0, ResultStr: "",  Systemno: systemno}
		data := svrctx.GetDeviceData(imei, svrctx.Get().PGPool)
		dataResult.Data = append(dataResult.Data, data.DataTime)
		dataResult.Data = append(dataResult.Data, int64(data.Lat * 1000000))
		dataResult.Data = append(dataResult.Data, int64(data.Lng * 1000000))
		dataResult.Data = append(dataResult.Data, data.Steps)
		dataResult.Data = append(dataResult.Data, data.Battery)
		dataResult.Data = append(dataResult.Data, data.AlarmType)
		dataResult.Data = append(dataResult.Data, data.ReadFlag)
		dataResult.Data = append(dataResult.Data, data.LocateType)
		dataResult.Data = append(dataResult.Data, data.ZoneName)
		dataResult.Data = append(dataResult.Data, data.Accracy)
		JSON(w, 200, &dataResult)
	}else if(datatype == "2"){
		dataResult := proto.PhpQueryLocationsResult{Result: 0, ResultStr: "",  Systemno: systemno}
		beginTime := proto.Str2Num("20" + r.FormValue("start"), 10)
		endTime := proto.Str2Num("20" + r.FormValue("end"), 10)

		locations = svrctx.QueryLocations(imei, svrctx.Get().PGPool, beginTime, endTime, true, false)
		if locations != nil && len(*locations) > 0 {
			for _, data := range *locations {
				dataItem := []interface{}{}
				dataItem = append(dataItem, data.DataTime)
				dataItem = append(dataItem, int64(data.Lat * 1000000))
				dataItem = append(dataItem, int64(data.Lng * 1000000))
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

		JSON(w, 200, &dataResult)
	}
}

func GetAppVersionOnline(w http.ResponseWriter, r *http.Request) {
	result := proto.HttpQueryAppVersionResult{}
	result.Status = -1

	w.Header().Set("Content-Type", "application/json")

	platform := r.FormValue("platform")
	reqUrl := ""
	if platform=="android" {
		reqUrl = svrctx.Get().AndroidAppURL
	}else if platform=="ios" {
		reqUrl = svrctx.Get().IOSAppURL
	}else{
		logging.Log("get app verion from online store failed, " + "bad params")
		JSON2(w, 500, (&result))
		return
	}

	resp, err := http.Get(reqUrl)
	if err != nil {
		logging.Log("get app verion from online store failed, " + err.Error() + " , " + reqUrl)
		JSON2(w, 500, (&result))
		return
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log("get app verion from online store response has err, " + err.Error() + " , " + reqUrl)
		JSON2(w, 500, (&result))
		return
	}

	if platform=="android" {
		pos := strings.Index(string(body), "softwareVersion")
		if pos < 0 {
			logging.Log("get app verion from online store response has err, " + err.Error() + " , " + reqUrl)
			JSON2(w, 500, (&result))
			return
		}

		if len(body[pos:]) < 30 {
			logging.Log("get app verion from online store response has err, " + err.Error() + " , " + reqUrl)
			JSON2(w, 500, (&result))
			return
		}

		strVer := string(body[pos: pos + 30])
		arr := strings.Split(strVer, " ")
		if len(arr) < 2 {
			logging.Log("get app verion from online store response has err, " + err.Error() + " , " + reqUrl)
			JSON2(w, 500, (&result))
			return
		}

		result.Version = strings.Split(arr[1], ".")
		result.AppUrl = svrctx.Get().AndroidAppURL
	}else{
		pos := strings.Index(string(body), "softwareVersion")
		if pos < 0 {
			logging.Log("get app verion from online store response has err, " + err.Error() + " , " + reqUrl)
			JSON2(w, 500, (&result))
			return
		}

		if len(body[pos:]) < 25 {
			logging.Log("get app verion from online store response has err, " + err.Error() + " , " + reqUrl)
			JSON2(w, 500, (&result))
			return
		}

		strVer := string(body[pos: pos + 25])
		leftPos := strings.Index(strVer, ">")
		rightPos := strings.Index(strVer, "<")
		if leftPos < 0 || rightPos < 0 || len(strVer) < (leftPos + 2) || len(strVer) < (rightPos + 1){
			logging.Log("get app verion from online store response has err, leftPos < 0 || rightPos < 0")
			JSON2(w, 500, (&result))
			return
		}

		ver := strVer[leftPos + 1: rightPos]

		result.Version = strings.Split(ver, ".")
		result.AppUrl = svrctx.Get().IOSAppURL
	}

	result.Status = 0
	JSON2(w, 200, (&result))
}
//
//func HandleApiCmd(w http.ResponseWriter, r *http.Request) {
//	var jsonData interface{}
//	err := ctx.ReadJSON(&jsonData)
//	if err != nil {
//		logging.Log("api cmd data is bad,  err: " + err.Error())
//		result := proto.HttpAPIResult{}
//		result.ErrCode = 500
//		result.ErrMsg = err.Error()
//		ctx.JSON(500, proto.MakeStructToJson(&result))
//		return
//	}
//
//	HandleAppURLRequest(ctx,  jsonData)
//}


func AddAppConnAccessToken(info ConnAccessTokenInfo) {
	if info.connID== 0 || info.username == "" ||   info.accessToken == "" {
		logging.Log("add a nil app access token")
		return
	}

	connTable, ok := AppConnTable[info.username]
	if !ok {
		//不存在，则首先创建新表，然后加入
		return
	}else{
		_, ok2 := connTable[info.connID]
		if ok2 {
			AppConnTable[info.username][info.connID].user.AccessToken = info.accessToken
		}
	}

	logging.Log(fmt.Sprintf("add conn access token: %d, %s, %s", info.connID, info.username, info.accessToken))
}

func AddNewAppConn(c *AppConnection) {
	if c == nil  || c.ID == 0 ||  c.user.Name == "" {
		logging.Log("add a nil app connection")
		return
	}

	_, ok := AppConnTable[c.user.Name]
	if !ok {
		//不存在，则首先创建新表，然后加入
		AppConnTable[c.user.Name] = map[uint64]*AppConnection{}
	}

	AppConnTable[c.user.Name][c.ID] = c

	logging.Log(fmt.Sprintf("add conn: %d, %s", c.ID, c.user.Name))
}


func UpdateAppConn(info *UpdateAppConnInfo) {
	if info.conn == nil || info.connID == 0 || info.usernameOld == "" || info.usernameNew == ""{
		logging.Log("update a nil app connection")
		return
	}

	userConnTable, ok := AppConnTable[info.usernameOld]
	if ok && userConnTable != nil && len(userConnTable) > 0 {
		conn, ok2 := userConnTable[info.connID]
		if ok2 && conn != nil && conn == info.conn {
			delete(AppConnTable[info.usernameOld], info.connID)
			if len(AppConnTable[info.usernameOld]) == 0 {
				delete(AppConnTable, info.usernameOld)
			}
		}
	}

	_, ok3 := AppConnTable[info.usernameNew]
	if !ok3 {
		//不存在，则首先创建新表，然后加入
		AppConnTable[info.usernameNew] = map[uint64]*AppConnection{}
	}

	AppConnTable[info.usernameNew][info.connID] = info.conn
	AppConnTable[info.usernameNew][info.connID].user.Name = info.usernameNew
	logging.Log(fmt.Sprintf("update conn:  %d, from %s to  %s", info.connID, info.usernameOld, info.usernameNew))
}

func RemoveAppConn(c *AppConnection) {
	if c == nil  || c.ID == 0 ||  c.user.Name == "" {
		logging.Log(fmt.Sprintf("remove a nil app connection, %v", c))
		return
	}

	userConnTable, ok := AppConnTable[c.user.Name]
	if ok && userConnTable != nil && len(userConnTable) > 0 {
		conn, ok2 := userConnTable[c.ID]
		if ok2 && conn != nil && c == conn {
			c.SetClosed()
			close(c.requestChan)
			close(c.responseChan)
			close(c.closeChan)
			c.conn.Close()

			delete(userConnTable, c.ID)
			logging.Log(fmt.Sprintf("connection deleted from AppConnTable, %d, %s", c.ID, c.user.Name))

			if len(AppConnTable[c.user.Name]) == 0 {
				delete(AppConnTable, c.user.Name)

				//当此用户下的所有连接都断开时，删除用户的accesstoken
				if c.user.GetAccessToken() != "" {
					AccessTokenTableLock.Lock()
					_, ok3 := AccessTokenTable[c.user.GetAccessToken()]
					if ok3 {
						delete(AccessTokenTable, c.user.GetAccessToken())
					}
					AccessTokenTableLock.Unlock()
				}
			}
		} else {
			logging.Log("will delete connection from appClientTable, but connection not found")
		}
	}

	logging.Log(fmt.Sprintf("delete AccessToken: %s, %s", c.user.GetAccessToken(), c.user.Name))
}

func AddAppDevice(info DeviceConnInfo) {
	if info.imei == 0  ||  info.username == ""{
		logging.Log("remove a nil app device connection")
		return
	}

	//添加imei下的该用户
	_, ok := AppDeviceTable[info.imei]
	if !ok {
		AppDeviceTable[info.imei] = map[string]bool{}
	}

	AppDeviceTable[info.imei][info.username] = true

	logging.Log(fmt.Sprintf("add device %d user %s conn",info.imei,  info.username))
}

func RemoveAppDevice(info DeviceConnInfo) {
	if info.imei == 0  ||  info.username == ""{
		logging.Log("remove a nil app device connection")
		return
	}

	//摘除imei下的该用户
	imeiAppUsers, ok := AppDeviceTable[info.imei]
	if ok && len(imeiAppUsers) > 0 {
		_, ok2 := imeiAppUsers[info.username]
		if ok2 {
			delete(AppDeviceTable[info.imei], info.username)
			if len(AppDeviceTable[info.imei]) == 0 {
				delete(AppDeviceTable, info.imei)
			}
		}
	}

	logging.Log(fmt.Sprintf("delete device %d user %s conn",info.imei,  info.username))
}

func AddAccessToken(accessToken string)  {
	AccessTokenTableLock.Lock()
	AccessTokenTable[accessToken] = true
	AccessTokenTableLock.Unlock()
}

func ValidAccessTokenFromService(AccessToken string)  bool {
	url := "https://watch.gatorcn.com/web/index.php?r=app/service/devices&access-token=" + AccessToken
	if svrctx.Get().IsDebugLocal {
		url = "https://watch.gatorcn.com/web/index.php?r=app/service/devices&access-token=" + AccessToken
	}

	logging.Log("url: " + url)
	resp, err := http.Get(url)
	if err != nil {
		logging.Log("get user devices failed, " + err.Error())
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

	userDevicesData := itf.(map[string]interface{})
	if userDevicesData == nil {
		return false
	}

	status := userDevicesData["status"].(float64)
	if status == 0  {
		return true
	}else{
		return false
	}

	return true
}

func getUserDevicesImei(username string) []uint64  {
	if username == "" {
		return nil
	}

	strSQL := fmt.Sprintf("SELECT  w.imei  from users u JOIN vehiclesinuser viu on u.recid = viu.UserID " +
		"JOIN  watchinfo w on w.recid = viu.VehId where u.loginname='%s' ", username)
	logging.Log("SQL: " + strSQL)
	rows, err := svrctx.Get().MySQLPool.Query(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%s] query user devices imei  in db failed, %s", username, err.Error()))
		return nil
	}

	defer rows.Close()

	imeis := []uint64{}
	for rows.Next() {
		imei := ""
		rows.Scan(&imei)
		fmt.Println(imei)
		imeis = append(imeis, proto.Str2Num(imei, 10))
	}

	return imeis
}

func getDeviceUserTable(imei uint64) map[string]bool  {
	if imei == 0 {
		return nil
	}

	strSQL := fmt.Sprintf("SELECT  u.loginname  from users u JOIN vehiclesinuser viu on u.recid = viu.UserID " +
		"JOIN  watchinfo w on w.recid = viu.VehId where w.imei='%d' ", imei)

	logging.Log("SQL: " + strSQL)
	rows, err := svrctx.Get().MySQLPool.Query(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] query user devices imei  in db failed, %s", imei, err.Error()))
		return nil
	}

	defer rows.Close()

	users := map[string]bool{}
	for rows.Next() {
		username := ""
		rows.Scan(&username)
		fmt.Println(username)
		users[username] = true
	}

	return users
}

func FormValue(r *http.Request, key string) string{
	if r == nil || key == "" {
		return ""
	}

	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return ""
	}

	if ct == "multipart/form-data" {
		r.ParseMultipartForm(32 << 20)
		if r.MultipartForm != nil {
			values := r.MultipartForm.Value[key]
			if len(values) > 0 {
				return values[0]
			}
		}
	}else{
		values := r.Form[key]
		if len(values) > 0 {
			return values[0]
		}else{
			values := r.PostForm[key]
			if len(values) > 0 {
				return values[0]
			}
		}
	}

	return ""
}


func TcpServerBridgeRunLoop(serverCtx *svrctx.ServerContext) {
	defer logging.PanicLogAndExit("TcpServerBridgeRunLoop")

	for {
		select {
		case msg := <-serverCtx.TcpServerChan:
			if msg == nil {
				logging.Log("main lopp exit or got error, TcpServerBridgeRunLoop goroutine exit")
				return
			}

			model := proto.GetDeviceModel(msg.Header.Header.Imei)
			switch model {
				case proto.DM_GT06:
				serverCtx.GT6TcpServerChan <- msg
			case proto.DM_GTI3:
				fallthrough
			case proto.DM_GT03:
				serverCtx.GT3TcpServerChan <- msg
			case proto.DM_WH01:
			default:

			}
		}
	}
}
