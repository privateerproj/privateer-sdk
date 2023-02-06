package utils

import (
	"math/rand"
	"time"
	"unsafe"
)

const letters = "abcdefghijklmnopqrstuvwxyz"
const (
	letterIndexBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIndexBits - 1 // All 1-bits, as many as letterIndexBits
	letterIdxMax  = 63 / letterIndexBits   // # of letter indices fitting in 63 bits
)

var src = rand.NewSource(time.Now().UnixNano())

// RandomString generates a pseudo-random number of characters of length n
func RandomString(n int) string {
	bytes := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letters) {
			bytes[i] = letters[idx]
			i--
		}
		cache >>= letterIndexBits
		remain--
	}

	return *(*string)(unsafe.Pointer(&bytes))
}
