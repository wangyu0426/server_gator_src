package logging

import (
	"fmt"
	"runtime"
	"os"
	"time"
	"sync"
)


var logFile *os.File
var logInputChan chan []byte
var closeLogOnce *sync.Once

func init()  {
	fmt.Println("log dir: ", GetLogDir())

	var err error
	logFile, err = os.OpenFile(GetLogDir() + "/go-server.log", os.O_WRONLY | os.O_CREATE | os.O_APPEND, 0666)
	if err != nil {
		fmt.Println("create log file failed, ", err)
		os.Exit(1)
	}

	logInputChan = make(chan []byte, 2048)
	closeLogOnce = &sync.Once{}

	go func() {
		for {
			select {
			case msg := <- logInputChan:
				if msg == nil {
					return
				}
				writeLog(string(msg))
			}
		}
	}()
}

func GetLogDir() string {
	if runtime.GOOS == "darwin" {
		return "/Users/macpc/logs"
	}else{
		return "/home/work/logs"
	}
}

func Log(msg string)  {
	funcAddr, file, line, ok := runtime.Caller(1)
	if ok {
		funcName := runtime.FuncForPC(funcAddr).Name()
		logInputChan <- []byte((fmt.Sprintf("[%s, %s:%d, %s()] %s\n", time.Now().String(), file, line, funcName, msg)))
	}else {
		logInputChan <- []byte((fmt.Sprint("[%s] %s\n", time.Now().String(), msg)))
	}

	fmt.Println(msg)
}

func writeLog(msg string)  {
	logFile.WriteString(msg)
}

func Close()  {
	closeLogOnce.Do(func() {
		close(logInputChan)

		if logFile != nil {
			logFile.Close()
		}
	})
}