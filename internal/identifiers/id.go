// Package identifiers Random String generator based upon a, case-sensitive, alphanumeric alphabet with 63 letters.
// Probability of duplicates equals N^63 where N is the length
package identifiers

import (
	secureRand "crypto/rand"
	"math/big"
	rand "math/rand"
)

const defaultShortLength = 16
const defaultLength = 32
const runes = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func Generate() string {
	return generate(defaultLength)
}
func GenerateShort() string {
	return generate(defaultShortLength)
}

func GenerateSecure() string {
	return generateSecure(defaultLength)
}

func GenerateShortSecure() string {
	return generateSecure(defaultShortLength)
}

func generate(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = runes[rand.Intn(len(runes))] // #nosec G404 -- request, host and generated-key ids, never secrets; generateSecure covers those
	}
	return string(b)
}

func generateSecure(n int) string {
	b := make([]byte, n)
	for i := range b {
		key, err := secureRand.Int(secureRand.Reader, big.NewInt(int64(len(runes))))
		if err != nil {
			panic(err)
		}
		b[i] = runes[key.Int64()]
	}
	return string(b)
}
