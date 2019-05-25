package main

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/xdarksome/tenvis/key"

	"github.com/xdarksome/tenvis"
	"google.golang.org/grpc"
)

func main() {
	conn, err := grpc.Dial(":9753", grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	client := tenvis.NewTenvisClient(conn)

	pub, priv := key.Generate()

	v1pub, v1priv := key.Generate()
	v2pub, v2priv := key.Generate()
	v3pub, v3priv := key.Generate()
	v4pub, v4priv := key.Generate()

	op := &tenvis.CreateVotingOperation{
		Title:        "Choose",
		Organizer:    pub.String(),
		Participants: []string{v1pub.String(), v2pub.String(), v3pub.String(), v4pub.String()},
		Options:      []string{"A", "B", "C"},
	}
	bytes, err := proto.Marshal(op)
	if err != nil {
		panic(err)
	}
	op.Signature = priv.Sign(bytes)

	_, err = client.CreateVoting(context.Background(), op)
	if err != nil {
		panic(err)
	}

	time.Sleep(time.Second * 10)

	op2 := &tenvis.VoteOperation{
		ParticipantId: v1pub.String(),
		VotingId:      15,
		Option:        "A",
	}
	bytes, err = proto.Marshal(op2)
	if err != nil {
		panic(err)
	}
	op2.Signature = v1priv.Sign(bytes)
	_, err = client.Vote(context.Background(), op2)
	if err != nil {
		panic(err)
	}

	time.Sleep(time.Second * 10)

	op2 = &tenvis.VoteOperation{
		ParticipantId: v2pub.String(),
		VotingId:      15,
		Option:        "B",
	}
	bytes, err = proto.Marshal(op2)
	if err != nil {
		panic(err)
	}
	op2.Signature = v2priv.Sign(bytes)
	_, err = client.Vote(context.Background(), op2)
	if err != nil {
		panic(err)
	}

	time.Sleep(time.Second * 10)

	op2 = &tenvis.VoteOperation{
		ParticipantId: v3pub.String(),
		VotingId:      15,
		Option:        "C",
	}
	bytes, err = proto.Marshal(op2)
	if err != nil {
		panic(err)
	}
	op2.Signature = v3priv.Sign(bytes)
	_, err = client.Vote(context.Background(), op2)
	if err != nil {
		panic(err)
	}

	time.Sleep(time.Second * 10)

	op2 = &tenvis.VoteOperation{
		ParticipantId: v4pub.String(),
		VotingId:      15,
		Option:        "A",
	}
	bytes, err = proto.Marshal(op2)
	if err != nil {
		panic(err)
	}
	op2.Signature = v4priv.Sign(bytes)
	_, err = client.Vote(context.Background(), op2)
	if err != nil {
		panic(err)
	}
}
