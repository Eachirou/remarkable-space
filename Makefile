# 产物放 dist/, 两个架构都出 (rm2 是 armv7, Paper Pro / Move 是 aarch64)
GO_FLAGS := -trimpath -ldflags=-s\ -w

.PHONY: all aarch64 armv7 vet clean
all: aarch64 armv7

aarch64:
	GOOS=linux GOARCH=arm64 go build $(GO_FLAGS) -o dist/space-server-aarch64 ./cmd/space-server

armv7:
	GOOS=linux GOARCH=arm GOARM=7 go build $(GO_FLAGS) -o dist/space-server-armv7 ./cmd/space-server

vet:
	go vet ./...

clean:
	rm -f dist/space-server-*
