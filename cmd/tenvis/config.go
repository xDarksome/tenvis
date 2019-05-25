package main

import (
	"fmt"
	"io/ioutil"

	"github.com/xdarksome/scp"

	"github.com/xdarksome/tenvis/key"

	"github.com/xdarksome/tenvis"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	yaml "gopkg.in/yaml.v2"
)

type config struct {
	Port       int               `yaml:"port"`
	PrivateKey string            `yaml:"private_key"`
	Threshold  uint32            `yaml:"threshold"`
	Quorum     []string          `yaml:"quorum"`
	Addresses  map[string]string `yaml:"addresses"`
	Redis      string            `yaml:"redis"`
}

func (cfg *config) load(c *cli.Context) (*config, error) {
	bytes, err := ioutil.ReadFile(c.GlobalString("config"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to read file")
	}

	if err := yaml.Unmarshal(bytes, cfg); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal")
	}

	//cfg.PrivateKey = os.Getenv(c.GlobalString("key"))

	return cfg, nil
}

func (cfg *config) validate() error {
	if cfg.Port == 0 {
		return fmt.Errorf("port should be set")
	}

	if cfg.PrivateKey == "" {
		return fmt.Errorf("private key should be set")
	}

	if cfg.Quorum == nil {
		return fmt.Errorf("quorum should be set")
	}

	if cfg.Addresses == nil {
		return fmt.Errorf("addresses should be set")
	}

	if cfg.Redis == "" {
		return fmt.Errorf("redis should be set")
	}

	if cfg.Threshold == 0 {
		return fmt.Errorf("threshold should be set")
	}

	return nil
}

func (cfg *config) peers() (map[string]tenvis.PublicKey, error) {
	peers := make(map[string]tenvis.PublicKey)

	for addr, pub := range cfg.Addresses {
		pubKey, err := key.ParsePublic(pub)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid public key: %s", pub)
		}
		peers[addr] = &pubKey
	}

	return peers, nil
}

func (cfg *config) quorumSlice() *scp.QuorumSlice {
	q := scp.QuorumSlice{
		Threshold:  cfg.Threshold,
		Validators: make(map[string]struct{}),
	}

	for _, node := range cfg.Quorum {
		q.Validators[node] = struct{}{}
	}

	return &q
}
