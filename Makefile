SHELL := /bin/bash
.DEFAULT_GOAL := help

.PHONY: help install dev-api dev-web build run test test-api test-web clean docker

help:
	@echo "Catch — 常用命令"
	@echo "  make install   安装前后端依赖"
	@echo "  make dev-api   启动 Go API (:8091)"
	@echo "  make dev-web   启动 Vite 前端 (:5173)"
	@echo "  make build     构建前端并编译 Go 二进制"
	@echo "  make run       Go 单进程托管前端 (:8080)"
	@echo "  make test      运行全部测试与类型检查"
	@echo "  make docker    docker compose up --build"
	@echo "  make clean     清理构建产物"

install:
	cd backend && go mod download
	cd frontend && pnpm install

dev-api:
	cd backend && \
		MONGODB_URI=$${MONGODB_URI:-mongodb://127.0.0.1:27017} \
		MONGODB_DB=$${MONGODB_DB:-catch} \
		PORT=8091 \
		PUBLIC_URL=http://localhost:8091 \
		APP_URL=http://localhost:5173 \
		CORS_ORIGINS=http://localhost:5173,http://localhost:8091 \
		ALLOW_DEV_LOGIN=true SEED_DEMO=$${SEED_DEMO:-true} \
		go run ./cmd/server

dev-web:
	cd frontend && pnpm dev

build:
	cd frontend && pnpm install --frozen-lockfile && pnpm build
	cd backend && mkdir -p bin && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/catch ./cmd/server

run: build
	STATIC_DIR=$(CURDIR)/frontend/dist \
		APP_ENV=development \
		PORT=8080 \
		PUBLIC_URL=http://localhost:8080 \
		APP_URL=http://localhost:8080 \
		CORS_ORIGINS=http://localhost:8080 \
		ALLOW_DEV_LOGIN=true SEED_DEMO=true \
		./backend/bin/catch

test:
	cd backend && go test ./...
	cd frontend && pnpm typecheck

test-api:
	cd backend && go test ./...

test-web:
	cd frontend && pnpm typecheck

clean:
	rm -rf frontend/dist backend/bin

docker:
	docker compose up --build
