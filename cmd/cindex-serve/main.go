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

	"github.com/google/codesearch/cmd/cindex-serve/cache"
	"github.com/google/codesearch/cmd/cindex-serve/service"
	oldindex "github.com/google/codesearch/index"
	"github.com/google/codesearch/index2"
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

func (s *server) Search(ctx context.Context, req *service.SearchRequest) (*service.SearchResponse, error) {
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

	var resp service.SearchResponse
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

var _ service.SearchServiceServer = (*server)(nil)

func main() {
	flag.Parse()
	ctx := context.Background()

	s := &server{
		cache: cache.New(ctx, *cacheSize, getSize, load),
	}

	g := grpc.NewServer()
	reflection.Register(g)
	service.RegisterSearchServiceServer(g, s)
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Print("Serving...")
	g.Serve(lis)
}
