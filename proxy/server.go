package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

type ServerConfig struct {
	listenAddr string
	privateKey ssh.Signer
}
type server struct {
	addr       string
	privateKey ssh.Signer
}

func MkConfig(addr string, signer ssh.Signer) ServerConfig {
	config := ServerConfig{
		listenAddr: addr,
		privateKey: signer,
	}
	return config
}

func Run(ctx context.Context, c ServerConfig) error {
	var err error
	rs := server{
		addr:       c.listenAddr,
		privateKey: c.privateKey,
	}

	sshCtx, sshCancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)

	// Spin off a little goroutine to monitor for shutdown.
	go func() {
		<-ctx.Done()
		log.Info("Shutting down ssh server")
		sshCancel()
		wg.Done()
	}()
	log.Infof("Starting sshproxy on '%s'", c.listenAddr)
	sshServer := rs.configure()
	err = rs.listen(sshCtx, sshServer)
	return err
}

func (rs *server) configure() *ssh.ServerConfig {
	config := &ssh.ServerConfig{
		PublicKeyCallback: rs.publicKeyHandler,
	}
	config.AddHostKey(rs.privateKey)
	log.Debug("configuring server to run at ", rs.addr)
	return config
}

// publicKeyHandler
func (rs *server) publicKeyHandler(_ ssh.ConnMetadata, _ ssh.PublicKey) (*ssh.Permissions, error) {
	// This is just a PoC, so we accept everything here.
	log.Info("Pubkeyhandler accepting")
	return nil, nil
}

// listen is called by Run and does the actual listen on the port. It'll will
// accept the incoming connections and call handleConn on each of them.
func (rs *server) listen(ctx context.Context, sshServer *ssh.ServerConfig) error {
	listener, err := net.Listen("tcp", rs.addr)
	if err != nil {
		panic(err)
	}
	go func() {
		<-ctx.Done()
		err := listener.Close()
		if err != nil {
			log.Error("Closing listener socket: ", err)
		}
	}()

	for ctx.Err() == nil {
		// Once a ServerConfig has been configured, connections can be accepted.
		// Todo: see if we can move this out to a separate function.
		conn, err := listener.Accept()
		if err != nil {
			log.Error("accept: ", err)
			continue
		}
		go func() {
			err = rs.handleConn(conn, sshServer)
			if err != nil {
				log.Error("handleConn: %s", err)
			}
		}()
	}
	return nil
}

func (rs *server) handleChannel(newChan ssh.NewChannel) error {
	if newChan.ChannelType() != "session" {
		_ = newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
		return fmt.Errorf("rejected channel of unknown type: %s", newChan.ChannelType())
	}
	srcChan, srcReqs, err := newChan.Accept()
	if err != nil {
		// handle error
		return fmt.Errorf("accept channel: %w", err)
	}
	dst, err := rs.dial()
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer func(dst *ssh.Client) {
		_ = dst.Close()
	}(dst)
	log.Debug("connection to destination established, talking to ", string(dst.ServerVersion()))

	// Open the channel to destination:
	dstChan, dstReqs, err := dst.OpenChannel(newChan.ChannelType(), newChan.ExtraData())
	if err != nil {
		return fmt.Errorf("open dst channel: %w", err)
	}

	handlerWg := sync.WaitGroup{}
	handlerWg.Add(2)
	// proxy requests from src --> dst. These are all the stuff the clients wanna do
	go func() {
		defer handlerWg.Done()
		proxyRequests(srcReqs, dstChan, "src --> dst") // End of servicing the channel
		time.Sleep(10 * time.Millisecond)              // Give any outstanding output time to get accross.
		_ = dstChan.Close()
	}()
	// proxy stuff back. afaik this is mostly "exit-status" to let the src know how remote invocation went
	go func() {
		defer handlerWg.Done()
		proxyRequests(dstReqs, srcChan, "dst --> src") // End of servicing the channel
		time.Sleep(10 * time.Millisecond)              // Give any outstanding output time to get accross.
		_ = srcChan.Close()
	}()

	// copy data between the client and the target session, both ways.
	copyWg := sync.WaitGroup{}
	copyWg.Add(3)
	go func() {
		defer copyWg.Done()
		_, err := io.Copy(srcChan, dstChan)
		if err != nil {
			log.Error("io copy dstChan --> srcChan: ", err)
		}
	}()
	go func() {
		defer copyWg.Done()
		_, err := io.Copy(dstChan, srcChan)
		if err != nil {
			log.Error("io copy dstChan --> srcChan: ", err)
		}
		_ = dstChan.Close() // things like scp close this one first.
	}()
	go func() {
		defer copyWg.Done()
		_, err := io.Copy(srcChan.Stderr(), dstChan.Stderr())
		if err != nil {
			log.Error("io copy src/stderr --> dst/stderr: ", err)
		}
	}()

	copyWg.Wait()
	handlerWg.Wait()
	log.Debug("io.Copy and request proxy done. Session ending.")
	return nil
}
