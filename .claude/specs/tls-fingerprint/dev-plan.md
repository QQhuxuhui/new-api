# TLS Fingerprint - Development Plan

## Overview
集成 uTLS 库模拟 Node.js v22 TLS 指纹，实现 JA3 Hash `0cce74b0d9b7f8528fb2181588d23793`，支持直连和代理客户端，默认启用。

## Task Breakdown

### Task 1: 定义 NodeJS22 TLS 指纹 spec 和 JA3 计算器
- **ID**: task-1
- **type**: default
- **Description**:
  - 基于 JA3 Text `771,4866-4867-4865-49199-...,0-11-10-35-...,29-23-30-25-...,0-1-2` 构建 `utls.ClientHelloSpec`
  - 实现 cipher suites 有序列表（包含 TLS 1.3 和 1.2 密码套件）
  - 配置 extensions（包含 SNI, EC point formats, supported groups, signature algorithms, ALPN 等）
  - 配置 supported groups（包含 FFDHE groups: 256, 257, 258, 259, 260 和 ECDHE groups）
  - 实现本地 JA3 计算 helper 用于测试验证
- **File Scope**: service/tls_fingerprint.go
- **Dependencies**: None
- **Test Command**: `go test ./service -run TestJA3 -v -cover`
- **Test Focus**:
  - ClientHelloSpec 字段完整性（cipher suites, extensions, groups, point formats）
  - JA3 hash 计算结果与目标值 `0cce74b0d9b7f8528fb2181588d23793` 一致
  - JA3 text 各部分（TLS version, ciphers, extensions, groups, point formats）格式正确

### Task 2: 集成 uTLS transport 到默认客户端
- **ID**: task-2
- **type**: default
- **Description**:
  - 在 `service/http_client.go` 中实现自定义 `http.RoundTripper`
  - 包装标准 `http.Transport`，覆盖 `DialTLSContext` 方法
  - 使用 uTLS 执行 TLS 握手，应用 NodeJS22 ClientHelloSpec
  - 配置 HTTP/1.1（`ForceAttemptHTTP2=false`，因 Node.js JA3 不包含 ALPN h2）
  - 更新 `GetDefaultHTTPClient()` 使用 uTLS transport
  - 保留超时、重试、连接池等现有配置
- **File Scope**: service/http_client.go
- **Dependencies**: task-1
- **Test Command**: `go test ./service -run TestDefaultClient -v -coverprofile=coverage.out && go tool cover -func=coverage.out | grep total`
- **Test Focus**:
  - 默认客户端成功建立 HTTPS 连接
  - TLS 握手使用正确的 ClientHelloSpec
  - HTTP/1.1 连接正常工作（非 HTTP/2）
  - 超时和重试机制保持不变
  - 错误处理（无效证书、网络错误）

### Task 3: 更新代理客户端共享 uTLS dial 路径
- **ID**: task-3
- **type**: default
- **Description**:
  - 更新 `GetProxyHTTPClient()` 支持 uTLS
  - 实现代理场景下的 uTLS dialer（HTTP/HTTPS/SOCKS5）
  - 保持 per-proxy client cache 语义（相同代理 URL 复用客户端）
  - 确保代理和直连使用相同的 TLS 指纹
  - 处理代理认证和连接池
- **File Scope**: service/http_client.go
- **Dependencies**: task-1, task-2
- **Test Command**: `go test ./service -run TestProxyClient -v -coverprofile=coverage_proxy.out && go tool cover -func=coverage_proxy.out | grep total`
- **Test Focus**:
  - HTTP/HTTPS/SOCKS5 代理连接成功
  - 代理客户端使用正确的 TLS 指纹
  - Client cache 机制正常工作
  - 代理认证流程正确
  - 代理失败场景的错误处理

### Task 4: 端到端 JA3 验证和覆盖率检查
- **ID**: task-4
- **type**: default
- **Description**:
  - 编写集成测试验证完整的 TLS 握手流程
  - 实现 mock TLS server 捕获 ClientHello 并计算 JA3
  - 验证直连和代理场景的 JA3 hash 一致性
  - 测试 relay 层使用 HTTP client 的场景（如 Claude API 调用）
  - 确保整体代码覆盖率 ≥90%
- **File Scope**: service/http_client_test.go, relay/channel/*/adaptor_test.go
- **Dependencies**: task-2, task-3
- **Test Command**: `go test ./service ./relay/channel/claude -v -coverprofile=coverage_full.out && go tool cover -func=coverage_full.out | grep total`
- **Test Focus**:
  - Mock TLS server 正确捕获 ClientHello
  - JA3 hash 验证通过（直连 + 代理）
  - Relay 层 API 调用使用正确的客户端
  - 边界条件（网络中断、证书过期、握手失败）
  - 整体覆盖率 ≥90%

## Acceptance Criteria
- [ ] 默认 HTTP 客户端使用 uTLS 模拟 Node.js v22 指纹
- [ ] JA3 Hash 验证为 `0cce74b0d9b7f8528fb2181588d23793`
- [ ] 支持直连和代理客户端（HTTP/HTTPS/SOCKS5）
- [ ] TLS 指纹默认启用，无需额外配置
- [ ] 保持现有 HTTP client 行为（超时、重试、连接池）
- [ ] Relay 层 API 调用（如 Claude）透明使用 uTLS 客户端
- [ ] 所有单元测试和集成测试通过
- [ ] 代码覆盖率 ≥90%

## Technical Notes
- **uTLS 库**: 使用 `github.com/refraction-networking/utls` 实现 TLS 指纹伪装
- **Node.js v22 特征**:
  - TLS version: 771 (TLS 1.2)
  - 支持 TLS 1.3 cipher suites (4866, 4867, 4865)
  - 包含 FFDHE groups (256-260) 和 ECDHE groups (29, 23, 30, 25, 24)
  - EC point formats: uncompressed, ansiX962_compressed_prime, ansiX962_compressed_char2
- **HTTP/2 约束**: Node.js JA3 不包含 ALPN `h2`，强制使用 HTTP/1.1（设置 `ForceAttemptHTTP2=false`）
- **代理兼容性**: uTLS dialer 需在代理 CONNECT 成功后执行握手，保持代理认证和连接复用
- **测试策略**: 使用 `crypto/tls` 的 `Config.GetConfigForClient` 回调捕获 ClientHello 进行 JA3 验证
- **向后兼容**: 现有代码无需修改，仅替换底层 transport 实现
