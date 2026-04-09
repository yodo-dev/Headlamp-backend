package util

import (
	"crypto/rand"
	"fmt"
	"math/big"
	mrand "math/rand"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const alphabet = "abcdefghijklmnopqrstuvwxyz"

var consonants = []string{"b", "c", "d", "f", "g", "h", "j", "k", "l", "m", "n", "p", "r", "s", "t", "v", "w", "z"}
var vowels = []string{"a", "e", "i", "o", "u"}

func init() {
	mrand.Seed(time.Now().UnixNano())
}

// RandomInt generates a random integer between min and max
func RandomInt(min, max int64) int64 {
	return min + mrand.Int63n(max-min+1)
}

// RandomNumeric generates a random *big.Int between min and max
func RandomNumeric(min, max int64) *big.Int {
	return big.NewInt(RandomInt(min, max))
}

// RandomString generates a random string of length n
func RandomString(n int) string {
	var sb strings.Builder
	k := len(alphabet)

	for i := 0; i < n; i++ {
		c := alphabet[mrand.Intn(k)]
		sb.WriteByte(c)
	}

	return sb.String()
}

// RandomDeviceId generates a random device id
func RandomDeviceId() string {
	return RandomString(6)
}

// RandomUsername generates a random username
func RandomUsername() string {
	length := mrand.Intn(3) + 6

	var sb strings.Builder
	for sb.Len() < length {
		sb.WriteString(consonants[mrand.Intn(len(consonants))])
		sb.WriteString(vowels[mrand.Intn(len(vowels))])
	}

	username := sb.String()
	if len(username) > length {
		username = username[:length]
	}

	c := cases.Title(language.English)
	return c.String(username)
}

// RandomPassword generates a random password id
func RandomPassword() string {
	pass, err := HashPassword(RandomString(8))
	if err != nil {
		return ""
	}

	return pass
}

// RandomEmail generates a random email
func RandomEmail() string {
	return fmt.Sprintf("%s@email.com", RandomString(6))
}

// RandomPhone generates a random phone number
func RandomPhone() string {
	// Generate a phone number in the format +1 (XXX) XXX-XXXX
	areaCode := RandomInt(200, 999)     // Area code (200-999 to avoid invalid codes)
	exchangeCode := RandomInt(200, 999) // Exchange code (200-999 to avoid invalid codes)
	lineNumber := RandomInt(1000, 9999) // Line number (4 digits)

	return fmt.Sprintf("+1 (%03d) %03d-%04d", areaCode, exchangeCode, lineNumber)
}

// RandomAccessToken generates a random email
func RandomAccessToken() string {
	return RandomString(100)

}

// RandomRefreshToken generates a random email
func RandomRefreshToken() string {
	return RandomString(100)
}

// GenerateLinkCode creates a cryptographically secure, 6-character alphanumeric string.
func GenerateLinkCode() (string, error) {
	const (
		codeLength = 6
		charset    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	)
	b := make([]byte, codeLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to read random bytes: %w", err)
	}
	for i := 0; i < codeLength; i++ {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}
