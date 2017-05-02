package models

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/satori/go.uuid"
	"../logging"
	"fmt"
)

type User struct {
	id uint64
	name string
	passwordMD5 string
	accessToken string
	logined bool
}

func (u *User) GetAccessToken()  string{
	return u.accessToken
}

func (u *User) Add(db *sql.DB, name, passwordMD5 string) (errcode int) {
	return
}

func (u *User) Delete(db *sql.DB, id uint64) (errcode int) {
	return
}

func (u *User) Find(db *sql.DB, name string) (errcode int) {
	return
}

func (u *User) Update(db *sql.DB, id uint64) (errcode int) {
	return
}

func (u *User) Register(db *sql.DB, name, passwordMD5 string) ( errcode int) {
	logging.Log(fmt.Sprint(db, name, passwordMD5))
	if db == nil || name == "" || passwordMD5 == "" {
		logging.Log("bad params")
		errcode = BadParams
		return
	}

	errcode = u.Find(db, name)

	//已存在同名用户，注册失败，用户名重复
	if errcode == ResultOK {
		errcode = RecordAlreadyExist
		return
	}

	if errcode == RecordNotExist {
		//用户名未重复，可以注册
		errcode = u.Add(db, name, passwordMD5)
	}else {
		errcode = DBInternalError
	}

	return
}

func (u *User) Login(db *sql.DB, name, passwordMD5 string) (errcode int) {
	logging.Log(fmt.Sprint(db, name, passwordMD5))
	if db == nil || name == "" || passwordMD5 == "" {
		logging.Log("bad params")
		errcode = BadParams
		return
	}

	errcode = u.Find(db, name)

	//未查询到此用户
	if errcode == RecordNotExist {
		return
	}

	if errcode == ResultOK {
		//查询到此用户，对比密码的哈希值
		if passwordMD5 == u.passwordMD5 {
			//密码哈希值匹配，Login成功
			uuid.NewV4()
			u.logined =  true
			//u.accessToken =
		}else {

		}
	}else {
		errcode = DBInternalError
	}

	return
}
