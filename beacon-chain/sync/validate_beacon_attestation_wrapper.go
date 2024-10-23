package sync

import (
	"context"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
)

type validateCommitteeIndexFunc func(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error)

// AttestationStats holds counts of successful and failed attestations.
type AttestationStats struct {
	mu             sync.Mutex
	successful     uint64
	failed         uint64
	failureReasons map[string]uint64
	epoch          uint64
}

func NewAttestationStats(epoch primitives.Epoch) *AttestationStats {
	return &AttestationStats{
		epoch:          uint64(epoch),
		failureReasons: make(map[string]uint64),
	}
}

func (st *AttestationStats) RecordReject(epoch uint64, err error) {
	go func() {
		st.mu.Lock()
		defer st.mu.Unlock()

		st.failed++
		reason := "unknown"
		if err != nil {
			reason = err.Error()
		}
		st.failureReasons[reason]++
		st.printSummaryOnNextEpoch(epoch)
	}()
}

func (st *AttestationStats) RecordAccept(epoch uint64) {
	go func() {
		st.mu.Lock()
		defer st.mu.Unlock()

		st.successful++
		st.printSummaryOnNextEpoch(epoch)
	}()
}

func (st *AttestationStats) printSummaryOnNextEpoch(currentEpoch uint64) {
	if st.epoch != currentEpoch {
		st.printStats()
		st.resetStats()
		st.epoch = currentEpoch
	}
}

func (st *AttestationStats) printStats() {
	log.Infof("Epoch %d Attestation Summary:", st.epoch)
	log.Infof("Successful Attestations: %d", st.successful)
	log.Infof("Failed Attestations: %d", st.failed)
	for reason, count := range st.failureReasons {
		log.Infof("Failure Reason: %s, Count: %d", reason, count)
	}
}

func (st *AttestationStats) resetStats() {
	st.successful = 0
	st.failed = 0
	st.failureReasons = make(map[string]uint64)
}

func (st *AttestationStats) validateWithStatsRecord(validate wrappedVal, epochFn func() uint64) wrappedVal {
	return func(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
		attestation, err := validate(ctx, pid, msg)
		epoch := epochFn()

		if attestation == pubsub.ValidationReject {
			st.RecordReject(epoch, err)
		} else {
			st.RecordAccept(epoch)
		}
		return attestation, err
	}
}

func (s *Service) wrapWithStatsRecord(
	next wrappedVal,
) wrappedVal {
	epochFn := func() uint64 {
		return uint64(slots.ToEpoch(s.cfg.clock.CurrentSlot()))
	}
	return s.attestationStats.validateWithStatsRecord(next, epochFn)
}
