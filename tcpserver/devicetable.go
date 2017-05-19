package tcpserver

type DeviceLocation struct {
	DateTime uint64
	Lat float64
	Lng float64
	Steps uint32
	Battery uint8
	AlarmType uint8
	LocateType uint8
	ReadFlag uint8
	States [8]uint8
}

type ChatInfo struct {
	DateTime uint64
	SenderType uint8
	Sender []byte
	ReceiverType uint8
	Receiver []byte
	ContentType uint8
	Content []byte
}

type DeviceCache struct {
	Imei uint64
	CurrentLocation DeviceLocation
	AlarmCache []DeviceLocation
	ChatCache []ChatInfo
}