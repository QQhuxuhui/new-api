FROM oven/bun:latest AS builder

WORKDIR /build

# 先复制依赖文件
COPY web/package.json .
COPY web/bun.lock .

# 安装依赖
RUN bun install

# 复制配置文件
COPY web/vite.config.js .
COPY web/postcss.config.js .
COPY web/tailwind.config.js .
COPY web/i18next.config.js .
COPY web/jsconfig.json .
COPY web/index.html .

# 复制源代码目录
COPY web/src ./src
COPY web/public ./public

# 复制版本文件
COPY ./VERSION .

# 构建
RUN DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat VERSION) bun run build

FROM golang:alpine AS builder2
ENV GO111MODULE=on CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}


WORKDIR /build

ADD go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=builder /build/dist ./web/dist
RUN go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" -o new-api

FROM alpine

RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates tzdata \
    && update-ca-certificates

COPY --from=builder2 /build/new-api /
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/new-api"]
