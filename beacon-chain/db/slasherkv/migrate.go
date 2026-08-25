package slasherkv

import (
	"context"
	"encoding/binary"
	"time"

	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

// Migrate , its corresponding usage and tests can be totally removed once Electra is on mainnet.
// Previously, the first 8 bytes of keys of `attestation-data-roots` and `proposal-records` buckets
// were stored as little-endian respectively round (then: epoch) and slots. It was the source of
// https://github.com/prysmaticlabs/prysm/issues/14142 and potentially
// https://github.com/prysmaticlabs/prysm/issues/13658.
// To solve this (or these) issue(s), we decided to store the first 8 bytes of keys as big-endian.
// See https://github.com/prysmaticlabs/prysm/pull/14151.
// However, not to break the backward compatibility, we need to migrate the existing data.
// The strategy is quite simple: If, for these bucket keys in the store, we detect
// a slot (resp. epoch) higher, than the current slot (resp. epoch), then we consider that the data
// is stored in little-endian. We create a new entry with the same value, but with the slot (resp. epoch)
// part in the key stored as a big-endian.
// We start the iterate by the highest key and iterate down until we reach the current slot (resp. epoch).
func (s *Store) Migrate(ctx context.Context, headRound, maxPruningRound primitives.Round, batchSize int) error {
	// Migrate attestations.
	log.Info("Starting migration of attestations. This may take a while.")
	start := time.Now()

	if err := s.migrateAttestations(ctx, headRound, maxPruningRound, batchSize); err != nil {
		return errors.Wrap(err, "migrate attestations")
	}

	log.WithField("duration", time.Since(start)).Info("Migration of attestations completed successfully")

	// Migrate proposals.
	log.Info("Starting migration of proposals. This may take a while.")
	start = time.Now()

	if err := s.migrateProposals(ctx, headRound, maxPruningRound, batchSize); err != nil {
		return errors.Wrap(err, "migrate proposals")
	}

	log.WithField("duration", time.Since(start)).Info("Migration of proposals completed successfully")

	return nil
}

func (s *Store) migrateAttestations(ctx context.Context, headRound, maxPruningRound primitives.Round, batchSize int) error {
	done := false
	var roundLittleEndian uint64

	for !done {
		count := 0

		if err := s.db.Update(func(tx *bolt.Tx) error {
			signingRootsBkt := tx.Bucket(attestationDataRootsBucket)
			attRecordsBkt := tx.Bucket(attestationRecordsBucket)

			// We begin a migrating iteration starting from the last item in the bucket.
			c := signingRootsBkt.Cursor()
			for k, v := c.Last(); k != nil; k, v = c.Prev() {
				if count >= batchSize {
					log.WithField("round", roundLittleEndian).Info("Migrated attestations")

					return nil
				}

				// Check if the context is done.
				if ctx.Err() != nil {
					return ctx.Err()
				}

				// Extract the round encoded in the first 8 bytes of the key.
				encodedRound := k[:8]

				// Convert it to an uint64, considering it is stored as big-endian.
				roundBigEndian := binary.BigEndian.Uint64(encodedRound)

				// If the round is smaller or equal to the current round, we are done.
				if roundBigEndian <= uint64(headRound) {
					break
				}

				// Otherwise, we consider that the round is stored as little-endian.
				roundLittleEndian = binary.LittleEndian.Uint64(encodedRound)

				// Increment the count of migrated items.
				count++

				// If the round is still higher than the current round, then it is an issue.
				// This should never happen.
				if roundLittleEndian > uint64(headRound) {
					log.WithFields(logrus.Fields{
						"roundLittleEndian": roundLittleEndian,
						"roundBigEndian":    roundBigEndian,
						"headRound":         headRound,
					}).Error("Round is higher than the current round both if stored as little-endian or as big-endian")

					continue
				}

				round := primitives.Round(roundLittleEndian)
				if err := signingRootsBkt.Delete(k); err != nil {
					return err
				}

				// We don't bother migrating data that is going to be pruned by the pruning routine.
				if round <= maxPruningRound {
					if err := attRecordsBkt.Delete(v); err != nil {
						return err
					}

					continue
				}

				// Create a new key with the round stored as big-endian.
				newK := make([]byte, 8)
				binary.BigEndian.PutUint64(newK, uint64(round))
				newK = append(newK, k[8:]...)

				// Store the same value with the new key.
				if err := signingRootsBkt.Put(newK, v); err != nil {
					return err
				}
			}

			done = true

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) migrateProposals(ctx context.Context, headRound, maxPruningRound primitives.Round, batchSize int) error {
	done := false

	if !done {
		count := 0

		// Compute the max pruning slot.
		maxPruningSlot, err := slots.RoundEnd(maxPruningRound)
		if err != nil {
			return errors.Wrap(err, "compute max pruning slot")
		}

		// Compute the head slot.
		headSlot, err := slots.RoundEnd(headRound)
		if err != nil {
			return errors.Wrap(err, "compute head slot")
		}

		if err := s.db.Update(func(tx *bolt.Tx) error {
			proposalBkt := tx.Bucket(proposalRecordsBucket)

			// We begin a migrating iteration starting from the last item in the bucket.
			c := proposalBkt.Cursor()
			for k, v := c.Last(); k != nil; k, v = c.Prev() {
				if count >= batchSize {
					return nil
				}

				// Check if the context is done.
				if ctx.Err() != nil {
					return ctx.Err()
				}

				// Extract the slot encoded in the first 8 bytes of the key.
				encodedSlot := k[:8]

				// Convert it to an uint64, considering it is stored as big-endian.
				slotBigEndian := binary.BigEndian.Uint64(encodedSlot)

				// If the slot is smaller or equal to the current slot, we are done.
				if slotBigEndian <= uint64(headSlot) {
					break
				}

				// Otherwise, we consider that the slot is stored as little-endian.
				slotLittleEndian := binary.LittleEndian.Uint64(encodedSlot)

				// If the slot is still higher than the current slot, then it is an issue.
				// This should never happen.
				if slotLittleEndian > uint64(headSlot) {
					log.WithFields(logrus.Fields{
						"slotLittleEndian": slotLittleEndian,
						"slotBigEndian":    slotBigEndian,
						"headSlot":         headSlot,
					}).Error("Slot is higher than the current slot both if stored as little-endian or as big-endian")

					continue
				}

				slot := primitives.Slot(slotLittleEndian)
				if err := proposalBkt.Delete(k); err != nil {
					return err
				}

				// We don't bother migrating data that is going to be pruned by the pruning routine.
				if slot <= maxPruningSlot {
					continue
				}

				// Create a new key with the slot stored as big-endian.
				newK := make([]byte, 8)
				binary.BigEndian.PutUint64(newK, uint64(slot))
				newK = append(newK, k[8:]...)

				// Store the same value with the new key.
				if err := proposalBkt.Put(newK, v); err != nil {
					return err
				}
			}

			done = true

			return nil
		}); err != nil {
			return err
		}
	}

	return nil
}
