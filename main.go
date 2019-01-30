package main

import (
	"./monitor"
	"./svrctx"
	"./logging"
	"./gt3tcpserver"
	"./tcpserver"
	"./tracetcpserver"
	"./appserver"
	"./admin"
	_ "net/http/pprof"
	"log"
	"net/http"
	"runtime"
	"os"
	"syscall"
)

func main() {
	monitor.PrintName()

	logFile, err := os.OpenFile("../logs/error.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0660)
	if err != nil {
		log.Println("服务启动出错",  "打开异常日志文件失败" , err)
		return
	}
	// 将进程标准出错重定向至文件，进程崩溃时运行时将向该文件记录协程调用栈信息
	syscall.Dup2(int(logFile.Fd()), int(os.Stderr.Fd()))

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
	go tracetcpserver.Tracegpsserver(svrctx.Get())
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