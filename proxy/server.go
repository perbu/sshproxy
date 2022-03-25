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
}
type server struct {
	addr    string
	privKey ssh.Signer
}

func MkConfig() ServerConfig {
	config := ServerConfig{
		ListenAddr:    "localhost:4222",
		ServerPrivKey: "id_rsa",
	}
	return config
}

func Run(ctx context.Context, c ServerConfig) error {
	var err error
	rs := server{
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

// publicKeyHandler
func (rs *server) publicKeyHandler(_ ssh.ConnMetadata, _ ssh.PublicKey) (*ssh.Permissions, error) {
	// This is just a PoC so we accept everything here.
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
		err = rs.handleConn(conn, sshServer)
	}
	return nil
}
