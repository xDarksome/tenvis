package main

import (
	"fmt"

	"github.com/urfave/cli"
	"github.com/xdarksome/tenvis/key"
)

func generateKeysCmd() cli.Command {
	return cli.Command{
		Name:   "generate-keys",
		Usage:  "generate public/private key pair",
		Action: generateKeysAction,
	}
}

func generateKeysAction(c *cli.Context) error {
	pub, private := key.Generate()
	fmt.Println(pub.String())
	fmt.Println(private.String())

	return nil
}
