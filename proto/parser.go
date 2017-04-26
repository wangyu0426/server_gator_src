package proto

type MsgData struct {
	Size uint16
	From  int
	Imei uint64
	ID uint64
	Data []byte
}

const (
	ImeiLen = 15
	CmdLen = 4
)

const (
	MsgFromTcpServer = iota
	MsgFromAppServer
)