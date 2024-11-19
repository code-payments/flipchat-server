package pg

import "github.com/mr-tron/base58"

func Encode(value []byte) string {
	// TODO: choose something more efficient, this is just for testing

	// Possible options:
	// - hex (probably the most efficient, while still being human-readable-ish)
	// - base64
	// - (or just store the bytes as-is, change the column type to "Byte")

	return base58.Encode(value)
}

func Decode(value string) ([]byte, error) {
	// TODO: would love to have something that can't fail here
	return base58.Decode(value)
}
