package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/codesearch/cmd/cindex-serve/service"
	"github.com/google/codesearch/index2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	indexMetadataService = flag.String("index_metadata_service", "localhost:8801", "Location to find the IndexMetadataService.")
	repoPath             = flag.String("repo", "", "Path to repo")
	repoName             = flag.String("name", "", "Name of the repo")
	revisionFlag         = revFlag{plumbing.Revision("HEAD")}
	force                = flag.Bool("force", false, "Force indexing even if shard exists")
)

func init() {
	flag.Var(&revisionFlag, "rev", "Revision to index")
}

type revFlag struct {
	plumbing.Revision
}

var _ flag.Value = (*revFlag)(nil)

func (r *revFlag) Set(v string) error {
	r.Revision = plumbing.Revision(v)
	return nil
}
func (r *revFlag) String() string { return r.Revision.String() }

func writeShard(iw *index.IndexWriter, path string) (uint64, [sha256.Size]byte, error) {
	b, err := iw.ToBytes()
	l := uint64(len(b))
	h := sha256.Sum256(b)
	if err != nil {
		return l, h, err
	}
	tmpFile := path + "~"
	if err := ioutil.WriteFile(tmpFile, b, 0o600); err != nil {
		os.Remove(tmpFile)
		return l, h, err
	}

	return l, h, os.Rename(tmpFile, path)
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

func main() {
	flag.Parse()

	ctx := context.Background()

	conn, err := grpc.DialContext(ctx, *indexMetadataService, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	indexMetadata := service.NewIndexMetadataServiceClient(conn)

	gitRepo, err := git.PlainOpen(*repoPath)
	if err != nil {
		log.Fatal(err)
	}

	hash, err := gitRepo.ResolveRevision(revisionFlag.Revision)
	if err != nil {
		log.Fatal(err)
	}

	c, err := gitRepo.CommitObject(*hash)
	if err != nil {
		log.Fatal(err)
	}

	if *repoName == "" {
		*repoName = *repoPath
	}

	if !*force {
		switch resp, err := indexMetadata.GetRepoRevision(ctx, &service.GetRepoRevisionRequest{
			RepoName: *repoName,
			Revision: revisionFlag.Revision.String(),
		}); status.Code(err) {
		case codes.OK:
			if resp.GetRevision().GetCommitHash() == hash.String() {
				log.Printf("Already indexed %v@%v:%v", *repoName, revisionFlag, hash)
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
			if _, err := indexMetadata.UpdateRepoShard(ctx, &service.UpdateRepoShardRequest{
				RepoName:   *repoName,
				Revision:   revisionFlag.Revision.String(),
				CommitHash: hash.String(),
				ShardId:    shards.GetShardIds()[0],
			}); err != nil {
				log.Fatal(err)
			}
			return
		}
	}

	// Build new index shard
	s, err := indexRepo(c)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := indexMetadata.AllocateShard(ctx, &service.AllocateShardRequest{})
	if err != nil {
		log.Fatal(err)
	}

	path := filepath.Join(index.ShardDir(), resp.GetShard().GetId())
	shardSize, sha256, err := writeShard(s, path)
	if err != nil {
		log.Fatal(err)
	}

	if _, err := indexMetadata.CompleteShard(ctx, &service.CompleteShardRequest{
		ShardId:  resp.GetShard().GetId(),
		TreeHash: c.TreeHash.String(),
		Size:     shardSize,
		Sha256:   hex.EncodeToString(sha256[:]),
	}); err != nil {
		log.Fatal(err)
	}

	if _, err := indexMetadata.UpdateRepoShard(ctx, &service.UpdateRepoShardRequest{
		RepoName:   *repoName,
		Revision:   revisionFlag.Revision.String(),
		CommitHash: hash.String(),
		ShardId:    resp.GetShard().GetId(),
	}); err != nil {
		log.Fatal(err)
	}
}
