package signal

import (
	"../svrctx"
	"../logging"
)

func init()  {
	logging.Log("signal package init for " + svrctx.Get().ProcessName)
}