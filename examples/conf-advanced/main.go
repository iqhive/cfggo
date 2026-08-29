package main

import (
	"fmt"
	"log"
	"time"

	"github.com/iqhive/cfggo"
	"github.com/iqhive/cfggo/conf"
)

type GlobalConfig struct {
	cfggo.Structure
	Deployment func() string `cfggo:"deployment" default:"development"`
	Region     func() string `cfggo:"region" default:"local"`
}

type APIConfig struct {
	cfggo.Structure
	Port           func() int           `cfggo:"port" default:"8080"`
	Timeout        func() time.Duration `cfggo:"timeout" default:"30s"`
	AllowedOrigins func() []string      `cfggo:"allowed_origins"`
}

type WorkerConfig struct {
	cfggo.Structure
	Concurrency func() int               `cfggo:"concurrency" default:"4"`
	Labels      func() map[string]string `cfggo:"labels"`
}

func main() {
	codec, err := conf.New(conf.WithLimits(conf.Limits{
		MaxInputBytes: 1 << 20,
		MaxEntries:    5_000,
	}))
	if err != nil {
		log.Fatal(err)
	}
	handler, err := conf.NewFileHandler(
		"config.conf",
		false,
		conf.WithCodec(codec),
		conf.RewriteOnSave(),
	)
	if err != nil {
		log.Fatal(err)
	}

	global := &GlobalConfig{}
	api := &APIConfig{}
	worker := &WorkerConfig{}
	group := cfggo.NewGroup(cfggo.GroupWithConfigHandler(handler))
	group.Register("", global, cfggo.WithStrictKeys())
	group.Register("api", api, cfggo.WithStrictKeys())
	group.Register("worker", worker, cfggo.WithStrictKeys())
	if err := group.Init(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("deployment=%s region=%s api=:%d timeout=%s origins=%v worker-concurrency=%d labels=%v\n",
		global.Deployment(), global.Region(), api.Port(), api.Timeout(), api.AllowedOrigins(), worker.Concurrency(), worker.Labels())

	// RewriteOnSave allows a deterministic semantic rewrite. Original comments
	// and formatting are intentionally not preserved.
	if err := worker.Set("concurrency", 12); err != nil {
		log.Fatal(err)
	}
	if err := group.SaveIfChanged(); err != nil {
		log.Fatal(err)
	}
}
