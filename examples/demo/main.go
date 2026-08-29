package main

import (
	"fmt"
	"log"
	"time"

	"github.com/iqhive/cfggo"
	"github.com/iqhive/cfggo/cfglogger"
)

type Config struct {
	cfggo.Structure

	Port     func() int    `cfggo:"port" default:"8080" help:"HTTP listen port"`
	LogLevel func() string `cfggo:"log_level" default:"info" help:"Logging level"`
}

func main() {
	config := &Config{}
	if err := cfggo.Init(config,
		cfggo.WithFileConfig("config.json"),
		cfggo.WithoutFlags(),
		cfggo.WithLogger(&cfglogger.NoopLogger{}),
	); err != nil {
		log.Fatalf("load config: %v", err)
	}

	config.OnChange(func(changes []cfggo.Change) {
		fmt.Println("\nconfig reloaded")
		for _, change := range changes {
			fmt.Printf("  %s: %v -> %v\n", change.Key, change.Old, change.New)
		}
		fmt.Printf("typed accessors: port=%d log_level=%s\n\n", config.Port(), config.LogLevel())
	})

	fmt.Printf("app running: port=%d log_level=%s\n", config.Port(), config.LogLevel())
	go func() {
		for {
			time.Sleep(500 * time.Millisecond)
			_ = config.ReloadConfig()
		}
	}()
	select {}
}
