package monitor

import (
	"fmt"
	"os"
	"syscall"
)

func init()  {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		fmt.Println("create tcp socket failed, " + err.Error())
		os.Exit(1)
	}

	//err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	//if err != nil {
	//	fmt.Println("set tcp socket reuse address  failed, " + err.Error())
	//	os.Exit(1)
	//}

	sockaddr := syscall.SockaddrInet4{}
	sockaddr.Addr = [4]byte{127, 0, 0, 1}
	sockaddr.Port = 9115

	addr := fmt.Sprintf("bind tcp address %d.%d.%d.%d:%d  ",  sockaddr.Addr[0], sockaddr.Addr[1],
		sockaddr.Addr[2], sockaddr.Addr[3], sockaddr.Port)

	err = syscall.Bind(fd, &sockaddr)
	if err != nil {
		fmt.Println(fmt.Sprintf("bind  tcp address %s failed, ", addr), err.Error())
		os.Exit(1)
	}

	//这个端口用来互斥主进程的，除非进程退出，否则永不关闭
	fmt.Println(addr)
}

func PrintName()  {
	fmt.Println("monitor package loaded")
}