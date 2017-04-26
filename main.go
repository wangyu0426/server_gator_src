package main

import (
	"./svrctx"
	"./logger"
	"./tcpserver"
	"./appserver"
	"fmt"
	"github.com/bsm/redeo"
)

func main() {
	//str := []byte("357593030571505")
	//fmt.Println(str[0] == 'a')
	////fmt.Println("sub str: ", string([]rune(str)[1:]))
	////num, _ := strconv.ParseUint("0x" + str, 0, 0)
	//num, _ := strconv.ParseUint(string(str), 0, 0)
	//fmt.Println(num)
	////fmt.Println(string(str[3: 3 + 2]))
	//return

	fmt.Println("server config: ", *svrctx.Get())

	go tcpserver.TcpServerRunLoop(svrctx.Get())
	go appserver.AppServerRunLoop(svrctx.Get())

	//svrctx.Get().WaitLock.Wait()
	srv := redeo.NewServer(&redeo.Config{Addr: "localhost:9736"})
	srv.HandleFunc("ping", func(out *redeo.Responder, _ *redeo.Request) error {
		out.WriteInlineString("PONG")
		return nil
	})

	srv.HandleFunc("shutdown", func(out *redeo.Responder, _ *redeo.Request) error {
		logger.Log("Server shutdown by redis command line")
		srv.Close()
		return nil
	})

	logger.Log(fmt.Sprintf("Listening on tcp://%s", srv.Addr()))
	srv.ListenAndServe()

	//svrctx.Get().WaitLock.Wait()

	logger.Log("go-tcp-server exit")
}