package historical_states_reader

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/cl/clparams"
	"github.com/ledgerwatch/erigon/cl/cltypes"
	"github.com/ledgerwatch/erigon/cl/cltypes/solid"
	"github.com/ledgerwatch/erigon/cl/persistence/base_encoding"
	state_accessors "github.com/ledgerwatch/erigon/cl/persistence/state"
	"github.com/ledgerwatch/erigon/cl/phase1/core/state"
	"github.com/ledgerwatch/erigon/cl/phase1/core/state/lru"
	"github.com/ledgerwatch/erigon/cl/utils"
	"github.com/ledgerwatch/erigon/turbo/snapshotsync/freezeblocks"
	"github.com/spf13/afero"
	"golang.org/x/exp/slices"
)

type HistoricalStatesReader struct {
	cfg            *clparams.BeaconChainConfig
	fs             afero.Fs                              // some data is on filesystem to avoid database fragmentation
	validatorTable *state_accessors.StaticValidatorTable // We can save 80% of the I/O by caching the validator table
	blockReader    freezeblocks.BeaconSnapshotReader
	genesisState   *state.CachingBeaconState

	// cache for shuffled sets
	shuffledSetsCache *lru.Cache[uint64, []uint64]
}

func NewHistoricalStatesReader(cfg *clparams.BeaconChainConfig, blockReader freezeblocks.BeaconSnapshotReader, validatorTable *state_accessors.StaticValidatorTable, fs afero.Fs, genesisState *state.CachingBeaconState) *HistoricalStatesReader {

	cache, err := lru.New[uint64, []uint64]("shuffledSetsCache_reader", 125)
	if err != nil {
		panic(err)
	}

	return &HistoricalStatesReader{
		cfg:               cfg,
		fs:                fs,
		blockReader:       blockReader,
		genesisState:      genesisState,
		validatorTable:    validatorTable,
		shuffledSetsCache: cache,
	}
}

func (r *HistoricalStatesReader) ReadHistoricalState(ctx context.Context, tx kv.Tx, slot uint64) (*state.CachingBeaconState, error) {
	ret := state.New(r.cfg)
	latestProcessedState, err := state_accessors.GetStateProcessingProgress(tx)
	if err != nil {
		return nil, err
	}

	// If this happens, we need to update our static tables
	if slot > latestProcessedState || slot > r.validatorTable.Slot() {
		return nil, fmt.Errorf("slot %d is greater than latest processed state %d", slot, latestProcessedState)
	}

	if slot == 0 {
		return r.genesisState.Copy()
	}
	// Read the current block (we need the block header) + other stuff
	block, err := r.blockReader.ReadBlockBySlot(ctx, tx, slot)
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, fmt.Errorf("block at slot %d not found", slot)
	}
	blockHeader := block.SignedBeaconBlockHeader().Header
	blockHeader.Root = common.Hash{}
	// Read the minimal beacon state which have the small fields.
	minimalBeaconState, err := state_accessors.ReadMinimalBeaconState(tx, slot)
	if err != nil {
		return nil, err
	}
	// State not found
	if minimalBeaconState == nil {
		return nil, nil
	}

	// Versioning
	ret.SetVersion(minimalBeaconState.Version)
	ret.SetGenesisTime(r.genesisState.GenesisTime())
	ret.SetGenesisValidatorsRoot(r.genesisState.GenesisValidatorsRoot())
	ret.SetSlot(slot)
	ret.SetFork(minimalBeaconState.Fork)
	// History
	stateRoots, blockRoots := solid.NewHashVector(int(r.cfg.SlotsPerHistoricalRoot)), solid.NewHashVector(int(r.cfg.SlotsPerHistoricalRoot))
	ret.SetLatestBlockHeader(blockHeader)

	if err := r.readHistoryHashVector(tx, r.genesisState.BlockRoots(), slot, r.cfg.SlotsPerHistoricalRoot, kv.BlockRoot, blockRoots); err != nil {
		return nil, fmt.Errorf("failed to read block roots: %w", err)
	}
	ret.SetBlockRoots(blockRoots)

	if err := r.readHistoryHashVector(tx, r.genesisState.StateRoots(), slot, r.cfg.SlotsPerHistoricalRoot, kv.StateRoot, stateRoots); err != nil {
		return nil, fmt.Errorf("failed to read state roots: %w", err)
	}
	ret.SetStateRoots(stateRoots)

	historicalRoots := solid.NewHashList(int(r.cfg.HistoricalRootsLimit))
	if err := state_accessors.ReadHistoricalRoots(tx, minimalBeaconState.HistoricalRootsLength, func(idx int, root common.Hash) error {
		historicalRoots.Append(root)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to read historical roots: %w", err)
	}
	ret.SetHistoricalRoots(historicalRoots)

	// Eth1
	eth1DataVotes := solid.NewStaticListSSZ[*cltypes.Eth1Data](int(r.cfg.Eth1DataVotesLength()), 72)
	if err := r.readEth1DataVotes(tx, minimalBeaconState.Eth1DataLength, slot, eth1DataVotes); err != nil {
		return nil, err
	}
	ret.SetEth1DataVotes(eth1DataVotes)
	ret.SetEth1Data(minimalBeaconState.Eth1Data)
	ret.SetEth1DepositIndex(minimalBeaconState.Eth1DepositIndex)
	// Registry (Validators + Balances)
	balancesBytes, err := r.reconstructBalances(tx, slot, kv.ValidatorBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to read validator balances: %w", err)
	}
	balances := solid.NewUint64ListSSZ(int(r.cfg.ValidatorRegistryLimit))
	if err := balances.DecodeSSZ(balancesBytes, 0); err != nil {
		return nil, fmt.Errorf("failed to decode validator balances: %w", err)
	}
	ret.SetBalances(balances)

	validatorSet, currActiveIdxs, prevActiveIdxs, err := r.readValidatorsForHistoricalState(tx, slot, minimalBeaconState.ValidatorLength)
	if err != nil {
		return nil, fmt.Errorf("failed to read validators: %w", err)
	}
	ret.SetValidators(validatorSet)
	// Randomness
	randaoMixes := solid.NewHashVector(int(r.cfg.EpochsPerHistoricalVector))
	if err := r.readRandaoMixes(tx, slot, randaoMixes); err != nil {
		return nil, fmt.Errorf("failed to read randao mixes: %w", err)
	}
	ret.SetRandaoMixes(randaoMixes)
	slashingsVector := solid.NewUint64VectorSSZ(int(r.cfg.EpochsPerSlashingsVector))
	// Slashings
	err = r.reconstructUint64ListDump(tx, slot, kv.ValidatorSlashings, int(r.cfg.EpochsPerSlashingsVector), slashingsVector)
	if err != nil {
		return nil, fmt.Errorf("failed to read slashings: %w", err)
	}
	ret.SetSlashings(slashingsVector)

	// Finality
	currentCheckpoint, previousCheckpoint, finalizedCheckpoint, err := state_accessors.ReadCheckpoints(tx, r.cfg.RoundSlotToEpoch(slot))
	if err != nil {
		return nil, err
	}
	if currentCheckpoint == nil {
		currentCheckpoint = r.genesisState.CurrentJustifiedCheckpoint()
	}
	if previousCheckpoint == nil {
		previousCheckpoint = r.genesisState.PreviousJustifiedCheckpoint()
	}
	if finalizedCheckpoint == nil {
		finalizedCheckpoint = r.genesisState.FinalizedCheckpoint()
	}
	ret.SetJustificationBits(*minimalBeaconState.JustificationBits)
	ret.SetPreviousJustifiedCheckpoint(previousCheckpoint)
	ret.SetCurrentJustifiedCheckpoint(currentCheckpoint)
	ret.SetFinalizedCheckpoint(finalizedCheckpoint)
	// Participation
	if ret.Version() == clparams.Phase0Version {
		currentAtts, previousAtts, err := r.readPendingEpochs(tx, slot, minimalBeaconState.CurrentEpochAttestationsLength, minimalBeaconState.PreviousEpochAttestationsLength)
		if err != nil {
			return nil, fmt.Errorf("failed to read pending attestations: %w", err)
		}
		ret.SetCurrentEpochAttestations(currentAtts)
		ret.SetPreviousEpochAttestations(previousAtts)
	} else {
		currentIdxs, previousIdxs, err := r.readPartecipations(tx, slot, minimalBeaconState.ValidatorLength, currActiveIdxs, prevActiveIdxs, ret, currentCheckpoint, previousCheckpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to read participations: %w", err)
		}
		ret.SetCurrentEpochParticipation(currentIdxs)
		ret.SetPreviousEpochParticipation(previousIdxs)
	}

	if ret.Version() < clparams.AltairVersion {
		return ret, ret.InitBeaconState()
	}
	inactivityScores := solid.NewUint64ListSSZ(int(r.cfg.ValidatorRegistryLimit))
	// Inactivity
	err = r.reconstructUint64ListDump(tx, slot, kv.InactivityScores, int(minimalBeaconState.ValidatorLength), inactivityScores)
	if err != nil {
		return nil, fmt.Errorf("failed to read inactivity scores: %w", err)
	}

	ret.SetInactivityScoresRaw(inactivityScores)
	// Sync
	syncCommitteeSlot := r.cfg.RoundSlotToSyncCommitteePeriod(slot)
	currentSyncCommittee, err := state_accessors.ReadCurrentSyncCommittee(tx, syncCommitteeSlot)
	if err != nil {
		return nil, fmt.Errorf("failed to read current sync committee: %w", err)
	}
	if currentSyncCommittee == nil {
		currentSyncCommittee = r.genesisState.CurrentSyncCommittee()
	}

	nextSyncCommittee, err := state_accessors.ReadNextSyncCommittee(tx, syncCommitteeSlot)
	if err != nil {
		return nil, fmt.Errorf("failed to read next sync committee: %w", err)
	}
	if nextSyncCommittee == nil {
		nextSyncCommittee = r.genesisState.NextSyncCommittee()
	}
	ret.SetCurrentSyncCommittee(currentSyncCommittee)
	ret.SetNextSyncCommittee(nextSyncCommittee)
	// Execution
	if ret.Version() < clparams.BellatrixVersion {
		return ret, ret.InitBeaconState()
	}
	payloadHeader, err := block.Block.Body.ExecutionPayload.PayloadHeader()
	if err != nil {
		return nil, fmt.Errorf("failed to read payload header: %w", err)
	}
	ret.SetLatestExecutionPayloadHeader(payloadHeader)
	if ret.Version() < clparams.CapellaVersion {
		return ret, ret.InitBeaconState()
	}

	// Withdrawals
	ret.SetNextWithdrawalIndex(minimalBeaconState.NextWithdrawalIndex)
	ret.SetNextWithdrawalValidatorIndex(minimalBeaconState.NextWithdrawalValidatorIndex)
	// Deep history valid from Capella onwards
	historicalSummaries := solid.NewStaticListSSZ[*cltypes.HistoricalSummary](int(r.cfg.HistoricalRootsLimit), 64)
	if err := state_accessors.ReadHistoricalSummaries(tx, minimalBeaconState.HistoricalSummariesLength, func(idx int, historicalSummary *cltypes.HistoricalSummary) error {
		historicalSummaries.Append(historicalSummary)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to read historical summaries: %w", err)
	}
	ret.SetHistoricalSummaries(historicalSummaries)
	return ret, ret.InitBeaconState()
}

func (r *HistoricalStatesReader) readHistoryHashVector(tx kv.Tx, genesisVector solid.HashVectorSSZ, slot, size uint64, table string, out solid.HashVectorSSZ) (err error) {
	var needFromGenesis, inserted uint64
	if size > slot || slot-size <= r.genesisState.Slot() {
		needFromGenesis = size - (slot - r.genesisState.Slot())
	}

	needFromDB := size - needFromGenesis
	cursor, err := tx.Cursor(table)
	if err != nil {
		return err
	}
	defer cursor.Close()
	var currKeySlot uint64
	for k, v, err := cursor.Seek(base_encoding.Encode64ToBytes4(slot - needFromDB)); err == nil && k != nil; k, v, err = cursor.Next() {
		if len(v) != 32 {
			return fmt.Errorf("invalid key %x", k)
		}
		currKeySlot = base_encoding.Decode64FromBytes4(k)
		out.Set(int(currKeySlot%size), common.BytesToHash(v))
		inserted++
		if inserted == needFromDB {
			break
		}
	}
	for i := 0; i < int(needFromGenesis); i++ {
		currKeySlot++
		out.Set(int(currKeySlot%size), genesisVector.Get(int(currKeySlot%size)))
	}
	return nil
}

func (r *HistoricalStatesReader) readEth1DataVotes(tx kv.Tx, eth1DataVotesLength, slot uint64, out *solid.ListSSZ[*cltypes.Eth1Data]) error {
	initialSlot := r.cfg.RoundSlotToVotePeriod(slot)
	initialKey := base_encoding.Encode64ToBytes4(initialSlot)
	cursor, err := tx.Cursor(kv.Eth1DataVotes)
	if err != nil {
		return err
	}
	defer cursor.Close()
	k, v, err := cursor.Seek(initialKey)
	if err != nil {
		return err
	}
	if initialSlot <= r.genesisState.Slot() {
		// We need to prepend the genesis votes
		for i := 0; i < r.genesisState.Eth1DataVotes().Len(); i++ {
			out.Append(r.genesisState.Eth1DataVotes().Get(i))
		}
	}

	endSlot := r.cfg.RoundSlotToVotePeriod(slot + r.cfg.SlotsPerEpoch*r.cfg.EpochsPerEth1VotingPeriod)

	for k != nil && base_encoding.Decode64FromBytes4(k) < endSlot {
		if out.Len() >= int(eth1DataVotesLength) {
			break
		}
		eth1Data := &cltypes.Eth1Data{}
		if err := eth1Data.DecodeSSZ(v, 0); err != nil {
			return err
		}
		out.Append(eth1Data)
		k, v, err = cursor.Next()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *HistoricalStatesReader) readRandaoMixes(tx kv.Tx, slot uint64, out solid.HashVectorSSZ) error {
	size := r.cfg.EpochsPerHistoricalVector
	genesisVector := r.genesisState.RandaoMixes()
	var needFromGenesis, inserted uint64
	roundedSlot := r.cfg.RoundSlotToEpoch(slot)
	epoch := slot / r.cfg.SlotsPerEpoch
	genesisEpoch := r.genesisState.Slot() / r.cfg.SlotsPerEpoch
	if size > epoch || epoch-size <= genesisEpoch {
		needFromGenesis = size - (epoch - genesisEpoch)
	}

	needFromDB := size - needFromGenesis
	cursor, err := tx.Cursor(kv.RandaoMixes)
	if err != nil {
		return err
	}
	defer cursor.Close()
	var currKeyEpoch uint64
	for k, v, err := cursor.Seek(base_encoding.Encode64ToBytes4(roundedSlot - (needFromDB)*r.cfg.SlotsPerEpoch)); err == nil && k != nil; k, v, err = cursor.Next() {
		if len(v) != 32 {
			return fmt.Errorf("invalid key %x", k)
		}
		currKeyEpoch = base_encoding.Decode64FromBytes4(k) / r.cfg.SlotsPerEpoch
		out.Set(int(currKeyEpoch%size), common.BytesToHash(v))
		inserted++
		if inserted == needFromDB {
			break
		}
	}
	for i := 0; i < int(needFromGenesis); i++ {
		currKeyEpoch++
		out.Set(int(currKeyEpoch%size), genesisVector.Get(int(currKeyEpoch%size)))
	}
	// Now we need to read the intra epoch randao mix.
	intraRandaoMix, err := tx.GetOne(kv.IntraRandaoMixes, base_encoding.Encode64ToBytes4(slot))
	if err != nil {
		return err
	}
	if len(intraRandaoMix) != 32 {
		return fmt.Errorf("invalid intra randao mix length %d", len(intraRandaoMix))
	}
	out.Set(int(epoch%r.cfg.EpochsPerHistoricalVector), common.BytesToHash(intraRandaoMix))
	return nil
}

func (r *HistoricalStatesReader) reconstructDiffedUint64List(tx kv.Tx, slot uint64, diffBucket string, fileSuffix string) ([]byte, error) {
	// Read the file
	freshDumpSlot := slot - slot%clparams.SlotsPerDump
	_, filePath := clparams.EpochToPaths(freshDumpSlot, r.cfg, fileSuffix)
	file, err := r.fs.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read the diff file
	zstdReader, err := zstd.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer zstdReader.Close()

	lenRaw := uint64(0)
	if err := binary.Read(file, binary.LittleEndian, &lenRaw); err != nil {
		return nil, err
	}
	currentList := make([]byte, lenRaw)

	if _, err = utils.ReadZSTD(zstdReader, currentList); err != nil {
		return nil, err
	}

	// now start diffing
	diffCursor, err := tx.Cursor(diffBucket)
	if err != nil {
		return nil, err
	}
	defer diffCursor.Close()

	for k, v, err := diffCursor.Seek(base_encoding.Encode64ToBytes4(freshDumpSlot)); err == nil && k != nil && base_encoding.Decode64FromBytes4(k) <= slot; k, v, err = diffCursor.Next() {
		if err != nil {
			return nil, err
		}
		if len(k) != 4 {
			return nil, fmt.Errorf("invalid key %x", k)
		}
		if base_encoding.Decode64FromBytes4(k) > slot {
			return nil, fmt.Errorf("diff not found for slot %d", slot)
		}
		s := time.Now()
		currentList, err = base_encoding.ApplyCompressedSerializedUint64ListDiff(currentList, currentList, v)
		if err != nil {
			return nil, err
		}
		fmt.Println("diffing", time.Since(s))
	}

	return currentList, err
}

func (r *HistoricalStatesReader) reconstructBalances(tx kv.Tx, slot uint64, diffBucket string) ([]byte, error) {
	// Read the file
	freshDumpSlot := slot - slot%clparams.SlotsPerDump
	_, filePath := clparams.EpochToPaths(freshDumpSlot, r.cfg, "balances")
	file, err := r.fs.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read the diff file
	zstdReader, err := zstd.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer zstdReader.Close()

	lenRaw := uint64(0)
	if err := binary.Read(file, binary.LittleEndian, &lenRaw); err != nil {
		return nil, err
	}
	currentList := make([]byte, lenRaw)

	if _, err = utils.ReadZSTD(zstdReader, currentList); err != nil {
		return nil, err
	}
	roundedSlot := r.cfg.RoundSlotToEpoch(slot)
	fmt.Println(roundedSlot, freshDumpSlot)
	for i := freshDumpSlot; i < roundedSlot; i += r.cfg.SlotsPerEpoch {
		diff, err := tx.GetOne(diffBucket, base_encoding.Encode64ToBytes4(i))
		if err != nil {
			return nil, err
		}
		if len(diff) == 0 {
			continue
		}
		fmt.Println(i)
		currentList, err = base_encoding.ApplyCompressedSerializedUint64ListDiff(currentList, currentList, diff)
		if err != nil {
			return nil, err
		}
	}

	// now start diffing
	diffCursor, err := tx.Cursor(diffBucket)
	if err != nil {
		return nil, err
	}
	defer diffCursor.Close()

	for k, v, err := diffCursor.Seek(base_encoding.Encode64ToBytes4(roundedSlot)); err == nil && k != nil && base_encoding.Decode64FromBytes4(k) <= slot; k, v, err = diffCursor.Next() {
		if err != nil {
			return nil, err
		}
		if len(k) != 4 {
			return nil, fmt.Errorf("invalid key %x", k)
		}
		if base_encoding.Decode64FromBytes4(k) > slot {
			return nil, fmt.Errorf("diff not found for slot %d", slot)
		}
		s := time.Now()
		currentList, err = base_encoding.ApplyCompressedSerializedUint64ListDiff(currentList, currentList, v)
		if err != nil {
			return nil, err
		}
		fmt.Println("diffing", time.Since(s))
	}

	return currentList, err
}

func (r *HistoricalStatesReader) reconstructUint64ListDump(tx kv.Tx, slot uint64, bkt string, size int, out solid.Uint64ListSSZ) error {
	diffCursor, err := tx.Cursor(bkt)
	if err != nil {
		return err
	}
	defer diffCursor.Close()

	k, v, err := diffCursor.Seek(base_encoding.Encode64ToBytes4(slot))
	if err != nil {
		return err
	}
	if k == nil {
		return fmt.Errorf("diff not found for slot %d", slot)
	}
	keySlot := base_encoding.Decode64FromBytes4(k)
	if keySlot > slot {
		_, v, err = diffCursor.Prev()
		if err != nil {
			return err
		}
	}
	var b bytes.Buffer
	if _, err := b.Write(v); err != nil {
		return err
	}
	// Read the diff file
	zstdReader, err := zstd.NewReader(&b)
	if err != nil {
		return err
	}
	defer zstdReader.Close()
	currentList := make([]byte, size*8)

	if _, err = utils.ReadZSTD(zstdReader, currentList); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to read dump: %w, len: %d", err, len(v))
	}

	return out.DecodeSSZ(currentList, 0)
}

func (r *HistoricalStatesReader) readValidatorsForHistoricalState(tx kv.Tx, slot, validatorSetLength uint64) (*solid.ValidatorSet, []uint64, []uint64, error) {
	out := solid.NewValidatorSetWithLength(int(r.cfg.ValidatorRegistryLimit), int(validatorSetLength))
	// Read the static validator field which are hot in memory (this is > 70% of the whole beacon state)
	activeIds := make([]uint64, 0, validatorSetLength)
	epoch := slot / r.cfg.SlotsPerEpoch

	prevActiveIds := make([]uint64, 0, validatorSetLength)
	if epoch == 0 {
		prevActiveIds = activeIds
	}
	r.validatorTable.ForEach(func(validatorIndex uint64, validator *state_accessors.StaticValidator) bool {
		if validatorIndex >= validatorSetLength {
			return false
		}
		currValidator := out.Get(int(validatorIndex))
		validator.ToValidator(currValidator, slot)
		if currValidator.Active(epoch) {
			activeIds = append(activeIds, validatorIndex)
		}
		if epoch == 0 {
			return true
		}
		if currValidator.Active(epoch - 1) {
			prevActiveIds = append(prevActiveIds, validatorIndex)
		}
		return true
	})
	// Read the balances

	bytesEffectiveBalances, err := r.reconstructDiffedUint64List(tx, slot, kv.ValidatorEffectiveBalance, "effective_balances")
	if err != nil {
		return nil, nil, nil, err
	}
	for i := 0; i < int(validatorSetLength); i++ {
		out.Get(i).
			SetEffectiveBalanceFromBytes(bytesEffectiveBalances[(i * 8) : (i*8)+8])
	}
	return out, activeIds, prevActiveIds, nil
}

func (r *HistoricalStatesReader) readPendingEpochs(tx kv.Tx, slot uint64, currentEpochAttestationsLength, previousEpochAttestationsLength uint64) (*solid.ListSSZ[*solid.PendingAttestation], *solid.ListSSZ[*solid.PendingAttestation], error) {
	if slot < r.cfg.SlotsPerEpoch {
		return r.genesisState.CurrentEpochAttestations(), r.genesisState.PreviousEpochAttestations(), nil
	}
	roundedSlot := r.cfg.RoundSlotToEpoch(slot)
	// Read the current epoch attestations
	currentEpochAttestations, err := state_accessors.ReadCurrentEpochAttestations(tx, roundedSlot, int(r.cfg.CurrentEpochAttestationsLength()))
	if err != nil {
		return nil, nil, err
	}
	previousEpochAttestations, err := state_accessors.ReadPreviousEpochAttestations(tx, roundedSlot, int(r.cfg.PreviousEpochAttestationsLength()))
	if err != nil {
		return nil, nil, err
	}
	previousEpochAttestations.Truncate(int(previousEpochAttestationsLength))
	currentEpochAttestations.Truncate(int(currentEpochAttestationsLength))
	return currentEpochAttestations, previousEpochAttestations, nil
}

// readParticipations shuffles active indicies and returns the participation flags for the given epoch.
func (r *HistoricalStatesReader) readPartecipations(tx kv.Tx, slot uint64, validatorLength uint64,
	currentActiveIndicies, previousActiveIndicies []uint64, ret *state.CachingBeaconState,
	currentJustifiedCheckpoint, previousJustifiedCheckpoint solid.Checkpoint) (*solid.BitList, *solid.BitList, error) {
	randaoMixes := ret.RandaoMixes()
	var beginSlot uint64
	epoch, prevEpoch := r.computeRelevantEpochs(slot)
	beginSlot = prevEpoch * r.cfg.SlotsPerEpoch

	currentIdxs := solid.NewBitList(int(validatorLength), int(r.cfg.ValidatorRegistryLimit))
	previousIdxs := solid.NewBitList(int(validatorLength), int(r.cfg.ValidatorRegistryLimit))
	// trigger the cache for shuffled sets in parallel
	s := time.Now()
	r.tryCachingEpochsInParallell(randaoMixes, [][]uint64{currentActiveIndicies, previousActiveIndicies}, []uint64{epoch, prevEpoch})
	fmt.Println("parallel", time.Since(s))
	// Read the previous idxs
	for i := beginSlot; i <= slot; i++ {
		// Read the block
		block, err := r.blockReader.ReadBlockBySlot(context.Background(), tx, i)
		if err != nil {
			return nil, nil, err
		}
		if block == nil {
			continue
		}
		ret.SetSlot(i)
		currentEpoch := i / r.cfg.SlotsPerEpoch

		// Read the participation flags
		block.Block.Body.Attestations.Range(func(index int, attestation *solid.Attestation, length int) bool {
			data := attestation.AttestantionData()
			isCurrentEpoch := data.Target().Epoch() == currentEpoch
			var activeIndicies []uint64
			// This looks horrible
			if isCurrentEpoch {
				if currentEpoch == prevEpoch {
					activeIndicies = previousActiveIndicies
				} else {
					activeIndicies = currentActiveIndicies
				}
			} else {
				if currentEpoch == prevEpoch {
					return true
				}
				activeIndicies = previousActiveIndicies
			}

			var attestingIndicies []uint64
			attestingIndicies, err = r.attestingIndicies(attestation.AttestantionData(), attestation.AggregationBits(), true, randaoMixes, activeIndicies)
			if err != nil {
				return false
			}
			var participationFlagsIndicies []uint8
			participationFlagsIndicies, err = ret.GetAttestationParticipationFlagIndicies(data, ret.Slot()-data.Slot(), true)
			if err != nil {
				return false
			}
			// apply the flags
			for _, idx := range attestingIndicies {
				for flagIndex := range r.cfg.ParticipationWeights() {
					var flagParticipation cltypes.ParticipationFlags
					if isCurrentEpoch && currentEpoch != prevEpoch {
						flagParticipation = cltypes.ParticipationFlags(currentIdxs.Get(int(idx)))
					} else {
						flagParticipation = cltypes.ParticipationFlags(previousIdxs.Get(int(idx)))
					}
					if !slices.Contains(participationFlagsIndicies, uint8(flagIndex)) || flagParticipation.HasFlag(flagIndex) {
						continue
					}
					if isCurrentEpoch && currentEpoch != prevEpoch {
						currentIdxs.Set(int(idx), byte(flagParticipation.Add(flagIndex)))
					} else {
						previousIdxs.Set(int(idx), byte(flagParticipation.Add(flagIndex)))
					}
				}
			}
			return true
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return currentIdxs, previousIdxs, nil
}

func (r *HistoricalStatesReader) computeRelevantEpochs(slot uint64) (uint64, uint64) {
	epoch := slot / r.cfg.SlotsPerEpoch
	if epoch <= r.cfg.AltairForkEpoch && r.genesisState.Version() < clparams.AltairVersion {
		return epoch, epoch
	}
	return epoch, epoch - 1
}

func (r *HistoricalStatesReader) tryCachingEpochsInParallell(randaoMixes solid.HashVectorSSZ, activeIdxs [][]uint64, epochs []uint64) {
	var wg sync.WaitGroup
	wg.Add(len(epochs))
	for i, epoch := range epochs {
		go func(epoch uint64, idxs []uint64) {
			defer wg.Done()
			_, _ = r.computeCommittee(randaoMixes, idxs, epoch*r.cfg.SlotsPerEpoch, r.cfg.TargetCommitteeSize, 0)
		}(epoch, activeIdxs[i])
	}
	wg.Wait()

}
