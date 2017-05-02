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
)

var addConnChan chan *AppConnection
var delConnChan chan *AppConnection

var appServerChan chan *proto.AppMsgData

func init() {
	logging.Log("appserver init")
	models.PrintSelf()

	addConnChan = make(chan *AppConnection, 1024)
	delConnChan = make(chan *AppConnection, 1024)

	//resp, err := http.PostForm("http://service.gatorcn.com/tracker/web/index.php?r=app/auth/login",
	//	url.Values{"username": {"2421714592@qq.com"}, "password": {"81dc9bdb52d04dc20036dbd8313ed055"}})
	//if err != nil {
	//	logging.Log("app login failed" + err.Error())
	//}
	//
	//defer resp.Body.Close()
	//
	//body, err := ioutil.ReadAll(resp.Body)
	//fmt.Println(string(body))
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
						appClient, ok := subTable[c.user.GetAccessToken()]
						if ok {
							appClient.SetClosed()
							close(appClient.requestChan)
							close(appClient.responseChan)
							close(appClient.closeChan)
							(*appClient.conn).Disconnect()

							delete(subTable, c.user.GetAccessToken())
							logging.Log("connection deleted from AppClientTable")
						} else {
							logging.Log("will delete connection from appClientTable, but connection not found")
						}
					}
				}
			case data := <- appServerChan:  //回发数据到APP
				fmt.Println(data)
			}
		}
	}()

	app.Listen(fmt.Sprintf("%s:%d", serverCtx.BindAddr, serverCtx.WSPort))
}

func OnClientConnected(conn websocket.Connection)  {
	logging.Log("websocket connected: " + conn.Context().RemoteAddr() + "Client ID: " + conn.ID())
	connection := newAppConn(&conn)
	conn.SetValue("ctx", connection)

	//for reading, 负责处理APP请求的业务逻辑
	go func(c *AppConnection) {
		for  {
			select {
			case <- c.closeChan:
				return
			case data := <- c.requestChan:
				proto.HandleAppRequest(appServerChan, data)
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
		delConnChan <- c
	})

}
