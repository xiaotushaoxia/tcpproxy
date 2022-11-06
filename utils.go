package tcpproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

type PrintFunc func(string, ...interface{})

type ProxyLogger interface {
	Printf(f string, v ...interface{})
	Write(p []byte) (int, error)
}

// HeadLogger 用来给PrintFunc前面加个公共的头
type HeadLogger struct {
	PrintFunc func(format string, v ...interface{})
	head      string
}

func NewHeadLogger(head string, pf PrintFunc) *HeadLogger {
	logger := HeadLogger{
		PrintFunc: pf,
		head:      head,
	}
	return &logger
}
func (cl *HeadLogger) Printf(f string, v ...interface{}) {
	cl.PrintFunc(cl.head+f, v...)
}

// 实现io.Writer 放在io.Copy里面可以看到传输的数据
func (cl *HeadLogger) Write(p []byte) (int, error) {
	cl.Printf("% X", p)
	return len(p), nil
}

func LoadJsonConfig(config interface{}, fileName string) error {
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		base := filepath.Base(fileName)
		var err2 error
		data, err2 = ioutil.ReadFile(base)
		if err2 != nil {
			return fmt.Errorf("%s %s", err, err2)
		}
	}
	if err = json.Unmarshal(data, config); err != nil {
		return err
	}
	return nil
}

func SignalControlCtx() context.Context {
	signalChan := make(chan os.Signal)
	signal.Notify(signalChan, syscall.SIGKILL, syscall.SIGTERM, syscall.SIGINT)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sig := <-signalChan
		fmt.Printf("signal : %v\n", sig)
		cancel()
	}()
	return ctx
}

func formatErr(err error) string {
	if err == nil {
		return "nil"
	}
	return err.Error()
}
