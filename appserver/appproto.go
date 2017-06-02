package appserver

import (
	"encoding/json"
	"../logging"
	"net/http"
	"net/url"
	"io/ioutil"
	"../proto"
	"../svrctx"
	"fmt"
	"time"
	"strconv"
)

func HandleAppRequest(c *AppConnection, appserverChan chan *proto.AppMsgData, data []byte) bool {
	var itf interface{}
	err:=json.Unmarshal(data, &itf)
	if err != nil {
		return false
	}

	msg:= itf.(map[string]interface{})
	cmd :=  msg["cmd"]
	if cmd == nil {
		return false
	}

	switch cmd.(string) {
	case "login":
		return login(c, msg["username"].(string), msg["password"].(string))

	case "heartbeat":
		appServerChan <- &proto.AppMsgData{Cmd: "heartbeat-ack",
			Data: (fmt.Sprintf("{\"timestamp\": \"%s\"}", time.Now().String())), Conn: c}
		break

	default:
		break
	}
	
	return true
}

func login(c *AppConnection, username, password string) bool {
	resp, err := http.PostForm("http://service.gatorcn.com/tracker/web/index.php?r=app/auth/login",
		url.Values{"username": {username}, "password": {password}})
	if err != nil {
		logging.Log("app login failed" + err.Error())
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logging.Log("response has err, " + err.Error())
		return false
	}

	var itf interface{}
	err = json.Unmarshal(body, &itf)
	if err != nil {
		logging.Log("parse login response as json failed, " + err.Error())
		return false
	}

	loginData := itf.(map[string]interface{})
	if loginData == nil {
		return false
	}

	status :=  loginData["status"]
	accessToken := loginData["accessToken"]
	devices := loginData["devices"]

	//logging.Log("status: " + fmt.Sprint(status))
	//logging.Log("accessToken: " + fmt.Sprint(accessToken))
	//logging.Log("devices: " + fmt.Sprint(devices))
	if status != nil && accessToken != nil && devices != nil {
		c.user.AccessToken = accessToken.(string)
		c.user.Logined = true
		c.user.Name = username
		c.user.PasswordMD5 = password

		addConnChan <- c

		//devicesLocationURL := "http://184.107.50.180:8012/GetMultiWatchData?systemno="
		locations := []proto.LocationData{}
		for _, d := range devices.([]interface{}) {
			device := d.(map[string]interface {})
			imei, _ := strconv.ParseUint(device["IMEI"].(string), 0, 0)
			logging.Log("device: " + fmt.Sprint(imei))
			c.imeis = append(c.imeis, imei)
			locations = append(locations, svrctx.GetDeviceData(imei, svrctx.Get().PGPool))

			//devicesLocationURL += device["IMEI"].(string)[4:]
			//
			//if i < len(devices.([]interface{})) - 1 {
			//	devicesLocationURL += "|"
			//}
		}

		jsonLocations , _ := json.Marshal(locations)

		//logging.Log("devicesLocationURL: " + devicesLocationURL)
		//
		//respLocation, err := http.Get(devicesLocationURL)
		//if err != nil {
		//	logging.Log("get devicesLocationURL failed" + err.Error())
		//}
		//
		//defer respLocation.Body.Close()

		//bodyLocation, err := ioutil.ReadAll(respLocation.Body)
		//if err != nil {
		//	logging.Log("response has err, " + err.Error())
		//}
		//
		//logging.Log("bodyLocation: " +  string(bodyLocation))

		appServerChan <- &proto.AppMsgData{Cmd: "login-ack",
			Data: (fmt.Sprintf("{\"user\": %s, \"location\": %s}", string(body), string(jsonLocations))), Conn: c}
	}

	return true
}

func appSync()  {

}