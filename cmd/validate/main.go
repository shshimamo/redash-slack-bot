package main

import (
	"fmt"
	"log"
	"os"

	"github.com/shshimamo/redash-slack-bot/internal/config"
)

func main() {
	configPath := "configs/queries.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Config validation failed: %v", err)
	}

	fmt.Printf("OK: %s\n", configPath)
}
