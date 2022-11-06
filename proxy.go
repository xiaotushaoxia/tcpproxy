package tcpproxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
)

type Config struct {
	ProxyPairs []ProxyPair `json:"proxy_pairs"`
}

type ProxyPair struct {
	LocalAddr  string `json:"local_addr"`
	RemoteAddr string `json:"remote_addr"`
}

func NewProxy(laddrs, raddrs string, logger ProxyLogger) (*Proxy, error) {
	raddr, err := net.ResolveTCPAddr("tcp", raddrs)
	if err != nil {
		return nil, err
	}

	laddr, err := net.ResolveTCPAddr("tcp", laddrs)
	if err != nil {
		return nil, err
	}
	return &Proxy{
		logger: logger,
		laddr:  laddr,
		raddr:  raddr,
	}, nil
}

type Proxy struct {
	logger ProxyLogger
	laddr  *net.TCPAddr
	raddr  *net.TCPAddr
}

func (p *Proxy) Run(ctx context.Context, wg *sync.WaitGroup) {
	p.logger.Printf("listen: %s", p.laddr)
	listener, err := net.ListenTCP("tcp", p.laddr)
	if err != nil {
		p.logger.Printf("listen err: %s", err)
		return
	}
	var er error
	exit := make(chan int) // 防止下面这个go泄露
	go func() {
		select {
		case <-ctx.Done():
			p.logger.Printf("ctx done, close listener")
			er = listener.Close()
			p.logger.Printf("listener close err:%s", formatErr(er))
		case <-exit:
		}
	}()

	connID := 0
	for {
		conn, er2 := listener.AcceptTCP()
		if er2 != nil {
			p.logger.Printf("Accept err: %s", er2)
			select {
			case <-ctx.Done():
				p.logger.Printf("ctx cancel, proxy exit")
				return
			default:
				// fixme  啥情况下ctx没有done 但是accept出错? 咋处理?
				close(exit)
				return
			}
		}
		connID++
		wg.Add(1)
		go p.handle(ctx, wg, connID, conn)
	}
}

func (p *Proxy) handle(ctx context.Context, wg *sync.WaitGroup, clientID int, conn *net.TCPConn) {
	defer wg.Done()
	cl := NewHeadLogger(fmt.Sprintf("[ClientID:%d]", clientID), p.logger.Printf) // conn logger
	cl.Printf("New Client. RemoteAddr:%s, LocalAddr:%s", conn.RemoteAddr(), conn.LocalAddr())
	rconn, err := net.DialTCP("tcp", nil, p.raddr)
	if err != nil {
		cl.Printf("Remote connection failed:%s", err)
		conn.Close()
		return
	}
	cl.Printf("ProxyConn RemoteAddr:%s, ProxyConn LocalAddr:%s", rconn.RemoteAddr(), rconn.LocalAddr())

	var once1 sync.Once
	var once2 sync.Once
	// fixme 感觉这里的关闭不是很优雅
	closeConn := func() {
		once2.Do(func() {
			conn.Close()
		})
		once1.Do(func() {
			rconn.Close()
		})
	}

	warpCopy := func(connID int, dst *net.TCPConn, src *net.TCPConn, connLogger *HeadLogger) {
		defer closeConn()
		wrote, er := io.Copy(io.MultiWriter(dst, connLogger), src)
		if src.LocalAddr().String() == p.laddr.String() {
			cl.Printf("Summary: copy to backend %d bytes, exit err:%s", wrote, formatErr(er))
		} else {
			cl.Printf("Summary: copy to client %d bytes, exit err:%s", wrote, formatErr(er))
		}
	}

	var wgc sync.WaitGroup
	wgc.Add(2)
	go func() {
		defer wgc.Done()
		warpCopy(clientID, conn, rconn, NewHeadLogger("send:", cl.Printf)) // 从远端收数据发给客户端
	}()
	go func() {
		defer wgc.Done()
		warpCopy(clientID, rconn, conn, NewHeadLogger("recv:", cl.Printf)) // 从客户端收数据发给远端
	}()
	// 这里send的意思是proxy发数据给client， recv的意思是从指proxy从client收到数据

	exit := make(chan int) // 防止下面这个go泄露
	go func() {
		select {
		case <-ctx.Done():
			cl.Printf("ctx done, close conn")
			closeConn()
		case <-exit:
			return
		}
	}()
	wgc.Wait()
	close(exit)
	cl.Printf("exit")
}
