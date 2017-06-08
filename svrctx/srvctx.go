package svrctx

import (
	"fmt"
	"sync"
	"time"
	"crypto/md5"
	"crypto/sha1"
	"../proto"
	"../logging"
	"github.com/jackc/pgx"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"os"
	"encoding/json"
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
	HttpServerName string
	WSPort        uint16
	HttpUploadURL string
	HttpStaticURL string
	HttpStaticDir string
	HttpStaticAvatarDir string
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
	IsDebug  bool

	AppServerChan chan *proto.AppMsgData
	TcpServerChan chan *proto.MsgData
	ServerExit chan struct{}
}

var serverCtx ServerContext

var DeviceTable = map[uint64]*proto.DeviceCache{}
var DeviceTableLock = &sync.RWMutex{}

var AppChatTaskTable = map[uint64]map[uint64]*proto.ChatTask{}
var AppChatTaskTableLock = &sync.RWMutex{}

var AppPhotoTaskTable = map[uint64]map[uint64]*proto.PhotoSettingTask{}
var AppPhotoTaskTableLock = &sync.RWMutex{}

func init()  {
	fmt.Println("server contextinit...")
	serverCtx.ProcessName = "go-server"
	serverCtx.MasterListenAddrPort = "localhost:9015"

	serverCtx.BindAddr = "0.0.0.0"
	serverCtx.Port = 7015

	serverCtx.HttpServerName = "http://192.168.3.97"
	serverCtx.WSPort = 8015
	serverCtx.HttpUploadURL = "/api/upload"
	serverCtx.HttpStaticURL = "/static"
	serverCtx.HttpStaticDir = "./static"
	serverCtx.HttpStaticAvatarDir = "/upload/avatar/"

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
	serverCtx.IsDebug = true

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


func GetChatData(imei uint64)  []proto.ChatInfo {
	chatData := []proto.ChatInfo{}
	DeviceTableLock.RLock()
	device, ok := DeviceTable[imei]
	if ok {
		if len(device.ChatCache) > 0 {
			for _, chat := range device.ChatCache {
				chatData = append(chatData, chat)
			}
		}
	}
	DeviceTableLock.RUnlock()

	return chatData
}

func AddChatData(imei uint64, chatData proto.ChatInfo) {
	DeviceTableLock.Lock()
	_, ok := DeviceTable[imei]
	if ok {
		DeviceTable[imei].ChatCache = append(DeviceTable[imei].ChatCache, chatData)
	}else {
		DeviceTable[imei] = &proto.DeviceCache{}
		DeviceTable[imei].Imei = imei
		DeviceTable[imei].ChatCache = append(DeviceTable[imei].ChatCache, chatData)
	}
	DeviceTableLock.Unlock()
}

func GetDeviceData(imei uint64, pgpool *pgx.ConnPool)  proto.LocationData {
	isQueryDB := false
	deviceData := proto.LocationData{Imei: imei}
	DeviceTableLock.RLock()
	device, ok := DeviceTable[imei]
	if ok {
		deviceData = device.CurrentLocation
	}else {
		isQueryDB = true
	}
	DeviceTableLock.RUnlock()

	if isQueryDB == false {
		return deviceData
	}else {
		//缓存中没有数据，将从数据库中查询
		strSQL := fmt.Sprintf("select * from  device_location where imei=%d order by location_time desc limit 1", imei)
		logging.Log("sql: " + strSQL)
		rows, err := pgpool.Query(strSQL)
		if err != nil {
			logging.Log(fmt.Sprintf("[%d] pg query failed, %s",  imei, err.Error()))
			return deviceData
		}

		if rows.Next() {
			values , err := rows.Values()
			logging.Log(fmt.Sprint("get device data: ", values, err))
			// [357593060571398 20170524141830 29.566889 106.45424 map[datatime:1.7052414183e+11 zoneAlarm:0 zoneIndex:0 lat:29.566888 steps:0 battery:3 readflag:0 zoneName: locateType:1 org_battery:5 lng:106.454239 alarm:2 accracy:0]]
			//deviceData.DataTime = uint64(values[1].(int64))
			//deviceData.Lat = float64(values[2].(float32))
			//deviceData.Lng = float64(values[3].(float32))
			//fmt.Println("deviceData: ", deviceData)
			jsonStr, _ := json.Marshal(values[4])
			json.Unmarshal(jsonStr, &deviceData)
			fmt.Println("deviceData: ", deviceData)
			deviceData.Imei = imei

			DeviceTableLock.Lock()
			_, ok := DeviceTable[imei]
			if ok {
				DeviceTable[imei].CurrentLocation =  deviceData
			}else {
				DeviceTable[imei] = &proto.DeviceCache{}
				DeviceTable[imei].Imei = imei
				DeviceTable[imei].CurrentLocation =  deviceData
			}
			DeviceTableLock.Unlock()

		}else {
			logging.Log(fmt.Sprintf("[%d] get device data: no data in db", imei))
		}

		return deviceData
	}
}

func SetDeviceData(imei uint64, updateType int, deviceData proto.LocationData) {
	DeviceTableLock.Lock()
	switch updateType {
	case proto.DEVICE_DATA_LOCATION:
		_, ok := DeviceTable[imei]
		if ok == false {
			DeviceTable[imei] = &proto.DeviceCache{Imei: imei}
		}
		DeviceTable[imei].CurrentLocation = deviceData
	case proto.DEVICE_DATA_STATUS:
		device, ok := DeviceTable[imei]
		if ok {
			device.CurrentLocation.LocateType = deviceData.LocateType
			device.CurrentLocation.DataTime = deviceData.DataTime
			device.CurrentLocation.Steps = deviceData.Steps
		}else{
			DeviceTable[imei].CurrentLocation = deviceData
		}
	case proto.DEVICE_DATA_BATTERY:
		device, ok := DeviceTable[imei]
		if ok {
			device.CurrentLocation.Battery = deviceData.Battery
			device.CurrentLocation.OrigBattery = deviceData.OrigBattery
		}else{
			DeviceTable[imei].CurrentLocation = deviceData
		}
	}
	DeviceTableLock.Unlock()
}