package cache

import (
	"fmt"
	"../svrctx"
	"../logging"
	"../proto"
	"github.com/go-redis/redis"
)

var sharedRedisClient *redis.Client

func init() {
	logging.Log("cache init")
	client := newRedisClient()
	if client != nil {

	}
}

func newRedisClient() (client *redis.Client) {
	client = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", svrctx.Get().RedisAddr, svrctx.Get().RedisPort),
		Password: svrctx.Get().RedisPassword,
		DB: 0,
	})

	_, err := client.Ping().Result()
	if err != nil {
		logging.Log("connect redis failed, " + err.Error())
		client = nil
	}

	return
}

func TestRedis() (bool) {
	//if sharedRedisClient == nil {
	//	sharedRedisClient := newRedisClient()
	//	if sharedRedisClient == nil {
	//		logging.Log("connected failed, exit")
	//		return false
	//	}
	//}
	//
	////从DB中加载数据到redis缓存
	//mq := NewMQ(uint64(123), 0)
	//if mq != nil {
	//	 ok := mq.Push(&proto.MsgData{})//{uint32(len([]byte("12345"))), 0, 123, 1, []byte("12345")})
	//	if ok {
	//		mq.Pop()
	//	}
	//}
	//os.Exit(0)
	return proto.MSG_HEADER_VER == proto.MSG_HEADER_VER
}
