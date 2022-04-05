package main

import (
	"errors"
	"fmt"
	"github.com/mailru/easygo/netpoll"
	"io"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

// N_READY_CONNECTIONS n worker thread
var N_READY_CONNECTIONS = (runtime.NumCPU() - 1) * 8

// ready ok connection channel
var readyConn = make(chan net.Conn,N_READY_CONNECTIONS)


func main() {
	poller, err := netpoll.New(nil)
	if err != nil {
		panic(err)
	}
	listener, err := net.Listen("tcp", "0.0.0.0:8080")
	if err != nil {
		panic(err)
	}
	acceptDesc, err := netpoll.HandleListener(listener, netpoll.EPOLLIN)
	if err != nil {
		panic(err)
	}
	cancel := initHandleGoroutine()
	// add accept goroutine cancel
	acceptCancel := make(chan struct{})
	cancel = append(cancel,acceptCancel)
	// signal handle
	signalChan := make(chan os.Signal,1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-signalChan
		for _,v := range cancel{
			v <- struct{}{}
		}
		// close readyConn chan write pipe
		close(readyConn)
		// close acceptConn chan write pipe
		close(acceptCancel)
		// check old fd is handle ok?
		for {
			time.Sleep(time.Millisecond * 10)
			if len(readyConn) == 0 && len(acceptCancel) == 0 {
				os.Exit(0)
			}
		}
	}()

	acceptDone := make(chan struct{},255)
	err = poller.Start(acceptDesc, func(event netpoll.Event) {
		acceptDone <- struct{}{}
	})
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			select {
			case <-acceptDone:
				conn,err := listener.Accept()
				if err != nil {
					fmt.Println("accept error : ",err)
					continue
				}
				readDesc, err := netpoll.HandleRead(conn)
				if err != nil {
					fmt.Println("handle read event error : ",err)
				}
				// on read
				err = poller.Start(readDesc, func(event netpoll.Event) {
					readyConn <- conn
				})
				if err != nil {
					fmt.Println("poller read start error : ",err)
				}
				// on close
				closeDesc, err := netpoll.Handle(conn,netpoll.EPOLLRDHUP)
				err = poller.Start(closeDesc, func(event netpoll.Event) {
					err := poller.Stop(readDesc)
					if err != nil {
						fmt.Println("poller read stop err : ", err)
					}
					err = poller.Stop(closeDesc)
					if err != nil {
						fmt.Println("poller close stop err : ", err)
					}
					err = readDesc.Close()
					if err != nil {
						fmt.Println("read desc close conn err : ", err)
					}
					err = closeDesc.Close()
					if err != nil {
						fmt.Println("close desc close conn err : ", err)
					}
				})
				if err != nil {
					fmt.Println("poller close start error : ",err)
				}
			case <-acceptCancel:
				return
			}
		}
	}()

	// hang
	select {}
}

func initHandleGoroutine() []chan struct{} {
	cancel := make([]chan struct{},0,N_READY_CONNECTIONS/8)
	for i := 0; i < N_READY_CONNECTIONS / 8; i++ {
		nCancel := make(chan struct{})
		cancel = append(cancel,nCancel)
		go func() {
			runtime.LockOSThread()
			for {
				select {
				case conn := <- readyConn:
					buffer := make([]byte,256)
					_, err := conn.Read(buffer)
					if err == io.EOF || errors.Is(err,net.ErrClosed) {
						continue
					}
					if err != nil {
						fmt.Println("read err : ",err)
						continue
					}
					// reset buffer len
					buffer = buffer[:0]
					buffer = append(buffer, "HTTP/1.1 200 OK\r\nServer: easygo\r\nContent-Type: text/plain\r\nDate: "...)
					buffer = time.Now().AppendFormat(buffer, "Mon, 02 Jan 2006 15:04:05 GMT")
					buffer = append(buffer, "\r\nContent-Length: 12\r\n\r\nHello World!"...)
					// write
					_, err = conn.Write(buffer)
					if err == io.EOF || errors.Is(err,net.ErrClosed) {
						continue
					}
					if err != nil {
						fmt.Println("write err : ",err)
						continue
					}
				case <- nCancel:
					return
				}
			}
		}()
	}
	return cancel
}