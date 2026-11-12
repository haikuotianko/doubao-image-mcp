# syntax=docker/dockerfile:1

# 最小运行镜像：只跑 MCP stdio 服务，不含 GUI（GUI 依赖 WebView2，只有 Windows 桌面端有）。
#
# 用途：给 Glama 这类需要「容器里能起来并响应 tools/list」的目录做自检，
# 也方便在 Linux 上跑一份现成的 MCP 服务。
#
#   docker build -t doubao-image-mcp .
#   docker run -i --rm doubao-image-mcp -mcp
#
# 注意：容器里没有浏览器，扫码登录用不了；先在本机用 `-set-session` 导入登录态，
# 再把这个 profile 目录挂进容器（默认在 ~/.config/doubao-image-mcp，Windows 在 %APPDATA%）。

FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# frontend/dist 在 .gitignore 里，仓库中没有；
# MCP 模式用不到界面，建个占位文件让 //go:embed all:frontend/dist 能编译通过。
RUN mkdir -p frontend/dist \
    && printf '<!doctype html><title>doubao-image-mcp</title>\n' > frontend/dist/index.html \
    && CGO_ENABLED=0 go build -trimpath \
        -ldflags "-s -w -X doubao-image-mcp/internal/version.Version=docker" \
        -o /out/doubao-image-mcp .

FROM alpine:3.20
RUN adduser -D -h /home/mcp mcp && apk add --no-cache ca-certificates

COPY --from=build /out/doubao-image-mcp /usr/local/bin/doubao-image-mcp

USER mcp
WORKDIR /home/mcp

ENTRYPOINT ["doubao-image-mcp"]
CMD ["-mcp"]
