package proto

import (
	"strings"
	"fmt"
)

var alarmFlags = []uint8{
	ALARM_SOS,
	ALARM_BATTERYLOW,
	ALARM_NEW_MINICHAT,
	ALARM_DEVICE_DETACHED ,
	ALARM_OUTZONE,
	ALARM_INZONE,
}

var alarmTitles = map[string]map[uint8]string{
	"zh":  map[uint8]string{
		ALARM_INZONE: "进入安全区域",
		ALARM_SOS: "紧急报警",
		ALARM_OUTZONE : "出安全区域",
		ALARM_BATTERYLOW: "低电量报警",
		ALARM_NEW_MINICHAT: "新的微聊信息",
		ALARM_DEVICE_DETACHED: "手表脱离",
	},

	"en":  map[uint8]string{
		ALARM_INZONE: "Enter Safe Zone",
		ALARM_SOS: "SOS Alerts",
		ALARM_OUTZONE : "Out of Safe Zone",
		ALARM_BATTERYLOW: "Low battery alarm",
		ALARM_NEW_MINICHAT: "New Voice Message",
		ALARM_DEVICE_DETACHED: "Watch Detached",
	},
	"fr":map[uint8]string{
		ALARM_INZONE: "Enter Safe Zone",
		ALARM_SOS: "SOS Alerts",
		ALARM_OUTZONE : "Out of Safe Zone",
		ALARM_BATTERYLOW: "Low battery alarm",
		ALARM_NEW_MINICHAT: "New Voice Message",
		ALARM_DEVICE_DETACHED: "Watch Detached",
	},
}

func getInZoneName(zoneName string) string{
	//4home,1home,4school,1school,4tianhong,1tianhong
	//zoneName = "4home,1school"
	if strings.Contains(zoneName, ",") == false{
		return zoneName
	}

	names := strings.Split(zoneName, ",")
	if names != nil  && len(names)  >= 2{
		if names[0][0] == '1' {
			return names[0][1:]
		}else if names[1][0] == '1' {
			return names[1][1: ]
		}
	}else{
		return zoneName
	}

	return zoneName
}

func getOutZoneName(zoneName string) string{
	if strings.Contains(zoneName,",") == false{
		return zoneName
	}
	names := strings.Split(zoneName,",")
	if names != nil	&& len(names) >= 2{
		if names[0][0] == '4'{
			return names[0][1:]
		}else if names[0][4] == '4'{
			return names[0][1:]
		}
	}else {
		return  zoneName
	}

	return zoneName
}
func formatTime(datatime uint64)  string{
	str := fmt.Sprintf("20%012d", datatime)
	return fmt.Sprintf("%s:%s:%s",  str[8:10], str[10:12], str[12:14])
}
func MakePushContent(language string, pushInfo *pushJSON)  string{
	lang := "en"
	if len(language) >= 2 && language[0:2] == "zh" {
		lang = "zh"
	}

	content := ""
	alarm := pushInfo.AlarmType
	//alarm = 255
	for _, flg := range alarmFlags{
		if alarm == 0{
			break
		}

		if alarm & uint8(flg) != 0 {//包含此报警标志
			if content != "" {
				content += ","
			}

			if flg == ALARM_INZONE {
				content += alarmTitles[lang][uint8(flg)] + " " + getInZoneName(pushInfo.ZoneName)
			}else if flg == ALARM_OUTZONE {
				content += alarmTitles[lang][uint8(flg)] + " " +  getOutZoneName(pushInfo.ZoneName)
			}else{
				content += alarmTitles[lang][uint8(flg)]
			}

			//content += alarmTitles[lang][uint8(flg)] + pushInfo.ZoneName + " Zone Name"
		}
	}

	content = pushInfo.OwnerName + " " + formatTime(pushInfo.Time) + " " + content
	return content
}
