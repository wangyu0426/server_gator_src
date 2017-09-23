package admin

import (
	"github.com/bsm/redeo"
	"fmt"
	"../svrctx"
	"../proto"
	"../logging"
	"io/ioutil"
	"os"
	"runtime"
)

func init()  {
	//cmd := "cmd"
	//switch cmd {
	//case "cmd":
	//	fmt.Println("1")
	//	break
	//	fmt.Println("2")
	//}
	//
	//fmt.Println("3")
}

func AdminServerLoop(exitServerFunc func())  {
	adminsvr := redeo.NewServer(&redeo.Config{Addr:  svrctx.Get().MasterListenAddrPort})
	adminsvr.HandleFunc("ping", func(out *redeo.Responder, _ *redeo.Request) error {
		out.WriteInlineString("PONG")
		return nil
	})

	adminsvr.HandleFunc("test", func(out *redeo.Responder, in *redeo.Request) error {
		//"test cmd data"
		//"test epo imei" -- 不需要做推送测试，因为epo是手表主动请求的
		//"test voice imei phone filename"
		//"test photo imei phone filename"

		testType := in.Args[0]
		result := "ok"

		switch testType {
		case "cmd":
			if len(in.Args) != 2 {
				out.WriteInlineString("bad args count")
				return nil
			}

			data := in.Args[1]
			//(003B357593060153353AP01221.18.79.110#123,0000000000000001)
			if data[0] != '(' || data[len(data) - 1] != ')' {
				result = "bad data format, must begin with ( and end with )"
				break
			}

			size := proto.Str2Num(data[1: 5], 16)
			if int(size) != len(data) {
				result = "bad data len"
				break
			}

			imei := proto.Str2Num(data[5: 20], 10)
			cmd := data[20: 24]
			id := uint64(0)
			requireAck := false
			if cmd == "AP01" ||  //set server ip, port
				cmd == "AP02" ||
				cmd == "AP03" ||
				cmd == "AP05" ||
				cmd == "AP06" ||
				cmd == "AP07" ||
				cmd == "AP08" ||
				cmd == "AP09" ||
				cmd == "AP10" ||
				cmd == "AP14" ||
				cmd == "AP15" ||
				cmd == "AP16" ||
				cmd == "AP18" ||
				cmd == "AP19" ||
				cmd == "AP20" ||
				cmd == "AP21" ||
				cmd == "AP22" ||
				cmd == "AP24" ||
				cmd == "AP25"{
				id = proto.Str2Num(data[len(data) - 17: len(data) - 1], 16)
				requireAck = true
			}else if cmd == "AP04" {

			}

			fmt.Println(imei, cmd, id)
			svrctx.Get().TcpServerChan <- proto.MakeReplyMsg(imei, requireAck, []byte(data), id)
		//case "epo": //AP13 EPO
		case "voice": //AP12 微聊
			if len(in.Args) != 4 {
				out.WriteInlineString("bad args count")
				return nil
			}

			imei := proto.Str2Num(in.Args[1], 10)
			phone := in.Args[2]
			filename := in.Args[3]
			if svrctx.IsPhoneNumberInFamilyList(imei, phone) == false {
				result = fmt.Sprintf("phone number %s is not in the family phone list of %d", phone, imei)
				break
			}

			chat := proto.ChatInfo{}
			chat.Sender = phone
			chat.ContentType = proto.ChatContentVoice
			chat.Content = proto.MakeTimestampIdString()
			chat.DateTime = proto.Str2Num(chat.Content[0:12], 10)

			voice, err := ioutil.ReadFile(svrctx.Get().HttpStaticDir + svrctx.Get().HttpStaticMinichatDir + filename + ".amr")
			if err != nil {
				result = "read voice file error: " + err.Error()
				break
			}

			os.MkdirAll(svrctx.Get().HttpStaticDir + svrctx.Get().HttpStaticMinichatDir + in.Args[1], 0755)
			err = ioutil.WriteFile(svrctx.Get().HttpStaticDir + svrctx.Get().HttpStaticMinichatDir + in.Args[1] + "/" +
				chat.Content + ".amr", voice, 0755)
			if err != nil {
				result = "upload new voice file error: " + err.Error()
				break
			}

			svrctx.AddChatData(imei, chat)

		case "photo": //AP23 亲情号码图片设置
			imei := proto.Str2Num(in.Args[1], 10)
			phone := in.Args[2]
			filename := in.Args[3]
			if svrctx.IsPhoneNumberInFamilyList(imei, phone) == false {
				result = fmt.Sprintf("phone number %s is not in the family phone list of %d", phone, imei)
				break
			}

			photoInfo := proto.PhotoSettingInfo{}
			photoInfo.Member.Phone = phone
			photoInfo.ContentType = proto.ChatContentPhoto
			photoInfo.Content = proto.MakeTimestampIdString()

			photo, err := ioutil.ReadFile(svrctx.Get().HttpStaticDir + svrctx.Get().HttpStaticAvatarDir +
				filename + ".jpg")
			if err != nil {
				result = "read photo file error: " + err.Error()
				break
			}

			os.MkdirAll(svrctx.Get().HttpStaticDir + svrctx.Get().HttpStaticAvatarDir + in.Args[1], 0755)
			err = ioutil.WriteFile(svrctx.Get().HttpStaticDir + svrctx.Get().HttpStaticAvatarDir + in.Args[1] + "/" +
				photoInfo.Content + ".jpg", photo, 0755)
			if err != nil {
				result = "upload new photo file error: " + err.Error()
				break
			}

			//svrctx.AddPhotoData(imei, photoInfo)

		default:
		}

		out.WriteInlineString(result)
		return nil
	})

	adminsvr.HandleFunc("reload", func(out *redeo.Responder, in *redeo.Request) error {
		if len(in.Args) != 1 {
			out.WriteInlineString("bad args")
			return nil
		}

		fileName := in.Args[0]
		result := "nil"

		switch fileName {
		case proto.ReloadEPOFileName:
			err := proto.ReloadEPO()
			if err == nil {
				result = "ok"
			}else{
				result = "reload epo failed: " + err.Error()
				logging.Log(result)
				logging.SendMailToDefaultReceiver(result)
			}
		case proto.ReloadConfigFileName:

		default:
		}

		out.WriteInlineString(result)
		return nil
	})

	adminsvr.HandleFunc("hget", func(out *redeo.Responder, in *redeo.Request) error {
		if len(in.Args) == 0 {
			out.WriteInlineString("nil")
			return nil
		}

		if len(in.Args) == 1 && in.Args[0] == "go" {
			out.WriteInlineString(proto.Num2Str(uint64(runtime.NumGoroutine()), 10))
			return nil
		}

		if len(in.Args) != 2 {
			out.WriteInlineString("bad args")
			return nil
		}

		imei := proto.Str2Num(in.Args[0], 10)
		fieldName := in.Args[1]
		result := "nil"

		proto.DeviceInfoListLock.Lock()
		device, ok := (*proto.DeviceInfoList)[imei]
		if ok {
			switch fieldName {
			case proto.ModelFieldName:
				if device.Model >= 0 && device.Model < len(proto.ModelNameList){
					result = proto.ModelNameList[device.Model]
				}
			default:
				result = proto.MakeStructToJson(device)
			}
		}
		proto.DeviceInfoListLock.Unlock()
		out.WriteInlineString(result)
		return nil
	})

	adminsvr.HandleFunc("hset", func(out *redeo.Responder, in *redeo.Request) error {
		if len(in.Args) == 0 {
			out.WriteInlineString("nil")
			return nil
		}

		if len(in.Args) != 3 {
			out.WriteInlineString("bad args")
			return nil
		}

		imei := proto.Str2Num(in.Args[0], 10)
		fieldName := in.Args[1]
		newValue := in.Args[2]
		result := "failed"

		proto.DeviceInfoListLock.Lock()
		device, ok := (*proto.DeviceInfoList)[imei]
		if ok {
			switch fieldName {
			case proto.ModelFieldName:
				newModel := proto.ParseDeviceModel(newValue)
				if newModel >= 0 {
					if newModel != proto.DM_GT06 {
						delete(*proto.DeviceInfoList, imei)
					}else{
						device.Model = newModel
					}
					result = "ok"
				}
			case proto.RedirectServerFieldName:
				if newValue == "1" {
					device.RedirectServer = true
					result = "ok"
				}else if newValue == "0" {
					device.RedirectServer = false
					result = "ok"
				}
			}
		}
		proto.DeviceInfoListLock.Unlock()

		if ok == false {
			if fieldName == proto.ModelFieldName {
				newModel := proto.ParseDeviceModel(newValue)
				if newModel >= 0 {
					if newModel == proto.DM_GT06 {
						ok2 := proto.LoadDeviceInfoFromDB(svrctx.Get().MySQLPool)
						if ok2 {
							result = "ok"
						}
					}
				}
			}
		}

		out.WriteInlineString(result)

		return nil
	})

	adminsvr.HandleFunc("shutdown", func(out *redeo.Responder, _ *redeo.Request) error {
		logging.Log("Server shutdown by redis command line")
		if exitServerFunc != nil {
			exitServerFunc()
		}
		adminsvr.Close()
		return nil
	})

	fmt.Println(fmt.Sprintf("Listening on tcp://%s", adminsvr.Addr()))
	adminsvr.ListenAndServe()
}

//将错误或报警信息通知管理员，可以通过发送邮件、短信、电话、QQ等？告知管理员
//通知内容中包含错误发生的时间、代码的位置、错误信息描述等
func NotifyAdmin()  {

}