portquiz: portquiz-client portquiz-server

deps: go.mod
	GOPROXY=direct go mod download
	GOPROXY=direct go get -u all

update-deps:
	go get -u
	go mod tidy

clean:
	rm -rf portquiz client server dist/

fmt:
	gofmt -s -w -l .

portquiz-client: go.mod go.sum client/*go
	go build -o $@ client/*.go

portquiz-server: go.mod go.sum server/*go
	go build -o $@ server/*.go

release:
	goreleaser build --snapshot --clean
