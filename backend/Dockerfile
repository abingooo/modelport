FROM golang:1.27.0-alpine

WORKDIR /app

# 安装必要的工具
RUN apk add --no-cache git

# 复制go.mod和go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN VERSION_VALUE="$(./scripts/resolve-version.sh)" && \
    go build -ldflags="-s -w -X main.Version=${VERSION_VALUE}" -o main ./cmd/server/

# The development image also runs the server without root privileges. Keep
# the writable runtime directory owned by the service account.
RUN addgroup -g 1000 -S sub2api && \
    adduser -u 1000 -S -D -H -s /sbin/nologin -G sub2api sub2api && \
    mkdir -p /app/data && \
    chown -R sub2api:sub2api /app/data

USER sub2api

# 暴露端口
EXPOSE 8080

# 运行应用
CMD ["./main"]
