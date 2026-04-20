package utils

import (
	"math/rand/v2"
)

const (
	letters         = "abcdefghijklmnopqrstuvwxyz"
	letterIndexBits = 6                      // 6 bits to represent a letter index
	letterIdxMask   = 1<<letterIndexBits - 1 // All 1-bits, as many as letterIndexBits
	letterIdxMax    = 63 / letterIndexBits   // # of letter indices fitting in 63 bits
)

// RandomString generates a pseudo-random string of lowercase characters of length n.
func RandomString(n int) string {
	b := make([]byte, n)
	for i, cache, remain := n-1, rand.Int64(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = rand.Int64(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letters) {
			b[i] = letters[idx]
			i--
		}
		cache >>= letterIndexBits
		remain--
	}
	return string(b)
}
