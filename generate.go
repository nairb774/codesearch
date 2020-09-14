package codesearch

//go:generate flatc --go --go-namespace index --gen-onefile -o index2 index2/idx.fbs

//go:generate protoc --go_opt=paths=source_relative --go_out=plugins=grpc:. cmd/cindex-serve/service/service.proto cmd/storage/service/service.proto expr/expr.proto
