package Algorithm

import "sync"

// 空结构体
var Exists = struct {}{}
// Set is the main interface
type Set struct {
	// struct为结构体类型的变量
	m map[interface{}]struct{}
	lock sync.RWMutex
}

func New(items ...interface{}) *Set {
	s := new(Set)
	s.m = make(map[interface{}]struct{})
	s.lock = sync.RWMutex{}
	s.Add(items...)

	return s
}

func (s *Set) Add(items ...interface{}) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	for _,item := range items{
		s.m[item] = Exists
	}

	return nil
}

func (s *Set) Contains(iterm interface{}) bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	_,ok := s.m[iterm]
	return ok
}

func (s *Set) Size() int  {
	s.lock.Lock()
	defer s.lock.Unlock()
	return len(s.m)
}

func (s *Set) Empty() bool  {
	s.lock.Lock()
	defer s.lock.Unlock()
	if len(s.m) == 0 {
		return true
	}

	return false
}

func (s *Set) Clear() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.m = make(map[interface{}]struct{})
}

func (s *Set) Equal(other *Set) bool  {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.Size() != other.Size(){
		return  false
	}

	for keyset,_ := range s.m{
		if !other.Contains(keyset){
			return false
		}
	}

	return true
}

//判断s是不是other的子集
func (s *Set) IsSubSet(other *Set) bool  {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.Size() > other.Size(){
		return false
	}

	for index,_ := range s.m{
		if !other.Contains(index){
			return false
		}
	}

	return  true
}

func (s *Set) Union(other *Set) *Set  {
	s.lock.Lock()
	defer s.lock.Unlock()
	ss := &Set{}
	ss.lock = sync.RWMutex{}
	ss.m = make(map[interface{}]struct{})
	for index,_ := range s.m{
		ss.Add(index)
	}
	for index,_:= range other.m{
		if ss.Contains(index){
			continue
		}
		ss.Add(index)
	}
	return ss
}
