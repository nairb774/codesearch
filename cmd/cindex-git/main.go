package main

import (
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

func writeShard(iw *index.IndexWriter, path string) (uint64, error) {
	b, err := iw.ToBytes()
	if err != nil {
		return 0, err
	}
	tmpFile := path + "~"
	if err := ioutil.WriteFile(tmpFile, b, 0o600); err != nil {
		os.Remove(tmpFile)
		return 0, err
	}

	return uint64(len(b)), os.Rename(tmpFile, path)
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

// func statusSplit(data []byte, atEOF bool) (advance int, token []byte, err error) {
// 	if len(data) == 0 {
// 		return 0, nil, nil
// 	}
//
// 	var offset int
// 	switch data[0] {
// 	case '1':
// 		// 1 XY $sub $mH    $mI    $mW    $hH                                      $hI                                      $path
// 		// 1 .. .... ...... ...... ...... f0a9573ea5bc00aaa16820155d03ebeb8dc3f48f f0a9573ea5bc00aaa16820155d03ebeb8dc3f48f f0
// 		// 1234567891123456789212345678931234567894123456789512345678961234567897123456789812345678991234567890123456789112345
// 		offset = 115
//
// 	case '?':
// 		// ? f0
// 		// 1234
// 		offset = 4
//
// 	case 'u':
// 		// u XY $sub $m1    $m2    $m3    $mW    $h1                                      $h2                                      $h3                                      $path
// 		// u .. .... ...... ...... ...... ...... f0a9573ea5bc00aaa16820155d03ebeb8dc3f48f f0a9573ea5bc00aaa16820155d03ebeb8dc3f48f f0a9573ea5bc00aaa16820155d03ebeb8dc3f48f f0
// 		// 1234567891123456789212345678931234567894123456789512345678961234567897123456789812345678991234567890123456789112345678921234567893123456789412345678951234567896123
// 		offset = 163
//
// 	default:
// 		return 0, nil, fmt.Errorf("unexpected prefix %02X", data[1])
// 	}
//
// 	if len(data) < offset {
// 		if atEOF {
// 			return 0, nil, errors.New("incomplete status")
// 		}
// 		return 0, nil, nil
// 	}
//
// 	idx := offset + bytes.IndexByte(data[offset:], 0)
// 	if idx < offset {
// 		if atEOF {
// 			return 0, nil, errors.New("incomplete status")
// 		}
// 		return 0, nil, nil
// 	}
// 	return idx + 1, data[:idx], nil
// }
//
// type indexMeta struct {
// 	name string
// 	mode filemode.FileMode
// 	hash plumbing.Hash
// }
//
// func gitStatus(ctx context.Context, path string) ([]indexMeta, []string, error) {
// 	cmd := exec.CommandContext(ctx, "git", "-C", path, "status", "--no-renames", "--porcelain=v2")
// 	var out bytes.Buffer
// 	cmd.Stdout = &out
//
// 	if err := cmd.Run(); err != nil {
// 		return nil, nil, err
// 	}
//
// 	var idx []indexMeta
// 	var ws []string
//
// 	s := bufio.NewScanner(&out)
// 	s.Split(statusSplit)
// 	for s.Scan() {
// 		parts := bytes.Split(s.Bytes(), []byte(" "))
// 		switch parts[0][0] {
// 		case '1':
// 			if !bytes.Equal(parts[3], parts[4]) || !bytes.Equal(parts[6], parts[7]) {
// 				fm, err := filemode.New(string(parts[4]))
// 				if err != nil {
// 					return nil, nil, err
// 				}
// 				i := len(idx)
// 				idx = append(idx, indexMeta{
// 					name: string(parts[8]),
// 					mode: fm,
// 				})
// 				if n, err := hex.Decode(idx[i].hash[:], parts[7]); err != nil || n != len(idx[0].hash) {
// 					return nil, nil, fmt.Errorf("unable to parse hash %q: %v", err)
// 				}
// 			}
// 			if parts[1][1] != '.' && parts[1][1] != 'M' && parts[1][1] != 'D' {
// 				panic(fmt.Errorf("off xy: %q", string(parts[1])))
// 			}
// 			if parts[1][1] == 'M' || !bytes.Equal(parts[4], parts[5]) {
// 				ws = append(ws, string(parts[8]))
// 			}
// 		case '?':
// 			ws = append(ws, string(parts[1]))
//
// 		case 'u':
// 			// Do something smarter, just index everything from the working tree for
// 			// now:
// 			ws = append(ws, string(parts[10]))
// 		}
// 	}
//
// 	if err := s.Err(); err != nil {
// 		return nil, nil, err
// 	}
//
// 	return idx, ws, nil
// }

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
	// ctx := context.Background()

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
			shardID, _ = manifest.CreateShard(c.TreeHash)
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
				shardID, _ = manifest.CreateShard(c.TreeHash)
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
	shardSize, err := writeShard(s, path)
	if err != nil {
		log.Fatal(err)
	}

	manifest, err := repo.LoadManifest(index.RepoDir())
	if err != nil {
		log.Fatal(err)
	}

	if shard := manifest.Shards[shardID]; !shard.Created(shardSize) {
		log.Fatalf("shard %v in bad state %#v", shardID, shard)
	}

	if err := setManifestData(manifest, *repoName, revisionFlag.Revision, *hash, shardID); err != nil {
		log.Fatal(err)
	}

	if err := repo.WriteManifest(manifest, index.RepoDir()); err != nil {
		log.Fatal(err)
	}
}
