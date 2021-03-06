package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"
	"sync"

	"github.com/RoaringBitmap/roaring"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/google/codesearch/cmd/cindex-serve/cache"
	"github.com/google/codesearch/cmd/cindex-serve/service"
	ss "github.com/google/codesearch/cmd/storage/service"
	"github.com/google/codesearch/expr"
	oldindex "github.com/google/codesearch/index"
	"github.com/google/codesearch/index2"
	"github.com/google/codesearch/repo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const suffixSeparator = "||"

var (
	storageService = flag.String("storage_service", "", "Address of storage service.")
	cacheSize      = flag.Int64("cache_size", 64<<20, "Amount of data to hold in ram at any given time.")
	port           = flag.Int("port", 0, "Port to listen on")
)

type cacheLoader struct {
	ss ss.StorageServiceClient
}

func (c cacheLoader) Size(ctx context.Context, name string) (int32, error) {
	req := &ss.GetShardMetadataRequest{
		ShardId: name,
	}
	if idx := strings.Index(name, suffixSeparator); idx >= 0 {
		req.ShardId = name[:idx]
		req.Suffix = name[idx+len(suffixSeparator):]
	}
	resp, err := c.ss.GetShardMetadata(ctx, req)
	if err != nil {
		return 0, err
	}
	ret := int32(resp.GetLength())
	if int64(ret) != resp.GetLength() {
		return 0, fmt.Errorf("%q overflows with %d", name, resp.GetLength())
	}
	return ret, nil
}

func (c cacheLoader) Load(ctx context.Context, name string, offset int64, body []byte) error {
	req := &ss.ReadShardRequest{
		ShardId: name,
		Offset:  offset,
		Length:  int32(len(body)),
	}
	if idx := strings.Index(name, suffixSeparator); idx >= 0 {
		req.ShardId = name[:idx]
		req.Suffix = name[idx+len(suffixSeparator):]
	}

	log.Printf("Loading %v", req)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := c.ss.ReadShard(ctx, req)
	if err != nil {
		return err
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF && len(body) == 0 {
				err = nil
			}
			return err
		}
		n := copy(body, resp.GetBlock())
		body = body[n:]
	}
}

func buildSnippets(doc *index.DocInner, matches [][2]int) []*service.Snippet {
	snippets := make([]*service.Snippet, len(matches))
	relMatches := make([]uint32, len(matches)*2)
	b := doc.DataBytes()
	for i, pos := range matches {
		block := pos[0] >> 12
		lNum := int32(bytes.Count(b[block<<12:pos[0]], []byte{'\n'}))
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

type Filterer interface {
	Filter(universe *roaring.Bitmap) (*roaring.Bitmap, error)
}

type queryFilterer struct {
	shard *index.IndexShard
	query *oldindex.Query
}

func (m *queryFilterer) Filter(universe *roaring.Bitmap) (*roaring.Bitmap, error) {
	return m.shard.PostingQuery(m.query, universe), nil
}

type fileFilterer struct {
	shard  *index.IndexShard
	prefix string
	expr   *regexp.Regexp
	match  bool
}

func (f *fileFilterer) Filter(universe *roaring.Bitmap) (*roaring.Bitmap, error) {
	if universe == nil {
		return f.scanAll()
	}

	ret := roaring.New()

	pfx := make([]byte, len(f.prefix), len(f.prefix)+int(f.shard.MaxPathLength()))
	copy(pfx, f.prefix)

	block := make([]uint32, 256)
	var doc index.Doc
	iter := universe.ManyIterator()
	for n := iter.NextMany(block); n > 0; n = iter.NextMany(block) {
		i := 0

		for j := 0; j < n; j++ {
			f.shard.Docs(&doc, int(block[j]))
			v := append(pfx, doc.Path()...)
			if f.expr.Match(v) == f.match {
				block[i] = block[j]
				i++
			}
		}

		ret.AddMany(block[:i])
	}

	return ret, nil
}

func (f *fileFilterer) scanAll() (*roaring.Bitmap, error) {
	ret := roaring.New()

	pfx := make([]byte, len(f.prefix), len(f.prefix)+int(f.shard.MaxPathLength()))
	copy(pfx, f.prefix)

	block := make([]uint32, 0, 256)
	var doc index.Doc
	for i, l := 0, f.shard.DocsLength(); i < l; i++ {
		f.shard.Docs(&doc, i)
		v := append(pfx, doc.Path()...)
		if f.expr.Match(v) == f.match {
			block = append(block, uint32(i))
			if len(block) == cap(block) {
				ret.AddMany(block)
				block = block[:0]
			}
		}
	}

	ret.AddMany(block)

	return ret, nil
}

type requestLoader struct {
	cache   *cache.Cache
	mu      sync.Mutex
	entries []cache.Entry
	data    map[index.SHA256][]byte
}

func (r *requestLoader) Get(ctx context.Context, key index.SHA256, name string, params cache.LoadParams) ([]byte, error) {
	e, b := func() (cache.Entry, []byte) {
		r.mu.Lock()
		defer r.mu.Unlock()
		b, ok := r.data[key]
		if ok {
			return nil, b
		}
		e := r.cache.Acquire(key, name, params)
		r.entries = append(r.entries, e)
		return e, nil
	}()
	var err error
	if e != nil {
		b, err = e.Get(ctx)
		if err == nil {
			func() {
				r.mu.Lock()
				defer r.mu.Unlock()
				r.data[key] = b
			}()
		}
	}
	return b, err
}

func (r *requestLoader) ReleaseAll() {
	for _, e := range r.entries {
		// TODO: Add batch release.
		r.cache.Release(e)
	}
}

type contentFilterer struct {
	ctx    context.Context
	name   string
	loader *requestLoader
	shard  *index.IndexShard

	// All expressions must pass.
	expr  []*regexp.Regexp
	match bool
}

func (c *contentFilterer) Filter(universe *roaring.Bitmap) (*roaring.Bitmap, error) {
	if universe == nil {
		return c.scanAll()
	}

	ret := roaring.New()

	block := make([]uint32, 256)
	var doc index.Doc
	iter := universe.ManyIterator()
	for n := iter.NextMany(block); n > 0; n = iter.NextMany(block) {
		i := 0

		for j := 0; j < n; j++ {
			c.shard.Docs(&doc, int(block[j]))
			keep, err := c.scan(&doc)
			if err != nil {
				return nil, err
			}
			if keep {
				block[i] = block[j]
				i++
			}
		}

		ret.AddMany(block[:i])
	}

	return ret, nil
}

func (c *contentFilterer) scanAll() (*roaring.Bitmap, error) {
	ret := roaring.New()

	block := make([]uint32, 0, 256)
	var doc index.Doc
	for i, l := 0, c.shard.DocsLength(); i < l; i++ {
		c.shard.Docs(&doc, i)
		keep, err := c.scan(&doc)
		if err != nil {
			return nil, err
		}
		if keep {
			block = append(block, uint32(i))
			if len(block) == cap(block) {
				ret.AddMany(block)
				block = block[:0]
			}
		}
	}

	ret.AddMany(block)

	return ret, nil
}

func (c *contentFilterer) scan(doc *index.Doc) (bool, error) {
	var rng index.ByteRange
	doc.Range(&rng)

	var docInnerBytes []byte
	if rb := c.shard.RawBytes(); rb != nil {
		start := rng.Start()
		docInnerBytes = rb[start : start+int64(rng.Length())]
	} else {
		var err error
		docInnerBytes, err = c.loader.Get(c.ctx, doc.Hash(), c.name, cache.LoadParams{
			Start:       rng.Start(),
			Length:      rng.Length(),
			IgnoreLimit: true,
		})
		if err != nil {
			return false, err
		}
	}

	for _, expr := range c.expr {
		if expr.Match(docInnerBytes) != c.match {
			return false, nil
		}
	}
	return true, nil
}

func (s *server) toFilterChain(ctx context.Context, loader *requestLoader, shard *index.IndexShard, req *service.SearchShardRequest) ([]Filterer, string, error) {
	// Order to apply parts:
	// -> Language Selection (positive and negative)
	// -> Positive content trigram selection
	// -> File path selection
	// -> Negative content selection (remember trigrams!)
	// -> Positive content selection
	// -> Snippet

	byCode := make(map[expr.ExpressionPart_Code][]*expr.ExpressionPart)
	for _, part := range req.GetExpression().GetParts() {
		byCode[part.GetCode()] = append(byCode[part.GetCode()], part)
	}

	var filters []Filterer

	if l := byCode[expr.ExpressionPart_LANGUAGE]; len(l) > 0 {
		return nil, "", status.Errorf(codes.Unimplemented, "language selection not implemented")
	}

	var positiveContentExprs []*regexp.Regexp
	var snippetParts []*syntax.Regexp
	for _, part := range byCode[expr.ExpressionPart_SNIPPET] {
		if part.GetNegated() {
			return nil, "", status.Error(codes.InvalidArgument, "unable to negate snippets")
		}

		re, err := regexp.Compile(part.GetExpression())
		if err != nil {
			return nil, "", err
		}
		positiveContentExprs = append(positiveContentExprs, re)

		syn, err := syntax.Parse(part.GetExpression(), syntax.Perl)
		if err != nil {
			return nil, "", err
		}
		snippetParts = append(snippetParts, syn)
		filters = append(filters, &queryFilterer{
			shard: shard,
			query: oldindex.RegexpQuery(syn),
		})
	}

	for _, part := range byCode[expr.ExpressionPart_MATCH] {
		if part.GetNegated() {
			continue
		}

		re, err := regexp.Compile(part.GetExpression())
		if err != nil {
			return nil, "", err
		}
		positiveContentExprs = append(positiveContentExprs, re)

		syn, err := syntax.Parse(part.GetExpression(), syntax.Perl)
		if err != nil {
			return nil, "", err
		}

		filters = append(filters, &queryFilterer{
			shard: shard,
			query: oldindex.RegexpQuery(syn),
		})
	}

	for _, part := range byCode[expr.ExpressionPart_FILE] {
		re, err := regexp.Compile(part.GetExpression())
		if err != nil {
			return nil, "", err
		}
		filters = append(filters, &fileFilterer{
			shard:  shard,
			expr:   re,
			prefix: req.GetPathPrefix(),
			match:  !part.GetNegated(),
		})
	}

	for _, part := range byCode[expr.ExpressionPart_MATCH] {
		if !part.GetNegated() {
			continue
		}
		return nil, "", status.Error(codes.Unimplemented, "negated content matches not yet available")
	}

	if len(positiveContentExprs) > 0 {
		filters = append(filters, &contentFilterer{
			ctx:    ctx,
			name:   fmt.Sprintf("%s%s%s", req.GetShardId(), suffixSeparator, "raw"),
			loader: loader,
			shard:  shard,
			expr:   positiveContentExprs,
			match:  true,
		})
	}

	return filters, (&syntax.Regexp{
		Op:  syntax.OpAlternate,
		Sub: snippetParts,
	}).String(), nil
}

func (s *server) SearchShard(ctx context.Context, req *service.SearchShardRequest) (*service.SearchShardResponse, error) {
	if len(req.GetShardSha256()) != len(index.SHA256{}) {
		return nil, status.Errorf(codes.InvalidArgument, "shard_sha256 wrong length %v, want %v", len(req.GetShardSha256()), len(index.SHA256{}))
	}

	loader := requestLoader{
		cache: s.cache,
		data:  make(map[index.SHA256][]byte),
	}
	defer loader.ReleaseAll()

	var sha index.SHA256
	copy(sha[:], req.GetShardSha256())

	v, err := loader.Get(ctx, sha, req.GetShardId(), cache.LoadParams{})
	if err != nil {
		if err == cache.ErrMemoryPressure {
			err = status.Errorf(codes.ResourceExhausted, "unable to load shard - cache full")
		}
		return nil, err
	}

	idx := index.Open(v)

	filters, snippetRE, err := s.toFilterChain(ctx, &loader, idx, req)
	if err != nil {
		return nil, err
	}

	re, err := regexp.Compile(snippetRE)
	if err != nil {
		return nil, err
	}

	var post *roaring.Bitmap
	for _, f := range filters {
		var err error
		post, err = f.Filter(post)
		if err != nil {
			return nil, err
		}
	}

	var resp service.SearchShardResponse
	resp.Matches = uint32(post.GetCardinality())
	var matchRange [][2]int
	post.Iterate(func(fileid uint32) bool {
		resp.DocsInspected++

		var doc index.Doc
		idx.Docs(&doc, int(fileid))

		// Load DocInner
		var rng index.ByteRange
		doc.Range(&rng)

		var docInnerBytes []byte
		if rb := idx.RawBytes(); rb != nil {
			start := rng.Start()
			docInnerBytes = rb[start : start+int64(rng.Length())]
		} else {
			var err error
			docInnerBytes, err = loader.Get(ctx, doc.Hash(), req.GetShardId()+".raw", cache.LoadParams{
				Start:       rng.Start(),
				Length:      rng.Length(),
				IgnoreLimit: true,
			})

			if err != nil {
				panic(err)
			}
		}

		in := index.GetRootAsDocInner(docInnerBytes, 0)
		b := in.DataBytes()

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
			Snippets: buildSnippets(in, matchRange),
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
	ret := &service.Shard{
		Id:       id.String(),
		TreeHash: s.TreeHash,
		State:    state,
		Size:     s.Size,
	}
	if s.SHA256 != nil {
		ret.Sha256 = s.SHA256[:]
	}
	return ret
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

	if len(req.GetSha256()) != len(index.SHA256{}) {
		return nil, status.Errorf(codes.InvalidArgument, "sha256 wrong length %v, want %v", len(req.GetSha256()), len(index.SHA256{}))
	}
	var sha index.SHA256
	copy(sha[:], req.GetSha256())

	return &service.CompleteShardResponse{}, manifestTxn(ctx, func(manifest *repo.Manifest) error {
		s := manifest.Shards[sID]
		if s == nil {
			return status.Errorf(codes.NotFound, "shard %q does not exist", req.GetShardId())
		}
		if !s.Created(plumbing.NewHash(req.GetTreeHash()), req.GetSize(), sha) {
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
		RepoRef: convertRepoRef(req.GetRepoName(), req.GetRef(), ref, manifest.Shards[ref.ShardID]),
	}, nil
}

func (indexMetadataServer) ListRepoRefs(ctx context.Context, req *service.ListRepoRefsRequest) (*service.ListRepoRefsResponse, error) {
	manifest, err := repo.LoadManifest(index.RepoDir())
	if err != nil {
		return nil, err
	}

	repos := make([]string, 0, len(manifest.Repos))
	for name := range manifest.Repos {
		repos = append(repos, name)
	}
	sort.Strings(repos)

	resp := &service.ListRepoRefsResponse{
		RepoRefs: make([]*service.RepoRef, len(repos)),
	}

	for i, name := range repos {
		repo := manifest.Repos[name]
		ref := repo.Refs[repo.DefaultRef]
		resp.RepoRefs[i] = convertRepoRef(name, repo.DefaultRef, ref, manifest.Shards[ref.ShardID])
	}

	return resp, nil
}

func (indexMetadataServer) SetDefaultRepoRef(ctx context.Context, req *service.SetDefaultRepoRefRequest) (*service.SetDefaultRepoRefResponse, error) {
	return &service.SetDefaultRepoRefResponse{}, manifestTxn(ctx, func(m *repo.Manifest) error {
		repo := m.Repos[req.GetRepoName()]
		if repo == nil {
			return status.Errorf(codes.NotFound, "repo %q does not exist", req.GetRepoName())
		}

		ref := repo.Refs[req.GetRef()]
		if ref == nil {
			return status.Errorf(codes.NotFound, "ref %q does not exist", req.GetRef())
		}

		repo.DefaultRef = req.GetRef()
		return nil
	})
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

	conn, err := grpc.DialContext(ctx, *storageService, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	s := &server{
		cache: cache.New(ctx, *cacheSize, cacheLoader{
			ss: ss.NewStorageServiceClient(conn),
		}),
	}

	g := grpc.NewServer()
	reflection.Register(g)
	service.RegisterIndexMetadataServiceServer(g, indexMetadataServer{})
	service.RegisterSearchShardServiceServer(g, s)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Printf("Serving on %v", lis.Addr())
	g.Serve(lis)
}
