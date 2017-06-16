package admin

import (
	"github.com/bsm/redeo"
	"fmt"
	"../svrctx"
	"../proto"
)

func AdminServerLoop(exitServerFunc func())  {
	adminsvr := redeo.NewServer(&redeo.Config{Addr:  svrctx.Get().MasterListenAddrPort})
	adminsvr.HandleFunc("ping", func(out *redeo.Responder, _ *redeo.Request) error {
		out.WriteInlineString("PONG")
		return nil
	})

	adminsvr.HandleFunc("reload", func(out *redeo.Responder, in *redeo.Request) error {
		if len(in.Args) == 0 {
			out.WriteInlineString("nil")
			return nil
		}

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