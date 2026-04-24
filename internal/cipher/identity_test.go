package cipher

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func Test_GenSecret(t *testing.T) {

	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	pub := priv.PublicKey()
	t.Log("privstring: ", base64.StdEncoding.EncodeToString(priv.Bytes()))
	t.Log("pubstring:", base64.StdEncoding.EncodeToString(pub.Bytes()))
}
