package flyctl

import (
	jww "github.com/spf13/jwalterweatherman"
)

type Logger struct {
}

func init() {
	jww.SetStdoutThreshold(jww.LevelTrace)
}
