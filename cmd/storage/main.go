package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"

	"github.com/google/codesearch/cmd/storage/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

var (
	port        = flag.Int("port", 0, "Port to listen on")
	storagePath = flag.String("storage_path", "", "Location where all data is to be stored")
)

var shardIDRegexp = regexp.MustCompile(`^[0-9a-f]+$`)

const blockSize = 64 << 10

type storage struct {
	path string
}

func validateShardIDAndSuffix(shardID, suffix string) error {
	if shardID == "" {
		return status.Error(codes.InvalidArgument, "shard_id missing")
	}
	if !shardIDRegexp.MatchString(shardID) {
		return status.Errorf(codes.InvalidArgument, "shard_id must match regexp: %q", shardIDRegexp.String())
	}
	if suffix != "" && suffix != "raw" {
		return status.Error(codes.InvalidArgument, "unsupported suffix")
	}
	return nil
}

func cleanupOSError(err error) error {
	if os.IsNotExist(err) {
		err = status.Error(codes.NotFound, err.Error())
	}
	return err
}

func (s *storage) getPath(shardID, suffix string) string {
	path := filepath.Join(s.path, shardID)
	if suffix != "" {
		path = fmt.Sprintf("%s.%s", path, suffix)
	}
	return path
}

func (s *storage) GetShardMetadata(_ context.Context, req *service.GetShardMetadataRequest) (*service.GetShardMetadataResponse, error) {
	shardID := req.GetShardId()
	suffix := req.GetSuffix()
	if err := validateShardIDAndSuffix(shardID, suffix); err != nil {
		return nil, err
	}

	fi, err := os.Stat(s.getPath(shardID, suffix))
	if err != nil {
		return nil, cleanupOSError(err)
	}

	return &service.GetShardMetadataResponse{
		Length: fi.Size(),
	}, nil
}

func (s *storage) ReadShard(req *service.ReadShardRequest, stream service.StorageService_ReadShardServer) error {
	shardID := req.GetShardId()
	suffix := req.GetSuffix()
	if err := validateShardIDAndSuffix(shardID, suffix); err != nil {
		return err
	}

	f, err := os.Open(s.getPath(shardID, suffix))
	if err != nil {
		return cleanupOSError(err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return cleanupOSError(err)
	}
	if want, got := req.GetOffset()+int64(req.GetLength()), fi.Size(); want > got {
		return status.Errorf(codes.OutOfRange, "would read out of bounds, want %v available %v", want, got)
	}

	if _, err := f.Seek(req.GetOffset(), os.SEEK_SET); err != nil {
		return cleanupOSError(err)
	}

	resp := &service.ReadShardResponse{
		Block: make([]byte, blockSize),
	}
	for remaining := int(req.GetLength()); remaining > 0; {
		size := cap(resp.Block)
		if remaining < size {
			size = remaining
		}

		n, err := f.Read(resp.Block[:size])
		if n > 0 {
			resp.Block = resp.Block[:n]
			if err := stream.Send(resp); err != nil {
				return err
			}
			remaining -= n
		}

		if err != nil {
			if remaining == 0 && err == io.EOF {
				err = nil
			}
			return cleanupOSError(err)
		}
	}

	return nil
}

func (s *storage) WriteShard(stream service.StorageService_WriteShardServer) error {
	head, err := stream.Recv()
	if err != nil {
		return err
	}

	shardID := head.GetShardId()
	suffix := head.GetSuffix()
	if err := validateShardIDAndSuffix(shardID, suffix); err != nil {
		return err
	}

	f, err := os.OpenFile(s.getPath(shardID, suffix), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return cleanupOSError(err)
	}
	defer f.Close()

	sum := sha256.New()

	out := io.MultiWriter(sum, f)
	if block := head.GetBlock(); len(block) > 0 {
		if _, err := out.Write(block); err != nil {
			return cleanupOSError(err)
		}
	}

	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		block := req.GetBlock()
		if len(block) == 0 {
			return status.Error(codes.InvalidArgument, "empty block")
		}

		if _, err := out.Write(block); err != nil {
			return cleanupOSError(err)
		}
	}

	if err := f.Close(); err != nil {
		return cleanupOSError(err)
	}

	return stream.SendAndClose(&service.WriteShardResponse{
		Sha256: sum.Sum(nil),
	})
}

func main() {
	flag.Parse()

	g := grpc.NewServer()
	reflection.Register(g)
	service.RegisterStorageServiceServer(g, &storage{
		path: *storagePath,
	})
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Printf("Serving on %v", lis.Addr())
	g.Serve(lis)
}
