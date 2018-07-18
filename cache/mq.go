package cache

import (
	"../svrctx"
	"../logging"
	"../proto"
	"github.com/go-redis/redis"
	"fmt"
)

type MsgQueue struct {
	redisClient *redis.Client
	keyName  string
}


func NewMQ(msgImei uint64, msgFrom int32)  ( *MsgQueue) {
	redisClient := newRedisClient()
	if redisClient == nil {
		logging.Log("create new redis connection failed")
		return nil
	}

	mq := &MsgQueue{redisClient: redisClient}

	if msgFrom == proto.MsgFromAppToAppServer {
		mq.keyName = fmt.Sprintf("%s%d", svrctx.Get().RedisAppMQKeyPrefix, msgImei)
	}else {
		mq.keyName = fmt.Sprintf("%s%d", svrctx.Get().RedisDeviceMQKeyPrefix, msgImei)
	}

	return mq
}

func (mq *MsgQueue) Push(msg *proto.MsgData) (bool) {
	err := mq.redisClient.RPush(mq.keyName, proto.Encode(msg)).Err()
	if err != nil {
		logging.Log(fmt.Sprintf("RPush %s  failed, ", mq.keyName, err.Error()))
		return false
	}

	return true
}

func (mq *MsgQueue) Pop() ( *proto.MsgData) {
	result := mq.redisClient.LPop(mq.keyName)
	if result.Err() != nil {
		logging.Log(fmt.Sprintf("RPush %s  failed, ", mq.keyName, result.Err().Error()))
		return nil
	}

	data, _ := result.Bytes()
	return proto.Decode(data)
}

func (mq *MsgQueue) Remove(msgId uint64) (bool) {
	//err := mq.redisClient.RPush(mq.keyName, msg).Err()
	//if err != nil {
	//	logging.Log(fmt.Sprintf("RPush %s  failed, ", keyName, err.Error()))
	//	return false
	//}

	return true
}