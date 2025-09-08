package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
)

func LoadConfig(filePath string) (*Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not open config file: %v", err)
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	bytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("could not read config file: %v", err)
	}

	var config Config
	if err := json.Unmarshal(bytes, &config); err != nil {
		return nil, fmt.Errorf("could not unmarshal config JSON: %v", err)
	}

	// Override server host and port with environment variables if set
	if serverHost := os.Getenv("SERVER_HOST"); serverHost != "" {
		config.Server.Host = serverHost
	}
	if serverPort := os.Getenv("SERVER_PORT"); serverPort != "" {
		if port, err := strconv.Atoi(serverPort); err == nil {
			config.Server.Port = port
		}
	}

	return &config, nil
}
