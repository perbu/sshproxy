package sshca

import (
	"fmt"
	"github.com/gliderlabs/ssh"
	log "github.com/sirupsen/logrus"
	gossh "golang.org/x/crypto/ssh"
	"io/ioutil"
	"time"
)

// GetPrivateKey reads a private key.
// It will cause a panic if we can't read or parse the key.
func GetPrivateKey(filename string) gossh.Signer {
	pemBytes, err := ioutil.ReadFile(filename)
	checkFatal(err, fmt.Sprintf("Could not read private key from %s: %s", filename, err))
	if len(pemBytes) == 0 {
		panic("Router server private key not set")
	}
	privKey, err := gossh.ParseRawPrivateKey(pemBytes)
	checkFatal(err, fmt.Sprintf("Could not parse key from %s: %s", filename, err))
	signer, err := gossh.NewSignerFromKey(privKey)
	checkFatal(err, "Could not create signer from key")
	return signer
}

func GetCa(filename string) gossh.PublicKey {
	caBytes, err := ioutil.ReadFile(filename)
	checkFatal(err, fmt.Sprintf("Could not read SSH CA from %s: %s", filename, err))
	ca, comment, _, _, err := gossh.ParseAuthorizedKey(caBytes)
	checkFatal(err, fmt.Sprintf("Could not instantiate CA read from %s: %s", filename, err))
	log.Infof("SSH CA loaded [%s]", comment)
	return ca
}

func GetPrivateCert(baseFilename string) gossh.Signer {
	signer := GetPrivateKey(baseFilename) // Will cause fatal errors on failure.
	filename := fmt.Sprintf("%s-cert.pub", baseFilename)
	cert, err := unmarshalCert(filename)

	checkFatal(err, fmt.Sprintf("could not load cert (%s): %s", filename, err))
	certSigner, err := gossh.NewCertSigner(cert, signer)
	checkFatal(err, "could not create a signer")
	// Todo: Log some more data on the loaded cert here.
	log.Infof("Loaded cert with ID '%s': Valid [%v --> %v]", cert.KeyId,
		time.Unix(int64(cert.ValidAfter), 0).UTC(), time.Unix(int64(cert.ValidBefore), 0).UTC())
	return certSigner
}

func unmarshalCert(path string) (*gossh.Certificate, error) {
	certBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey(certBytes)
	if err != nil {
		return nil, err
	}
	cert, ok := pub.(*gossh.Certificate)
	if !ok {
		return nil, fmt.Errorf("failed to cast to certificate")
	}
	return cert, nil
}

func checkFatal(e error, message string) {
	if e != nil {
		panic(message + " Error: " + e.Error())
	}
}

type SshKeyMap map[string]bool

// GetAuthorizedKeys fetches the list of publickeys which allows
// access to the admin shell. Returns a map of stringified public keys that are allowed.
// This will abort if it can't read the file as it is a fatal error.
// purpose is only used for logging, to give more meaning.
func GetAuthorizedKeys(filename, purpose string) map[string]bool {
	keyMap := make(map[string]bool)
	authKeyFile := filename
	authKeyContent, err := ioutil.ReadFile(authKeyFile)
	if err != nil {
		log.Fatalf("GetAuthorizedKeys(%s): Reading authorized-keys file (%s) yielded error: %s",
			purpose, authKeyFile, err)
	}

	for len(authKeyContent) > 0 {
		pubKey, comment, _, rest, err := ssh.ParseAuthorizedKey(authKeyContent)
		if err != nil {
			log.Fatalf("Parsing authorized keys for %s (%s):", purpose, err)
		}
		pubKeyStr := string(pubKey.Marshal())
		keyMap[pubKeyStr] = true
		if comment == "" {
			comment = "(no comment)"
		}
		log.Debugf("Added ssh key to admin access map for %s: %s", purpose, comment)
		authKeyContent = rest
	}
	return keyMap
}
