package svrctx

import (
	"fmt"
	"sync"
	"time"
	"encoding/json"
	"io/ioutil"
	"os"
	"crypto/md5"
	"crypto/sha1"
	"../proto"
)

type FieldConfig struct {
	FieldType string
	FieldLen int
}

type TableConfig struct {
	TableId uint32
	TableShards uint32
	KeyNum uint32
	Fields []FieldConfig
}

type  DBConfig struct {
	DBHost string
	DBPort uint16
 	DBName string
	DBUser string
	DBPasswd string
	DBShards uint32
	Tables []TableConfig
}

type ServerContext struct {
	WaitLock      *sync.WaitGroup
	ProcessName   string
	BindAddr      string
	Port          uint16
	WSPort        uint16
	AcceptTimeout time.Duration
	RecvTimeout   time.Duration
	RedisAddr     string
	RedisPort     uint16

	PasswordSalt  string
	SessionSecret string

	Dbconfig      DBConfig

	AppServerChan chan *proto.MsgData
	TcpServerChan chan *proto.MsgData
}

var serverCtx ServerContext

func init()  {
	fmt.Println("server contextinit...")
	serverCtx.ProcessName = "go-server"
	serverCtx.BindAddr = "0.0.0.0"
	serverCtx.Port = 7015
	serverCtx.WSPort = 8015

	serverCtx.AcceptTimeout = 30
	serverCtx.RecvTimeout = 30

	serverCtx.RedisAddr = "127.0.0.1"
	serverCtx.RedisPort = 6379

	hs := sha1.New()
	hs.Write([]byte("service.gatorcn.com"))
	serverCtx.PasswordSalt = fmt.Sprintf("%x", hs.Sum(nil))

	m := md5.New()
	m.Write([]byte("com.gatorcn.service"))
	serverCtx.SessionSecret = fmt.Sprintf("%x",m.Sum(nil))

	jsonData, err := ioutil.ReadFile("./gts_db.json")
	if err != nil {
		fmt.Println("read gts_db.json failed", err)
		os.Exit(1)
	}

	 err = json.Unmarshal(jsonData, &serverCtx.Dbconfig)
	if err != nil {
		fmt.Println("parse gts_db.json content failed", err)
		os.Exit(1)
	}

	serverCtx.AppServerChan = make(chan *proto.MsgData, 1024)
	serverCtx.TcpServerChan = make(chan *proto.MsgData, 1024)

	serverCtx.WaitLock = &sync.WaitGroup{}
	serverCtx.WaitLock.Add(2)
}

func Get() *ServerContext {
	return &serverCtx
}
