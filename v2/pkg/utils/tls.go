package utils

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrorFingerprintDidNotMatch   = errors.New("fingerprint did not match")
	ErrorManualVerificationFailed = errors.New("manual fingerprint verification failed")
	ErrorCouldNotReadKnownHosts   = errors.New("could not read known hosts file")
	ErrorNoFingerprintFound       = errors.New("could not find fingerprint for address")
)

const (
	SSHLikePreamble = `@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!
Someone could be eavesdropping on you right now (man-in-the-middle attack)!
It is also possible that a TLS certificate has just been changed.`
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

func GetInteractiveTLSConfig(insecureSkipVerify bool, knownFingerprint string, knownHostsPath string, remoteAddress string, onGiveUp func(error)) *tls.Config {
	if insecureSkipVerify {
		return &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	return &tls.Config{
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			fingerprint := GetFingerprint(rawCerts[0])

			// Validate using pre-shared fingerprint
			if knownFingerprint != "" {
				if fingerprint == knownFingerprint {
					return nil
				}

				fmt.Printf(`%v
TLS certificate SHA1 fingerprint is %v.
Please contact your system administrator.
Provide correct TLS certificate fingerprint to get rid of this message.
TLS certificate verification failed.
`, SSHLikePreamble, fingerprint)

				onGiveUp(ErrorFingerprintDidNotMatch)

				return ErrorFingerprintDidNotMatch
			}

			// Get known fingerprint from known_hosts
			candidateFingerprint, err := GetKnownHostFingerprint(knownHostsPath, remoteAddress)
			if err != nil {
				if err == ErrorNoFingerprintFound {
					// Validate SSH-style by typing yes, no or the fingerprint
					fmt.Printf("The authenticity of signaling server '%v' can't be established.\nTLS certificate SHA1 fingerprint is %v.\nAre you sure you want to continue connecting (yes/no/[fingerprint])? ", remoteAddress, fingerprint)

					// Read answer
					scanner := bufio.NewScanner(os.Stdin)
					scanner.Scan()
					if scanner.Err() != nil {
						onGiveUp(ErrorFingerprintDidNotMatch)

						return err
					}

					// Check if input is yes, the fingerprint or anything else
					input := strings.TrimSuffix(scanner.Text(), "\n")
					if input == "yes" || input == fingerprint {
						// Add fingerprint to known hosts
						if err := AddKnownHostFingerprint(knownHostsPath, remoteAddress, fingerprint); err != nil {
							onGiveUp(ErrorFingerprintDidNotMatch)

							return err
						}

						return nil
					}
				} else {
					onGiveUp(ErrorCouldNotReadKnownHosts)

					return ErrorCouldNotReadKnownHosts
				}
			} else if candidateFingerprint == fingerprint {
				// User has manually trusted cert, continue

				return nil
			} else if candidateFingerprint != fingerprint {
				// Invalid cert
				fmt.Printf(`%v
TLS certificate SHA1 fingerprint is %v.
Please contact your system administrator.
Add correct TLS certificate fingerprint in %v to get rid of this message.
TLS certificate verification failed.
`, SSHLikePreamble, fingerprint, knownHostsPath)

				onGiveUp(ErrorFingerprintDidNotMatch)

				return ErrorFingerprintDidNotMatch

			}

			// User entered !yes or wrong fingerprint
			onGiveUp(ErrorManualVerificationFailed)

			return ErrorManualVerificationFailed
		},
	}
}

func GetKnownHostFingerprint(configFileLocation string, raddr string) (string, error) {
	// Open config file
	file, err := os.Open(configFileLocation)
	if err != nil {
		return "", err
	}
	defer file.Close()

	currentLine := 1
	fingerprint := ""

	// Read file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}

		line := scanner.Text()

		// Split the line into address and fingerprint parts
		parts := strings.Split(line, " ")
		if len(parts) < 2 {
			return "", fmt.Errorf("could not get address or fingerprint: syntax error in known_hosts in line %v", currentLine)
		}

		candidateRaddr := parts[0]
		candidateFingerprint := parts[1]

		// If address matches, break
		if candidateRaddr == raddr {
			fingerprint = candidateFingerprint

			break
		}

		currentLine++
	}

	// No fingerprint found
	if fingerprint == "" {
		return "", ErrorNoFingerprintFound
	}

	return fingerprint, nil
}

func AddKnownHostFingerprint(configFileLocation string, raddr string, fingerprint string) error {
	// Open config file
	file, err := os.OpenFile(configFileLocation, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	// Append to file
	_, err = file.WriteString(raddr + " " + fingerprint)

	return err
}
