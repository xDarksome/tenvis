package tenvis

import (
	"bytes"
	"context"
	"sort"
	"time"

	"github.com/go-redis/redis"

	"github.com/xdarksome/tenvis/key"

	"github.com/sirupsen/logrus"

	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/xdarksome/scp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//go:generate protoc --grpc-gateway_out=logtostderr=true:. tenvis.proto
//go:generate protoc --go_out=plugins=grpc:. tenvis.proto

type PublicKey interface {
	Verify(message, sig []byte) bool
	String() string
}

type PrivateKey interface {
	Sign(message []byte) []byte
	String() string
	Public() string
}

type Config struct {
	Redis       *redis.Client
	Key         PrivateKey
	Peers       map[string]PublicKey
	QuorumSlice *scp.QuorumSlice
}

func (cfg Config) Validate() error {
	if cfg.Redis == nil {
		return errors.New("redis should be set")
	}

	if cfg.Key == nil {
		return errors.New("key should be set")
	}

	if cfg.Peers == nil {
		return errors.New("peers should be set")
	}

	if cfg.QuorumSlice == nil {
		return errors.New("quorum slice should be set")
	}

	return nil
}

type Node struct {
	Streaming
	consensus *scp.Consensus
	storage   Storage
}

func New(cfg Config) (*Node, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	storage := Storage{cfg.Redis}
	idx, err := storage.GetLastLedgerIndex()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get last ledger index from redis")
	}

	node := &Node{
		Streaming: *NewStreaming(cfg.Key, cfg.Peers, storage),
		storage:   storage,
	}

	scpConfig := scp.Config{
		NodeID:       cfg.Key.Public(),
		CurrentSlot:  idx + 1,
		Validator:    node,
		Combiner:     node,
		Ledger:       node,
		SlotsLoader:  node,
		QuorumSlices: []*scp.QuorumSlice{cfg.QuorumSlice},
	}

	node.consensus = scp.New(scpConfig)
	return node, nil
}

func (n *Node) Run() {
	n.Streaming.run()

	go func() {
		for {
			n.Broadcaster.input <- n.consensus.OutputMessage()
		}
	}()
	n.consensus.Run()
	for {
		var m *scp.Message
		select {
		case m = <-n.Listener.output:
			n.consensus.InputMessage(m)
		}
	}
}

func (n *Node) GetVoting(ctx context.Context, req *GetVotingRequest) (*Voting, error) {
	voting, err := n.storage.GetVoting(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to get voting from redis")
	}

	if voting == nil {
		return nil, status.Error(codes.NotFound, "voting not found")
	}

	return voting, nil
}

func (n *Node) CreateVoting(ctx context.Context, op *CreateVotingOperation) (*Voting, error) {
	if err := n.validateCreateVotingOperation(op); err != nil {
		return new(Voting), err
	}

	ops := TenvisOperations{
		CreateVoting: []*CreateVotingOperation{op},
	}

	value, err := proto.Marshal(&ops)
	if err != nil {
		return new(Voting), status.Error(codes.Internal, "failed to marshal operations")
	}

	n.consensus.Propose(value)

	return new(Voting), nil
}

func (n *Node) Vote(ctx context.Context, op *VoteOperation) (*Voting, error) {
	if err := n.validateVoteOperation(op); err != nil {
		return new(Voting), err
	}

	ops := TenvisOperations{
		Vote: []*VoteOperation{op},
	}

	value, err := proto.Marshal(&ops)
	if err != nil {
		return new(Voting), status.Error(codes.Internal, "failed to marshal operations")
	}

	n.consensus.Propose(value)

	return new(Voting), nil
}

func (n *Node) validateSignature(signer string, signature []byte, message proto.Message) bool {
	pub, err := key.ParsePublic(signer)
	if err != nil {
		logrus.WithError(err).Error("validateSignature: failed to parse signer public key")
		return false
	}

	bytes, err := proto.Marshal(message)
	if err != nil {
		logrus.WithError(err).Error("validateSignature: failed to proto marshal message")
		return false
	}

	return pub.Verify(bytes, signature)
}

func (n *Node) validateCreateVotingOperation(op *CreateVotingOperation) error {
	signature := op.Signature
	op.Signature = nil
	if !n.validateSignature(op.Organizer, signature, op) {
		logrus.Warnf("createVotingOperation: invalid signature of %s", op.Organizer)
		return status.Error(codes.Unauthenticated, "invalid signature")
	}
	op.Signature = signature

	if len(op.Options) < 2 {
		return status.Error(codes.InvalidArgument, "options count should be greater than 1")
	}

	return nil
}

func (n *Node) validateVoteOperation(op *VoteOperation) error {
	signature := op.Signature
	op.Signature = nil
	if !n.validateSignature(op.ParticipantId, signature, op) {
		logrus.Warnf("voteOperation: invalid signature of %s", op.ParticipantId)
		return status.Error(codes.Unauthenticated, "invalid signature")
	}
	op.Signature = signature

	voting, err := n.storage.GetVoting(op.VotingId)
	if err != nil {
		return status.Error(codes.NotFound, "voting not found")
	}

	var isParticipant bool
	for _, p := range voting.Participants {
		if p == op.ParticipantId {
			isParticipant = true
			break
		}
	}

	if !isParticipant {
		return status.Error(codes.InvalidArgument, "you are not participant")
	}

	for _, p := range voting.Voted {
		if p == op.ParticipantId {
			return status.Error(codes.AlreadyExists, "you have already voted")
		}
	}

	return nil
}

type combiningValue struct {
	bytes   []byte
	message proto.Message
}

type combiningValues []combiningValue

func (c combiningValues) Len() int {
	return len(c)
}

func (c combiningValues) Less(i, j int) bool {
	return bytes.Compare(c[i].bytes, c[j].bytes) < 0
}

func (c *combiningValues) Swap(i, j int) {
	tmp := (*c)[i]
	(*c)[i] = (*c)[j]
	(*c)[j] = tmp
}

func (n *Node) CombineValues(values ...scp.Value) scp.Value {
	var create combiningValues
	var vote combiningValues

	for _, value := range values {
		operations := new(TenvisOperations)
		if err := proto.Unmarshal(value, operations); err != nil {
			logrus.WithError(err).Panic("combineValues: failed to proto unmarshal tenvis operations")
		}

		for _, op := range operations.CreateVoting {
			b, err := proto.Marshal(op)
			if err != nil {
				logrus.WithError(err).Panic("combineValues: failed to proto marshal create voting op")
			}
			create = append(create, combiningValue{
				bytes:   b,
				message: op,
			})
		}

		for _, op := range operations.Vote {
			b, err := proto.Marshal(op)
			if err != nil {
				logrus.WithError(err).Panic("combineValues: failed to proto marshal vote op")
			}
			vote = append(vote, combiningValue{
				bytes:   b,
				message: op,
			})
		}
	}

	sort.Sort(&create)
	sort.Sort(&vote)

	ops := new(TenvisOperations)
	for _, c := range create {
		ops.CreateVoting = append(ops.CreateVoting, c.message.(*CreateVotingOperation))
	}
	for _, v := range vote {
		ops.Vote = append(ops.Vote, v.message.(*VoteOperation))
	}

	combinedValue, err := proto.Marshal(ops)
	if err != nil {
		logrus.WithError(err).Panic("combineValues: failed to proto marshal result")
	}

	return combinedValue
}

func (n *Node) ValidateValue(v scp.Value) bool {
	operations := new(TenvisOperations)
	if err := proto.Unmarshal(v, operations); err != nil {
		logrus.WithError(err).Error("failed to proto unmarshal tenvis operations")
		return false
	}

	for _, c := range operations.CreateVoting {
		if err := n.validateCreateVotingOperation(c); err != nil {
			logrus.WithError(err).Error("invalid create voting operation")
			return false
		}
	}

	for _, v := range operations.Vote {
		if err := n.validateVoteOperation(v); err != nil {
			logrus.WithError(err).Error("invalid vote operation")
			return false
		}
	}

	return true
}

func (n *Node) applyCreateVotingOp(op *CreateVotingOperation) error {
	last, err := n.storage.GetLastVotingID()
	if err != nil {
		return errors.Wrap(err, "failed to get last voting id")
	}

	voting := &Voting{
		Id:           last + 1,
		Title:        op.Title,
		Organizer:    op.Organizer,
		Participants: op.Participants,
	}
	options := make(map[string]uint32)
	for _, opt := range op.Options {
		options[opt] = 0
	}
	voting.Options = options

	if err := n.storage.SetVoting(voting); err != nil {
		return errors.Wrap(err, "failed to set voting")
	}

	return nil
}

type optionVotes struct {
	name       string
	votesCount uint32
}

func (n *Node) tryFinish(v *Voting) bool {
	options := make([]optionVotes, len(v.Options))
	for opt, votes := range v.Options {
		options = append(options, optionVotes{
			name:       opt,
			votesCount: votes,
		})
	}

	sort.Slice(options, func(i, j int) bool {
		return options[i].votesCount < options[j].votesCount
	})

	first := options[len(options)-1]
	second := options[len(options)-2]

	if first.votesCount > (second.votesCount + uint32(len(v.Participants)) - uint32(len(v.Voted))) {
		v.Result = first.name
		return true
	}

	return false
}

func (n *Node) applyVoteOp(op *VoteOperation) error {
	voting, err := n.storage.GetVoting(op.VotingId)
	if err != nil {
		return errors.Wrap(err, "failed to get voting from storage")
	}

	voting.Voted = append(voting.Voted, op.ParticipantId)
	voting.Options[op.Option]++

	n.tryFinish(voting)
	if err := n.storage.UpdateVoting(voting); err != nil {
		return errors.Wrap(err, "failed to set voting to storage")
	}

	return nil
}

func (n *Node) PersistSlot(s scp.Slot) {
	operations := new(TenvisOperations)
	if err := proto.Unmarshal(s.Value, operations); err != nil {
		logrus.WithError(err).Panic("persistSlot: failed to proto unmarshal tenvis operations")
	}

	for _, op := range operations.CreateVoting {
		if err := n.applyCreateVotingOp(op); err != nil {
			logrus.WithError(err).Panic("persistSlot: failed to apply create voting op")
		}
	}

	for _, op := range operations.Vote {
		if err := n.applyVoteOp(op); err != nil {
			logrus.WithError(err).Panic("persistSlot: failed to apply vote op")
		}
	}

	if err := n.storage.SetLedger(&Ledger{
		Index: s.Index,
		Value: s.Value,
	}); err != nil {
		panic(errors.Wrap(err, "failed to persist ledger"))
	}
}

func (n *Node) LoadSlots(from uint64, to scp.Slot) (slots []scp.Slot) {
	for idx := from; idx < to.Index; idx++ {
		slots = append(slots, n.loadSlot(idx))
	}

	return slots
}

func (n *Node) loadSlot(id uint64) scp.Slot {
	for {
		ledger, err := n.Streaming.Listener.GetLedger(id)
		if err != nil {
			logrus.WithError(err).Error("failed to load ledger from peers")
			time.Sleep(5 * time.Second)
			continue
		}

		return scp.Slot{
			Index: ledger.Index,
			Value: ledger.Value,
		}
	}
}
