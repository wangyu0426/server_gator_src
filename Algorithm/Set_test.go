package Algorithm

import (
	"testing"
	"fmt"
)

func TestSet(t *testing.T)  {
	fmt.Println("begin test set...")
	var newSet *Set
	newSet = New(1,2,3,4,5,5)
	fmt.Println(newSet.Size())
	fmt.Println(newSet.Contains(6))
	fmt.Println(newSet.Size())
	fmt.Println(newSet.Empty())
	fmt.Println("end test queue...")
}
