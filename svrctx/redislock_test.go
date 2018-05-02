package svrctx


import (
	"fmt"

	"time"
	"../logging"
	"github.com/garyburd/redigo/redis"
	"testing"
)

func TestLock(t *testing.T){
	rd := Redispool.Get()
	defer rd.Close()

	go func() {
		Alock := RedisLock{LockKey: "xxxxx"}
		err := Alock.Lock(&rd,5)
		time.Sleep(7 * time.Second)
		fmt.Println("111",err)
		Alock.Unlock(&rd)
	}()

	time.Sleep(6 * time.Second)
	Block := RedisLock{LockKey:"xxxxx"}
	err := Block.Lock(&rd,5)
	fmt.Println("222",err)

	time.Sleep(2 * time.Second)
	Clock := RedisLock{LockKey: "xxxxx"}
	err = Clock.Lock(&rd, 5) //想获取新的lock Clock，但由于 Block还存在，返回错误
	fmt.Println("333", err)

	time.Sleep(10 * time.Second)
}
var Redispool *redis.Pool

func initRedis(redisUrl string)  {
	if redisUrl == ""{
		redisUrl = ":6379"
	}

	Redispool = &redis.Pool{
		MaxIdle:   100,
		MaxActive: 12000,
		Dial: func() (redis.Conn, error) {
			c,err := redis.Dial("tcp",redisUrl)
			if err != nil{
				logging.PanicLogAndExit(fmt.Sprintf("connect to redis(%s) got error: %s", redisUrl, err))
			}
			return c,nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

func init()  {
	initRedis(":6379")
}
