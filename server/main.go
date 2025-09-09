package main

import (
	"github.com/amitschendel/curing/pkg/config"
	"github.com/amitschendel/curing/pkg/server"
)

func main() {
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		panic(err)
	}
	s, err := server.NewServer(cfg.Server.Port, "commands.json")
	if err != nil {
		panic(err)
	}
	s.Run()
}
