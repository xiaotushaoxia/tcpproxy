# tcpproxy

# 使用方法

go get github.com/xiaotushaoxia/tcpproxy
或者
require github.com/xiaotushaoxia/tcpproxy

`
package main

import (
	"fmt"
	"github.com/xiaotushaoxia/tcpproxy"
	"log"
	"sync"
)

func main() {
	proxy, er := tcpproxy.NewProxy(
		"127.0.0.1:1515",
		"127.0.0.1:1516",
		tcpproxy.NewHeadLogger(fmt.Sprintf("[Proxy %s to %s]", "127.0.0.1:1515", "127.0.0.1:1516"), log.Printf),
	)
	if er != nil {
		log.Printf("new proxy error: %s", er)
		return
	}
	ctx := tcpproxy.SignalControlCtx()
	var wg sync.WaitGroup
	proxy.Run(ctx, &wg)
	wg.Wait()
}
`
