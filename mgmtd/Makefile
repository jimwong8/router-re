# mgmtd — 配置管理守护进程构建工具

# 默认目标
.PHONY: all build test clean

# 构建
build:
	CGO_ENABLED=0 go build -o bin/mgmtd ./cmd/mgmtd

# 测试
test:
	go test ./...

# 清理
clean:
	rm -rf bin

# 默认执行 build
all: build
