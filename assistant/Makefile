# 空间助手 (Wails v2 桌面程序)
#   make bundle STORE_KEY=…  生成"空间 + 底座"载荷包 (底座产物取自 RMKIT_REPO, 空间取自本仓库)
#                            STORE_KEY 是官方商店的客户端标识, 发版必须带; 留空也能打,
#                            但那个包装出来的设备连不上官方商店 (自建商店不受影响)
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
# 发布: 先 make bundle 出安装包, 再 make build-mac / build-win —— 助手里会写死这个安装包的
# sha256 和发布标签 (RELEASE_TAG, 默认 = VERSION), 下载后对不上就不装。
# 国内镜像地址放在不进仓库的 assistant/.cn-mirror (一行, 如 http://x.x.x.x/space/)
RELEASE_TAG ?= $(VERSION)
SPACE_SHA := $(shell [ -f dist/space-bundle.zip ] && shasum -a 256 dist/space-bundle.zip | cut -d' ' -f1)
CN_MIRROR ?= $(strip $(shell cat .cn-mirror 2>/dev/null))
# Android 包 (几百 MB, 几个月才变一次) 不跟着每版重传: ANDROID_TAG 是它放在哪个发布标签下
# (GitHub Release 与国内镜像同名目录), ANDROID_SHA 是它的 sha256, ANDROID_VER 是包里 manifest
# 的版本 (和设备上记的比, 判断「已是最新」)。换了 Android 包就把新包传到当次发布, 再改这三个值。
ANDROID_TAG ?= v1.0.0
ANDROID_SHA ?= e3e2ec81916f0984b490c1b135c919c20f5aa53af2f6b3d691f5fd3de21f0453
ANDROID_VER ?= v1.2.1-23-g288baa3
LDFLAGS := -X main.version=$(VERSION) -X "main.buildTime=$(BUILD_TIME)" \
	-X main.releaseTag=$(RELEASE_TAG) -X main.spaceSHA=$(SPACE_SHA) -X main.cnMirror=$(CN_MIRROR) \
	-X main.androidTag=$(ANDROID_TAG) -X main.androidSHA=$(ANDROID_SHA) -X main.androidVer=$(ANDROID_VER)

.PHONY: bundle bundle-android build-mac build-win cli dev vet

# STORE_KEY 透给仓库根的 make。不自动编译的话很容易打进一个旧的、没有商店密钥的
# 二进制 —— 装出来的设备取不到商店, 而且设备上看不出是这个原因。
bundle:
	$(MAKE) -C .. all STORE_KEY=$(STORE_KEY)
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
