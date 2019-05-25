package tenvis

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/xdarksome/scp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type messageChannel struct {
	initial  chan []*SCPMessage
	messages chan *SCPMessage
	done     chan struct{}
}

func (s *messageChannel) Done() {
	s.done <- struct{}{}
}

func newMessageChannel() messageChannel {
	return messageChannel{
		make(chan []*SCPMessage, 1),
		make(chan *SCPMessage, 100),
		make(chan struct{}),
	}
}

type Broadcaster struct {
	key     PrivateKey
	storage Storage

	input  chan *scp.Message
	output map[chan struct{}]messageChannel
	buffer []*SCPMessage
	queue  chan messageChannel
}

func NewBroadcaster(key PrivateKey, storage Storage) *Broadcaster {
	return &Broadcaster{
		key:     key,
		storage: storage,
		input:   make(chan *scp.Message, 1000000),
		output:  make(map[chan struct{}]messageChannel),
		queue:   make(chan messageChannel),
	}
}

func (b *Broadcaster) broadcast(m *SCPMessage) {
	message := &SCPMessage{
		Type:      SCPMessage_Type(m.Type),
		SlotIndex: m.SlotIndex,
		Counter:   m.Counter,
		Value:     m.Value,
	}

	for done, ch := range b.output {
		select {
		case <-done:
			delete(b.output, done)
		default:
			ch.messages <- message
		}
	}
}

func (b *Broadcaster) appendBuffer(m *SCPMessage) {
	if b.buffer == nil || b.buffer[len(b.buffer)-1].SlotIndex != m.SlotIndex {
		b.buffer = []*SCPMessage{m}
		return
	}

	b.buffer = append(b.buffer, m)
}

func (b *Broadcaster) run() {
	for {
		select {
		case m := <-b.input:
			message := &SCPMessage{
				Type:      SCPMessage_Type(m.Type),
				SlotIndex: m.SlotIndex,
				Counter:   m.Counter,
				Value:     m.Value,
			}
			b.appendBuffer(message)
			b.broadcast(message)
		case ch := <-b.queue:
			b.output[ch.done] = ch
			ch.initial <- b.buffer
		}
	}
}

func (b *Broadcaster) newStream() messageChannel {
	s := newMessageChannel()
	b.queue <- s
	return s
}

func (b *Broadcaster) sendMessages(stream Tenvis_StreamSCPMessagesServer, messages ...*SCPMessage) error {
	m := &SCPMessages{
		List: messages,
	}

	bytes, err := proto.Marshal(m)
	if err != nil {
		return status.Error(codes.Internal, "failed to proto marshal output")
	}

	m.Signature = b.key.Sign(bytes)
	if err := stream.Send(m); err != nil {
		return status.Error(codes.Internal, "failed to send output")
	}

	return nil
}

func (b *Broadcaster) StreamSCPMessages(req *empty.Empty, stream Tenvis_StreamSCPMessagesServer) error {
	ch := b.newStream()
	defer ch.Done()

	init := <-ch.initial
	if init != nil {
		if err := b.sendMessages(stream, init...); err != nil {
			return err
		}
	}

	for {
		m := <-ch.messages
		if err := b.sendMessages(stream, m); err != nil {
			return err
		}
	}
}

func (b *Broadcaster) GetLedger(ctx context.Context, req *GetLedgerRequest) (*LedgerMessage, error) {
	ledger, err := b.storage.GetLedger(req.Index)
	if err != nil {
		return new(LedgerMessage), status.Error(codes.Internal, "failed to get ledger from redis")
	}

	if ledger == nil {
		return new(LedgerMessage), status.Error(codes.NotFound, "ledger not found")
	}

	message := &LedgerMessage{
		Ledger: ledger,
	}

	bytes, err := proto.Marshal(message)
	if err != nil {
		return new(LedgerMessage), status.Error(codes.Internal, "failed to proto marshal")
	}
	message.Signature = b.key.Sign(bytes)

	return message, nil
}
