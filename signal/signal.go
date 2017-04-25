package signal

import (
	"../svrctx"
	"../logger"
)

func init()  {
	logger.Log("signal package init for " + svrctx.Get().ProcessName)
}