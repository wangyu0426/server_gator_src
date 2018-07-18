package main

import (
	"./monitor"
	"./svrctx"
	"./logging"
	"./gt3tcpserver"
	"./tcpserver"
	"./appserver"
	"./admin"
	_ "net/http/pprof"
	"log"
	"net/http"
	"runtime"
)

func main() {
	monitor.PrintName()

	defer logging.PanicLogAndExit("main: ")

	go func() {
		log.Println(http.ListenAndServe(":6060", nil))
	}()

	runtime.GOMAXPROCS(runtime.NumCPU())
	//app data send to watch
	go appserver.TcpServerBridgeRunLoop(svrctx.Get())
	go appserver.LocalAPIServerRunLoop(svrctx.Get())
	go gt3tcpserver.TcpServerRunLoop(svrctx.Get())
	//receive data from watch
	go tcpserver.TcpServerRunLoop(svrctx.Get())
	//receive data from app
	go appserver.AppServerRunLoop(svrctx.Get())
	logging.Log("main start...")
	//svrctx.Get().WaitLock.Wait()
	//svrctx.Get().WaitLock.Wait()

	admin.AdminServerLoop(ExitServer)

	logging.Log("go-tcp-server exit")
}

func ExitServer()  {

}