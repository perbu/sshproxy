package proxy

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"io"
	"strings"
	"sync"
)

func (rs *server) handleServerConn(srcChans <-chan ssh.NewChannel) error {
	// Todo: defer some teardown here.
	dst, err := rs.dial()
	if err != nil {
		log.Errorf("ssh dial: %s", err)
		return fmt.Errorf("ssh dial: %w", err)
	}

	for newChan := range srcChans {
		log.Debug("Incoming chan of type: ", newChan.ChannelType())
		if newChan.ChannelType() != "session" {
			err := newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			if err != nil {
				log.Errorf("rejecting channel (type: %s) resultet in error: %s", newChan.ChannelType(), err)
			}
			continue
		}
		// Open the channel to destination:
		log.Debugf("opening dst channel; type: %s e", newChan.ChannelType())
		dstChan, dstReqs, err := dst.OpenChannel(newChan.ChannelType(), newChan.ExtraData())
		// todo: dst.Close() at some point.
		if err != nil {
			log.Error("opening channel to dst: ", err)
			return fmt.Errorf("open dst channel: %w", err)
		}
		srcChan, srcReqs, err := newChan.Accept()
		if err != nil {
			// handle error
			log.Error("accepting incoming channel error: ", err)
			continue
		}

		// proxy requests from src --> dst. These are all the stuff the clients wanna do
		go func() {
			proxyRequests(srcReqs, dstChan, "src --> dst") // End of servicing the channel
			dstChan.Close()
		}()
		// proxy stuff back. afaik this is mostly "exit-status" to let the src know how remote invocation went
		go func() {
			proxyRequests(dstReqs, srcChan, "dst --> src") // End of servicing the channel
			srcChan.Close()
		}()

		// todo: consider wrapping channels if things get fishy

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
			_, err := io.Copy(dstChan.Stderr(), srcChan.Stderr())
			if err != nil {
				log.Error("io copy dst/stderr --> src/stderr: ", err)
			}
		}()

		log.Debug("io.Copy running. Waiting for them to finish.")
		wg.Wait()
		log.Debug("io.Copy done.")
		err = srcChan.Close()
		if err != nil {
			log.Error("closing src chan:", err)
		}
		err = dstChan.Close()
		if err != nil {
			log.Error("closing dst chan:", err)
		}
	} // for newChan ...

	/*
		err = dst.Wait()
		if err != nil {
			log.Error("waiting for session: ", err)
		}
	*/
	defer dst.Close()

	return nil
}

// proxyRequests will proxy request in one direction. It ends when the channel closes.
// note that src and dst are relative here and their meaning is dependent on the invocation
// set debugDescription to indicate what direction it is operating in.
func proxyRequests(srcIn <-chan *ssh.Request, dstChan ssh.Channel, debugDescription string) {
	// For each request, handle it.
	defer func() {
		log.Debugf("== proxy(%s) ending ==", debugDescription)
		err := dstChan.Close() // close the opposite end when we exit.
		if err != nil {
			log.Errorf("defer proxy(%s) closing dstChan: %s", debugDescription, err)
		}
	}()
	log.Debugf("proxyRequests(%s) running", debugDescription)
	for req := range srcIn {
		log.Infof("proxy(%s) req type: %s, wantReply: %t, payload: '%x' / '%s'",
			debugDescription, strings.ToUpper(req.Type), req.WantReply, req.Payload, clean(req.Payload))
		reply, err := dstChan.SendRequest(req.Type, req.WantReply, req.Payload)
		if err != nil {
			log.Errorf("proxyRequests/SendRequest err: %s", err)
			continue
		}
		log.Tracef("proxyRequests(%s)/reply status: %t", debugDescription, reply)
		if reply {
			log.Debug("sending response to ", req.Type)
			err := req.Reply(reply, nil)
			if err != nil {
				log.Error("proxyRequests/reply error: ", err)
				continue
			}
		}
		if req.Type == "exit-status" {
			// log.Warnf("proxy(%s) ======== EXIT STATUS ===========", debugDescription)
			continue
		}
		if req.Type == "exec" {
			// log.Warnf("proxy(%s) ======== EXEC ===========", debugDescription)
			log.Debug("exec done. now what?")
			// dstChan.Write([]byte("well done"))
		}

	} // end for range srcIn
}

func clean(bytes []byte) string {
	res := make([]byte, 0)
	for _, b := range bytes {
		if ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z') || ('0' <= b && b <= '9') || b == ' ' || b == '-' || b == '/' || b == '_' {
			res = append(res, b)
		}
	}
	return string(res)
}
