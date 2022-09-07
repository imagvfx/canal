package main

import (
	"errors"
	"io"
	"os"
)

// readConfigFile reads data from a config file.
func readConfigFile(filename string) ([]byte, error) {
	confd, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(confd + "/canal/" + filename)
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
	err = os.MkdirAll(confd+"/canal", 0755)
	if err != nil {
		return err
	}
	f, err := os.Create(confd + "/canal/" + filename)
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
	err = os.Remove(confd + "/canal/" + filename)
	if err != nil {
		return err
	}
	return nil
}
