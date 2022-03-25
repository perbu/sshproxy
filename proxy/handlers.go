package proxy

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"net"
)

func (rs *server) handleChannels(srcChans <-chan ssh.NewChannel) error {

	for newChan := range srcChans {
		err := rs.handleChannel(newChan)
		if err != nil {
			log.Errorf("handleChannel: %s", err)
		}
	} // for newChan ...

	return nil
}

// proxyRequests will proxy request in one direction. It ends when the channel closes.
// note that src and dst are relative here and their meaning is dependent on the invocation
// set debugDescription to indicate what direction it is operating in.
func proxyRequests(srcIn <-chan *ssh.Request, dstChan ssh.Channel, debugDescription string) {
	log.Tracef("proxyRequests(%s) running", debugDescription)
	for req := range srcIn {
		if req == nil {
			log.Warn("proxyRequests got a nil request. aborting.")
			return
		}
		log.Tracef("proxy(%s) req type: %s, wantReply: %t, payload: '%x' / '%s'",
			debugDescription, req.Type, req.WantReply, req.Payload, clean(req.Payload))
		reply, err := dstChan.SendRequest(req.Type, req.WantReply, req.Payload)
		if err != nil {
			log.Errorf("proxyRequests/SendRequest err: %s", err)
			continue
		}
		log.Tracef("proxyRequests(%s)/reply status: %t", debugDescription, reply)
		if reply {
			log.Trace("sending response to req type ", req.Type)
			err := req.Reply(reply, nil)
			if err != nil {
				log.Error("proxyRequests/reply error: ", err)
				continue
			}
		}
	} // end for range srcIn
}

// handleConn handles a single ssh connection. Invoked after the auth has succeeded.
func (rs *server) handleConn(conn net.Conn, sshServer *ssh.ServerConfig) error {
	// Before use, a handshake must be performed on the incoming net.Conn.
	sConn, channels, reqs, err := ssh.NewServerConn(conn, sshServer)

	if err != nil {
		return fmt.Errorf("handleConn handshake: %w", err)
	}
	user := sConn.Conn.User()
	log.Debug("Accepted connection from user: ", user)
	go ssh.DiscardRequests(reqs) // The incoming Request channel must be serviced.
	go func() {
		err = rs.handleChannels(channels)
		if err != nil {
			log.Error("handleChannels: %s", err)
		}
	}()
	return nil
}

func clean(bytes []byte) string {
	res := make([]byte, 0)
	for _, b := range bytes {
		if ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z') || ('0' <= b && b <= '9') ||
			b == ' ' || b == '-' || b == '/' || b == '_' {
			res = append(res, b)
		}
	}
	return string(res)
}
