package node

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/version"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
)

type l2EthClient interface {
	InfoByHash(ctx context.Context, hash common.Hash) (eth.BlockInfo, error)
	// GetProof returns a proof of the account, it may return a nil result without error if the address was not found.
	// Optionally keys of the account storage trie can be specified to include with corresponding values in the proof.
	GetProof(ctx context.Context, address common.Address, storage []common.Hash, blockTag string) (*eth.AccountResult, error)
	OutputV0AtBlock(ctx context.Context, blockHash common.Hash) (*eth.OutputV0, error)
}

type driverClient interface {
	SyncStatus(ctx context.Context) (*eth.SyncStatus, error)
	BlockRefWithStatus(ctx context.Context, num uint64) (eth.L2BlockRef, *eth.SyncStatus, error)
	ResetDerivationPipeline(context.Context) error
	StartSequencer(ctx context.Context, blockHash common.Hash) error
	StopSequencer(context.Context) (common.Hash, error)
	SequencerActive(context.Context) (bool, error)
}

type rpcMetrics interface {
	// RecordRPCServerRequest returns a function that records the duration of serving the given RPC method
	RecordRPCServerRequest(method string) func()
}

type adminAPI struct {
	dr  driverClient
	m   rpcMetrics
	log log.Logger
}

func NewAdminAPI(dr driverClient, m rpcMetrics, log log.Logger) *adminAPI {
	return &adminAPI{
		dr:  dr,
		m:   m,
		log: log,
	}
}

func (n *adminAPI) ResetDerivationPipeline(ctx context.Context) error {
	recordDur := n.m.RecordRPCServerRequest("admin_resetDerivationPipeline")
	defer recordDur()
	return n.dr.ResetDerivationPipeline(ctx)
}

func (n *adminAPI) StartSequencer(ctx context.Context, blockHash common.Hash) error {
	recordDur := n.m.RecordRPCServerRequest("admin_startSequencer")
	defer recordDur()
	return n.dr.StartSequencer(ctx, blockHash)
}

func (n *adminAPI) StopSequencer(ctx context.Context) (common.Hash, error) {
	recordDur := n.m.RecordRPCServerRequest("admin_stopSequencer")
	defer recordDur()
	return n.dr.StopSequencer(ctx)
}

func (n *adminAPI) SequencerActive(ctx context.Context) (bool, error) {
	recordDur := n.m.RecordRPCServerRequest("admin_sequencerActive")
	defer recordDur()
	return n.dr.SequencerActive(ctx)
}

func (n *adminAPI) SetLogLevel(ctx context.Context, lvlStr string) error {
	recordDur := n.m.RecordRPCServerRequest("admin_setLogLevel")
	defer recordDur()

	h := n.log.GetHandler()

	lvl, err := log.LvlFromString(lvlStr)
	if err != nil {
		return err
	}

	// We set the log level, and do not wrap the handler with an additional filter handler,
	// as the underlying handler would otherwise also still filter with the previous log level.
	lvlSetter, ok := h.(oplog.LvlSetter)
	if !ok {
		return fmt.Errorf("log handler type %T cannot change log level", h)
	}
	lvlSetter.SetLogLevel(lvl)
	return nil
}

type nodeAPI struct {
	config *rollup.Config
	client l2EthClient
	dr     driverClient
	log    log.Logger
	m      rpcMetrics
}

func NewNodeAPI(config *rollup.Config, l2Client l2EthClient, dr driverClient, log log.Logger, m rpcMetrics) *nodeAPI {
	return &nodeAPI{
		config: config,
		client: l2Client,
		dr:     dr,
		log:    log,
		m:      m,
	}
}

func (n *nodeAPI) OutputAtBlock(ctx context.Context, number hexutil.Uint64) (*eth.OutputResponse, error) {
	recordDur := n.m.RecordRPCServerRequest("optimism_outputAtBlock")
	defer recordDur()

	ref, status, err := n.dr.BlockRefWithStatus(ctx, uint64(number))
	if err != nil {
		return nil, fmt.Errorf("failed to get L2 block ref with sync status: %w", err)
	}

	output, err := n.client.OutputV0AtBlock(ctx, ref.Hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get L2 output at block %s: %w", ref, err)
	}
	return &eth.OutputResponse{
		Version:               output.Version(),
		OutputRoot:            eth.OutputRoot(output),
		BlockRef:              ref,
		WithdrawalStorageRoot: common.Hash(output.MessagePasserStorageRoot),
		StateRoot:             common.Hash(output.StateRoot),
		Status:                status,
	}, nil
}

func (n *nodeAPI) SyncStatus(ctx context.Context) (*eth.SyncStatus, error) {
	recordDur := n.m.RecordRPCServerRequest("optimism_syncStatus")
	defer recordDur()
	return n.dr.SyncStatus(ctx)
}

func (n *nodeAPI) RollupConfig(_ context.Context) (*rollup.Config, error) {
	recordDur := n.m.RecordRPCServerRequest("optimism_rollupConfig")
	defer recordDur()
	return n.config, nil
}

func (n *nodeAPI) Version(ctx context.Context) (string, error) {
	recordDur := n.m.RecordRPCServerRequest("optimism_version")
	defer recordDur()
	return version.Version + "-" + version.Meta, nil
}
