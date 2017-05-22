package admin

import (
	"github.com/bsm/redeo"
	"fmt"
	"../svrctx"
	"../proto"
	"encoding/json"
)

func AdminServerLoop(exitServerFunc func())  {
	adminsvr := redeo.NewServer(&redeo.Config{Addr:  svrctx.Get().MasterListenAddrPort})
	adminsvr.HandleFunc("ping", func(out *redeo.Responder, _ *redeo.Request) error {
		out.WriteInlineString("PONG")
		return nil
	})

	adminsvr.HandleFunc("hget", func(out *redeo.Responder, in *redeo.Request) error {
		if len(in.Args) == 0 {
			out.WriteInlineString("nil")
			return nil
		}

		proto.DeviceInfoListLock.RLock()
		device, ok := (*proto.DeviceInfoList)[proto.Str2Num(in.Args[0], 10)]
		if ok {
			jsonData, err := json.Marshal(device)
			if err != nil {
				out.WriteInlineString(err.Error())
			} else {
				out.WriteInlineString(string(jsonData))
			}
		}else {
			out.WriteInlineString("nil")
		}
		proto.DeviceInfoListLock.RUnlock()
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