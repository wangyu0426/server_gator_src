package Algorithm


type Myqueue struct {
	Head int
	Tail int
	Length uint32
	Capacity uint32
	Data []interface{}
}

func NewMyqueue(len uint32) *Myqueue {
	return &Myqueue{Head:0,
	Tail:0,
	Length:len,
	Capacity:0,
	Data:make([]interface{},len)}
}

func (q *Myqueue) IsEmpty() bool {
	return q.Capacity == 0
}

func (q *Myqueue) IsFull() bool  {
	return q.Capacity == q.Length
}

func (q *Myqueue) Size() int  {
	return int(q.Capacity)
}

func (q *Myqueue) Append( e interface{}) bool {
	if q.IsFull() {
		return  false
	}

	q.Data[q.Capacity] = e
	q.Capacity++
	q.Tail++

	return true
}

func (q *Myqueue) Pop() interface{} {
	if q.IsEmpty() {
		return nil
	}

	defer func() {
		q.Head++
		q.Capacity--

	}()

	e := q.Data[q.Head]

	return e
}

func (q *Myqueue) Clear() bool {
	q.Capacity = 0
	q.Head = 0
	q.Tail = 0
	q.Data = make([]interface{},q.Length)

	return true
}

func (q *Myqueue) Each(fn func(node interface{}))  {
	for i := q.Head;i < q.Head + int(q.Capacity);i++  {
		fn(q.Data[i % int(q.Length)])
	}
}