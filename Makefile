BINARY_NAME=quickdrop
VERSION=1.0.0
BUILD_DIR=build

.PHONY: all clean build compress release

all: clean build

clean:
	rm -rf $(BUILD_DIR)
	mkdir -p $(BUILD_DIR)

build:
	@echo "🔨 Собираем $(BINARY_NAME) v$(VERSION)..."
	@echo ""
	
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64
	
	@echo ""
	@echo "✅ Сборка завершена!"
	@ls -lh $(BUILD_DIR)/

compress: build
	@echo "🗜️  Сжимаем бинарники..."
	@which upx > /dev/null || (echo "❌ UPX не установлен. Установи: brew install upx" && exit 1)
	upx --best --lzma $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe
	upx --best --lzma $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64
	upx --best --lzma $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64
	upx --best --lzma $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64
	@ls -lh $(BUILD_DIR)/

release: build
	@echo "📦 Создаем архивы..."
	cd $(BUILD_DIR) && zip $(BINARY_NAME)-windows-amd64.zip $(BINARY_NAME)-windows-amd64.exe
	cd $(BUILD_DIR) && zip $(BINARY_NAME)-darwin-amd64.zip $(BINARY_NAME)-darwin-amd64
	cd $(BUILD_DIR) && zip $(BINARY_NAME)-darwin-arm64.zip $(BINARY_NAME)-darwin-arm64
	cd $(BUILD_DIR) && zip $(BINARY_NAME)-linux-amd64.zip $(BINARY_NAME)-linux-amd64
	@echo "✅ Архивы готовы!"
	@ls -lh $(BUILD_DIR)/*.zip