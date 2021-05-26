package config

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	ErrorNoFingerprintFound = "could not find fingerprint for address"
)

func CreateKnownHostsIfNotExists(configFileLocation string) error {
	// If config file does not exist, create and write to it
	if _, err := os.Stat(configFileLocation); os.IsNotExist(err) {
		// Create leading directories
		leadingDir, _ := filepath.Split(configFileLocation)
		if err := os.MkdirAll(leadingDir, os.ModePerm); err != nil {
			return err
		}

		// Create file
		out, err := os.Create(configFileLocation)
		if err != nil {
			return err
		}
		defer out.Close()

		return nil
	}

	return nil
}

func GetKnownHostFingerprint(configFileLocation string, raddr string) (string, error) {
	// Open config file
	file, err := os.Open(configFileLocation)
	if err != nil {
		log.Fatal(err)
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
		return "", errors.New(ErrorNoFingerprintFound)
	}

	return fingerprint, nil
}

func AddKnownHostFingerprint(configFileLocation string, raddr string, fingerprint string) error {
	// Open config file
	file, err := os.OpenFile(configFileLocation, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Append to file
	_, err = file.WriteString(raddr + " " + fingerprint)

	return err
}
