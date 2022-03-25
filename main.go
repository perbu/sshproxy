package main

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/perbu/sshproxy/proxy"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

//go:embed id_rsa
var privKey []byte

func main() {
	err := realMain()
	if err != nil {
		log.Fatal(err)
	}
}

func realMain() error {
	log.SetLevel(log.TraceLevel)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
	log.Info("sshproxy starting up")
	signer, err := GetPrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("GetPrivateKey: %w", err)
	}
	log.Debug("private key (signer) loaded successfully")
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := proxy.Run(ctx, proxy.MkConfig("localhost:4222", signer))
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

// GetPrivateKey reads a private key.
// It will cause a panic if we can't read or parse the key.
func GetPrivateKey(pemBytes []byte) (signer ssh.Signer, err error) {
	privKey, err := ssh.ParseRawPrivateKey(pemBytes)
	if err != nil {
		return signer, fmt.Errorf("could not parse key: %w", err)
	}
	signer, err = ssh.NewSignerFromKey(privKey)
	if err != nil {
		return signer, fmt.Errorf("could not create a signer: %w", err)
	}
	return signer, nil
}
