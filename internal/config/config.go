package config

import (
	"encoding/json"
	"os"
)


func ParseConfig() (map[string]interface{}, error) {
	file, err := os.Open("config.json")
	if err != nil {
		return nil, err
	}

	defer file.Close()

	var config map[string]interface{}


	err = json.NewDecoder(file).Decode(&config)

	if err != nil {
		return nil, err
	}

	return config, nil
}