package utils

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

func GenerateTLSKeyAndCert(organization string, validity time.Duration) (keyString string, certString string, err error) {
	// Generate public and private keys
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}

	// Set metadata
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return "", "", err
	}
	now := time.Now()

	// Create template based on metadata
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{organization},
		},
		NotBefore: now,
		NotAfter:  now.Add(validity),

		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Serialize TLS key
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", "", err
	}

	keyOut := &bytes.Buffer{}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return "", "", err
	}

	// Serialize TLS cert
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey, privateKey)
	if err != nil {
		return "", "", err
	}

	certOut := &bytes.Buffer{}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		return "", "", err
	}

	return keyOut.String(), certOut.String(), nil
}

func CreateFileAndLeadingDirectories(location string, content string) error {
	// If config file does not exist, create and write to it
	if _, err := os.Stat(location); os.IsNotExist(err) {
		// Create leading directories
		leadingDir, _ := filepath.Split(location)
		if err := os.MkdirAll(leadingDir, os.ModePerm); err != nil {
			return err
		}

		// Create file
		out, err := os.Create(location)
		if err != nil {
			return err
		}
		defer out.Close()

		// Write to file
		if err := ioutil.WriteFile(location, []byte(content), os.ModePerm); err != nil {
			return err
		}

		return nil
	}

	return nil
}
