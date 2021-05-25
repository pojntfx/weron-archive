package utils

import (
	"crypto/sha1"
	"crypto/tls"
	"fmt"
	"strings"
)

// See https://play.golang.org/p/GTxajcr3NY
func GetFingerprint(cert tls.Certificate) string {
	rawHash := sha1.Sum(cert.Certificate[0])

	hex := fmt.Sprintf("%x", rawHash)
	if len(hex)%2 == 1 {
		hex = "0" + hex
	}

	numberOfColons := len(hex)/2 - 1

	colonedHex := make([]byte, len(hex)+numberOfColons)
	for i, j := 0, 0; i < len(hex)-1; i, j = i+1, j+1 {
		colonedHex[j] = hex[i]
		if i%2 == 1 {
			j++
			colonedHex[j] = []byte(":")[0]
		}
	}

	// We skipped the last one to avoid the colon at the end
	colonedHex[len(colonedHex)-1] = hex[len(hex)-1]

	return strings.ToUpper(string(colonedHex))
}
