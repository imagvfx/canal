package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// readConfigFile reads data from a config file.
func readConfigFile(filename string) ([]byte, error) {
	confd, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(confd + "/" + filename)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return []byte{}, nil
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// writeConfigFile writes data to a config file.
func writeConfigFile(filename string, data []byte) error {
	confd, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Dir(confd+"/"+filename), 0755)
	if err != nil {
		return err
	}
	f, err := os.Create(confd + "/" + filename)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	return nil
}

// removeConfigFile removes a config file.
func removeConfigFile(filename string) error {
	confd, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	err = os.Remove(confd + "/" + filename)
	if err != nil {
		return err
	}
	return nil
}
