package main

import (
	"fmt"
	"log"
	"time"

	"github.com/iqhive/cfggo"
	"github.com/iqhive/cfggo/conf"
)

type Config struct {
	cfggo.Structure
	Name    func() string        `cfggo:"name" default:"my-service"`
	Port    func() int           `cfggo:"port" default:"8080"`
	Debug   func() bool          `cfggo:"debug" default:"false"`
	Timeout func() time.Duration `cfggo:"timeout" default:"30s"`
}

func main() {
	handler, err := conf.NewFileHandler("config.conf", false)
	if err != nil {
		log.Fatal(err)
	}

	config := &Config{}
	if err := config.Init(config,
		cfggo.WithConfigHandler(handler),
		cfggo.WithStrictKeys(),
	); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%s listening on :%d (debug=%t, timeout=%s)\n",
		config.Name(), config.Port(), config.Debug(), config.Timeout())
}
