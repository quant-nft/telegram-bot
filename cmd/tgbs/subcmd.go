package main

import (
	"github.com/quant-nft/telegram-bot/node"
	"github.com/urfave/cli/v2"
	"github.com/xyths/hs"
)

var (
	serveCommand = &cli.Command{
		Action: serve,
		Name:   "serve",
		Usage:  "Serve as daemon",
	}
)

func serve(c *cli.Context) error {
	configFile := c.String(ConfigFlag.Name)
	cfg := node.Config{}
	if err := hs.ParseJsonConfig(configFile, &cfg); err != nil {
		return err
	}
	n := node.New(cfg)
	if err := n.Init(c.Context); err != nil {
		return err
	}
	defer n.Close(c.Context)
	if err := n.Serve(c.Context); err != nil {
		return err
	}
	return nil
}
