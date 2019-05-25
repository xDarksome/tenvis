package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/xdarksome/tenvis/key"

	"github.com/go-redis/redis"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/pkg/errors"
	"github.com/soheilhy/cmux"
	"github.com/urfave/cli"
	"github.com/xdarksome/tenvis"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

func runCmd() cli.Command {
	return cli.Command{
		Name:   "run",
		Usage:  "run tenvis node",
		Action: runAction,
	}
}

func runAction(c *cli.Context) error {
	cfg, err := new(config).load(c)
	if err != nil {
		return errors.Wrap(err, "failed to load config")
	}

	if err := cfg.validate(); err != nil {
		return errors.Wrap(err, "invalid config")
	}

	peers, err := cfg.peers()
	if err != nil {
		return errors.Wrap(err, "invalid addresses")
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: cfg.Redis,
	})

	k, err := key.ParsePrivate(cfg.PrivateKey)
	if err != nil {
		return errors.Wrap(err, "invalid private key")
	}

	tenvisCfg := tenvis.Config{
		Redis:       redisClient,
		Key:         &k,
		Peers:       peers,
		QuorumSlice: cfg.quorumSlice(),
	}

	node, err := tenvis.New(tenvisCfg)
	if err != nil {
		return errors.Wrap(err, "failed to initialize tenvis node")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return errors.Wrap(err, "failed to run net listener")
	}

	mult := cmux.New(listener)
	httpListener := mult.Match(cmux.HTTP1Fast())
	grpcListener := mult.Match(cmux.Any())

	grpcServer := grpc.NewServer()
	tenvis.RegisterTenvisServer(grpcServer, node)

	mux := runtime.NewServeMux()
	err = tenvis.RegisterTenvisHandlerFromEndpoint(context.Background(), mux, fmt.Sprintf("localhost:%d", cfg.Port), []grpc.DialOption{grpc.WithInsecure()})
	if err != nil {
		return errors.Wrap(err, "failed to start reverse proxy")
	}

	g := new(errgroup.Group)
	g.Go(func() error { return errors.Wrap(mult.Serve(), "multiplexer failed") })
	g.Go(func() error { return errors.Wrap(grpcServer.Serve(grpcListener), "grpc server failed") })
	g.Go(func() error { return errors.Wrap(http.Serve(httpListener, mux), "http server failed") })
	go node.Run()

	return g.Wait()
}
