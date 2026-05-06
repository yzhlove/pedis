package cipher

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/yzhlove/pedis/internal/config"
)

func Test_GenSecret(t *testing.T) {

	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	pub := priv.PublicKey()
	e1 := base64.StdEncoding.EncodeToString(priv.Bytes())
	e2 := base64.StdEncoding.EncodeToString(pub.Bytes())
	t.Log("privstring: ", e1)
	t.Log("pubstring:", e2)

	x11, err := base64.StdEncoding.DecodeString("KN9o0mr60NDi7MPIIqP9jGIXBJdO1G4RqqRAUFi4m/k=")
	if err != nil {
		t.Fatal(err)
	}

	x22, err := base64.StdEncoding.DecodeString("QXJs/4h59g9GJe17Izk4jb+PNg5hux4nMKfAca/ZRwI=")
	if err != nil {
		t.Fatal(err)
	}

	x1, err := ecdh.X25519().NewPublicKey(x22)
	if err != nil {
		t.Fatal(err)
	}

	x2, err := ecdh.X25519().NewPrivateKey(x11)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("equal => ", x1.Equal(x2.PublicKey()))

}

func Test_Load(t *testing.T) {

	cfg := &config.Config{
		Role:             config.ServerRole,
		CharacterSet:     "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz@#$%*!+=/?",
		TimeSeed:         "psMrfhLXrUxbdyYdzzaAjQLr8mDwuu0c",
		ServerPublicKey:  "QXJs/4h59g9GJe17Izk4jb+PNg5hux4nMKfAca/ZRwI=",
		ServerPrivateKey: "KN9o0mr60NDi7MPIIqP9jGIXBJdO1G4RqqRAUFi4m/k=",
	}

	x11, err := base64.StdEncoding.DecodeString(cfg.ServerPrivateKey)
	if err != nil {
		t.Fatal(err)
	}

	x22, err := base64.StdEncoding.DecodeString(cfg.ServerPublicKey)
	if err != nil {
		t.Fatal(err)
	}

	x1, err := ecdh.X25519().NewPublicKey(x22)
	if err != nil {
		t.Fatal(err)
	}

	x2, err := ecdh.X25519().NewPrivateKey(x11)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("equal => ", x1.Equal(x2.PublicKey()))
	identity := New(cfg)
	if err := identity.Apply(); err != nil {
		t.Fatal(err)
	}

}
