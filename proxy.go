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

	exit := make(chan int) // 防止下面这个go泄露
	defer close(exit)
	go func() {
		select {
		case <-ctx.Done():
			p.logger.Printf("ctx done, close listener")
		case <-exit:
		}
		p.logger.Printf("listener close err:%s", formatErr(listener.Close()))
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
				// fixme  啥情况下ctx没有done(listener没close) 但是accept出错? 咋处理?
				// fixme 到这里以后在跑的p.handle要不要都退出?
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

	// fixme 感觉这里的关闭不是很优雅
	closeConn := func() {
		conn.Close()
		rconn.Close()
	}

	var wgc sync.WaitGroup
	wgc.Add(2)

	warpCopy := func(connID int, dst *net.TCPConn, src *net.TCPConn, connLogger *HeadLogger) {
		defer func() {
			select {
			case <-ctx.Done():
				//到这里表示ctx结束 不需要再closeConn
			default:
				closeConn()
			}
			wgc.Done()
		}()

		wrote, er := io.Copy(io.MultiWriter(dst, connLogger), src)
		end := "client"
		if src.LocalAddr().String() == p.laddr.String() {
			end = "backend"
		}
		cl.Printf("Summary: copy to %s %d bytes, exit err:%s", end, wrote, formatErr(er))
	}

	go warpCopy(clientID, conn, rconn, NewHeadLogger("send:", cl.Printf)) // 从远端收数据发给客户端
	go warpCopy(clientID, rconn, conn, NewHeadLogger("recv:", cl.Printf)) // 从客户端收数据发给远端
	// 这里send的意思是proxy发数据给client， recv的意思是从指proxy从client收到数据

	exit := make(chan int) // 防止下面这个go泄露
	defer close(exit)
	go func() {
		select {
		case <-ctx.Done():
			cl.Printf("ctx done, close conn")
			closeConn()
		case <-exit: // 到这里表示warpCopy出错退出了 不需要再closeConn
		}
	}()
	wgc.Wait()
	cl.Printf("exit")
}
