package proxy

import (
	"context"
	"github.com/perbu/sshproxy/sshca"
	log "github.com/sirupsen/logrus"
	ssh "golang.org/x/crypto/ssh"
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
		// Todo: see if we can move this out to a separate function.
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
		go ssh.DiscardRequests(reqs) // The incoming Request channel must be serviced.
		go func() {
			err = rs.handleServerConn(chans)
			if err != nil {
				log.Error("handleServerConn: %s", err)
			}
		}()
	}
	return nil
}
