# 性能优化工具包

本目录包含了针对高峰期日志页面性能问题的完整优化方案。

## 📋 问题描述

高峰期访问日志页面时，API请求首字节时间（TTFB）超过30秒，用户体验极差。

## 🔍 根本原因

1. **数据库查询性能瓶颈**：日志表数据量大，缺少针对查询场景的复合索引
2. **多次数据库操作**：每次请求执行 COUNT + SELECT + JOIN 多次查询
3. **连接池配置不足**：高峰期数据库连接池可能耗尽

## 🚀 快速开始

### 方案1：一键优化（推荐）⭐⭐⭐⭐⭐

```bash
cd /usr/src/workspace/github/QQhuxuhui/new-api
./docs/optimize.sh
```

这个脚本会自动：
- ✅ 备份当前配置
- ✅ 添加10个优化索引（5-15分钟）
- ✅ 优化docker-compose配置
- ✅ 重启服务

**预期效果**：TTFB从30秒降低到 **1-3秒**

### 方案2：手动优化

如果你想分步执行或只执行部分优化：

#### 步骤1：诊断当前状态

```bash
./docs/diagnose.sh
```

这会生成详细的性能诊断报告，包括：
- 数据库表大小
- 索引使用情况
- 连接状态
- 慢查询统计
- 资源使用情况

#### 步骤2：添加数据库索引

```bash
# 方式1：使用SQL文件
docker exec -i postgres psql -U root -d new-api < docs/optimize-indexes.sql

# 方式2：手动执行
docker exec -it postgres psql -U root -d new-api
# 然后复制粘贴 optimize-indexes.sql 中的SQL语句
```

#### 步骤3：优化Docker配置

```bash
# 备份当前配置
cp docker-compose.yml docker-compose.yml.backup

# 使用优化配置
cp docs/docker-compose-optimized.yml docker-compose.yml

# 重启服务
docker-compose down
docker-compose up -d
```

## 📁 文件说明

| 文件 | 说明 |
|------|------|
| `performance-optimization.md` | 完整的性能优化方案文档 |
| `optimize.sh` | 一键优化脚本（推荐使用） |
| `diagnose.sh` | 性能诊断脚本 |
| `optimize-indexes.sql` | 数据库索引优化SQL |
| `docker-compose-optimized.yml` | 优化后的Docker配置 |

## 🎯 优化效果

| 优化方案 | 预期TTFB | 实施难度 | 停机时间 |
|---------|---------|---------|---------|
| 仅添加索引 | 2-5秒 | ⭐ 简单 | 0分钟 |
| 索引+连接池 | 1-3秒 | ⭐⭐ 简单 | 1-2分钟 |
| 索引+连接池+PG优化 | 0.5-1秒 | ⭐⭐ 简单 | 1-2分钟 |
| 完整方案（含缓存） | 0.2-0.5秒 | ⭐⭐⭐ 中等 | 需要开发 |

## 📊 监控建议

优化后，建议持续监控以下指标：

```bash
# 1. 查看应用日志
docker-compose logs -f new-api

# 2. 查看数据库日志
docker-compose logs -f postgres

# 3. 监控资源使用
docker stats

# 4. 定期运行诊断
./docs/diagnose.sh

# 5. 查看慢查询（如果启用了pg_stat_statements）
docker exec postgres psql -U root -d new-api -c \
  "SELECT query, calls, mean_exec_time FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10;"
```

## 🔧 故障排除

### 问题1：索引创建失败

```bash
# 检查数据库连接
docker exec postgres psql -U root -d new-api -c "SELECT version();"

# 查看错误日志
docker-compose logs postgres
```

### 问题2：服务启动失败

```bash
# 恢复备份配置
cp docker-compose.yml.backup docker-compose.yml
docker-compose down
docker-compose up -d

# 查看详细日志
docker-compose logs -f
```

### 问题3：性能仍然不佳

1. 运行诊断脚本查看详细信息：`./docs/diagnose.sh`
2. 检查日志表大小，如果超过10GB建议归档历史数据
3. 检查服务器资源使用情况（CPU、内存、磁盘IO）
4. 考虑升级服务器配置或实施缓存方案

## 📈 进阶优化

如果基础优化后性能仍不满意，可以考虑：

### 1. 数据归档

定期归档历史日志数据：

```bash
# 删除3个月前的日志（通过API）
curl -X GET "http://localhost:3000/api/log/delete?target_timestamp=$(date -d '3 months ago' +%s)"
```

### 2. 添加Redis缓存

修改代码，对统计查询添加缓存：
- 缓存时间：30-60秒
- 缓存键：基于查询参数生成
- 高峰期直接返回缓存数据

### 3. 读写分离

如果数据量持续增长，可以考虑：
- 使用PostgreSQL主从复制
- 读操作走从库
- 写操作走主库

### 4. 分表策略

对于超大数据量（千万级），可以考虑：
- 按月分表
- 使用PostgreSQL分区表
- 历史数据归档到冷存储

## 🆘 需要帮助？

如果遇到问题或需要进一步优化建议，请：

1. 运行 `./docs/diagnose.sh` 生成诊断报告
2. 查看应用和数据库日志
3. 在项目Issues中提问，附上诊断报告

## 📝 更新日志

- 2026-03-03: 初始版本，包含索引优化和配置优化方案
