package repo

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/google/orderedcode"
)

// Action     : prior        -> next
//
// Create     : nil          -> CREATING
//
// Created    :	CREATING     -> UNREFERENCED
//
// Reference  : UNREFERENCED -> REFERENCED
//
// Unreference: REFERENCED   -> UNREFERENCED
//
// Prune      : CREATING     -> DELETING
// Prune      : UNREFERENCED -> DELETING
//
// Deleted    : DELETING     -> nil

type State string

const (
	CreatingState     State = "CREATING"
	UnreferencedState State = "UNREFERENCED"
	ReferencedState   State = "REFERENCED"
	DeletingState     State = "DELETING"
)

type ShardID [8]byte

var ZeroShardID = ShardID{}

func (s ShardID) MarshalText() (text []byte, err error) {
	text = make([]byte, 2*len(s))
	hex.Encode(text, s[:])
	return
}

func (s ShardID) String() string { return hex.EncodeToString(s[:]) }

func (s *ShardID) UnmarshalText(text []byte) error {
	if len(text) != 2*len(s) {
		return errors.New("ShardID of wrong length")
	}
	_, err := hex.Decode(s[:], text)
	return err
}

type Manifest struct {
	// Repos holds information about currently indexed repositories, keyed by the
	// repository name.
	Repos map[string]*Repo

	// Shards holds information about shard files that are available. Keyed by
	// the shard ID.
	Shards map[ShardID]*Shard

	// Revision is managed automatically - do not edit.
	Revision uint64
}

func rid() (ret [8]byte) {
	if _, err := rand.Read(ret[:]); err != nil {
		panic(err)
	}
	return
}

func (m *Manifest) GetOrCreateRepo(name string) (r *Repo) {
	if m.Repos == nil {
		m.Repos = make(map[string]*Repo)
	}
	r = m.Repos[name]
	if r == nil {
		r = &Repo{}
		m.Repos[name] = r
	}
	return
}

func (m *Manifest) ReferencableShard(hash plumbing.Hash) (ShardID, bool) {
	hashString := hash.String()
	for id, s := range m.Shards {
		if s.TreeHash != hashString {
			continue
		}
		if _, ok := allowed[stateAction{s.State, referenced}]; ok {
			return id, true
		}
	}
	return ShardID{}, false
}

func (m *Manifest) CreateShard(hash plumbing.Hash) ShardID {
	var id ShardID
	for {
		id = ShardID(rid())
		if _, ok := m.Shards[id]; !ok && id != ZeroShardID {
			break
		}
	}
	s := &Shard{
		AllocatedAt: time.Now(),
		TreeHash:    hash.String(),
	}
	if !s.transition(allocate) {
		panic("Unable to allocate")
	}
	if m.Shards == nil {
		m.Shards = make(map[ShardID]*Shard)
	}
	m.Shards[id] = s
	return id
}

func (m *Manifest) ReconcileShardStates() bool {
	inUse := make(map[ShardID]bool, len(m.Shards))
	for _, r := range m.Repos {
		for _, r := range r.Revisions {
			inUse[r.ShardID] = true
		}
	}

	success := true
	for id, s := range m.Shards {
		if inUse[id] {
			success = success && s.Referenced()
		} else {
			success = success && s.Unreferenced()
		}
	}
	return success
}

type Repo struct {
	// DefaultRevision if set, is the revision to use for searching by default.
	DefaultRevision string

	// Revision is the revision that is used for resolving the hash to index.
	// This could be a tag or a branch name.
	Revisions map[string]*RepoRevision
}

func (r *Repo) GetOrCreateRevision(name plumbing.Revision) (rev *RepoRevision) {
	if r.Revisions == nil {
		r.Revisions = make(map[string]*RepoRevision)
	}
	rev = r.Revisions[name.String()]
	if rev == nil {
		rev = &RepoRevision{}
		r.Revisions[name.String()] = rev
	}
	return
}

func (r *Repo) GetRevision(name plumbing.Revision) *RepoRevision {
	if r == nil {
		return nil
	}
	return r.Revisions[name.String()]
}

type RepoRevision struct {
	// CommitHash is the commit hash which was used for producing the index
	// shards.
	CommitHash string

	// ShardID is the shard that this Repo is indexed by.
	ShardID ShardID
}

func (r *RepoRevision) GetCommitHash() (hash plumbing.Hash) {
	if r != nil {
		hash = plumbing.NewHash(r.CommitHash)
	}
	return
}

func (r *RepoRevision) GetShardID() (id ShardID) {
	if r != nil {
		id = r.ShardID
	}
	return
}

type Shard struct {
	// AllocatedAt is the timea t which the shard was allocated.
	AllocatedAt time.Time

	// TreeHash is the git hash of the tree object this shard was created from.
	TreeHash string

	// CreatedAt is the time of the created transition.
	CreatedAt *time.Time `json:",omitempty"`

	// Size is the size of the shard - only set once created.
	Size uint64 `json:",omitempty"`
	// SHA256 of the shard - only set once created.
	SHA256 string `json:",omitempty"`

	// State holds the current state of the shard.
	State State

	// StateWhen records the time at which the shard entered this state.
	StateWhen time.Time

	// TombstoneAt is the time at which the shard started deletion.
	TombstoneAt *time.Time `json:",omitempty"`
}

type action struct {
	string
}

var (
	allocate     = action{"ALLOCATE"}
	deleted      = action{"DELETED"}
	created      = action{"CREATED"}
	referenced   = action{"REFERENCED"}
	unreferenced = action{"UNREFERENCED"}
	tombstone    = action{"TOMBSTONE"}
)

type stateAction struct {
	state  State
	action action
}

var allowed = map[stateAction]State{
	stateAction{"", allocate}: CreatingState,

	stateAction{CreatingState, created}:   UnreferencedState,
	stateAction{CreatingState, tombstone}: DeletingState,

	stateAction{UnreferencedState, unreferenced}: UnreferencedState,
	stateAction{UnreferencedState, referenced}:   ReferencedState,
	stateAction{UnreferencedState, tombstone}:    DeletingState,

	stateAction{ReferencedState, referenced}:   ReferencedState,
	stateAction{ReferencedState, unreferenced}: UnreferencedState,

	stateAction{DeletingState, deleted}: "",
}

func (s *Shard) transition(action action) bool {
	if s == nil {
		return false
	}

	r, ok := allowed[stateAction{s.State, action}]
	if ok {
		s.State = r
		s.StateWhen = time.Now()
	}
	return ok
}

func (s *Shard) Created(size uint64, sha256 string) bool {
	ok := s.transition(created)
	if ok && s.CreatedAt == nil {
		now := time.Now()
		s.CreatedAt = &now
		s.Size = size
		s.SHA256 = sha256
	}
	return ok
}
func (s *Shard) Referenced() bool   { return s.transition(referenced) }
func (s *Shard) Unreferenced() bool { return s.transition(unreferenced) }
func (s *Shard) Tombstone() bool {
	ok := s.transition(tombstone)
	if ok && s.TombstoneAt == nil {
		now := time.Now()
		s.TombstoneAt = &now
	}
	return ok
}

func CleanupManifests(dir string) error {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	max := ""
	for _, e := range entries {
		if name := e.Name(); strings.IndexByte(name, '.') == -1 && name > max {
			max = name
		}
	}

	if max == "" {
		return nil
	}

	var firstErr error
	for _, e := range entries {
		if time.Since(e.ModTime()) < 24*time.Hour {
			continue
		}

		name := e.Name()
		idx := strings.IndexByte(name, '.')
		if idx == -1 {
			idx = len(name)
		}
		if rev := name[:idx]; rev < max || rev == max && rev != name {
			if err := os.RemoveAll(filepath.Join(dir, name)); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

func LoadManifest(dir string) (*Manifest, error) {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	max := ""
	for _, e := range entries {
		if name := e.Name(); strings.IndexByte(name, '.') == -1 && name > max {
			max = name
		}
	}
	if max == "" {
		return &Manifest{}, nil
	}

	f, err := os.Open(filepath.Join(dir, max, "manifest.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var m Manifest
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func WriteManifest(m *Manifest, dir string) error {
	m.Revision++
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}

	revBytes, err := orderedcode.Append(nil, m.Revision)
	if err != nil {
		return err
	}
	revID := hex.EncodeToString(revBytes)
	endDir := filepath.Join(dir, revID)

	if _, err := os.Stat(endDir); !os.IsNotExist(err) {
		if err == nil {
			err = os.ErrExist
		}
		return err
	}

	aIDBytes := rid()
	aIDDir := filepath.Join(dir, revID+"."+hex.EncodeToString(aIDBytes[:]))
	if err := os.Mkdir(aIDDir, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(aIDDir)

	if err := ioutil.WriteFile(filepath.Join(aIDDir, "manifest.json"), b, 0o600); err != nil {
		return err
	}

	return os.Rename(aIDDir, endDir)
}