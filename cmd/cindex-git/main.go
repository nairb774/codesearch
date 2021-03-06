package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bgentry/go-netrc/netrc"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/codesearch/cmd/cindex-serve/service"
	ss "github.com/google/codesearch/cmd/storage/service"
	"github.com/google/codesearch/index2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxBlockSize = 64 << 10

var (
	indexMetadataService = flag.String("index_metadata_service", "localhost:8801", "Location to find the IndexMetadataService.")
	storageService       = flag.String("storage_service", "", "Address of storage service.")
	repoPath             = flag.String("repo", "", "Path to working repo")

	repoURL  = flag.String("url", "", "Url of repo to index")
	repoName = flag.String("name", "", "Name of the repo. If empty, will be determined from the URL")
	ref      = refFlag{plumbing.HEAD}

	netrcPath = flag.String("netrc", "", "If set, this file will be used as a netrc file for performing the fetch.")

	force = flag.Bool("force", false, "Force indexing even if shard exists")
)

func init() {
	flag.Var(&ref, "ref", "Ref to index")
}

type refFlag struct {
	plumbing.ReferenceName
}

var _ flag.Value = (*refFlag)(nil)

func (r *refFlag) Set(v string) error {
	r.ReferenceName = plumbing.ReferenceName(v)
	return nil
}
func (r *refFlag) String() string { return r.ReferenceName.String() }

func streamBytes(stream ss.StorageService_WriteShardClient, b []byte) error {
	var req ss.WriteShardRequest
	for len(b) > 0 {
		chunk := maxBlockSize
		if chunk > len(b) {
			chunk = len(b)
		}
		req.Block = b[:chunk]
		if err := stream.Send(&req); err != nil {
			return err
		}
		b = b[chunk:]
	}
	return nil
}

func writeShard(ctx context.Context, storage ss.StorageServiceClient, shardID string, iw *index.IndexWriter) (uint64, [sha256.Size]byte, error) {
	b, err := iw.ToBytes()
	l := uint64(len(b))
	h := sha256.Sum256(b)
	if err != nil {
		return l, h, err
	}

	if err := func() error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		stream, err := storage.WriteShard(ctx)
		if err != nil {
			return err
		}

		if err := stream.Send(&ss.WriteShardRequest{
			ShardId: shardID,
		}); err != nil {
			return err
		}

		if err := streamBytes(stream, b); err != nil {
			return err
		}

		resp, err := stream.CloseAndRecv()
		if err != nil {
			return err
		}
		if !bytes.Equal(resp.GetSha256(), h[:]) {
			return errors.New("corruption during write of shard")
		}
		return nil
	}(); err != nil {
		return l, h, err
	}

	rb := iw.RawBytes()
	if len(rb) == 0 {
		return l, h, nil
	}

	if err := func() error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		stream, err := storage.WriteShard(ctx)
		if err != nil {
			return err
		}

		if err := stream.Send(&ss.WriteShardRequest{
			ShardId: shardID,
			Suffix:  "raw",
		}); err != nil {
			return err
		}

		sum := sha256.New()

		for _, b := range rb {
			if _, err := sum.Write(b); err != nil {
				return fmt.Errorf("while calculating hash: %v", err)
			}
			if err := streamBytes(stream, b); err != nil {
				return err
			}
		}

		resp, err := stream.CloseAndRecv()
		if err != nil {
			return err
		}

		if !bytes.Equal(resp.GetSha256(), sum.Sum(nil)) {
			return errors.New("corruption during write of shard raw suffix")
		}

		return nil
	}(); err != nil {
		return l, h, err
	}

	return l, h, nil
}

func indexRepo(c *object.Commit) (*index.IndexWriter, error) {
	iter, err := c.Files()
	if err != nil {
		return nil, err
	}

	var iw index.IndexWriter
	return &iw, iter.ForEach(func(f *object.File) error {
		if err := iw.AddObject(time.Time{}, f); err != nil {
			if _, ok := err.(*index.FileContentsError); ok {
				log.Printf("%v: %v", f.Name, err)
			} else {
				return fmt.Errorf("%v: %v", f.Name, err)
			}
		}
		return nil
	})
}

func simplifyURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	if prefix := "git@github.com:"; strings.HasPrefix(url, prefix) {
		url = strings.TrimPrefix(url, prefix)
		url = "github.com/" + url
	}

	return url
}

type netrcAuth struct {
	path string
	rc   *netrc.Netrc
}

var _ githttp.AuthMethod = (*netrcAuth)(nil)

func (rc *netrcAuth) String() string { return rc.path }
func (rc *netrcAuth) Name() string   { return "netrc-auth" }
func (rc *netrcAuth) SetAuth(r *http.Request) {
	m := rc.rc.FindMachine(r.URL.Hostname())
	if m == nil {
		return
	}
	if m.Login != "" && m.Password != "" {
		r.SetBasicAuth(m.Login, m.Password)
	} else if m.Account != "" {
		r.Header.Add("Authorization", fmt.Sprintf("Bearer %s", m.Account))
	}

}

func syncRemote(ctx context.Context, repo *git.Repository, url string, ref plumbing.ReferenceName) (plumbing.ReferenceName, error) {
	hash := sha256.Sum256([]byte(url))
	hexHash := hex.EncodeToString(hash[:])
	mappedRef := plumbing.NewRemoteReferenceName(hexHash, ref.String())

	// This needs to be larger than git.maxHavesToVisitPerRef. The
	// implementation, as of v5.1.0, tries to walk 100 commits to provide as
	// "haves" to the remote during negotiation. If the depth is less than the
	// number of objects walked, we get a really useful "object not found" error.
	//
	// TODO: Fix
	// https://github.com/go-git/go-git/blob/8019144b6534ff58ad234a355e5b143f1c99b45e/remote.go#L635
	// to pass in the shallow roots to prevent walking off the repo's history.
	depth := 101
	r, err := repo.Remote(hexHash)
	if err == git.ErrRemoteNotFound {
		log.Printf("Adding %s as remote %s", url, hexHash)
		r, err = repo.CreateRemote(&config.RemoteConfig{
			Name: hexHash,
			URLs: []string{url},
		})
	} else if _, err := repo.Reference(mappedRef, false); err == nil {
		depth = 0 // Just extend what we have already - no need to shorten.
	}

	if err != nil {
		return mappedRef, err
	}

	refSpec := config.RefSpec(fmt.Sprintf("%v:%v", ref, mappedRef))
	log.Printf("Fetching %v", refSpec)

	var auth transport.AuthMethod
	if path := *netrcPath; path != "" {
		n, err := netrc.ParseFile(path)
		if err != nil {
			log.Fatal(err)
		}
		auth = &netrcAuth{
			path: path,
			rc:   n,
		}
	}

	_ = depth // TODO: Fix shallow handling in go-git
	err = r.FetchContext(ctx, &git.FetchOptions{
		RefSpecs: []config.RefSpec{refSpec},
		Depth:    0, // depth,
		Progress: os.Stdout,
		Tags:     git.NoTags,
		Auth:     auth,
	})
	if err == git.NoErrAlreadyUpToDate {
		err = nil
	}

	return mappedRef, err
}

func main() {
	flag.Parse()

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigchan)
	go func() {
		for range sigchan {
			cancel()
		}
	}()

	indexMetadataConn, err := grpc.DialContext(ctx, *indexMetadataService, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer indexMetadataConn.Close()
	indexMetadata := service.NewIndexMetadataServiceClient(indexMetadataConn)

	ssConn, err := grpc.DialContext(ctx, *storageService, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer ssConn.Close()
	storage := ss.NewStorageServiceClient(ssConn)

	gitRepo, err := git.PlainOpen(*repoPath)
	if err != nil {
		if err != git.ErrRepositoryNotExists {
			log.Fatal(err)
		}
		gitRepo, err = git.PlainInit(*repoPath, true /*isBare*/)
		if err != nil {
			log.Fatal(err)
		}
	}

	localRefName, err := syncRemote(ctx, gitRepo, *repoURL, ref.ReferenceName)
	if err != nil {
		log.Fatalf("While syncing remote: %v", err)
	}

	resolved, err := gitRepo.Reference(localRefName, true)
	if err != nil {
		log.Fatal(err)
	}

	c, err := gitRepo.CommitObject(resolved.Hash())
	if err != nil {
		t, tErr := gitRepo.TagObject(resolved.Hash())
		if tErr != nil {
			log.Fatal(err)
		}
		c, err = t.Commit()
		if err != nil {
			log.Fatal(err)
		}
	}

	if *repoName == "" {
		*repoName = simplifyURL(*repoURL)
	}

	if !*force {
		switch resp, err := indexMetadata.GetRepoRef(ctx, &service.GetRepoRefRequest{
			RepoName: *repoName,
			Ref:      ref.ReferenceName.String(),
		}); status.Code(err) {
		case codes.OK:
			if resp.GetRepoRef().GetCommitHash() == c.Hash.String() {
				log.Printf("Already indexed %v@%v:%v", *repoName, ref, c.Hash)
				return
			}

		case codes.NotFound:
		default:
			log.Fatal(err)
		}

		// See if the tree exists?
		shards, err := indexMetadata.SearchShards(ctx, &service.SearchShardsRequest{
			TreeHash: c.TreeHash.String(),
			States: []service.Shard_State{
				service.Shard_UNREFERENCED,
				service.Shard_REFERENCED,
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		// Found an existing shard, update to that:
		if len(shards.GetShardIds()) > 0 {
			req := &service.UpdateRepoShardRequest{
				RepoName:   *repoName,
				Ref:        ref.ReferenceName.String(),
				CommitHash: c.Hash.String(),
				ShardId:    shards.GetShardIds()[0],
			}
			if _, err := indexMetadata.UpdateRepoShard(ctx, req); err != nil {
				log.Fatalf("%v %v", req, err)
			}
			return
		}
	}

	log.Printf("Building index for %v", c)

	// Build new index shard
	s, err := indexRepo(c)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := indexMetadata.AllocateShard(ctx, &service.AllocateShardRequest{})
	if err != nil {
		log.Fatal(err)
	}

	shardSize, sha256, err := writeShard(ctx, storage, resp.GetShard().GetId(), s)
	if err != nil {
		log.Fatal(err)
	}

	if _, err := indexMetadata.CompleteShard(ctx, &service.CompleteShardRequest{
		ShardId:  resp.GetShard().GetId(),
		TreeHash: c.TreeHash.String(),
		Size:     shardSize,
		Sha256:   sha256[:],
	}); err != nil {
		log.Fatal(err)
	}

	if _, err := indexMetadata.UpdateRepoShard(ctx, &service.UpdateRepoShardRequest{
		RepoName:   *repoName,
		Ref:        ref.ReferenceName.String(),
		CommitHash: c.Hash.String(),
		ShardId:    resp.GetShard().GetId(),
	}); err != nil {
		log.Fatal(err)
	}
}
