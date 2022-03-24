package sshca

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
)

// GetPrivateKey reads a private key.
// It will cause a panic if we can't read or parse the key.
func GetPrivateKey(filename string) ssh.Signer {
	pemBytes, err := ioutil.ReadFile(filename)
	checkFatal(err, fmt.Sprintf("Could not read private key from %s: %s", filename, err))
	if len(pemBytes) == 0 {
		panic("Router server private key not set")
	}
	privKey, err := ssh.ParseRawPrivateKey(pemBytes)
	checkFatal(err, fmt.Sprintf("Could not parse key from %s: %s", filename, err))
	signer, err := ssh.NewSignerFromKey(privKey)
	checkFatal(err, "Could not create signer from key")
	return signer
}

func checkFatal(e error, message string) {
	if e != nil {
		panic(message + " Error: " + e.Error())
	}
}
