package proxy

import "golang.org/x/crypto/ssh"

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
