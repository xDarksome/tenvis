package tenvis

import "context"

type Streaming struct {
	Listener
	Broadcaster
}

func NewStreaming(key PrivateKey, addresses map[string]PublicKey, storage Storage) *Streaming {
	return &Streaming{
		Listener:    *NewListener(addresses),
		Broadcaster: *NewBroadcaster(key, storage),
	}
}

func (s *Streaming) run() {
	go s.Broadcaster.run()
	go s.Listener.run()
}

func (s *Streaming) ID() string {
	return s.Broadcaster.key.String()
}

func (s *Streaming) GetLedger(ctx context.Context, req *GetLedgerRequest) (*LedgerMessage, error) {
	return s.Broadcaster.GetLedger(ctx, req)
}
