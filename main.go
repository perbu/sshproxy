package main

import (
	"context"
	"github.com/perbu/sshproxy/proxy"
	log "github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func main() {
	err := realMain()
	if err != nil {
		log.Fatal(err)
	}
}

func realMain() error {
	log.SetLevel(log.TraceLevel)
	log.Info("sshproxy starting up")

	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := proxy.Run(ctx, proxy.MkConfig())
		log.Info("Proxy shut down")
		if err != nil {
			log.Error("sshd: ", err)
		}
		wg.Done()
	}()

	<-sigChan
	log.Info("signal caught")
	cancel()
	wg.Wait()
	log.Info("Program exiting")
	return nil
}
