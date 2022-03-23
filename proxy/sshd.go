package proxy

import (
	"context"
	"fmt"
	"github.com/perbu/sshproxy/sshca"
	log "github.com/sirupsen/logrus"
	ssh "golang.org/x/crypto/ssh"
	"io"
	"net"
	"sync"
)

type ServerConfig struct {
	ListenAddr    string
	ServerPrivKey string
	SshCa         string
}
type server struct {
	ca      ssh.PublicKey
	checker ssh.CertChecker
	addr    string
	privKey ssh.Signer
}

func MkConfig() ServerConfig {
	config := ServerConfig{
		ListenAddr:    "localhost:4222",
		ServerPrivKey: "id_rsa",
		SshCa:         "CA",
	}
	return config
}

func Run(ctx context.Context, c ServerConfig) error {
	var err error
	rs := server{
		ca:      sshca.GetCa(c.SshCa),
		addr:    c.ListenAddr,
		privKey: sshca.GetPrivateKey(c.ServerPrivKey),
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
	log.Infof("Starting sshproxy on '%s'", c.ListenAddr)
	sshServer := rs.configure()
	err = rs.listen(sshCtx, sshServer)
	return err
}

func (rs *server) configure() *ssh.ServerConfig {
	config := &ssh.ServerConfig{
		PublicKeyCallback: rs.publicKeyHandler,
	}
	config.AddHostKey(rs.privKey)
	log.Debug("configuring server to run at ", rs.addr)
	return config
}

func (rs *server) publicKeyHandler(_ ssh.ConnMetadata, _ ssh.PublicKey) (*ssh.Permissions, error) {
	// This is just a PoC so we accept everything here.
	log.Info("Pubkeyhandler accepting")
	return nil, nil
}

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
		conn, err := listener.Accept()
		if err != nil {
			log.Error("accept: ", err)
			continue
		}
		// Before use, a handshake must be performed on the incoming net.Conn.
		sConn, chans, reqs, err := ssh.NewServerConn(conn, sshServer)
		if err != nil {
			// handle error
			log.Error("handshake: ", err)
			continue
		}
		user := sConn.Conn.User()
		log.Debug("Accepted user: ", user)
		// The incoming Request channel must be serviced.
		go ssh.DiscardRequests(reqs)
		go rs.handleServerConn(chans)
	}
	return nil
}

func (rs *server) dial() (*ssh.Client, error) {

	conf := &ssh.ClientConfig{
		User:            "celerway",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(rs.privKey),
		},
	}
	return ssh.Dial("tcp", "localhost:3222", conf)
}

func (rs *server) handleServerConn(chans <-chan ssh.NewChannel) error {
	// Todo: defer some teardown here.
	dst, err := rs.dial()
	if err != nil {
		log.Error("ssh dial: %s", err)
	}

	for newChan := range chans {
		log.Debug("Incoming chan of type: ", newChan.ChannelType())
		if newChan.ChannelType() != "session" {
			err := newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			if err != nil {
				log.Errorf("rejecting channel (type: %s) resultet in error: %s", newChan.ChannelType(), err)
			}
			continue
		}
		// Open the channel to destination:
		dstChan, dstReqs, err := dst.OpenChannel(newChan.ChannelType(), newChan.ExtraData())
		if err != nil {
			log.Error("opening channel to dst: ", err)
			return fmt.Errorf("open dst channel: %w", err)
		}
		go ssh.DiscardRequests(dstReqs) // Not sure about this.
		ch, reqs, err := newChan.Accept()
		if err != nil {
			// handle error
			log.Error("accepting incoming channel error: ", err)
			continue
		}
		if err != nil {
			log.Error("creating dst session:", err)
		}
		// Service the channel here:
		go func(in <-chan *ssh.Request) {
			// Teardown
			defer func(ch ssh.Channel) {
				err := ch.Close()
				if err != nil {
					log.Error("closing request channel: ", err)
				}
			}(ch)

			// For each request, handle it.
			for req := range in {
				log.Info("handling incoming req type: ", req.Type)

				reply, err := dstChan.SendRequest(req.Type, req.WantReply, req.Payload)
				if err != nil {
					log.Errorf("dst.SendRequest err: %s", err)
					continue
				}
				log.Trace("dest sendreq reply: %t", reply)

			}
		}(reqs) // End of servicing the channel

		// wrappedChannel := cwrapper.NewTypeWriterReadCloser(newChan)
		go func() {
			_, err := io.Copy(ch, dstChan)
			if err != nil {
				log.Error("io copy ch --> dstChan: ", err)
			}
		}()
		go func() {
			_, err := io.Copy(dstChan, ch)
			if err != nil {
				log.Error("io copy dstChan --> ch: ", err)
			}
		}()
		defer ch.Close()
		defer dstChan.Close()

	} // for newChan ...
	defer dst.Close()
	return nil
}

/*
    // Proxy request handling code:
	if req.WantReply {
		reply, err := dst.SendRequest(req.Type, req.WantReply, req.Payload)
		if err != nil {
			log.Errorf("server(): dst.SendRequest err: %s", err)
		} else {
			log.Tracef("SendRequest returned %t", reply)
		}
		err = req.Reply(reply, []byte{0, 0, 0, 0})
		if err != nil {
			log.Errorf("Responding to WantReply: %s", err)
			return
		}
		log.Trace("Sent nil reply")
	}
	if req.Type == "exit-status" {
		break requestLoop
	}

*/
