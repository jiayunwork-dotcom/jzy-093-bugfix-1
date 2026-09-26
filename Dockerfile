# syntax=docker/dockerfile:1
# ---- 构建 + 测试阶段：测试随仓库一起在容器内跑通 ----
FROM golang:1.22-bookworm AS build
WORKDIR /src

# 先拉依赖（本项目只用标准库，这层主要利用缓存）
COPY go.mod ./
RUN go mod download

COPY . .

# 容器构建期执行全部自动化测试（-race 在跨架构 CI 上偶发开销/兼容问题，
# 竞态检测放在本地 make test 里，容器内保证测试本身跑通）。
RUN go test ./... -count=1

# 静态构建，固定监听 8080
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/rocalc ./cmd/server

# ---- 运行阶段 ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/rocalc /rocalc
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/rocalc"]
