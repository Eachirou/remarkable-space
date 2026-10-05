# 产物放 dist/, 两个架构都出 (rm2 是 armv7, Paper Pro / Move 是 aarch64)

# 官方商店的客户端标识。带上它取索引才有东西, 浏览器直接打开索引地址是 404。
# 不进仓库 —— 发版时 make STORE_KEY=xxx, 或放进环境变量 SPACE_STORE_KEY。
# 留空照样能编, 自建商店与本地开发不受影响。
STORE_KEY ?=
KEY_PKG   := github.com/boangs/remarkable-space/internal/space
GO_FLAGS  := -trimpath -ldflags "-s -w -X '$(KEY_PKG).storeKey=$(STORE_KEY)'"

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
