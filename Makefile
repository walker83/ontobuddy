# myonto Makefile
# 用法：make <target>，常用：setup / link / build / test / install-skills

BINARY   := myonto
CMD_DIR  := ./cmd/myonto
BIN_DIR  := bin
# 全局安装目录：符号链接目标。优先用 ~/.local/bin（通常已在 PATH），
# 不存在则回退到 /usr/local/bin（需要 sudo）。
LOCAL_BIN := $(HOME)/.local/bin

# 跨平台构建用变量。
GOOS     ?= $(shell go env GOOS)
GOARCH   ?= $(shell go env GOARCH)
LDFLAGS  := -s -w

.PHONY: all setup build test vet fmt install link unlink install-skills uninstall-skills clean help cross-compile eval test-all check coverage

all: build

## setup: 配置 Go 国内镜像（GOPROXY=goproxy.cn, GOSUMDB=阿里云），首次 clone 后运行一次即可
setup:
	@echo "==> 配置 Go 国内镜像"
	go env -w GOPROXY=https://goproxy.cn,direct
	go env -w GOSUMDB=sum.golang.google.cn
	@echo "    GOPROXY = $$(go env GOPROXY)"
	@echo "    GOSUMDB = $$(go env GOSUMDB)"

## build: 编译二进制到 ./bin/myonto
build:
	@mkdir -p $(BIN_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD_DIR)
	@echo "==> 已构建 $(BIN_DIR)/$(BINARY) ($(GOOS)/$(GOARCH))"

## link: 创建符号链接 ~/.local/bin/myonto -> 本项目 ./bin/myonto（全局调用，开发期重编译自动生效）
link: build
	@mkdir -p $(LOCAL_BIN)
	@rm -f $(LOCAL_BIN)/$(BINARY)
	@ln -s $$(pwd)/$(BIN_DIR)/$(BINARY) $(LOCAL_BIN)/$(BINARY)
	@echo "==> 已创建符号链接 $(LOCAL_BIN)/$(BINARY) -> $$(pwd)/$(BIN_DIR)/$(BINARY)"
	@echo ""
	@echo "现在可以在任意目录直接用 myonto 命令。"
	@command -v myonto >/dev/null 2>&1 || { \
	  echo ""; \
	  echo "⚠️  $(LOCAL_BIN) 不在 PATH 中。请把以下内容加入 ~/.zshrc："; \
	  echo "    export PATH=\"$(LOCAL_BIN):\$$PATH\""; \
	}

## unlink: 移除符号链接
unlink:
	@rm -f $(LOCAL_BIN)/$(BINARY)
	@echo "==> 已移除 $(LOCAL_BIN)/$(BINARY)"

## test: 跑全部单测
test:
	go test -v ./...

## vet: 静态检查
vet:
	go vet ./...

## eval: 跑推理 golden 评估（库层 P/R/F1，无副作用）
## 每个 golden case 必须 precision=recall=F1=1.0 才算过。
## 用例见 internal/reasoning/eval/testdata/cases/
eval:
	go test -v ./internal/reasoning/eval/ -run 'TestRunAllCases'

## coverage: 生成覆盖率报告到 coverage.txt，并断言整体覆盖率达标
coverage:
	go test -coverprofile=coverage.txt ./...
	@echo "==> 覆盖率摘要："
	@go tool cover -func=coverage.txt | tail -1

## check: 本地一站式门禁（vet + test + eval），CI 跑等价命令
check: vet test eval
	@echo "==> ✓ vet / test / eval 全过"

## crosscheck: 跑 owlrl 对拍（需 Python3 + rdflib + owlrl；pip install -r eval/requirements.txt）
## 非每 PR 必跑——作为周期性基线，验证 myonto 不比 OWL 2 RL 标准多推/少推。
## 加 --strict 让 FP>0 时 exit 1（CI 硬门禁用）。
crosscheck: build
	python3 eval/crosscheck.py eval/cases_for_crosscheck/ --report eval/report.md

## fmt: 格式化代码
fmt:
	@gofmt -w .

## install: 用 go install 编译并放到 $GOPATH/bin
install:
	go install $(CMD_DIR)

## install-skills: 把 skills/myonto/ 复制到 ~/.claude/skills/myonto/，并附带 myonto 二进制
## 装好后 skill 完全自含：LLM 可用 ../../bin/myonto 或 $(skill_dir)/bin/myonto 直接调用
install-skills: build
	@SKILL_DST="$$HOME/.claude/skills/myonto"; \
	rm -rf "$$SKILL_DST"; \
	mkdir -p "$$SKILL_DST"/bin; \
	cp skills/myonto/SKILL.md "$$SKILL_DST"/; \
	cp skills/myonto/INSTALLATION.md "$$SKILL_DST"/ 2>/dev/null || true; \
	cp -R skills/myonto/references "$$SKILL_DST"/; \
	cp $(BIN_DIR)/$(BINARY) "$$SKILL_DST"/bin/$(BINARY); \
	chmod +x "$$SKILL_DST"/bin/$(BINARY); \
	echo "==> 已安装 skill 到 $$SKILL_DST"; \
	echo "    文件清单："; \
	find "$$SKILL_DST" -type f | sed 's|^$$SKILL_DST|  |'; \
	echo ""; \
	echo "    二进制大小："; \
	du -h "$$SKILL_DST/bin/$(BINARY)" | awk '{print "      " $$1 "  " $$2}'

## uninstall-skills: 移除 ~/.claude/skills/myonto/
uninstall-skills:
	@rm -rf "$$HOME/.claude/skills/myonto"
	@echo "==> 已卸载 myonto skill"

## cross-compile: 交叉编译多平台二进制到 dist/
cross-compile:
	@mkdir -p dist
	@for os in darwin linux windows; do \
	  for arch in amd64 arm64; do \
	    ext=""; [ $$os = windows ] && ext=".exe"; \
	    echo "==> 构建 $$os/$$arch"; \
	    GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-$$os-$$arch$$ext $(CMD_DIR); \
	  done; \
	done

## clean: 清理构建产物
clean:
	rm -rf $(BIN_DIR) dist

## help: 显示本帮助
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //; s/:/:\t/' | column -t -s $$'\t'

.DEFAULT_GOAL := help
