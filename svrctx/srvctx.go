package svrctx

import (
	"fmt"
	"sync"
	"time"
	"crypto/md5"
	"crypto/sha1"
	"../proto"
	"github.com/jackc/pgx"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"os"
)

type  DBConfig struct {
	DBHost string
	DBPort uint16
 	DBName string
	DBUser string
	DBPasswd string
	DBPoolMaxConn int
}

type ServerContext struct {
	WaitLock      *sync.WaitGroup
	ProcessName   string
	MasterListenAddrPort string
	BindAddr      string
	Port          uint16
	WSPort        uint16
	AcceptTimeout time.Duration
	RecvTimeout   time.Duration
	RedisAddr     string
	RedisPort     uint16
	RedisPassword string
	RedisDeviceMQKeyPrefix string
	RedisAppMQKeyPrefix string

	PasswordSalt  string
	SessionSecret string

	DbMysqlConfig      DBConfig
	DbPgsqlConfig      DBConfig

	PGPool *pgx.ConnPool
	MySQLPool *sql.DB
	UseGoogleMap  bool

	AppServerChan chan *proto.AppMsgData
	TcpServerChan chan *proto.MsgData
	ServerExit chan struct{}
}

var serverCtx ServerContext

func init()  {
	fmt.Println("server contextinit...")
	serverCtx.ProcessName = "go-server"
	serverCtx.MasterListenAddrPort = "localhost:9015"

	serverCtx.BindAddr = "0.0.0.0"
	serverCtx.Port = 7015
	serverCtx.WSPort = 8015

	serverCtx.AcceptTimeout = 30
	serverCtx.RecvTimeout = 30

	serverCtx.RedisAddr = "127.0.0.1"
	serverCtx.RedisPort = 6379
	serverCtx.RedisPassword = ""
	serverCtx.RedisDeviceMQKeyPrefix = "mq:device:"
	serverCtx.RedisAppMQKeyPrefix = "mq:app:"

	hs := sha1.New()
	hs.Write([]byte("service.gatorcn.com"))
	serverCtx.PasswordSalt = fmt.Sprintf("%x", hs.Sum(nil))

	m := md5.New()
	m.Write([]byte("com.gatorcn.service"))
	serverCtx.SessionSecret = fmt.Sprintf("%x",m.Sum(nil))
	serverCtx.DbMysqlConfig = DBConfig{
		DBHost:"127.0.0.1",
		DBPort:3306,
		DBName:"gpsbaseinfo",
		DBUser:"root",
		DBPasswd:"1234",
		DBPoolMaxConn: 1024,
	}

	serverCtx.DbPgsqlConfig = DBConfig{
		DBHost:"127.0.0.1",
		DBPort:5432,
		DBName:"gator_db",
		DBUser:"postgres",
		DBPasswd:"",
		DBPoolMaxConn: 1024,
	}

	serverCtx.UseGoogleMap = true

	//创建postgresql连接池
	var err error
	pgconfig := pgx.ConnConfig{}
	pgconfig.Host = serverCtx.DbPgsqlConfig.DBHost
	pgconfig.Port = serverCtx.DbPgsqlConfig.DBPort
	pgconfig.Database = serverCtx.DbPgsqlConfig.DBName
	pgconfig.User = serverCtx.DbPgsqlConfig.DBUser
	pgconfig.Password = serverCtx.DbPgsqlConfig.DBPasswd
	serverCtx.PGPool, err = pgx.NewConnPool(pgx.ConnPoolConfig{ConnConfig: pgconfig,
		MaxConnections: serverCtx.DbPgsqlConfig.DBPoolMaxConn,
		AcquireTimeout: 0,
	})

	if err != nil {
		fmt.Println("create pg connection pool failed, ", err)
		os.Exit(1)
	}else{
		fmt.Println("create pg connection pool OK")
	}

	//创建MySQL连接池
	strConn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		serverCtx.DbMysqlConfig.DBUser,
		serverCtx.DbMysqlConfig.DBPasswd,
		serverCtx.DbMysqlConfig.DBHost,
		serverCtx.DbMysqlConfig.DBPort,
		serverCtx.DbMysqlConfig.DBName)

	serverCtx.MySQLPool, err = sql.Open("mysql", strConn)

	if err != nil {
		fmt.Println(fmt.Sprintf("connect mysql (%s) failed, %s", strConn, err.Error()))
		os.Exit(1)
	}
	serverCtx.MySQLPool.SetMaxOpenConns(serverCtx.DbMysqlConfig.DBPoolMaxConn)
	fmt.Println(fmt.Sprintf("connect mysql db %s OK", serverCtx.DbMysqlConfig.DBName))

	proto.LoadDeviceInfoFromDB(serverCtx.MySQLPool)

	//jsonData, err := ioutil.ReadFile("./gts_db.json")
	//if err != nil {
	//	fmt.Println("read gts_db.json failed", err)
	//	os.Exit(1)
	//}
	//
	// err = json.Unmarshal(jsonData, &serverCtx.Dbconfig)
	//if err != nil {
	//	fmt.Println("parse gts_db.json content failed", err)
	//	os.Exit(1)
	//}

	serverCtx.AppServerChan = make(chan *proto.AppMsgData, 1024)
	serverCtx.TcpServerChan = make(chan *proto.MsgData, 1024)

	serverCtx.ServerExit = make(chan struct{})

	serverCtx.WaitLock = &sync.WaitGroup{}
	serverCtx.WaitLock.Add(2)
}

func Get() *ServerContext {
	return &serverCtx
}
