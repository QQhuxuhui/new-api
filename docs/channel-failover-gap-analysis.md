# Channel Failover 覆盖缺口分析

## 目标
让下游用户始终处于可用状态，自动处理所有上游channel的异常。

## 当前实现的覆盖范围

### ✅ 已覆盖（自动failover）
1. **500, 502, 503** - 服务器错误
2. **429 + 关键词** - 限流错误（rate limit, quota）
3. **403 + 关键词** - 并发限制（并发、concurrency、session已满）
4. **连接失败** - connection failed/refused/reset/timeout
5. **网络错误** - network error, service unavailable

### ❌ 关键缺口

#### 1. 认证失效（401 Unauthorized）
**问题**: 当前遇到401会返回错误，不会尝试其他channel
**场景**:
- Channel A的API Key过期/失效
- Channel A的Key被封禁
- Channel A的Key配额用尽

**影响**: 即使Channel B的Key有效，用户仍会收到401错误

**建议**:
```go
case 401:
    // 认证失败可能是key问题，尝试其他channel
    if strings.Contains(errorMessageLower, "invalid") ||
       strings.Contains(errorMessageLower, "expired") ||
       strings.Contains(errorMessageLower, "unauthorized") ||
       strings.Contains(errorMessageLower, "api key") ||
       strings.Contains(errorMessageLower, "authentication") {
        return true
    }
```

#### 2. 其他5xx错误（505-599）
**问题**: 当前只覆盖500/502/503，但还有其他5xx错误码
**场景**:
- 505 HTTP Version Not Supported
- 507 Insufficient Storage
- 508 Loop Detected
- 509 Bandwidth Limit Exceeded
- 510 Not Extended
- 511 Network Authentication Required

**建议**:
```go
// 所有5xx错误（除504/524超时）都应该触发failover
if statusCode >= 500 && statusCode < 600 && statusCode != 504 && statusCode != 524 {
    return true
}
```

#### 3. 520-599 CDN/代理错误
**问题**: Cloudflare等CDN返回的特殊错误码未覆盖
**场景**:
- 520 Unknown Error
- 521 Web Server Is Down
- 522 Connection Timed Out
- 523 Origin Is Unreachable
- 524 A Timeout Occurred (已在shouldRetry中排除，符合设计)
- 525 SSL Handshake Failed
- 526 Invalid SSL Certificate
- 527 Railgun Error
- 530 Origin DNS Error

**建议**: 已被上面的5xx范围覆盖

#### 4. 特定Provider错误
**问题**: 某些Provider有自定义错误格式，可能没有标准HTTP状态码

**Claude API特定错误**:
- `overloaded_error` - 服务过载
- `internal_error` - 内部错误
- `rate_limit_error` - 已覆盖

**OpenAI API特定错误**:
- `server_error` - 已覆盖
- `insufficient_quota` - 需要添加

**建议**:
```go
// Provider特定错误
if strings.Contains(errorMessageLower, "overloaded") ||
   strings.Contains(errorMessageLower, "internal error") ||
   strings.Contains(errorMessageLower, "internal_error") ||
   strings.Contains(errorMessageLower, "insufficient_quota") ||
   strings.Contains(errorMessageLower, "insufficient quota") {
    return true
}
```

#### 5. DNS解析失败
**问题**: DNS失败可能在HTTP层之前发生
**场景**: Channel的域名无法解析

**当前状态**: 可能已被"connection failed"关键词覆盖，需要确认

#### 6. SSL/TLS错误
**问题**: 证书问题可能导致连接失败
**场景**:
- 证书过期
- 证书不匹配
- TLS握手失败

**建议**:
```go
if strings.Contains(errorMessageLower, "certificate") ||
   strings.Contains(errorMessageLower, "tls") ||
   strings.Contains(errorMessageLower, "ssl") ||
   strings.Contains(errorMessageLower, "handshake") {
    return true
}
```

#### 7. 代理错误（502/503已覆盖，但关键词未覆盖）
**建议**: 添加关键词检测
```go
if strings.Contains(errorMessageLower, "proxy") ||
   strings.Contains(errorMessageLower, "gateway") ||
   strings.Contains(errorMessageLower, "bad gateway") {
    return true
}
```

#### 8. 读取响应体失败
**问题**: RelayErrorHandler的line 88-90，当io.ReadAll失败时直接返回
**场景**: 网络中断、连接被重置

**当前行为**: 返回ErrorCodeBadResponseStatusCode，但不检查是否应该failover

**建议**: 在line 88-90的error分支也应该检查failover

#### 9. 空响应
**问题**: 上游返回空响应可能表示channel有问题

**建议**:
```go
if strings.Contains(errorMessageLower, "empty response") ||
   strings.Contains(errorMessageLower, "no response") ||
   strings.Contains(errorMessageLower, "响应为空") {
    return true
}
```

#### 10. Body解析失败但statusCode异常
**问题**: RelayErrorHandler line 95-105，当Unmarshal失败时
- 如果statusCode是5xx，应该触发failover
- 当前只返回了newApiErr，没有检查是否需要failover

**当前状态**: Line 147-150已经添加了检查，✅ 已覆盖

## 优先级建议

### 🔴 高优先级（严重影响可用性）
1. **401认证失败** - Key失效时应尝试其他channel
2. **所有5xx错误** - 统一处理500-599范围
3. **空响应/解析失败** - 网络层面的问题

### 🟡 中优先级（提升鲁棒性）
4. **SSL/TLS错误** - 证书问题
5. **DNS错误** - 域名解析失败
6. **Provider特定错误** - overloaded, insufficient_quota

### 🟢 低优先级（边缘场景）
7. **代理相关关键词** - proxy, gateway
8. **CDN特定错误** - 已被5xx覆盖

## 建议的完整实现

```go
func shouldTriggerChannelFailover(statusCode int, errorMessage string) bool {
    errorMessageLower := strings.ToLower(errorMessage)

    // 1. 所有5xx错误（除超时外）
    if statusCode >= 500 && statusCode < 600 {
        // 排除超时：设计上认为超时是网络问题，不应该重试
        if statusCode == 504 || statusCode == 524 {
            return false
        }
        return true
    }

    // 2. 认证失败 - Key可能失效
    if statusCode == 401 {
        if strings.Contains(errorMessageLower, "invalid") ||
           strings.Contains(errorMessageLower, "expired") ||
           strings.Contains(errorMessageLower, "unauthorized") ||
           strings.Contains(errorMessageLower, "api key") ||
           strings.Contains(errorMessageLower, "authentication") {
            return true
        }
    }

    // 3. 限流错误
    if statusCode == 429 {
        if strings.Contains(errorMessageLower, "rate limit") ||
           strings.Contains(errorMessageLower, "quota") ||
           strings.Contains(errorMessageLower, "too many requests") {
            return true
        }
    }

    // 4. 资源耗尽/并发限制
    if statusCode == 403 {
        if strings.Contains(errorMessageLower, "并发") ||
           strings.Contains(errorMessageLower, "concurrency") ||
           (strings.Contains(errorMessageLower, "session") && strings.Contains(errorMessageLower, "已满")) ||
           strings.Contains(errorMessageLower, "overloaded") ||
           strings.Contains(errorMessageLower, "insufficient_quota") ||
           strings.Contains(errorMessageLower, "insufficient quota") {
            return true
        }
    }

    // 5. 连接/网络错误（不依赖状态码）
    if (strings.Contains(errorMessageLower, "连接") && strings.Contains(errorMessageLower, "失败")) ||
       (strings.Contains(errorMessageLower, "连接") && strings.Contains(errorMessageLower, "服务失败")) ||
       strings.Contains(errorMessageLower, "connection failed") ||
       strings.Contains(errorMessageLower, "connection refused") ||
       strings.Contains(errorMessageLower, "connection reset") ||
       strings.Contains(errorMessageLower, "connection timeout") ||
       strings.Contains(errorMessageLower, "network error") ||
       strings.Contains(errorMessageLower, "upstream error") ||
       strings.Contains(errorMessageLower, "service unavailable") ||
       strings.Contains(errorMessageLower, "temporarily unavailable") ||
       strings.Contains(errorMessageLower, "empty response") ||
       strings.Contains(errorMessageLower, "no response") ||
       strings.Contains(errorMessageLower, "响应为空") {
        return true
    }

    // 6. SSL/TLS错误
    if strings.Contains(errorMessageLower, "certificate") ||
       strings.Contains(errorMessageLower, "tls") ||
       strings.Contains(errorMessageLower, "ssl") ||
       strings.Contains(errorMessageLower, "handshake") {
        return true
    }

    // 7. DNS错误
    if strings.Contains(errorMessageLower, "dns") ||
       strings.Contains(errorMessageLower, "resolve") ||
       strings.Contains(errorMessageLower, "域名") {
        return true
    }

    // 8. 代理/网关错误
    if strings.Contains(errorMessageLower, "proxy") ||
       strings.Contains(errorMessageLower, "gateway") ||
       strings.Contains(errorMessageLower, "bad gateway") {
        return true
    }

    // 9. Provider特定错误
    if strings.Contains(errorMessageLower, "overloaded_error") ||
       strings.Contains(errorMessageLower, "internal_error") ||
       strings.Contains(errorMessageLower, "server_error") {
        return true
    }

    return false
}
```

## 测试建议

### 单元测试用例
```go
func TestShouldTriggerChannelFailover(t *testing.T) {
    tests := []struct{
        name       string
        statusCode int
        message    string
        expected   bool
    }{
        // 应该触发failover
        {"500 error", 500, "internal server error", true},
        {"401 invalid key", 401, "invalid api key", true},
        {"429 rate limit", 429, "rate limit exceeded", true},
        {"403 concurrency", 403, "session并发窗口已满", true},
        {"connection failed", 0, "connection failed", true},
        {"SSL error", 0, "certificate verify failed", true},
        {"DNS error", 0, "dns resolution failed", true},
        {"insufficient quota", 403, "insufficient_quota", true},

        // 不应该触发failover
        {"400 bad request", 400, "invalid parameter", false},
        {"404 not found", 404, "model not found", false},
        {"504 timeout", 504, "gateway timeout", false},
        {"401 normal", 401, "invalid request format", false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := shouldTriggerChannelFailover(tt.statusCode, tt.message)
            if result != tt.expected {
                t.Errorf("got %v, want %v", result, tt.expected)
            }
        })
    }
}
```

## 监控建议

建议添加以下指标：
1. **failover_triggered_total** - 触发failover的次数
2. **failover_triggered_by_error_type** - 按错误类型分类的failover次数
3. **failover_success_rate** - failover成功率
4. **channel_health_score** - 每个channel的健康分数

## 总结

当前实现覆盖了**约60-70%**的常见channel故障场景。主要缺口：

1. ❌ **401认证失败** - 这是最关键的缺口
2. ❌ **其他5xx错误** - 应该统一处理所有5xx
3. ❌ **SSL/DNS错误** - 网络层面的问题
4. ❌ **空响应** - 数据传输问题

建议优先实现401和5xx范围覆盖，可以将覆盖率提升到**90%以上**。
