package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"sort"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/google/codesearch/cmd/cindex-serve/cache"
	"github.com/google/codesearch/cmd/cindex-serve/service"
	oldindex "github.com/google/codesearch/index"
	"github.com/google/codesearch/index2"
	"github.com/google/codesearch/repo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

var (
	cacheSize = flag.Uint64("cache_size", 64<<20, "Amount of data to hold in ram at any given time.")
	port      = flag.Int("port", 0, "Port to listen on")
)

var (
	ErrNotFound = errors.New("Not found")
)

func getSize(ctx context.Context, key string) (int, error) {
	name := filepath.Join(index.ShardDir(), key)
	s, err := os.Stat(name)
	if err != nil {
		return 0, err
	}
	return int(s.Size()), nil
}

func load(ctx context.Context, key string, v []byte) error {
	name := filepath.Join(index.ShardDir(), key)
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	log.Printf("Loading: %s", key)
	_, err = io.ReadFull(f, v)
	return err
}

func buildSnippets(doc *index.Doc, matches [][2]int) []*service.Snippet {
	snippets := make([]*service.Snippet, len(matches))
	relMatches := make([]uint32, len(matches)*2)
	b := doc.DataBytes()
	for i, pos := range matches {
		block := pos[0] >> 12
		lNum := uint32(bytes.Count(b[block<<12:pos[0]], []byte{'\n'}))
		if block == 0 {
			lNum++
		} else {
			lNum += doc.BlockToLine(block - 1)
		}

		lineEnd := bytes.IndexByte(b[pos[1]:], '\n') + pos[1] + 1
		if lineEnd <= pos[1] {
			lineEnd = len(b)
		}

		lineStart := bytes.LastIndexByte(b[:pos[0]], '\n') + 1
		line := b[lineStart:lineEnd]

		relMatches[2*i] = uint32(pos[0] - lineStart)
		relMatches[2*i+1] = uint32(pos[1] - lineStart)
		snippets[i] = &service.Snippet{
			Lines:     string(line),
			FirstLine: lNum,
			Matches:   relMatches[2*i : 2*i+2],
		}
	}

	return snippets
}

type server struct {
	cache *cache.Cache
}

var _ service.SearchShardServiceServer = (*server)(nil)

func (s *server) SearchShard(ctx context.Context, req *service.SearchShardRequest) (*service.SearchShardResponse, error) {
	syn, err := syntax.Parse(req.GetExpression(), syntax.Perl)
	if err != nil {
		return nil, err
	}
	q := oldindex.RegexpQuery(syn)

	re, err := regexp.Compile(req.GetExpression())
	if err != nil {
		return nil, err
	}

	e := s.cache.Acquire(req.GetShardId())
	defer s.cache.Release(e)

	v, err := e.Get(ctx)
	if err != nil {
		if err == cache.ErrMemoryPressure {
			err = status.Errorf(codes.ResourceExhausted, "unable to load shard - cache full")
		}
		return nil, err
	}

	idx := index.Open(v)
	post := idx.PostingQuery(q)

	var resp service.SearchShardResponse
	resp.Matches = uint32(post.GetCardinality())
	var matchRange [][2]int
	post.Iterate(func(fileid uint32) bool {
		resp.DocsInspected++

		var doc index.Doc
		idx.Docs(&doc, int(fileid))
		b := doc.DataBytes()

		matchRange = matchRange[:0]
		off := 0
		for {
			pos := re.FindIndex(b[off:])
			if pos == nil {
				break
			}
			pos[0] += off
			pos[1] += off
			matchRange = append(matchRange, [2]int{pos[0], pos[1]})
			off = pos[1]
		}
		if len(matchRange) == 0 {
			resp.Matches--
			return true
		}

		resp.Docs = append(resp.Docs, &service.Doc{
			Id:       fileid,
			Path:     string(doc.Path()),
			Snippets: buildSnippets(&doc, matchRange),
		})
		return len(resp.Docs) < 100
	})

	return &resp, nil
}

type indexMetadataServer struct{}

var _ service.IndexMetadataServiceServer = indexMetadataServer{}

func convertShard(id repo.ShardID, s *repo.Shard) *service.Shard {
	var state service.Shard_State
	switch s.State {
	case repo.CreatingState:
		state = service.Shard_CREATING
	case repo.UnreferencedState:
		state = service.Shard_UNREFERENCED
	case repo.ReferencedState:
		state = service.Shard_REFERENCED
	case repo.DeletingState:
		state = service.Shard_DELETING
	}
	return &service.Shard{
		Id:       id.String(),
		TreeHash: s.TreeHash,
		State:    state,
		Size:     s.Size,
		Sha256:   s.SHA256,
	}
}

func manifestTxn(ctx context.Context, f func(*repo.Manifest) error) error {
	for {
		manifest, err := repo.LoadManifest(index.RepoDir())
		if err != nil {
			return err
		}
		if err := f(manifest); err != nil {
			return err
		}

		err = repo.WriteManifest(manifest, index.RepoDir())
		if err == os.ErrExist {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			continue
		}
		return err
	}
}

func (indexMetadataServer) AllocateShard(ctx context.Context, req *service.AllocateShardRequest) (*service.AllocateShardResponse, error) {
	var resp *service.AllocateShardResponse
	err := manifestTxn(ctx, func(manifest *repo.Manifest) error {
		id := manifest.CreateShard()
		resp = &service.AllocateShardResponse{
			Shard: convertShard(id, manifest.Shards[id]),
		}
		return nil
	})
	return resp, err
}

func (indexMetadataServer) CompleteShard(ctx context.Context, req *service.CompleteShardRequest) (*service.CompleteShardResponse, error) {
	var sID repo.ShardID
	if err := sID.UnmarshalText([]byte(req.GetShardId())); err != nil {
		return nil, err
	}

	return &service.CompleteShardResponse{}, manifestTxn(ctx, func(manifest *repo.Manifest) error {
		s := manifest.Shards[sID]
		if s == nil {
			return status.Errorf(codes.NotFound, "shard %q does not exist", req.GetShardId())
		}
		if !s.Created(plumbing.NewHash(req.GetTreeHash()), req.GetSize(), req.GetSha256()) {
			return status.Errorf(codes.FailedPrecondition, "shard %q in state %s unable to be marked created", req.GetShardId(), s.State)
		}
		return nil
	})
}

func (indexMetadataServer) GetShard(ctx context.Context, req *service.GetShardRequest) (*service.GetShardResponse, error) {
	var sID repo.ShardID
	if err := sID.UnmarshalText([]byte(req.GetShardId())); err != nil {
		return nil, err
	}

	manifest, err := repo.LoadManifest(index.RepoDir())
	if err != nil {
		return nil, err
	}

	s := manifest.Shards[sID]
	if s == nil {
		return nil, status.Errorf(codes.NotFound, "shard %q does not exist", req.GetShardId())
	}

	return &service.GetShardResponse{
		Shard: convertShard(sID, s),
	}, nil
}

func (indexMetadataServer) SearchShards(ctx context.Context, req *service.SearchShardsRequest) (*service.SearchShardsResponse, error) {
	manifest, err := repo.LoadManifest(index.RepoDir())
	if err != nil {
		return nil, err
	}

	resp := &service.SearchShardsResponse{}

	filter := func(id repo.ShardID, s *repo.Shard) {
		resp.ShardIds = append(resp.ShardIds, id.String())
	}

	if treeHash := req.GetTreeHash(); treeHash != "" {
		inner := filter
		filter = func(id repo.ShardID, s *repo.Shard) {
			if s.TreeHash == treeHash {
				inner(id, s)
			}
		}
	}

	if states := req.GetStates(); len(states) > 0 {
		repoStates := make([]repo.State, len(states))
		for i, s := range states {
			switch s {
			case service.Shard_CREATING:
				repoStates[i] = repo.CreatingState
			case service.Shard_UNREFERENCED:
				repoStates[i] = repo.UnreferencedState
			case service.Shard_REFERENCED:
				repoStates[i] = repo.ReferencedState
			case service.Shard_DELETING:
				repoStates[i] = repo.DeletingState
			}
		}
		inner := filter
		filter = func(id repo.ShardID, s *repo.Shard) {
			for _, state := range repoStates {
				if s.State == state {
					inner(id, s)
					break
				}
			}
		}
	}

	for id, s := range manifest.Shards {
		filter(id, s)
	}
	return resp, nil
}

func convertRepoRef(repoName, ref string, rev *repo.RepoRef, shard *repo.Shard) *service.RepoRef {
	return &service.RepoRef{
		RepoName:   repoName,
		Ref:        ref,
		CommitHash: rev.CommitHash,
		Shard:      convertShard(rev.ShardID, shard),
	}
}

func (indexMetadataServer) GetRepo(ctx context.Context, req *service.GetRepoRequest) (*service.GetRepoResponse, error) {
	manifest, err := repo.LoadManifest(index.RepoDir())
	if err != nil {
		return nil, err
	}

	repo := manifest.Repos[req.GetRepoName()]
	if repo == nil {
		return nil, status.Errorf(codes.NotFound, "repo %q does not exist", req.GetRepoName())
	}

	refs := make([]string, 0, len(repo.Refs))
	for k := range repo.Refs {
		refs = append(refs, k)
	}
	sort.Strings(refs)

	rev := repo.Refs[repo.DefaultRef]
	return &service.GetRepoResponse{
		DefaultRef: convertRepoRef(req.GetRepoName(), repo.DefaultRef, rev, manifest.Shards[rev.ShardID]),
		Refs:       refs,
	}, nil
}

func (indexMetadataServer) GetRepoRef(ctx context.Context, req *service.GetRepoRefRequest) (*service.GetRepoRefResponse, error) {
	manifest, err := repo.LoadManifest(index.RepoDir())
	if err != nil {
		return nil, err
	}

	repo := manifest.Repos[req.GetRepoName()]
	if repo == nil {
		return nil, status.Errorf(codes.NotFound, "repo %q does not exist", req.GetRepoName())
	}

	ref := repo.Refs[req.GetRef()]
	if ref == nil {
		return nil, status.Errorf(codes.NotFound, "ref %q does not exist", req.GetRef())
	}

	return &service.GetRepoRefResponse{
		Revision: convertRepoRef(req.GetRepoName(), req.GetRef(), ref, manifest.Shards[ref.ShardID]),
	}, nil
}

func (indexMetadataServer) UpdateRepoShard(ctx context.Context, req *service.UpdateRepoShardRequest) (*service.UpdateRepoShardResponse, error) {
	var sID repo.ShardID
	if err := sID.UnmarshalText([]byte(req.GetShardId())); err != nil {
		return nil, err
	}

	return &service.UpdateRepoShardResponse{}, manifestTxn(ctx, func(m *repo.Manifest) error {
		if s := m.Shards[sID]; s == nil {
			return status.Errorf(codes.NotFound, "shard %q does not exist", req.GetShardId())
		}

		r := m.GetOrCreateRepo(req.GetRepoName())
		if r.DefaultRef == "" {
			r.DefaultRef = req.GetRef()
		}
		repoRev := r.GetOrCreateRef(plumbing.ReferenceName(req.GetRef()))
		repoRev.CommitHash = req.GetCommitHash()
		repoRev.ShardID = sID

		if !m.ReconcileShardStates() {
			return status.Errorf(codes.FailedPrecondition, "unable to reconcile %#v", m)
		}

		return nil
	})
}

func main() {
	flag.Parse()
	ctx := context.Background()

	s := &server{
		cache: cache.New(ctx, *cacheSize, getSize, load),
	}

	g := grpc.NewServer()
	reflection.Register(g)
	service.RegisterIndexMetadataServiceServer(g, indexMetadataServer{})
	service.RegisterSearchShardServiceServer(g, s)
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Print("Serving...")
	g.Serve(lis)
}
