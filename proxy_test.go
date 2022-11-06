package tcpproxy

import (
	"fmt"
	"log"
	"sync"
	"testing"
)

func TestProxy(t *testing.T) {
	proxy, er := NewProxy(
		"127.0.0.1:1515",
		"127.0.0.1:1516",
		NewHeadLogger(fmt.Sprintf("[Proxy %s to %s]", "127.0.0.1:1515", "127.0.0.1:1516"), log.Printf),
	)
	if er != nil {
		log.Printf("new proxy error: %s", er)
		return
	}
	ctx := SignalControlCtx()
	var wg sync.WaitGroup
	proxy.Run(ctx, &wg)
	wg.Wait()
}

func TestNProxy(t *testing.T) {
	cfg := Config{}
	err := LoadJsonConfig(&cfg, "proxy.json")
	if err != nil {
		panic(err)
	}

	var proxys []*Proxy
	for _, pair := range cfg.ProxyPairs {
		proxy, er := NewProxy(
			pair.LocalAddr,
			pair.RemoteAddr,
			NewHeadLogger(fmt.Sprintf("[Proxy %s to %s]", pair.LocalAddr, pair.RemoteAddr), log.Printf),
		)
		if er != nil {
			log.Printf("new proxy error: %s", er)
			continue
		}
		proxys = append(proxys, proxy)
	}

	ctx := SignalControlCtx()
	var wg sync.WaitGroup
	wg.Add(len(proxys))
	for _, proxy := range proxys {
		go func(p *Proxy) {
			defer wg.Done()
			p.Run(ctx, &wg)
		}(proxy)
	}
	wg.Wait()
	log.Printf("exit")
}
