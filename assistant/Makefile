# 空间助手 (Wails v2 桌面程序)
#   make bundle            生成"空间 + 底座"载荷包 (底座产物取自 RMKIT_REPO, 空间取自本仓库)
#   make bundle-android    从 ANDROID_DIR 生成 android-rmppm 载荷包
#   make build-mac         构建 macOS .app (build/bin/)
#   make build-win         交叉构建 Windows exe
#   make cli               构建命令行版 (无 GUI, 同一套内核)
WAILS ?= $(HOME)/go/bin/wails
ANDROID_DIR ?= ../../paper-move-android/android-bundle
# 底座 (冻结): 注入链/汉化/输入法后端的产物仍在 rmkit-cn 仓库里
RMKIT_REPO ?= ../../rmkit-cn
VERSION ?= $(shell git -C .. describe --tags --always 2>/dev/null || echo dev)
BUILD_TIME := $(shell date +%Y-%m-%d\ %H:%M)
LDFLAGS := -X main.version=$(VERSION) -X "main.buildTime=$(BUILD_TIME)"

.PHONY: bundle bundle-android build-mac build-win cli dev vet

bundle:
	@test -f ../dist/space-server-aarch64 || { echo "先在仓库根跑 make all 出后端二进制"; exit 1; }
	mkdir -p dist
	go run ./cmd/mkbundle -component space -repo $(RMKIT_REPO) -space-repo .. -version $(VERSION) -out dist/space-bundle.zip

bundle-android:
	go run ./cmd/mkbundle -component android-rmppm -android-dir $(ANDROID_DIR) -version $(VERSION) -out dist/android-rmppm-bundle.zip

build-mac:
	$(WAILS) build -clean -platform darwin/universal -ldflags '$(LDFLAGS)'
	touch build/bin/space-assistant.app
	mkdir -p dist
	cd build/bin && rm -f ../../dist/space-assistant-mac.zip && zip -qry ../../dist/space-assistant-mac.zip space-assistant.app

build-win:
	$(WAILS) build -platform windows/amd64 -o space-assistant.exe -ldflags '$(LDFLAGS)'
	mkdir -p dist
	cp build/bin/space-assistant.exe dist/space-assistant-windows.exe

cli:
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -trimpath -o dist/assistant-cli-darwin-arm64 ./cmd/assistant-cli
	GOOS=windows GOARCH=amd64 go build -trimpath -o dist/assistant-cli-windows-amd64.exe ./cmd/assistant-cli

dev:
	$(WAILS) dev

vet:
	go vet ./... && cd frontend && npx tsc --noEmit
