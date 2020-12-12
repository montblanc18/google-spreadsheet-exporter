#############
# Build
#############

.PHONY: deps clean build unittest test

build: deps
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -buildid=" -trimpath -o bin/lambda ./cmd/lambda/lambda.go

clean:
	rm -rf ./bin/*

deps:
	go mod download
	go mod tidy

unittest:
	go test -v ./

test:
	export DYNAMO_ENDPOINT=http://localhost:4566;\
	export DYNAMO_TABLE_EXPORT=local_export;\
	export GOOGLE_EXPORT_ROOT_DIR="";\
	go test -race -v ./