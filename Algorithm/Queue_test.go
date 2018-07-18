package Algorithm

import (
	"testing"
	"fmt"
)

func TestQueue(t *testing.T)  {
	fmt.Println("begin test queue...")
	var qq *Myqueue
	qq = NewMyqueue(64)
	fmt.Println(qq.IsEmpty())
	qq.Append(1)
	qq.Append("2")
	fmt.Println(qq.Size())
	fmt.Println(qq.Pop())
	fmt.Println(qq.Size())
	fmt.Println(qq.Pop())
	fmt.Println(qq.IsEmpty())
	fmt.Println("end test queue...")
}
