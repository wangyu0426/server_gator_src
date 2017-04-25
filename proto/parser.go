package proto

import (
	"../logger"
	"runtime"
	"fmt"
)

type MsgData struct {
	Size uint16
	Data []byte
}

const (
	ImeiLen = 15
	CmdLen = 4
)

func HandleMsg(responseChan chan *MsgData, msg *MsgData)  {
	imei := string(msg.Data[0: ImeiLen])
	cmd := string(msg.Data[ImeiLen: ImeiLen + CmdLen])

	logger.Log("imei: " + imei + " cmd: " + cmd + " go routines: " + fmt.Sprintf("%d", runtime.NumGoroutine()))

	responseChan <- &MsgData{6, []byte("Hello!")}
}