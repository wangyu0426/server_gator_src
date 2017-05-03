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
var AppClientTable = map[uint64]map[string]*AppConnection{}

var appServerChan chan *proto.AppMsgData

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
				if msg.Cmd == "login" {
					c := msg.Conn.(*AppConnection)
					err :=(*c.conn).EmitMessage(msg.Data)
					if err != nil {
						logging.Log("send msg to app failed, " + err.Error())
					}
					break
				}

				//从表中找出所有跟此IMEI关联的APP客户端，并将数据发送至每一个APP客户端
				subTable, ok := AppClientTable[msg.Imei]
				if ok {
					for _, c := range subTable{
						err :=(*c.conn).EmitMessage(msg.Data)
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

		delConnChan <- c
	})

}
