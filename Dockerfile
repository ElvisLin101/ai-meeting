# ============================================================
# AI-Meeting Dockerfile
# 多阶段构建: 编译 → 运行, 最终镜像约 30MB
# ============================================================

# 阶段1: 编译
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

# 先拷依赖文件, 利用 Docker 缓存
COPY go.mod go.sum ./
RUN go mod download

# 拷源码
COPY . .

# 编译 (CGO 禁用, 静态链接)
RUN CGO_ENABLED=0 GOOS=linux go build -o ai-meeting .

# 阶段2: 运行
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app

# 拷二进制和静态资源
COPY --from=builder /build/ai-meeting .
COPY --from=builder /build/static ./static

# 配置文件通过 volume 挂载, 不打进镜像
# docker run -v $(pwd)/config:/app/config ...

EXPOSE 8080

ENTRYPOINT ["./ai-meeting"]
