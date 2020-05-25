package main

import (
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
	"github.com/google/codesearch/index2"
	"github.com/google/codesearch/repo"
)

var (
	repoPath     = flag.String("repo", "", "Path to repo")
	repoName     = flag.String("name", "", "Name of the repo")
	revisionFlag = revFlag{plumbing.Revision("HEAD")}
	force        = flag.Bool("force", false, "Force indexing even if shard exists")
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

func setManifestData(m *repo.Manifest, name string, rev plumbing.Revision, hash plumbing.Hash, shardID repo.ShardID) error {
	r := m.GetOrCreateRepo(name)
	if r.DefaultRevision == "" {
		r.DefaultRevision = rev.String()
	}
	repoRev := r.GetOrCreateRevision(rev)
	repoRev.CommitHash = hash.String()
	repoRev.ShardID = shardID

	if !m.ReconcileShardStates() {
		return fmt.Errorf("unable to reconcile %#v", m)
	}
	return nil
}

func main() {
	flag.Parse()

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

	var shardID repo.ShardID
	{
		manifest, err := repo.LoadManifest(index.RepoDir())
		if err != nil {
			log.Fatal(err)
		}

		done := false
		if *force {
			shardID = manifest.CreateShard(c.TreeHash)
		} else if manifest.Repos[*repoName].GetRevision(revisionFlag.Revision).GetCommitHash() == *hash {
			log.Printf("Already indexed %v@%v:%v", *repoName, revisionFlag, hash)
			return
		} else {
			var ok bool
			shardID, ok = manifest.ReferencableShard(c.TreeHash)
			if ok {
				if err := setManifestData(manifest, *repoName, revisionFlag.Revision, *hash, shardID); err != nil {
					log.Fatal(err)
				}
				done = true
			} else {
				shardID = manifest.CreateShard(c.TreeHash)
			}
		}

		if err := repo.WriteManifest(manifest, index.RepoDir()); err != nil {
			log.Fatal(err)
		}

		if done {
			return
		}
	}

	path := filepath.Join(index.ShardDir(), shardID.String())

	// Build new index shards
	s, err := indexRepo(c)
	if err != nil {
		log.Fatal(err)
	}
	shardSize, sha256, err := writeShard(s, path)
	if err != nil {
		log.Fatal(err)
	}

	manifest, err := repo.LoadManifest(index.RepoDir())
	if err != nil {
		log.Fatal(err)
	}

	if shard := manifest.Shards[shardID]; !shard.Created(shardSize, hex.EncodeToString(sha256[:])) {
		log.Fatalf("shard %v in bad state %#v", shardID, shard)
	}

	if err := setManifestData(manifest, *repoName, revisionFlag.Revision, *hash, shardID); err != nil {
		log.Fatal(err)
	}

	if err := repo.WriteManifest(manifest, index.RepoDir()); err != nil {
		log.Fatal(err)
	}
}
