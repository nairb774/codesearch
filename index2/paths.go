package index

import (
	"log"
	"os"
	"path/filepath"
)

func File() string {
	return filepath.Join(Storage(), "index2")
}

func Storage() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}

	dir := filepath.Join(cfg, "codesearch")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.Mkdir(dir, 0o770); err != nil {
			log.Fatal(err)
		}
	}

	return filepath.Clean(dir)
}

func RepoDir() string {
	dir := filepath.Join(Storage(), "repos")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.Mkdir(dir, 0o770); err != nil {
			log.Fatal(err)
		}
	}
	return dir
}

func ShardDir() string {
	dir := filepath.Join(Storage(), "shards")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.Mkdir(dir, 0o770); err != nil {
			log.Fatal(err)
		}
	}
	return dir
}
