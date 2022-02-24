package encryption

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/pojntfx/weron/pkg/config"
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

				onGiveUp(config.ErrFingerprintDidNotMatch)

				return config.ErrFingerprintDidNotMatch
			}

			// Get known fingerprint from known_hosts
			candidateFingerprint, err := GetKnownHostFingerprint(knownHostsPath, remoteAddress)
			if err != nil {
				if err == config.ErrNoFingerprintFound {
					// Validate SSH-style by typing yes, no or the fingerprint
					input, err := onRead("The authenticity of signaling server '%v' can't be established.\nTLS certificate SHA1 fingerprint is %v.\nAre you sure you want to continue connecting (yes/no/[fingerprint])? ", remoteAddress, fingerprint)
					if err != nil {
						onGiveUp(config.ErrCouldNotGetUserInput)

						return err
					}

					if input == "yes" || input == fingerprint {
						// Add fingerprint to known hosts
						if err := AddKnownHostFingerprint(knownHostsPath, remoteAddress, fingerprint); err != nil {
							onGiveUp(config.ErrFingerprintDidNotMatch)

							return err
						}

						return nil
					}
				} else {
					onGiveUp(config.ErrCouldNotReadKnownHosts)

					return config.ErrCouldNotReadKnownHosts
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

				onGiveUp(config.ErrFingerprintDidNotMatch)

				return config.ErrFingerprintDidNotMatch

			}

			// User entered !yes or wrong fingerprint
			onGiveUp(config.ErrManualVerificationFailed)

			return config.ErrManualVerificationFailed
		},
	}
}

func GetKnownHostFingerprint(configFileLocation string, raddr string) (string, error) {
	// Open config file
	file, err := os.OpenFile(configFileLocation, os.O_CREATE|os.O_RDONLY, os.ModePerm)
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
			return "", fmt.Errorf("%v: in line %v", config.ErrKnownHostsSyntax, currentLine)
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
		return "", config.ErrNoFingerprintFound
	}

	return fingerprint, nil
}

func AddKnownHostFingerprint(configFileLocation string, raddr string, fingerprint string) error {
	// Open config file
	file, err := os.OpenFile(configFileLocation, os.O_CREATE|os.O_APPEND|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	// Append to file
	_, err = file.WriteString(raddr + " " + fingerprint)

	return err
}

// See https://play.golang.org/p/GTxajcr3NY
func GetFingerprint(cert []byte) string {
	rawHash := sha1.Sum(cert)

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
