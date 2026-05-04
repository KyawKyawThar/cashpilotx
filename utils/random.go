package utils

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/google/uuid"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz"
const digits = "0123456789"

// RandomString generates a random string of a given length.
func RandomString(maxLength int) string {
	b := make([]byte, maxLength)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}

	for i := range b {
		b[i] = alphabet[b[i]%byte(len(alphabet))]
	}

	return string(b)
}

// RandomInteger generates a random integer between min and max.
func RandomInteger(minVal, maxVal int64) int64 {
	if minVal > maxVal {
		panic("min cannot be greater than max")
	}

	n, err := rand.Int(rand.Reader, big.NewInt(maxVal-minVal+1))
	if err != nil {
		panic(err)
	}
	return n.Int64() + minVal
}

// RandomOwner generates a random owner name.
func RandomOwner() string {
	return RandomString(7)
}

// RandomEmail generates a random email address.
func RandomEmail() string {
	return fmt.Sprintf("%s@%s.com", RandomString(8), RandomString(5))
}

// RandomSlug generates a random slug.
func RandomSlug() string {
	return fmt.Sprintf("%s-%s", RandomString(4), RandomString(4))
}

// RandomUUID generates a random UUID.
func RandomUUID() uuid.UUID {
	return uuid.New()
}

// RandomPhone generates a phone number matching ^\+?[0-9]{8,15}$.
// Format: +66XXXXXXXXX (Thai prefix, 9 random digits).
// func RandomPhone(n int64) string {
// 	result, err := rand.Int(rand.Reader, big.NewInt(n))
// 	if err != nil {
// 		panic(err)
// 	}
// 	return fmt.Sprintf("+66%09d", result.Int64()+100_000_000)
// }

func RandomNumberString(length int) string {
	if length <= 0 {
		return ""
	}

	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}

	for i := range b {
		b[i] = digits[b[i]%byte(len(digits))]
	}

	return string(b)
}

func RandomPhone(length int) string {
	// generate 8–15 digit number
	// length := 10 // safe default inside valid range

	return "+" + RandomNumberString(length)
}
