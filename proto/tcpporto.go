package proto

import (
	"fmt"
	"runtime"
	"strconv"
	"../logging"
)

func HandleRequest(tcpserverChan chan *MsgData, msg *MsgData)  {
	imei, _ := strconv.ParseUint(string(msg.Data[0: ImeiLen]), 0, 0)
	cmd := string(msg.Data[ImeiLen: ImeiLen + CmdLen])

	logging.Log(fmt.Sprintf("imei: %d cmd: %s; go routines: %d", imei, cmd, runtime.NumGoroutine()))


	//responseChan <- &MsgData{6,  imei, 0, []byte("Hello!")}
}