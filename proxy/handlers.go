package proxy

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"sync"
	"time"
)

func (rs *server) handleChannels(srcChans <-chan ssh.NewChannel) error {
	// Todo: defer some teardown here.
	dst, err := rs.dial()
	if err != nil {
		log.Errorf("ssh dial: %s", err)
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer func(dst *ssh.Client) {
		err := dst.Close()
		if err != nil {
			log.Errorf("handleChannels client teardown: %s", err)
		}
	}(dst)
	log.Debug("connection to destination established, talking to ", string(dst.ServerVersion()))

	for newChan := range srcChans {
		if newChan.ChannelType() != "session" {
			err := newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			if err != nil {
				log.Errorf("rejecting channel (type: %s) resultet in error: %s", newChan.ChannelType(), err)
			}
			log.Warnf("rejected channel of unknown type: %s", newChan.ChannelType())
			continue
		}
		// Open the channel to destination:
		dstChan, dstReqs, err := dst.OpenChannel(newChan.ChannelType(), newChan.ExtraData())
		if err != nil {
			log.Error("opening channel to dst: ", err)
			return fmt.Errorf("open dst channel: %w", err)
		}
		srcChan, srcReqs, err := newChan.Accept()
		if err != nil {
			// handle error
			log.Error("accepting incoming channel error: ", err)
			err := srcChan.Close()
			if err != nil {
				log.Error("error closing failed channel: ", err)
			}
			continue
		}

		handlerWg := sync.WaitGroup{}
		handlerWg.Add(2)
		// proxy requests from src --> dst. These are all the stuff the clients wanna do
		go func() {
			defer handlerWg.Done()
			proxyRequests(srcReqs, dstChan, "src --> dst") // End of servicing the channel
			time.Sleep(10 * time.Millisecond)              // Give any outstanding output time to get accross.
			err := dstChan.Close()
			if err != nil {
				if err.Error() != "EOF" {
					log.Errorf("proxyRequests src --> dst, closing dstChan: %s", err)
				}
			}
		}()
		// proxy stuff back. afaik this is mostly "exit-status" to let the src know how remote invocation went
		go func() {
			defer handlerWg.Done()
			proxyRequests(dstReqs, srcChan, "dst --> src") // End of servicing the channel
			time.Sleep(10 * time.Millisecond)              // Give any outstanding output time to get accross.
			err := srcChan.Close()
			if err != nil {
				if err.Error() != "EOF" {
					log.Errorf("proxyRequests dst --> src, closing srcChan %s", err)
				}
			}
		}()

		// copy data between the client and the target session, both ways.
		wg := sync.WaitGroup{}
		wg.Add(3)
		go func() {
			defer wg.Done()
			_, err := io.Copy(srcChan, dstChan)
			if err != nil {
				log.Error("io copy srcChan --> dstChan: ", err)
			}
		}()
		go func() {
			defer wg.Done()
			_, err := io.Copy(dstChan, srcChan)
			if err != nil {
				log.Error("io copy dstChan --> srcChan: ", err)
			}
		}()
		go func() {
			defer wg.Done()
			_, err := io.Copy(srcChan.Stderr(), dstChan.Stderr())
			if err != nil {
				log.Error("io copy src/stderr --> dst/stderr: ", err)
			}
		}()

		wg.Wait()
		handlerWg.Wait()
		log.Debug("io.Copy and request proxy done. Session ending.")
	} // for newChan ...

	return nil
}

// proxyRequests will proxy request in one direction. It ends when the channel closes.
// note that src and dst are relative here and their meaning is dependent on the invocation
// set debugDescription to indicate what direction it is operating in.
func proxyRequests(srcIn <-chan *ssh.Request, dstChan ssh.Channel, debugDescription string) {
	log.Tracef("proxyRequests(%s) running", debugDescription)
	for req := range srcIn {
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

// handleConn handles a single ssh connection.
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
