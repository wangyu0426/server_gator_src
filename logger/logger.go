package logger

import (
	"../svrctx"
	"fmt"
	"runtime"
	"os"
	"time"
)

var logFile *os.File

func init()  {
	fmt.Println("log dir: ", svrctx.Get().LogDir)

	var err error
	logFile, err = os.OpenFile(svrctx.Get().LogDir + "/go-server.log", os.O_WRONLY | os.O_CREATE | os.O_APPEND, 0666)
	if err != nil {
		fmt.Println("create log file failed, ", err)
		os.Exit(1)
	}
}

func Log(msg string)  {
	funcAddr, file, line, ok := runtime.Caller(1)
	if ok {
		funcName := runtime.FuncForPC(funcAddr).Name()
		logFile.WriteString(fmt.Sprintf("[%s, %s:%d, %s()] %s\n", time.Now().String(), file, line, funcName, msg))
	}else {
		logFile.WriteString(fmt.Sprint("[%s] %s\n", time.Now().String(), msg))
	}

	fmt.Println(msg)
}

func Close()  {
	if logFile != nil {
		logFile.Close()
	}
}