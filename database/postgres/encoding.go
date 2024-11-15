package pg

import "github.com/mr-tron/base58"

func Encode(value []byte) string {
	return base58.Encode(value)
}

func Decode(value string) ([]byte, error) {
	return base58.Decode(value)
}
