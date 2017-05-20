package main

import (
	"./svrctx"
	"./logging"
	"./tcpserver"
	"./appserver"
	"./cache"
	"./admin"
)

func main() {
	//str := []byte("357593030571505")
	//fmt.Println(str[0] == 'a')
	////fmt.Println("sub str: ", string([]rune(str)[1:]))
	////num, _ := strconv.ParseUint("0x" + str, 0, 0)
	//num, _ := strconv.ParseUint(string(str), 0, 0)
	//fmt.Println(num)
	////fmt.Println(string(str[3: 3 + 2]))
	//data1 := []byte("123456")
	//data2 := data1
	//data1[0] = '0'
	//fmt.Println(string(data1), string(data2))
	//return

	//fmt.Println("server config: ", *svrctx.Get())

	cache.TestRedis()
	go tcpserver.TcpServerRunLoop(svrctx.Get())
	go appserver.AppServerRunLoop(svrctx.Get())

	//svrctx.Get().WaitLock.Wait()
	//svrctx.Get().WaitLock.Wait()

	admin.AdminServerLoop(ExitServer)

	logging.Log("go-tcp-server exit")
}

func ExitServer()  {

}