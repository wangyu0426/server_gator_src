package appserver

import (
	"../logging"
	"../svrctx"
	"../models"

	"gopkg.in/kataras/iris.v6"
	"gopkg.in/kataras/iris.v6/adaptors/httprouter"
	"gopkg.in/kataras/iris.v6/adaptors/websocket"
	"fmt"
)

func init() {
	logging.Log("appserver init")
	models.PrintSelf()
}

func AppServerRunLoop(serverCtx *svrctx.ServerContext)  {
	app := iris.New()
	//app.Adapt(iris.DevLogger())
	app.Adapt(httprouter.New())
	ws := websocket.New(websocket.Config{Endpoint: "/wsapi"})
	app.Adapt(ws)
	ws.OnConnection(OnClientConnected)
	app.Listen(fmt.Sprintf("%s:%d", serverCtx.BindAddr, serverCtx.WSPort))
}

func OnClientConnected(c websocket.Connection)  {
	logging.Log("websocket connected: " + c.Context().RemoteAddr() + "Client ID: " + c.ID())
	c.OnMessage(func(data []byte) {
		message := string(data)
		logging.Log("recv from client: " + message)
		c.EmitMessage([]byte("Message from: " + c.ID() + "-> " + message)) // broadcast to all clients except this
		//c.EmitMessage([]byte("Me: " + message))                                                    // writes to itself
	})

	c.OnDisconnect(func() {
		logging.Log("websocket disconnected: " + c.Context().RemoteAddr() + "Client ID: " + c.ID())
	})

}