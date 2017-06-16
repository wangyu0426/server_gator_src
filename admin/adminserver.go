package admin

import (
	"github.com/bsm/redeo"
	"fmt"
	"../svrctx"
	"../proto"
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
		//"test epo imei"
		//"test voice imei filename"
		//"test photo imei filename"

		if len(in.Args) != 2 && len(in.Args) != 3 {
			out.WriteInlineString("bad args")
			return nil
		}

		testType := in.Args[0]
		result := "ok"

		switch testType {
		case "cmd":
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
				cmd == "AP05" ||
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
			}

			fmt.Println(imei, cmd, id)
			svrctx.Get().TcpServerChan <- proto.MakeReplyMsg(imei, requireAck, []byte(data), id)
		case "epo":
		case "voice":
		case "photo":
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
			err := proto.LoadEPOFromFile(true)
			if err == nil {
				result = "ok"
			}
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

		if len(in.Args) != 2 {
			out.WriteInlineString("bad args")
			return nil
		}

		imei := proto.Str2Num(in.Args[0], 10)
		fieldName := in.Args[1]
		result := "nil"

		proto.DeviceInfoListLock.RLock()
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
		proto.DeviceInfoListLock.RUnlock()
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

		proto.DeviceInfoListLock.RLock()
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
			}
		}
		proto.DeviceInfoListLock.RUnlock()

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
		fmt.Println("Server shutdown by redis command line")
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