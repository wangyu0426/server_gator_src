package logging

import (
	"fmt"
	"runtime"
	"os"
	"time"
	"sync"
	"gopkg.in/gomail.v2"
	"strings"
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
		//defer logging.PanicLogAndExit("")

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
		return "~/logs"
	}else{
		return "/home/work/logs"
	}
}

func Log(msg string)  {
	funcAddr, file, line, ok := runtime.Caller(0)
	if strings.Contains(file, "appserver.go") || strings.Contains(file, "appproto.go") {
		if strings.Contains(msg, "heartbeat") {
			return
		}
	}

	if ok {
		funcName := runtime.FuncForPC(funcAddr).Name()
		logInputChan <- []byte((fmt.Sprintf("[%s, %s:%d, %s()] %s\n", time.Now().String(), file, line, funcName, msg)))
	}else {
		logInputChan <- []byte((fmt.Sprint("[%s] %s\n", time.Now().String(), msg)))
	}

	tNow := time.Now()
	timeNow := tNow.Format("2006-01-02 15:04:05")
	fmt.Println(timeNow + " " + msg)

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
	close(logInputChan)

	if logFile != nil {
		logFile.Close()
	}
	//closeLogOnce.Do(func() {
	//	close(logInputChan)
	//
	//	if logFile != nil {
	//		logFile.Close()
	//	}
	//})
}

//当显式或隐式的调用panic，会立马停止当前函数的执行，然后开始当前goroutinue的栈展开，
// 如果有defer函数，会一并被执行。通过在defer函数里调用recover，可以使程序恢复正常的执行。
// recover函数只有在defer函数里有效，其他地方都返回nil。
func PanicLogAndExit(errmsg string){
	err := recover()
	if err == nil {
		return
	}

	errText := fmt.Sprint("get panic: ",  errmsg, ", ", err, ", ", string(PanicTrace(4)))
	Log(errText)

	SendMail("835280181@qq.com", "835280181@qq.com",  "Go Server Panic", errText, "",
		"smtp.qq.com", 587, "835280181@qq.com", "wbsvaskutjpbbdfc")

	panic("panic error cause exit")
}


func PanicLogAndCatch(errmsg string){
	err := recover()
	if err == nil {
		return
	}

	errText := fmt.Sprint("get panic: ",  errmsg, ", ", err, ", ", string(PanicTrace(4)))
	Log(errText)

	SendMail("835280181@qq.com", "835280181@qq.com",  "Go Server Panic", errText, "",
		"smtp.qq.com", 587, "835280181@qq.com", "wbsvaskutjpbbdfc")
}

func SendMailToDefaultReceiver(body string)  {
	SendMail("835280181@qq.com", "835280181@qq.com",  "Go Server Msg", body, "",
		"smtp.qq.com", 587, "835280181@qq.com", "wbsvaskutjpbbdfc")
}

func SendMail(from, to, subject, text, attachmentFile, mailServer  string, mailPort int, user, password string)  {
	m := gomail.NewMessage()
	m.SetHeader("From", from)
	m.SetHeader("To", to)
	//if len(cc) != 0{
	//	m.SetAddressHeader("Cc", "dan@example.com", "Dan")
	//}
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", fmt.Sprintf("panic error: <b>%s</b>", text))
	if len(attachmentFile) != 0 {
		m.Attach(attachmentFile)
	}

	d := gomail.NewDialer(mailServer, mailPort, user, password)

	if err := d.DialAndSend(m); err != nil {
		panic(err)
	}
}

func PanicTrace(kb int) []byte {
	//s := []byte("/src/runtime/panic.go")
	//e := []byte("/ngoroutine ")
	//line := []byte("/n")
	//stack := make([]byte, kb * 1024)
	//length := runtime.Stack(stack, true)
	//start := bytes.Index(stack, s)
	//stack = stack[start:length]
	//start = bytes.Index(stack, line) + 1
	//stack = stack[start:]
	//end := bytes.LastIndex(stack, line)
	//if end != -1 {
	//	stack = stack[:end]
	//}
	//end = bytes.Index(stack, e)
	//if end != -1 {
	//	stack = stack[:end]
	//}
	//stack = bytes.TrimRight(stack, "/n")
	//stack := make([]byte, kb * 1024)

	//打印出当前栈信息
	stack := make([]byte, kb * 1024)
	runtime.Stack(stack, true)
	return stack
}
