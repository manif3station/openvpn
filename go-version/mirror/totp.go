package mirror

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var nonBase32 = regexp.MustCompile(`[^A-Z2-7]`)

func CurrentMFACode(token string, now func() time.Time) (string, error) {
	if token == "" {
		return "", nil
	}
	if len(token) == 6 && isDigits(token) {
		return token, nil
	}
	secret, err := extractSecret(token)
	if err != nil {
		return "", err
	}
	return generateTOTP(secret, now())
}

func extractSecret(token string) (string, error) {
	if strings.HasPrefix(token, "otpauth://") {
		parsed, err := url.Parse(token)
		if err != nil {
			return "", err
		}
		secret := parsed.Query().Get("secret")
		if secret == "" {
			return "", fmt.Errorf("missing secret in otpauth uri")
		}
		token = secret
	}
	token = strings.ToUpper(token)
	token = nonBase32.ReplaceAllString(token, "")
	if token == "" {
		return "", fmt.Errorf("invalid base32 secret")
	}
	return token, nil
}

func generateTOTP(secret string, now time.Time) (string, error) {
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("invalid base32 secret")
	}
	counter := uint64(now.Unix() / 30)
	msg := make([]byte, 8)
	binary.BigEndian.PutUint64(msg, counter)
	hash := hmac.New(sha1.New, raw)
	_, _ = hash.Write(msg)
	sum := hash.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := (int(sum[offset])&0x7f)<<24 |
		int(sum[offset+1])<<16 |
		int(sum[offset+2])<<8 |
		int(sum[offset+3])
	code = code % 1000000
	return fmt.Sprintf("%06d", code), nil
}

func isDigits(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
