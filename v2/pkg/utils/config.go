package utils

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

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
