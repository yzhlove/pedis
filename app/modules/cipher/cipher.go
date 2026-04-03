package cipher

type Aead interface {
	Seal(data []byte) ([]byte, error)
	Open(data []byte) ([]byte, error)
}
