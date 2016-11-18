package main

import (
	"encoding/json"
	"fmt"
	"github.com/bytemine/go-icinga2/event"
	"os"
)

type icingaConfig struct {
	URL      string
	User     string
	Password string
	Insecure bool
	Retries  int
}

type rtConfig struct {
	URL      string
	User     string
	Password string
	Insecure bool
}

type cacheConfig struct {
	File string
}

type ticketConfig struct {
	PermitFunction []event.State
	Nobody         string
	Queue          string
}

type config struct {
	Icinga icingaConfig
	RT     rtConfig
	Cache  cacheConfig
	Ticket ticketConfig
}

var defaultConfig = config{
	Icinga: icingaConfig{
		URL:      "https://monitoring.example.com:5665",
		User:     "root",
		Password: "secret",
		Insecure: true,
		Retries:  5,
	},
	RT: rtConfig{
		URL:      "https://support.example.com",
		User:     "apiuser",
		Password: "secret",
		Insecure: true,
	},
	Cache: cacheConfig{
		File: "/var/lib/icinga2rt/icinga2rt.bolt",
	},
	Ticket: ticketConfig{
		PermitFunction: []event.State{
			event.StateOK,
			event.StateWarning,
			event.StateCritical,
			event.StateUnknown,
		},
		Nobody: "Nobody",
		Queue:  "general",
	},
}

func checkConfig(conf *config) error {
	if conf.Icinga.URL == "" {
		return fmt.Errorf("Icinga.URL must be set.")
	}

	if conf.Icinga.User == "" {
		return fmt.Errorf("Icinga.User must be set.")
	}

	if conf.Icinga.Retries == 0 {
		return fmt.Errorf("Icinga.Retries must be > 0.")
	}

	if conf.Ticket.Queue == "" {
		return fmt.Errorf("Ticket.Queue must be set.")
	}

	if conf.Ticket.Nobody == "" {
		return fmt.Errorf("Ticket.Nobody must be set.")
	}

	if conf.Ticket.PermitFunction == nil || len(conf.Ticket.PermitFunction) == 0 {
		return fmt.Errorf("Ticket.PermitFunction must be set.")
	}

	if conf.Cache.File == "" {
		return fmt.Errorf("Cache.File must be set.")
	}

	return nil
}

func readConfig(filename string) (*config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	var c config

	dec := json.NewDecoder(f)

	err = dec.Decode(&c)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func writeConfig(filename string, c *config) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}

	x, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		return err
	}

	_, err = f.Write(x)
	if err != nil {
		return err
	}

	return nil
}