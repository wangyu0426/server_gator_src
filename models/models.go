package models

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"../svrctx"
	"../logging"
	"fmt"
	"os"
)

const (
	ResultOK = iota
	BadParams
	DBInternalError
	RecordNotExist
	RecordAlreadyExist

)

func init()  {
	//[username[:password]@][protocol[(address)]]/dbname

	for i := uint32(0); i < svrctx.Get().Dbconfig.DBShards; i++ {
		strConn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s%02d",
			svrctx.Get().Dbconfig.DBUser,
			svrctx.Get().Dbconfig.DBPasswd,
			svrctx.Get().Dbconfig.DBHost,
			svrctx.Get().Dbconfig.DBPort,
			svrctx.Get().Dbconfig.DBName, i)
		logging.Log("connect mysql string: " + strConn)
		db, err := sql.Open("mysql", strConn)

		if err != nil {
			logging.Log(fmt.Sprintf("connect mysql db %s%02d failed", svrctx.Get().Dbconfig.DBName, i) + err.Error())
			os.Exit(1)
		}

		logging.Log(fmt.Sprintf("connect mysql db %s%02d OK", svrctx.Get().Dbconfig.DBName, i))
		defer db.Close()

		createTables(db)
	}
}

func PrintSelf()  {
	fmt.Println("this is models packege!")
}

func createTables(db *sql.DB)  {
	for i := 0; i < len(svrctx.Get().Dbconfig.Tables); i++ {
		for j := uint32(0); j < svrctx.Get().Dbconfig.Tables[i].TableShards; j++ {
			strSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS Cube%d_%02d  (FUin INT UNSIGNED NOT NULL, ",
				svrctx.Get().Dbconfig.Tables[i].TableId, j)
			strTmp := ""
			for k := 0; k < len(svrctx.Get().Dbconfig.Tables[i].Fields); k++ {
				switch svrctx.Get().Dbconfig.Tables[i].Fields[k].FieldType {
				case "int64" :
					strTmp = fmt.Sprintf("FField%d BIGINT NOT NULL, ", k)
				case "int" :
					strTmp = fmt.Sprintf("FField%d INT NOT NULL, ", k)
				case "smallint" :
					strTmp = fmt.Sprintf("FField%d SMALLINT NOT NULL, ", k)
				case "uint" :
					strTmp = fmt.Sprintf("FField%d INT UNSIGNED NOT NULL, ", k)
				case "char" :
					strTmp = fmt.Sprintf("FField%d CHAR(%d) NOT NULL, ", k, svrctx.Get().Dbconfig.Tables[i].Fields[k].FieldLen);
				}
				strSQL += strTmp
			}

			strSQL +=  "PRIMARY KEY("

			for j  := uint32(0); j < svrctx.Get().Dbconfig.Tables[i].KeyNum; j++ {
				strTmp := fmt.Sprintf("FField%d", j)
				strSQL += strTmp
				strSQL += ", "
			}

			strSQL += "FUin)) ENGINE=INNODB;"
			//logging.Log(strSQL)

			_, err := db.Exec(strSQL)
			if err != nil {
				logging.Log("mysql exec failed, " + err.Error())
				os.Exit(1)
			}

			//lastId, _ := result.LastInsertId()
			//affectedRows, _ := result.RowsAffected()
			//logging.Log(fmt.Sprintf("last id inserted:  %d, RowsAffected: %d", lastId,  affectedRows))
		}
	}
}