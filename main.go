package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mysql-pitr-helper/collector"
	"mysql-pitr-helper/recoverer"

	"github.com/caarlos0/env"
)

func main() {
	command := "collect"
	var cfgPath string
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if len(os.Args) > 2 {
		cfgPath = os.Args[2]
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	switch command {
	case "collect":
		runCollector(ctx, cfgPath)
	case "recover":
		runRecoverer(ctx)
	default:
		fmt.Fprintf(os.Stderr, "ERROR: unknown command \"%s\".\nCommands:\n  collect - collect binlogs\n  recover - recover from binlogs\n", command)
		os.Exit(1)
	}
}

func runCollector(ctx context.Context, cfgPath string) {
	config, err := getCollectorConfig(cfgPath)
	if err != nil {
		log.Fatalln("ERROR: get config:", err)
	}
	c, err := collector.New(ctx, config)
	if err != nil {
		log.Fatalln("ERROR: new controller:", err)
	}
	log.Println("run binlog collector")
	for {
		timeout, cancel := context.WithTimeout(ctx, time.Duration(config.CollectSpanSec)*time.Second)
		defer cancel()

		err := c.Run(timeout)
		if err != nil {
			log.Fatalln("ERROR:", err)
		}

		t := time.NewTimer(time.Duration(config.CollectSpanSec) * time.Second)
		select {
		case <-ctx.Done():
			log.Fatalln("ERROR:", ctx.Err().Error())
		case <-t.C:
			break
		}
	}
}

func runRecoverer(ctx context.Context) {
	config, err := getRecovererConfig()
	if err != nil {
		log.Fatalln("ERROR: get recoverer config:", err)
	}
	c, err := recoverer.New(ctx, config)
	if err != nil {
		log.Fatalln("ERROR: new recoverer controller:", err)
	}
	log.Println("run recover")
	err = c.Run(ctx)
	if err != nil {
		log.Fatalln("ERROR: recover:", err)
	}
}

func getCollectorConfig(cfgPath string) (collector.Config, error) {
	cfg := collector.Config{}
	cfg.SetDefaults()

	if len(cfgPath) == 0 {
		// Read from envs
		if err := env.Parse(&cfg); err != nil {
			return cfg, err
		}
		if err := env.Parse(&cfg.BackupStorageS3); err != nil {
			return cfg, err
		}
		if err := env.Parse(&cfg.BackupStorageAzure); err != nil {
			return cfg, err
		}
	} else {
		// Read from yaml
		cfgFile, err := os.ReadFile(cfgPath)
		if err != nil {
			return cfg, err
		}
		if err = yaml.Unmarshal(cfgFile, &cfg); err != nil {
			return cfg, err
		}
	}

	v := validator.New()
	if err := v.Struct(cfg); err != nil {
		return cfg, err
	}
	if cfg.StorageType == "s3" {
		if err := v.Struct(cfg.BackupStorageS3); err != nil {
			return cfg, err
		}
	}
	if cfg.StorageType == "azure" {
		if err := v.Struct(cfg.BackupStorageAzure); err != nil {
			return cfg, err
		}
	}

	return cfg, nil
}

func getRecovererConfig() (recoverer.Config, error) {
	cfg := recoverer.Config{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}
	switch cfg.StorageType {
	case "s3":
		if err := env.Parse(&cfg.BinlogStorageS3); err != nil {
			return cfg, err
		}
	case "azure":
		if err := env.Parse(&cfg.BinlogStorageAzure); err != nil {
			return cfg, err
		}
	default:
		return cfg, errors.New("unknown STORAGE_TYPE")
	}

	return cfg, nil
}
