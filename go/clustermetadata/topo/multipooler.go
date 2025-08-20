// Copyright 2025 The Multigres Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package topo

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"sync"

	"github.com/multigres/multigres/go/mterrors"

	"google.golang.org/protobuf/proto"

	clustermetadatapb "github.com/multigres/multigres/go/pb/clustermetadata"
)

// IsTrivialPoolerTypeChange returns if this pooler type can be trivially reassigned
// without changes to the replication graph
func IsTrivialPoolerTypeChange(oldPoolerType, newPoolerType clustermetadatapb.PoolerType) bool {
	switch oldPoolerType {
	case clustermetadatapb.PoolerType_REPLICA:
		switch newPoolerType {
		case clustermetadatapb.PoolerType_REPLICA:
			return true
		}
	}
	return false
}

// IsInServingGraph returns if a pooler appears in the serving graph
func IsInServingGraph(pt clustermetadatapb.PoolerType) bool {
	switch pt {
	case clustermetadatapb.PoolerType_PRIMARY, clustermetadatapb.PoolerType_REPLICA:
		return true
	}
	return false
}

// IsRunningQueryService returns if a pooler is running the query service
func IsRunningQueryService(pt clustermetadatapb.PoolerType) bool {
	switch pt {
	case clustermetadatapb.PoolerType_PRIMARY, clustermetadatapb.PoolerType_REPLICA:
		return true
	}
	return false
}

// IsReplicaPoolerType returns if this type should be connected to a primary pooler
// and actively replicating? PRIMARY is not obviously (only support one level replication graph)
func IsReplicaPoolerType(pt clustermetadatapb.PoolerType) bool {
	switch pt {
	case clustermetadatapb.PoolerType_PRIMARY:
		return false
	}
	return true
}

// NewMultiPooler creates a new MultiPooler record with the given id, cell, and hostname.
func NewMultiPooler(uid uint32, cell, host string) *clustermetadatapb.MultiPooler {
	return &clustermetadatapb.MultiPooler{
		Identifier: &clustermetadatapb.ID{
			Component: clustermetadatapb.MultigresComponent_MULTIPOOLER,
			Cell:      cell,
			Uid:       uid,
		},
		Hostname: host,
		PortMap:  make(map[string]int32),
	}
}

// MultiPoolerInfo is the container for a MultiPooler, read from the topology server.
type MultiPoolerInfo struct {
	version Version // node version - used to prevent stomping concurrent writes
	*clustermetadatapb.MultiPooler
}

// String returns a string describing the multipooler.
func (mpi *MultiPoolerInfo) String() string {
	return fmt.Sprintf("MultiPooler{%v}", MultiPoolerIDString(mpi.Identifier))
}

// IDString returns the string representation of the multipooler identifier
func (mpi *MultiPoolerInfo) IDString() string {
	return MultiPoolerIDString(mpi.Identifier)
}

// Addr returns hostname:grpc port.
func (mpi *MultiPoolerInfo) Addr() string {
	grpcPort, ok := mpi.PortMap["grpc"]
	if !ok {
		return mpi.Hostname
	}
	return fmt.Sprintf("%s:%d", mpi.Hostname, grpcPort)
}

// Version returns the version of this multipooler from last time it was read or updated.
func (mpi *MultiPoolerInfo) Version() Version {
	return mpi.version
}

// IsInServingGraph returns if this multipooler is in the serving graph
func (mpi *MultiPoolerInfo) IsInServingGraph() bool {
	return IsInServingGraph(mpi.Type)
}

// IsReplicaType returns if this multipooler's type is a replica
func (mpi *MultiPoolerInfo) IsReplicaType() bool {
	return IsReplicaPoolerType(mpi.Type)
}

// NewMultiPoolerInfo returns a MultiPoolerInfo based on multipooler with the
// version set. This function should be only used by Server implementations.
func NewMultiPoolerInfo(multipooler *clustermetadatapb.MultiPooler, version Version) *MultiPoolerInfo {
	return &MultiPoolerInfo{version: version, MultiPooler: multipooler}
}

// MultiPoolerIDString returns the string representation of a MultiPooler ID
func MultiPoolerIDString(id *clustermetadatapb.ID) string {
	return fmt.Sprintf("%s-%d", id.Cell, id.Uid)
}

// ParseMultiPoolerID parses a string representation of a MultiPooler ID
func ParseMultiPoolerID(idStr string) (*clustermetadatapb.ID, error) {
	var cell string
	var uid uint32
	n, err := fmt.Sscanf(idStr, "%s-%d", &cell, &uid)
	if err != nil || n != 2 {
		return nil, fmt.Errorf("invalid MultiPooler ID format: %s", idStr)
	}
	return &clustermetadatapb.ID{
		Component: clustermetadatapb.MultigresComponent_MULTIPOOLER,
		Cell:      cell,
		Uid:       uid,
	}, nil
}

// GetMultiPooler is a high level function to read multipooler data.
func (ts *store) GetMultiPooler(ctx context.Context, id *clustermetadatapb.ID) (*MultiPoolerInfo, error) {
	conn, err := ts.ConnForCell(ctx, id.Cell)
	if err != nil {
		return nil, mterrors.Wrap(err, fmt.Sprintf("unable to get connection for cell %q", id.Cell))
	}

	poolerPath := path.Join(PoolersPath, MultiPoolerIDString(id), PoolerFile)
	data, version, err := conn.Get(ctx, poolerPath)
	if err != nil {
		return nil, mterrors.Wrap(err, fmt.Sprintf("unable to connect to multipooler %q", id))
	}
	multipooler := &clustermetadatapb.MultiPooler{}
	if err := proto.Unmarshal(data, multipooler); err != nil {
		return nil, mterrors.Wrap(err, "failed to unmarshal multipooler data")
	}

	return &MultiPoolerInfo{
		version:     version,
		MultiPooler: multipooler,
	}, nil
}

// GetMultiPoolerIDsByCell returns all the multipooler IDs in a cell.
// It returns ErrNoNode if the cell doesn't exist.
// It returns (nil, nil) if the cell exists, but there are no multipoolers in it.
func (ts *store) GetMultiPoolerIDsByCell(ctx context.Context, cell string) ([]*clustermetadatapb.ID, error) {
	// If the cell doesn't exist, this will return ErrNoNode.
	conn, err := ts.ConnForCell(ctx, cell)
	if err != nil {
		return nil, err
	}

	// List the directory, and parse the IDs
	children, err := conn.ListDir(ctx, PoolersPath, false /*full*/)
	if err != nil {
		if errors.Is(err, &TopoError{Code: NoNode}) {
			// directory doesn't exist, empty list, no error.
			return nil, nil
		}
		return nil, err
	}

	result := make([]*clustermetadatapb.ID, len(children))
	for i, child := range children {
		result[i], err = ParseMultiPoolerID(child.Name)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// GetMultiPoolersByCellOptions controls the behavior of GetMultiPoolersByCell.
type GetMultiPoolersByCellOptions struct {
	// DatabaseShard is the optional database/shard that multipoolers must match.
	// An empty shard value will match all shards in the database.
	DatabaseShard *DatabaseShard
}

// DatabaseShard represents a database and shard pair.
type DatabaseShard struct {
	Database string
	Shard    string
}

// GetMultiPoolersByCell returns all the multipoolers in the cell.
// It returns ErrNoNode if the cell doesn't exist.
// It returns ErrPartialResult if some multipoolers couldn't be read. The results in the slice are incomplete.
// It returns (nil, nil) if the cell exists, but there are no multipoolers in it.
func (ts *store) GetMultiPoolersByCell(ctx context.Context, cellName string, opt *GetMultiPoolersByCellOptions) ([]*MultiPoolerInfo, error) {
	// If the cell doesn't exist, this will return ErrNoNode.
	cellConn, err := ts.ConnForCell(ctx, cellName)
	if err != nil {
		return nil, err
	}
	listResults, err := cellConn.List(ctx, PoolersPath)
	if err != nil || len(listResults) == 0 {
		// Fall back to fetching the multipoolers one by one
		if errors.Is(err, &TopoError{Code: NoImplementation}) || errors.Is(err, &TopoError{Code: ResourceExhausted}) {
			return ts.GetMultiPoolersIndividuallyByCell(ctx, cellName, opt)
		}
		if errors.Is(err, &TopoError{Code: NoNode}) {
			return nil, nil
		}
		return nil, err
	}

	var capHint int
	if opt != nil && opt.DatabaseShard == nil {
		capHint = len(listResults)
	}

	multipoolers := make([]*MultiPoolerInfo, 0, capHint)
	for n := range listResults {
		multipooler := &clustermetadatapb.MultiPooler{}
		if err := proto.Unmarshal(listResults[n].Value, multipooler); err != nil {
			return nil, err
		}
		if opt != nil && opt.DatabaseShard != nil && opt.DatabaseShard.Database != "" {
			if opt.DatabaseShard.Database != multipooler.Database {
				continue
			}
			if opt.DatabaseShard.Shard != "" && opt.DatabaseShard.Shard != multipooler.Shard {
				continue
			}
		}
		multipoolers = append(multipoolers, &MultiPoolerInfo{MultiPooler: multipooler, version: listResults[n].Version})
	}
	return multipoolers, nil
}

// GetMultiPoolersIndividuallyByCell returns a sorted list of multipoolers for topo servers that do not
// directly support the topoConn.List() functionality.
// It returns ErrNoNode if the cell doesn't exist.
// It returns ErrPartialResult if some multipoolers couldn't be read. The results in the slice are incomplete.
// It returns (nil, nil) if the cell exists, but there are no multipoolers in it.
func (ts *store) GetMultiPoolersIndividuallyByCell(ctx context.Context, cell string, opt *GetMultiPoolersByCellOptions) ([]*MultiPoolerInfo, error) {
	// If the cell doesn't exist, this will return ErrNoNode.
	ids, err := ts.GetMultiPoolerIDsByCell(ctx, cell)
	if err != nil {
		return nil, err
	}
	sort.Slice(ids, func(i, j int) bool {
		return MultiPoolerIDString(ids[i]) < MultiPoolerIDString(ids[j])
	})

	var partialResultErr error
	multipoolerMap, err := ts.GetMultiPoolerMap(ctx, ids, opt)
	if err != nil {
		if errors.Is(err, &TopoError{Code: PartialResult}) {
			partialResultErr = err
		} else {
			return nil, err
		}
	}
	multipoolers := make([]*MultiPoolerInfo, 0, len(ids))
	for _, id := range ids {
		multipoolerInfo, ok := multipoolerMap[MultiPoolerIDString(id)]
		if !ok {
			// multipooler disappeared on us (GetMultiPoolerMap ignores
			// topo.ErrNoNode), just echo a warning
			fmt.Printf("failed to load multipooler %v\n", id)
		} else {
			multipoolers = append(multipoolers, multipoolerInfo)
		}
	}

	return multipoolers, partialResultErr
}

// UpdateMultiPooler updates the multipooler data only - not associated replication paths.
func (ts *store) UpdateMultiPooler(ctx context.Context, mpi *MultiPoolerInfo) error {
	conn, err := ts.ConnForCell(ctx, mpi.Identifier.Cell)
	if err != nil {
		return err
	}

	data, err := proto.Marshal(mpi.MultiPooler)
	if err != nil {
		return err
	}
	poolerPath := path.Join(PoolersPath, MultiPoolerIDString(mpi.Identifier), PoolerFile)
	newVersion, err := conn.Update(ctx, poolerPath, data, mpi.version)
	if err != nil {
		return err
	}
	mpi.version = newVersion

	return nil
}

// UpdateMultiPoolerFields is a high level helper to read a multipooler record, call an
// update function on it, and then write it back. If the write fails due to
// a version mismatch, it will re-read the record and retry the update.
// If the update succeeds, it returns the updated multipooler.
// If the update method returns ErrNoUpdateNeeded, nothing is written,
// and nil,nil is returned.
func (ts *store) UpdateMultiPoolerFields(ctx context.Context, id *clustermetadatapb.ID, update func(*clustermetadatapb.MultiPooler) error) (*clustermetadatapb.MultiPooler, error) {
	for {
		mpi, err := ts.GetMultiPooler(ctx, id)
		if err != nil {
			return nil, err
		}
		if err = update(mpi.MultiPooler); err != nil {
			if errors.Is(err, &TopoError{Code: NoUpdateNeeded}) {
				return nil, nil
			}
			return nil, err
		}
		if err = ts.UpdateMultiPooler(ctx, mpi); !errors.Is(err, &TopoError{Code: BadVersion}) {
			return mpi.MultiPooler, err
		}
	}
}

// CreateMultiPooler creates a new multipooler and all associated paths.
func (ts *store) CreateMultiPooler(ctx context.Context, multipooler *clustermetadatapb.MultiPooler) error {
	conn, err := ts.ConnForCell(ctx, multipooler.Identifier.Cell)
	if err != nil {
		return err
	}

	data, err := proto.Marshal(multipooler)
	if err != nil {
		return err
	}
	poolerPath := path.Join(PoolersPath, MultiPoolerIDString(multipooler.Identifier), PoolerFile)
	if _, err := conn.Create(ctx, poolerPath, data); err != nil {
		return err
	}

	return nil
}

// DeleteMultiPooler deletes the specified multipooler.
func (ts *store) DeleteMultiPooler(ctx context.Context, id *clustermetadatapb.ID) error {
	conn, err := ts.ConnForCell(ctx, id.Cell)
	if err != nil {
		return err
	}

	poolerPath := path.Join(PoolersPath, MultiPoolerIDString(id), PoolerFile)
	if err := conn.Delete(ctx, poolerPath, nil); err != nil {
		return err
	}

	return nil
}

// GetMultiPoolerMap tries to read all the multipoolers in the provided list,
// and returns them in a map.
// If error is ErrPartialResult, the results in the map are
// incomplete, meaning some multipoolers couldn't be read.
// The map is indexed by MultiPoolerIDString(multipooler id).
func (ts *store) GetMultiPoolerMap(ctx context.Context, ids []*clustermetadatapb.ID, opt *GetMultiPoolersByCellOptions) (map[string]*MultiPoolerInfo, error) {
	var (
		mu             sync.Mutex
		wg             sync.WaitGroup
		multipoolerMap = make(map[string]*MultiPoolerInfo)
		returnErr      error
	)

	for _, id := range ids {
		if id == nil {
			return nil, fmt.Errorf("nil multipooler id in list")
		}
		wg.Add(1)
		go func(id *clustermetadatapb.ID) {
			defer wg.Done()
			multipoolerInfo, err := ts.GetMultiPooler(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fmt.Printf("%v: %v\n", id, err)
				// There can be data races removing nodes - ignore them for now.
				// We only need to set this on first error.
				if returnErr == nil && !errors.Is(err, &TopoError{Code: NoNode}) {
					returnErr = NewError(PartialResult, id.Cell)
				}
			} else {
				if opt != nil && opt.DatabaseShard != nil {
					if opt.DatabaseShard.Database != "" && opt.DatabaseShard.Database != multipoolerInfo.Database {
						return
					}
					if opt.DatabaseShard.Shard != "" && opt.DatabaseShard.Shard != multipoolerInfo.Shard {
						return
					}
				}
				multipoolerMap[MultiPoolerIDString(id)] = multipoolerInfo
			}
		}(id)
	}
	wg.Wait()
	return multipoolerMap, returnErr
}

// GetMultiPoolerList tries to read all the multipoolers in the provided list,
// and returns them in a list.
// If error is ErrPartialResult, the results in the list are
// incomplete, meaning some multipoolers couldn't be read.
func (ts *store) GetMultiPoolerList(ctx context.Context, ids []*clustermetadatapb.ID, opt *GetMultiPoolersByCellOptions) ([]*MultiPoolerInfo, error) {
	var (
		mu              sync.Mutex
		wg              sync.WaitGroup
		multipoolerList = make([]*MultiPoolerInfo, 0)
		returnErr       error
	)

	for _, id := range ids {
		if id == nil {
			return nil, fmt.Errorf("nil multipooler id in list")
		}
		wg.Add(1)
		go func(id *clustermetadatapb.ID) {
			defer wg.Done()
			multipoolerInfo, err := ts.GetMultiPooler(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fmt.Printf("%v: %v\n", id, err)
				// There can be data races removing nodes - ignore them for now.
				// We only need to set this on first error.
				if returnErr == nil && !errors.Is(err, &TopoError{Code: NoNode}) {
					returnErr = NewError(PartialResult, id.Cell)
				}
			} else {
				if opt != nil && opt.DatabaseShard != nil {
					if opt.DatabaseShard.Database != "" && opt.DatabaseShard.Database != multipoolerInfo.Database {
						return
					}
					if opt.DatabaseShard.Shard != "" && opt.DatabaseShard.Shard != multipoolerInfo.Shard {
						return
					}
				}
				multipoolerList = append(multipoolerList, multipoolerInfo)
			}
		}(id)
	}
	wg.Wait()
	return multipoolerList, returnErr
}

// InitMultiPooler creates or updates a multipooler. If allowUpdate is true,
// and a multipooler with the same ID exists, just update it.
// If a multipooler is created as primary, and there is already a different
// primary in the shard, allowPrimaryOverride must be set.
func (ts *store) InitMultiPooler(ctx context.Context, multipooler *clustermetadatapb.MultiPooler, allowPrimaryOverride, allowUpdate bool) error {
	// TODO: Add shard validation and primary override checking
	// This would require implementing shard management first

	err := ts.CreateMultiPooler(ctx, multipooler)
	if errors.Is(err, &TopoError{Code: NodeExists}) && allowUpdate {
		// Try to update then
		oldMultiPooler, err := ts.GetMultiPooler(ctx, multipooler.Identifier)
		if err != nil {
			return fmt.Errorf("failed reading existing multipooler %v: %v", MultiPoolerIDString(multipooler.Identifier), err)
		}

		// Check we have the same database / shard, and if not,
		// require the allowDifferentShard flag.
		if oldMultiPooler.Database != multipooler.Database || oldMultiPooler.Shard != multipooler.Shard {
			return fmt.Errorf("old multipooler has shard %v/%v. Cannot override with shard %v/%v. Delete and re-add multipooler if you want to change the multipooler's database/shard", oldMultiPooler.Database, oldMultiPooler.Shard, multipooler.Database, multipooler.Shard)
		}
		oldMultiPooler.MultiPooler = proto.Clone(multipooler).(*clustermetadatapb.MultiPooler)
		if err := ts.UpdateMultiPooler(ctx, oldMultiPooler); err != nil {
			return fmt.Errorf("failed updating multipooler %v: %v", MultiPoolerIDString(multipooler.Identifier), err)
		}
		return nil
	}
	return err
}

// ValidateMultiPooler makes sure a multipooler is represented correctly in the topology server.
func (ts *store) ValidateMultiPooler(ctx context.Context, id *clustermetadatapb.ID) error {
	// read the multipooler record, make sure it parses
	multipooler, err := ts.GetMultiPooler(ctx, id)
	if err != nil {
		return err
	}
	if multipooler.Identifier.Cell != id.Cell || multipooler.Identifier.Uid != id.Uid {
		return fmt.Errorf("bad multipooler id data for multipooler %v: %#v", MultiPoolerIDString(id), multipooler.Identifier)
	}

	// TODO: Add validation for shard replication nodes
	// This would require implementing shard replication management first

	return nil
}
