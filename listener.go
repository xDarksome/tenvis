package tenvis

import (
	"context"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/xdarksome/scp"
	"google.golang.org/grpc"
)

type Listener struct {
	nodes  map[string]*remoteNode
	output chan *scp.Message
}

func NewListener(addresses map[string]PublicKey) *Listener {
	l := &Listener{
		nodes:  map[string]*remoteNode{},
		output: make(chan *scp.Message, 1000000),
	}

	for addr, id := range addresses {
		l.nodes[id.String()] = newRemoteNode(id, addr, l.output)
	}

	return l
}

func (l *Listener) listenTo(n *remoteNode) {
	for {
		if err := n.listen(); err != nil {
			logrus.WithError(err).Errorf("failed to listen to %s (%s)", n.key.String(), n.address)
			time.Sleep(5 * time.Second)
		}
	}
}

func (l *Listener) GetLedger(id uint64) (*Ledger, error) {
	for _, peer := range l.nodes {
		ledger, err := peer.getLedger(id)
		if err != nil {
			logrus.WithError(err).Errorf("failed to get ledger from %s", peer.key.String())
			continue
		}

		return ledger, nil
	}

	return nil, errors.New("failed to get ledger from peers")
}

func (l *Listener) run() {
	for i := range l.nodes {
		go l.listenTo(l.nodes[i])
	}
}

func (l *Listener) Stop() {
	for _, node := range l.nodes {
		node.stop <- struct{}{}
	}
}

type remoteNode struct {
	key     PublicKey
	address string

	stop     chan struct{}
	messages chan *scp.Message
}

func newRemoteNode(id PublicKey, address string, messages chan *scp.Message) *remoteNode {
	return &remoteNode{
		key:      id,
		address:  address,
		stop:     make(chan struct{}),
		messages: messages,
	}
}

func (r *remoteNode) newMessages(messages *SCPMessages) {
	for _, m := range messages.List {
		r.messages <- &scp.Message{
			Type:      scp.MessageType(m.Type),
			SlotIndex: m.SlotIndex,
			NodeID:    r.key.String(),
			Counter:   m.Counter,
			Value:     m.Value,
		}
	}
}

func (r *remoteNode) getLedger(id uint64) (*Ledger, error) {
	conn, err := grpc.Dial(r.address, grpc.WithInsecure())
	if err != nil {
		return nil, errors.Wrap(err, "failed to dial tcp")
	}
	defer conn.Close()

	client := NewTenvisClient(conn)
	message, err := client.GetLedger(context.Background(), &GetLedgerRequest{
		Index: id,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get ledger message")
	}

	signature := message.Signature
	message.Signature = nil

	bytes, err := proto.Marshal(message)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal proto message")
	}

	if !r.key.Verify(bytes, signature) {
		return nil, errors.New("invalid signature")
	}

	return message.Ledger, nil
}

func (r *remoteNode) listen() error {
	conn, err := grpc.Dial(r.address, grpc.WithInsecure())
	if err != nil {
		return errors.Wrap(err, "failed to dial tcp")
	}
	defer conn.Close()

	client := NewTenvisClient(conn)
	streamChannel, err := client.StreamSCPMessages(context.Background(), &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get messageChannel")
	}

	for {
		select {
		case <-r.stop:
			return nil
		default:
			m, err := streamChannel.Recv()
			if err != nil {
				return errors.Wrap(err, "failed to receive message")
			}
			if r.checkSignature(m) {
				r.newMessages(m)
			}
		}
	}
}

func (r *remoteNode) checkSignature(m *SCPMessages) bool {
	signature := m.Signature
	m.Signature = nil

	bytes, err := proto.Marshal(m)
	if err != nil {
		logrus.WithError(err).Errorf("failed to unmarshal proto message from %s", r.key.String())
		return false
	}

	if !r.key.Verify(bytes, signature) {
		logrus.WithError(err).Errorf("received message with invalid signature from %s", r.key.String())
		return false
	}

	return true
}
