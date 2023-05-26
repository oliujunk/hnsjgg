package config

import (
	"encoding/json"
	"log"
	"os"
)

type GlobalConfig struct {
	Database struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"database"`
}

var GlobalConfiguration GlobalConfig

func init() {
	config, err := os.ReadFile("config.json")
	if err != nil {
		log.Printf(err.Error())
	}
	err = json.Unmarshal(config, &GlobalConfiguration)
	if err != nil {
		log.Printf(err.Error())
	}
}
