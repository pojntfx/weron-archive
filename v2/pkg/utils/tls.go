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
	"math/big"
	"os"
	"strings"
	"time"
)

var (
	ErrorFingerprintDidNotMatch   = errors.New("fingerprint did not match")
	ErrorManualVerificationFailed = errors.New("manual fingerprint verification failed")
	ErrorCouldNotReadKnownHosts   = errors.New("could not read known hosts file")
	ErrorNoFingerprintFound       = errors.New("could not find fingerprint for address")
	ErrorCouldNotGetUserInput     = errors.New("could not get user input")
	ErrorKnownHostsSyntax         = errors.New("syntax error in known hosts")
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

func GetInteractiveTLSConfig(
	insecureSkipVerify bool,
	knownFingerprint string,
	knownHostsPath string,
	remoteAddress string,
	onGiveUp func(error),
	onMessage func(string, ...interface{}),
	onRead func(string, ...interface{}) (string, error),
) *tls.Config {
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

				onMessage(`%v
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
					input, err := onRead("The authenticity of signaling server '%v' can't be established.\nTLS certificate SHA1 fingerprint is %v.\nAre you sure you want to continue connecting (yes/no/[fingerprint])? ", remoteAddress, fingerprint)
					if err != nil {
						onGiveUp(ErrorCouldNotGetUserInput)

						return err
					}

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
				onMessage(`%v
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
			return "", fmt.Errorf("%v: in line %v", ErrorKnownHostsSyntax, currentLine)
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
