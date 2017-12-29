package svrctx

import (
	"fmt"
	"sync"
	"time"
	//"crypto/md5"
	//"crypto/sha1"
	"../proto"
	"../logging"
	"github.com/jackc/pgx"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"os"
	"encoding/json"
	"io/ioutil"
	//"../tcpserver"
)

type  DBConfig struct {
	DBHost string
	DBPort uint16
 	DBName string
	DBUser string
	DBPasswd string
	DBPoolMaxConn int
}

type DeviceRedirectApiInfo struct {
	CompanyName string
	AccessToken string
}

type ServerContext struct {
	WaitLock      *sync.WaitGroup  `json:"-"`
	ProcessName   string
	MasterListenAddrPort string
	BindAddr      string
	Gt3Port          uint16
	Port          uint16
	UseHttps bool
	HttpServerName string
	WSPort        uint16
	WSSPort        uint16
	HttpUploadURL string
	HttpStaticURL string
	HttpStaticDir string
	HttpStaticAvatarDir string
	HttpStaticMinichatDir string
	APNSServerApiBase string
	LocalAPIBindAddr      string
	LocalAPIPort          uint16
	AcceptTimeout time.Duration
	RecvTimeout   time.Duration
	MaxDeviceIdleTimeSecs int
	MaxPushDataTimeSecs int
	MinPushDataDelayTimeSecs int
	BackgroundCleanerDelayTimeSecs int
	MaxMinichatKeepTimeSecs int
	RedisAddr     string
	RedisPort     uint16
	RedisPassword string
	RedisDeviceMQKeyPrefix string
	RedisAppMQKeyPrefix string

	PasswordSalt  string
	SessionSecret string

	DbMysqlConfig      DBConfig
	DbPgsqlConfig      DBConfig

	RedirectApiCompanyList []DeviceRedirectApiInfo

	PGPool *pgx.ConnPool  `json:"-"`
	MySQLPool *sql.DB  `json:"-"`
	UseGoogleMap  bool
	IsDebug  bool
	IsDebugLocal  bool
	LocalDebugHttpServerName  string

	AndroidAppURL string
	IOSAppURL string

	//channel that communicates with phone app
	AppServerChan chan *proto.AppMsgData  `json:"-"`
	//app has updated or setted device ,so send msg to device
	TcpServerChan chan *proto.MsgData `json:"-"`
	GT6TcpServerChan chan *proto.MsgData `json:"-"`
	GT3TcpServerChan chan *proto.MsgData `json:"-"`
	ServerExit chan struct{} `json:"-"`
}

var serverCtx ServerContext

var DeviceTable = map[uint64]*proto.DeviceCache{}
var DeviceTableLock = &sync.Mutex{}

func init()  {
	fmt.Println("server contextinit...")

	//serverCtx.ProcessName = "go-server"
	//serverCtx.MasterListenAddrPort = "localhost:9015"
	//
	//serverCtx.BindAddr = "0.0.0.0"
	//serverCtx.Port = 7016
	//
	//serverCtx.HttpServerName = "http://watch.gatorcn.com"  //"http://192.168.3.97"
	//serverCtx.WSPort = 8015
	//serverCtx.HttpUploadURL = "/api/upload"
	//serverCtx.HttpStaticURL = "/static"
	//serverCtx.HttpStaticDir = "./static"
	//serverCtx.HttpStaticAvatarDir = "/upload/avatar/"
	//
	//serverCtx.AcceptTimeout = 30
	//serverCtx.RecvTimeout = 30
	//serverCtx.MaxDeviceIdleTimeSecs = 60
	//
	//serverCtx.RedisAddr = "127.0.0.1"
	//serverCtx.RedisPort = 6379
	//serverCtx.RedisPassword = ""
	//serverCtx.RedisDeviceMQKeyPrefix = "mq:device:"
	//serverCtx.RedisAppMQKeyPrefix = "mq:app:"
	//
	//hs := sha1.New()
	//hs.Write([]byte("service.gatorcn.com"))
	//serverCtx.PasswordSalt = fmt.Sprintf("%x", hs.Sum(nil))
	//
	//m := md5.New()
	//m.Write([]byte("com.gatorcn.service"))
	//serverCtx.SessionSecret = fmt.Sprintf("%x",m.Sum(nil))
	//serverCtx.DbMysqlConfig = DBConfig{
	//	DBHost:"127.0.0.1",
	//	DBPort:3306,
	//	DBName:"gpsbaseinfo",
	//	DBUser:"root",
	//	DBPasswd:"",
	//	DBPoolMaxConn: 1024,
	//}
	//
	//serverCtx.DbPgsqlConfig = DBConfig{
	//	DBHost:"127.0.0.1",
	//	DBPort:5432,
	//	DBName:"gator_db",
	//	DBUser:"postgres",
	//	DBPasswd:"",
	//	DBPoolMaxConn: 1024,
	//}
	//
	//serverCtx.UseGoogleMap = true
	//serverCtx.IsDebug = true

	confJson, err0 := ioutil.ReadFile("config.json")
	if err0 != nil {
		fmt.Println("read server config file failed, ", err0.Error())
		os.Exit(-1)
	}

	err0 = json.Unmarshal(confJson, &serverCtx)
	if err0 != nil {
		fmt.Println("parse config file failed, ", err0.Error())
		os.Exit(-1)
	}

	if serverCtx.IsDebugLocal {
		serverCtx.HttpServerName = serverCtx.LocalDebugHttpServerName
	}

	fmt.Println("serverCtx.HttpServerName: ", serverCtx.HttpServerName, serverCtx.RedirectApiCompanyList)

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

	//confString, err2 := json.Marshal(&serverCtx)
	//if err2 != nil {
	//	fmt.Println("make json for server ctx failed, ", err2.Error())
	//	os.Exit(1)
	//}
	//
	//ioutil.WriteFile("config.json", confString, 0666)
	//os.Exit(1)

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

	serverCtx.AppServerChan = make(chan *proto.AppMsgData, 10240)
	serverCtx.TcpServerChan = make(chan *proto.MsgData, 10240)
	serverCtx.GT6TcpServerChan = make(chan *proto.MsgData, 10240)
	serverCtx.GT3TcpServerChan = make(chan *proto.MsgData, 10240)

	serverCtx.ServerExit = make(chan struct{})

	serverCtx.WaitLock = &sync.WaitGroup{}
	serverCtx.WaitLock.Add(2)
}

func Get() *ServerContext {
	return &serverCtx
}


func GetChatData(imei uint64, index int)  []proto.ChatInfo {
	chatData := []proto.ChatInfo{}
	proto.AppSendChatListLock.Lock()
	chatList, ok := proto.AppSendChatList[imei]
	if ok {
		if len(*chatList) > 0 {
			for i, chat := range *chatList {
				if index == -1 ||  index == i {
					chatData = append(chatData, (*chat).Info)
				}

				if index == i {
					break
				}
			}
		}
	}
	proto.AppSendChatListLock.Unlock()

	return chatData
}

func GetPhotoData(imei uint64, index int)  []proto.PhotoSettingInfo {
	photosData := []proto.PhotoSettingInfo{}
	proto.AppNewPhotoListLock.Lock()
	photoList, ok := proto.AppNewPhotoList[imei]
	if ok {
		if len(*photoList) > 0 {
			for i, photo := range *photoList {
				if index == -1 ||  index == i {
					photosData = append(photosData, (*photo).Info)
				}

				if index == i {
					break
				}
			}
		}
	}
	proto.AppNewPhotoListLock.Unlock()

	return photosData
}

func IsPhoneNumberInFamilyList(imei uint64, phone string) bool {
	isInvalid := false
	proto.DeviceInfoListLock.Lock()
	device, ok := (*proto.DeviceInfoList)[imei]
	if ok && device != nil {
		for _, member := range device.Family{
			logging.Log(fmt.Sprintf("[%d]member phone %s, to match %s", imei, member.Phone,  phone))
			if member.Phone == phone {
				isInvalid = true
				break
			}
		}
	}else{
		logging.Log(fmt.Sprintf("[%d] not found phone %s", imei, phone))
	}
	proto.DeviceInfoListLock.Unlock()

	return isInvalid
}

func PushPhotoNum(imei uint64) bool {
	photoData := GetPhotoData(imei, -1)
	if len(photoData) > 0 {
		//通知终端有新的图片设置信息
		serverCtx.TcpServerChan <- proto.MakeReplyMsg(imei, false,
			proto.MakeFileNumReplyMsg(imei, proto.ChatContentPhoto, len(photoData),  true),
			proto.NewMsgID())
	}

	return true
}

func PushChatNum(imei uint64) bool {
	//get the count of the minchat,notify the watch to download the minchat with cmd AP11
	chatData := GetChatData(imei, -1)
	if len(chatData) > 0 {
		//if the connect is unlinked,then send the whole chat with once AP11,not send AP11 everytime
		//_,ok := tcpserver.TcpClientTable[imei]
		//if ok {
			//通知终端有聊天信息
			model := proto.GetDeviceModel(imei)
			if model == proto.DM_GT06 {
				msg := proto.MakeReplyMsg(imei, false,
					proto.MakeFileNumReplyMsg(imei, proto.ChatContentVoice, len(chatData), true),
					proto.NewMsgID())
				msg.Header.Header.Cmd = proto.CMD_AP11
				serverCtx.TcpServerChan <- msg
			} else if model == proto.DM_GTI3 || model == proto.DM_GT03 {
				msg := proto.MakeReplyMsg(imei, false,
					proto.MakeFileNumReplyMsg(imei, proto.ChatContentVoice, len(chatData), false),
					proto.NewMsgID())
				msg.Header.Header.Cmd = proto.CMD_GT3_AP15_PUSH_CHAT_COUNT
				serverCtx.TcpServerChan <- msg
			}
		//}
	}

	return true
}

func AddPendingPhotoData(imei uint64, photoData proto.PhotoSettingInfo) {
	logging.Log("AddPendingPhotoData:1")
	photoTask := proto.PhotoSettingTask{Info: photoData}
	proto.AppNewPhotoPendingListLock.Lock()
	photoList, ok := proto.AppNewPhotoPendingList[imei]
	if ok {
		*photoList = append(*photoList, &photoTask)
	}else {
		proto.AppNewPhotoPendingList[imei] = &[]*proto.PhotoSettingTask{}
		*proto.AppNewPhotoPendingList[imei] = append(*proto.AppNewPhotoPendingList[imei], &photoTask)
	}
	proto.AppNewPhotoPendingListLock.Unlock()
}

func AddChatData(imei uint64, chatData proto.ChatInfo) {
	chatTask := proto.ChatTask{Info: chatData}
	proto.AppSendChatListLock.Lock()
	chatList, ok := proto.AppSendChatList[imei]
	if ok {
		*chatList = append(*chatList, &chatTask)
	}else {
		proto.AppSendChatList[imei] = &[]*proto.ChatTask{}
		*proto.AppSendChatList[imei] = append(*proto.AppSendChatList[imei], &chatTask)
		logging.Log(fmt.Sprintf("%d after append AppSendChatList len: %d", imei, len(*proto.AppSendChatList[imei])))
	}
	proto.AppSendChatListLock.Unlock()

	PushChatNum(imei)
}

func GetDeviceData(imei uint64, pgpool *pgx.ConnPool)  proto.LocationData {
	isQueryDB := false
	deviceData := proto.LocationData{Imei: imei}
	DeviceTableLock.Lock()
	device, ok := DeviceTable[imei]
	if ok {
		deviceData = device.CurrentLocation
	}else {
		isQueryDB = true
	}
	DeviceTableLock.Unlock()

	if isQueryDB == false {
		deviceData.Imei = imei
		return deviceData
	}else {
		//缓存中没有数据，将从数据库中查询
		strSQL := fmt.Sprintf("select * from  gator3_device_location where imei=%d order by location_time desc limit 1", imei)
		logging.Log("sql: " + strSQL)
		rows, err := pgpool.Query(strSQL)
		if err != nil {
			logging.Log(fmt.Sprintf("[%d] pg query failed, %s",  imei, err.Error()))
			deviceData.Imei = imei
			return deviceData
		}

		defer rows.Close()

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

		deviceData.Imei = imei
		return deviceData
	}
}


func QueryLocations(imei uint64, pgpool *pgx.ConnPool, beginTime, endTime uint64, lbs bool, alarmOnly bool)  *[]proto.LocationData {
	if imei == 0 || pgpool == nil || endTime <= beginTime {
		logging.Log(fmt.Sprintf("[imei: %d] bad input parms: %d, %d", imei, beginTime, endTime))
		return nil
	}

	locations := []proto.LocationData{}
	strSQL := ""
	if alarmOnly{
		strSQL = fmt.Sprintf("select * from  gator3_device_location where imei=%d and location_time >= %d and location_time <= %d  " +
			" and data->'alarm' != '0'  " +
			" order by location_time ", imei, beginTime, endTime)
	}else{
		if lbs{
			strSQL = fmt.Sprintf("select * from  gator3_device_location where imei=%d and location_time >= %d and location_time <= %d " +
				" order by location_time ", imei, beginTime, endTime)
		}else{
			strSQL = fmt.Sprintf("select * from  gator3_device_location where imei=%d and location_time >= %d and location_time <= %d  " +
				" and (data @> '{\"locateType\": 1}' or data @> '{\"locateType\": 3}')  " +
				" order by location_time ", imei, beginTime, endTime)
		}
	}

	logging.Log("sql: " + strSQL)
	rows, err := pgpool.Query(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] pg query failed, %s",  imei, err.Error()))
		return nil
	}

	defer rows.Close()

	for rows.Next() {
		values , _ := rows.Values()
		//logging.Log(fmt.Sprint("query location data: ", values, err))
		locationItem := proto.LocationData{}
		// [357593060571398 20170524141830 29.566889 106.45424 map[datatime:1.7052414183e+11 zoneAlarm:0 zoneIndex:0 lat:29.566888 steps:0 battery:3 readflag:0 zoneName: locateType:1 org_battery:5 lng:106.454239 alarm:2 accracy:0]]
		//deviceData.DataTime = uint64(values[1].(int64))
		//deviceData.Lat = float64(values[2].(float32))
		//deviceData.Lng = float64(values[3].(float32))
		//fmt.Println("deviceData: ", deviceData)
		jsonStr, _ := json.Marshal(values[4])
		json.Unmarshal(jsonStr, &locationItem)
		//fmt.Println("locationItem: ", locationItem)
		locationItem.Imei = imei
		locations = append(locations, locationItem)
	}

	if len(locations) == 0{
		logging.Log(fmt.Sprintf("[%d] query location: no data in db", imei))
		return nil
	}else{
		return &locations
	}
}


func DeleteAlarms(imei uint64, pgpool *pgx.ConnPool, beginTime, endTime uint64)  bool {
	if imei == 0 || pgpool == nil || endTime < beginTime {
		logging.Log(fmt.Sprintf("[imei: %d] bad input parms: %d, %d", imei, beginTime, endTime))
		return false
	}

	strSQL := fmt.Sprintf("delete from  gator3_device_location where imei=%d and location_time >= %d and location_time <= %d  " +
			" and data->'alarm' != '0'  ", imei, beginTime, endTime)

	logging.Log("sql: " + strSQL)
	_, err := pgpool.Exec(strSQL)
	if err != nil {
		logging.Log(fmt.Sprintf("[%d] pg exec failed, %s",  imei, err.Error()))
		return false
	}

	return true
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
		if ok && device != nil {
			device.CurrentLocation.LocateType = deviceData.LocateType
			device.CurrentLocation.DataTime = deviceData.DataTime
			device.CurrentLocation.Steps = deviceData.Steps
			device.CurrentLocation.AlarmType = deviceData.AlarmType
			device.CurrentLocation.Battery = deviceData.Battery
		}else{
			DeviceTable[imei] = &proto.DeviceCache{Imei: imei}
			DeviceTable[imei].CurrentLocation = deviceData
		}
	case proto.DEVICE_DATA_BATTERY:
		device, ok := DeviceTable[imei]
		if ok && device != nil {
			device.CurrentLocation.Battery = deviceData.Battery
			device.CurrentLocation.OrigBattery = deviceData.OrigBattery
		}else{
			DeviceTable[imei] = &proto.DeviceCache{Imei: imei}
			DeviceTable[imei].CurrentLocation = deviceData
		}
	}
	DeviceTableLock.Unlock()
}