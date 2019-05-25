package tenvis

import (
	"fmt"

	"github.com/go-redis/redis"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

const (
	VotingKey       = "voting"
	VotingSerialKey = "voting_serial"
	LedgersKey      = "ledgers"
	LedgerSerialKey = "ledger_serial"
)

type Storage struct {
	client *redis.Client
}

func (s *Storage) SetVoting(v *Voting) error {
	bytes, err := proto.Marshal(v)
	if err != nil {
		return errors.Wrap(err, "failed to proto marshal")
	}

	return s.client.Watch(func(tx *redis.Tx) error {
		id, err := s.client.Incr(VotingSerialKey).Result()
		if err != nil {
			return errors.Wrap(err, "failed to INCR voting serial")
		}

		if err := s.client.HSet(VotingKey, fmt.Sprint(id), bytes).Err(); err != nil {
			return errors.Wrap(err, "HSET failed")
		}

		return nil
	})
}

func (s *Storage) UpdateVoting(v *Voting) error {
	bytes, err := proto.Marshal(v)
	if err != nil {
		return errors.Wrap(err, "failed to proto marshal")
	}

	if err := s.client.HSet(VotingKey, fmt.Sprint(v.Id), bytes).Err(); err != nil {
		return errors.Wrap(err, "HSET failed")
	}

	return nil
}

func (s *Storage) GetVoting(id uint64) (*Voting, error) {
	bytes, err := s.client.HGet(VotingKey, fmt.Sprint(id)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}

	if err != nil {
		return nil, errors.Wrapf(err, "HGET failed")
	}

	voting := new(Voting)
	if err := proto.Unmarshal(bytes, voting); err != nil {
		return nil, errors.Wrapf(err, "failed to proto unmarshal")
	}

	return voting, nil
}

func (s *Storage) SetLedger(l *Ledger) error {
	bytes, err := proto.Marshal(l)
	if err != nil {
		return errors.Wrap(err, "failed to proto marshal")
	}

	return s.client.Watch(func(tx *redis.Tx) error {
		if err := tx.Set(LedgerSerialKey, l.Index, 0).Err(); err != nil {
			return errors.Wrap(err, "failed to SET ledger index")
		}

		if err := tx.HSet(LedgersKey, fmt.Sprint(l.Index), bytes).Err(); err != nil {
			return errors.Wrap(err, "failed to HSET ledger")
		}

		return nil
	})
}

func (s *Storage) GetLedger(id uint64) (*Ledger, error) {
	bytes, err := s.client.HGet(LedgersKey, fmt.Sprint(id)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}

	if err != nil {
		return nil, errors.Wrapf(err, "HGET failed")
	}

	ledger := new(Ledger)
	if err := proto.Unmarshal(bytes, ledger); err != nil {
		return nil, errors.Wrapf(err, "failed to proto unmarshal")
	}

	return ledger, nil
}

func (s *Storage) GetLastLedgerIndex() (uint64, error) {
	index, err := s.client.Get(LedgerSerialKey).Uint64()
	if err == redis.Nil {
		return 0, nil
	}

	if err != nil {
		return 0, errors.Wrap(err, "failed to GET")
	}

	return index, nil
}

func (s *Storage) GetLastVotingID() (uint64, error) {
	id, err := s.client.Get(VotingSerialKey).Uint64()
	if err == redis.Nil {
		return 0, nil
	}

	if err != nil {
		return 0, errors.Wrap(err, "failed to GET")
	}

	return id, nil
}
