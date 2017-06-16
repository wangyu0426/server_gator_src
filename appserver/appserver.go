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
)

var addConnChan chan *AppConnection
var delConnChan chan *AppConnection
var AppClientTable = map[uint64]map[string]*AppConnection{}
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

func AppServerRunLoop(serverCtx *svrctx.ServerContext)  {
	appServerChan = serverCtx.AppServerChan
	app := iris.New()
	//app.Adapt(iris.DevLogger())
	app.Adapt(httprouter.New())
	ws := websocket.New(websocket.Config{Endpoint: "/wsapi"})
	app.Adapt(ws)
	ws.OnConnection(OnClientConnected)

	app.StaticWeb(svrctx.Get().HttpStaticURL, svrctx.Get().HttpStaticDir)

	app.Get("/api/hi", func(ctx *iris.Context) {
		result := proto.HttpAPIResult{
			ErrCode: 0,
			ErrMsg: "",
			Imei: "0",
		}
		ctx.JSON(200, result)
	})

	app.Post(svrctx.Get().HttpUploadURL, func(ctx *iris.Context) {
		result := proto.HttpAPIResult{
			ErrCode: 0,
			ErrMsg: "",
			Imei: "0",
		}

		imei := ctx.FormValue("imei")
		fieldname := ctx.FormValue("fieldname")
		uploadType := ctx.FormValue("type")
		fmt.Println(uploadType)

		_,fileInfo, err1 := ctx.FormFile(uploadType)
		if err1 != nil {
			result.ErrCode = 500
			ctx.JSON(500, result)
			return
		}

		file,err2 :=fileInfo.Open()
		if err2 != nil {
			result.ErrCode = 500
			result.ErrMsg = "server failed to open the uploaded  file"
			ctx.JSON(500, result)
			return
		}

		defer file.Close()
		fileData, err3 :=ioutil.ReadAll(file)
		if err3 != nil {
			result.ErrCode = 500
			result.ErrMsg = "server failed to read the data of  the uploaded  file"
			ctx.JSON(500, result)
			return
		}

		err4 := ioutil.WriteFile(svrctx.Get().HttpStaticDir + svrctx.Get().HttpStaticAvatarDir + fileInfo.Filename, fileData, 0666)
		if err4 != nil {
			result.ErrCode = 500
			result.ErrMsg = "server failed to save the uploaded  file"
			ctx.JSON(500, result)
			return
		}

		settings := make([]proto.SettingParam, 1)
		settings[0].FieldName = fieldname
		settings[0].NewValue = svrctx.Get().HttpStaticAvatarDir +  fileInfo.Filename

		ret := SaveDeviceSettings(proto.Str2Num(imei, 10), settings, nil)
		if ret {
			result.Data = fmt.Sprintf("%s:%d%s", svrctx.Get().HttpServerName, svrctx.Get().WSPort,svrctx.Get().HttpStaticURL +
				svrctx.Get().HttpStaticAvatarDir +  fileInfo.Filename)
			fmt.Println(fileInfo.Filename)
			ctx.JSON(200, result)
		}else{
			result.ErrCode = 500
			result.ErrMsg = "server failed to update the device setting in db"
			ctx.JSON(500, result)
			return
		}
	})

	go AddDeviceManagerLoop()

	//负责管理连接、并且回发数据到app端
	go func() {
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
						_, ok := subTable[c.user.GetAccessToken()]
						if ok {
							logging.Log("begin delete connection from AppClientTable")
							delete(subTable, c.user.GetAccessToken())
							logging.Log("connection deleted from AppClientTable")
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
						AppClientTable[imei] = map[string]*AppConnection{}
					}

					AppClientTable[imei][c.user.GetAccessToken()] = c
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
					(msg.Cmd == proto.HearbeatAckCmdName && msg.Conn != nil) ||
					msg.Conn == proto.VerifyCodeAckCmdName ||
					msg.Cmd == proto.GetDeviceByImeiAckCmdName ||
					msg.Cmd == proto.AddDeviceAckCmdName ||
					msg.Cmd == proto.DeleteDeviceAckCmdName {
					if msg.Cmd == proto.HearbeatCmdName {
						getAppClientsByImei(msg)
					}
					c := msg.Conn.(*AppConnection)
					data, err := json.Marshal(&msg)
					err = (*c.conn).EmitMessage(data)
					if err != nil {
						logging.Log("send msg to app failed, " + err.Error())
					}
					break
				}

				//从表中找出所有跟此IMEI关联的APP客户端，并将数据发送至每一个APP客户端
				//如果APP客户端已经登陆过，但服务器上没有该客户端关注的手表的数据，
				//那么收到该APP客户端的第一个请求，应该首先读取该客户端关注的手表数据

				subTable := getAppClientsByImei(msg)
				logging.Log("send msg: " + fmt.Sprint(msg))
				if subTable != nil {
					for _, c := range subTable{
						data, err := json.Marshal(&msg)
						err =(*c.conn).EmitMessage(data)
						if err != nil {
							logging.Log("send msg to app failed, " + err.Error())
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

func getAppClientsByImei(msg *proto.AppMsgData)  map[string]*AppConnection {
	subTable, ok := AppClientTable[msg.Imei]
	if ok {
		return subTable
	}else {
		if msg.Conn == nil || len(msg.AccessToken) == 0 {
			return nil
		}

		url := "http://127.0.0.1/tracker/web/index.php?r=app/service/devices&access-token=" + msg.AccessToken
		if svrctx.Get().IsDebugLocal {
			url = "http://120.25.214.188/tracker/web/index.php?r=app/service/devices&access-token=" + msg.AccessToken
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
					AppClientTable[imei] = map[string]*AppConnection{}
				}

				AppClientTable[imei][c.user.GetAccessToken()] = c
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
