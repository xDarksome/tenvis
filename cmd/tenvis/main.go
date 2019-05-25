package main

import (
	"log"
	"os"

	"github.com/urfave/cli"
)

func configFlag() cli.Flag {
	return cli.StringFlag{
		Name:  "config, c",
		Value: "config.yaml",
		Usage: "load configuration from `FILE`",
	}
}

func privateKeyEnvFlag() cli.Flag {
	return cli.StringFlag{
		Name:  "key, k",
		Value: "PRIVATE_KEY",
		Usage: "load private key from specified env",
	}
}

func main() {
	app := cli.App{
		Name:     "tenvis",
		Commands: []cli.Command{runCmd(), generateKeysCmd()},
		Flags:    []cli.Flag{configFlag(), privateKeyEnvFlag()},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
