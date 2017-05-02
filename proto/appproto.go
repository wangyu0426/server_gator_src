package proto

import (
	"encoding/json"
)

type AppMsgData struct {
	id uint64
	cmd string
	data []byte
}

func HandleAppRequest(appserverChan chan *AppMsgData, data []byte) bool {
	var itf interface{}
	json.Unmarshal(data, &itf)
	msg:= itf.(map[string]interface{})
	switch msg["cmd"].(string) {
	case "login":
		return login(msg["username"].(string), msg["password"].(string))

	case "sync":

	default:
		break
	}
	
	return true
}

func login(username, password string) bool {
	return true
}

func sync()  {

}