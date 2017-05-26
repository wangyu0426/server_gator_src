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
	createLogFile()

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

func createLogFile()  {
	var err error
	realDir := GetLogDir() + "/" + time.Now().Format("2006-01-02")
	err = os.MkdirAll(realDir, 0755)
	if err != nil {
		fmt.Println("mkdir -p faled, ", realDir, err)
		realDir = GetLogDir()
	}

	logFile, err = os.OpenFile(realDir +  "/go-server.log", os.O_WRONLY | os.O_CREATE | os.O_APPEND,  0666)
	if err != nil {
		fmt.Println("create log file failed, ", err)
		os.Exit(1)
	}

	logFile.WriteString(time.Now().String() + "Open Log File!!!\n")
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
	dir, err := os.Stat(GetLogDir() + "/" + time.Now().Format("2006-01-02") + "/go-server.log")
	if err == nil {  //路径存在
		if dir.IsDir() {  //路径存在并且是一个目录，错误
			fmt.Println("create log file failed, the file name is a dir")
			return
		}else {  //路径存在，是一个文件，继续写
			logFile.WriteString(msg)
			return
		}
	}else {  //路径不存在
		if logFile != nil {
			logFile.Close()
		}
		createLogFile()
		logFile.WriteString(msg)
	}
}

func Close()  {
	closeLogOnce.Do(func() {
		close(logInputChan)

		if logFile != nil {
			logFile.Close()
		}
	})
}