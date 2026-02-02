# 伪装数据分析功能实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为 Claude 渠道添加伪装追踪功能，记录最近 100 条请求的原始/伪装对比数据，并在 Dashboard 中提供可视化分析界面。

**Architecture:** 后端使用环形缓冲区存储追踪记录（已存在 `masquerade_trace.go`），新增 API 控制器暴露数据。前端在 Dashboard 组件中新增 Tab 页签，使用表格+弹出框展示对比详情。

**Tech Stack:** Go (Gin), React, Semi Design UI

---

## Task 1: 完善后端追踪数据采集

**Files:**
- Modify: `relay/channel/claude/adaptor.go:103-140`
- Modify: `relay/channel/claude/relay-claude.go:416-440`
- Modify: `relay/channel/claude/masquerade_trace.go`

**Step 1: 扩展 MasqueradeTraceRecord 结构体**

在 `masquerade_trace.go` 中已有基础结构，确认字段完整性：

```go
// masquerade_trace.go - 确认现有结构体包含所有必要字段
type MasqueradeTraceRecord struct {
    ID          string `json:"id"`
    Timestamp   int64  `json:"timestamp"`
    Model       string `json:"model"`
    ChannelID   int    `json:"channel_id"`
    ChannelName string `json:"channel_name"`

    OriginalHeaders map[string]string `json:"original_headers"`
    OriginalBody    string            `json:"original_body"`

    MaskedHeaders map[string]string `json:"masked_headers"`
    MaskedBody    string            `json:"masked_body"`

    OriginalUserID  string `json:"original_user_id"`
    MaskedUserID    string `json:"masked_user_id"`
    OriginalSession string `json:"original_session"`
    MaskedSession   string `json:"masked_session"`
}
```

**Step 2: 在 SetupRequestHeader 中采集 Headers 数据**

修改 `adaptor.go` 的 `SetupRequestHeader` 方法，在设置伪装 Headers 前后采集数据：

```go
// adaptor.go - SetupRequestHeader 方法中添加追踪逻辑
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
    // 采集原始请求头
    originalHeaders := make(map[string]string)
    for key, values := range c.Request.Header {
        if len(values) > 0 {
            originalHeaders[key] = values[0]
        }
    }

    // 现有的 header 设置逻辑...
    channel.SetupApiRequestHeader(info, c, req)
    req.Set("x-api-key", info.ApiKey)
    // ... 其他 header 设置 ...

    // 采集伪装后的请求头
    maskedHeaders := make(map[string]string)
    for key, values := range *req {
        if len(values) > 0 {
            maskedHeaders[key] = values[0]
        }
    }

    // 存储到 context 供后续使用
    c.Set("masquerade_original_headers", originalHeaders)
    c.Set("masquerade_masked_headers", maskedHeaders)

    CommonClaudeHeadersOperation(c, req, info)
    return nil
}
```

**Step 3: 在请求处理完成后记录完整追踪数据**

在 `relay-claude.go` 的 `RequestOpenAI2ClaudeMessage` 函数末尾添加追踪记录：

```go
// relay-claude.go - RequestOpenAI2ClaudeMessage 函数末尾
func RequestOpenAI2ClaudeMessage(c *gin.Context, info *relaycommon.RelayInfo, textRequest dto.GeneralOpenAIRequest) (*dto.ClaudeRequest, error) {
    // ... 现有逻辑 ...

    masked, originalUserID, maskedUserID := masqueradeMetadata(claudeRequest.Metadata, channelID, channelHash, maxSessions, apiKey)
    claudeRequest.Metadata = masked

    // 提取 session ID
    originalSession := extractSessionFromUserID(originalUserID)
    maskedSession := extractSessionFromUserID(maskedUserID)

    // 记录追踪数据
    if info != nil && info.Channel != nil {
        originalBody, _ := json.Marshal(textRequest)
        maskedBody, _ := json.Marshal(claudeRequest)

        originalHeaders, _ := c.Get("masquerade_original_headers")
        maskedHeaders, _ := c.Get("masquerade_masked_headers")

        record := &MasqueradeTraceRecord{
            Model:           textRequest.Model,
            ChannelID:       info.Channel.Id,
            ChannelName:     info.Channel.Name,
            OriginalHeaders: originalHeaders.(map[string]string),
            OriginalBody:    string(originalBody),
            MaskedHeaders:   maskedHeaders.(map[string]string),
            MaskedBody:      string(maskedBody),
            OriginalUserID:  originalUserID,
            MaskedUserID:    maskedUserID,
            OriginalSession: originalSession,
            MaskedSession:   maskedSession,
        }
        GetMasqueradeTraceStore().Add(record)
    }

    logger.LogInfo(c, fmt.Sprintf("[OpenAI->Claude] metadata.user_id 伪装: 下游=%s -> 上游=%s", originalUserID, maskedUserID))

    return &claudeRequest, nil
}

// 辅助函数：从 user_id 中提取 session UUID
func extractSessionFromUserID(userID string) string {
    if userID == "" || userID == "<empty>" {
        return ""
    }
    if idx := strings.Index(userID, "session_"); idx != -1 {
        return userID[idx+8:]
    }
    return ""
}
```

**Step 4: 运行测试验证**

Run: `go build ./...`
Expected: BUILD SUCCESS

**Step 5: Commit**

```bash
git add relay/channel/claude/adaptor.go relay/channel/claude/relay-claude.go relay/channel/claude/masquerade_trace.go
git commit -m "feat(claude): add masquerade trace data collection"
```

---

## Task 2: 创建 API 控制器

**Files:**
- Create: `controller/masquerade_trace.go`
- Modify: `router/api-router.go`

**Step 1: 创建控制器文件**

```go
// controller/masquerade_trace.go
package controller

import (
    "net/http"

    "github.com/QuantumNous/new-api/relay/channel/claude"
    "github.com/gin-gonic/gin"
)

// GetMasqueradeTraces 获取伪装追踪记录
func GetMasqueradeTraces(c *gin.Context) {
    records := claude.GetMasqueradeTraceStore().GetAll()
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "",
        "data":    records,
    })
}

// ClearMasqueradeTraces 清空伪装追踪记录
func ClearMasqueradeTraces(c *gin.Context) {
    claude.GetMasqueradeTraceStore().Clear()
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "追踪记录已清空",
    })
}
```

**Step 2: 运行测试验证控制器编译**

Run: `go build ./controller/...`
Expected: BUILD SUCCESS

**Step 3: 注册 API 路由**

在 `router/api-router.go` 的 admin 路由组中添加：

```go
// router/api-router.go - 在 analyticsRoute 之后添加
// Masquerade trace routes (admin only)
masqueradeRoute := apiRouter.Group("/masquerade")
masqueradeRoute.Use(middleware.AdminAuth())
{
    masqueradeRoute.GET("/traces", controller.GetMasqueradeTraces)
    masqueradeRoute.POST("/clear", controller.ClearMasqueradeTraces)
}
```

**Step 4: 运行完整构建测试**

Run: `go build ./...`
Expected: BUILD SUCCESS

**Step 5: Commit**

```bash
git add controller/masquerade_trace.go router/api-router.go
git commit -m "feat(api): add masquerade trace API endpoints"
```

---

## Task 3: 创建前端 API 服务

**Files:**
- Create: `web/src/api/masquerade.js`

**Step 1: 创建 API 服务文件**

```javascript
// web/src/api/masquerade.js
import { API } from '../helpers';

export async function getMasqueradeTraces() {
  const res = await API.get('/api/masquerade/traces');
  return res.data;
}

export async function clearMasqueradeTraces() {
  const res = await API.post('/api/masquerade/clear');
  return res.data;
}
```

**Step 2: Commit**

```bash
git add web/src/api/masquerade.js
git commit -m "feat(web): add masquerade trace API service"
```

---

## Task 4: 创建伪装追踪详情弹出框组件

**Files:**
- Create: `web/src/components/dashboard/MasqueradeDetailModal.jsx`

**Step 1: 创建弹出框组件**

```jsx
// web/src/components/dashboard/MasqueradeDetailModal.jsx
import React, { useState } from 'react';
import { Modal, Tabs, TabPane, Typography, Button, Toast } from '@douyinfe/semi-ui';
import { IconCopy } from '@douyinfe/semi-icons';

const { Text } = Typography;

const MasqueradeDetailModal = ({ visible, record, onClose, t }) => {
  const [activeTab, setActiveTab] = useState('headers');

  if (!record) return null;

  const copyToClipboard = (text, label) => {
    navigator.clipboard.writeText(text).then(() => {
      Toast.success(`${label} 已复制`);
    });
  };

  const renderDiff = (original, masked, isJson = false) => {
    const originalKeys = Object.keys(original || {});
    const maskedKeys = Object.keys(masked || {});
    const allKeys = [...new Set([...originalKeys, ...maskedKeys])];

    return (
      <div className="grid grid-cols-2 gap-4">
        {/* 原始数据 */}
        <div className="border rounded p-3 bg-gray-50 dark:bg-gray-800">
          <div className="font-semibold mb-2 text-gray-700 dark:text-gray-300">
            原始请求
          </div>
          <pre className="text-xs overflow-auto max-h-96 whitespace-pre-wrap">
            {isJson
              ? JSON.stringify(JSON.parse(original || '{}'), null, 2)
              : allKeys.map((key) => {
                  const value = (original || {})[key];
                  const maskedValue = (masked || {})[key];
                  const isModified = value !== maskedValue && value !== undefined;
                  const isRemoved = value !== undefined && maskedValue === undefined;
                  return (
                    <div
                      key={key}
                      className={`${isModified ? 'bg-yellow-100 dark:bg-yellow-900' : ''} ${isRemoved ? 'bg-red-100 dark:bg-red-900' : ''}`}
                    >
                      <span className="text-blue-600 dark:text-blue-400">{key}</span>: {value || '(empty)'}
                    </div>
                  );
                })}
          </pre>
        </div>

        {/* 伪装后数据 */}
        <div className="border rounded p-3 bg-gray-50 dark:bg-gray-800">
          <div className="font-semibold mb-2 text-gray-700 dark:text-gray-300">
            伪装后请求
          </div>
          <pre className="text-xs overflow-auto max-h-96 whitespace-pre-wrap">
            {isJson
              ? JSON.stringify(JSON.parse(masked || '{}'), null, 2)
              : allKeys.map((key) => {
                  const originalValue = (original || {})[key];
                  const value = (masked || {})[key];
                  const isModified = originalValue !== value && originalValue !== undefined;
                  const isNew = originalValue === undefined && value !== undefined;
                  return (
                    <div
                      key={key}
                      className={`${isModified ? 'bg-yellow-100 dark:bg-yellow-900' : ''} ${isNew ? 'bg-green-100 dark:bg-green-900' : ''}`}
                    >
                      <span className="text-blue-600 dark:text-blue-400">{key}</span>: {value || '(empty)'}
                    </div>
                  );
                })}
          </pre>
        </div>
      </div>
    );
  };

  const formatTime = (timestamp) => {
    const date = new Date(timestamp * 1000);
    return date.toLocaleString('zh-CN');
  };

  return (
    <Modal
      title="伪装对比详情"
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={900}
      bodyStyle={{ maxHeight: '70vh', overflow: 'auto' }}
    >
      {/* 基本信息 */}
      <div className="mb-4 p-3 bg-gray-100 dark:bg-gray-700 rounded">
        <div className="grid grid-cols-4 gap-4 text-sm">
          <div>
            <Text type="tertiary">时间</Text>
            <div>{formatTime(record.timestamp)}</div>
          </div>
          <div>
            <Text type="tertiary">模型</Text>
            <div>{record.model}</div>
          </div>
          <div>
            <Text type="tertiary">渠道</Text>
            <div>{record.channel_name} (ID: {record.channel_id})</div>
          </div>
          <div>
            <Text type="tertiary">Session</Text>
            <div className="truncate" title={record.masked_session}>
              {record.masked_session?.substring(0, 8)}...
            </div>
          </div>
        </div>
      </div>

      {/* Tab 切换 */}
      <Tabs activeKey={activeTab} onChange={setActiveTab}>
        <TabPane tab="请求头对比" itemKey="headers">
          {renderDiff(record.original_headers, record.masked_headers)}
          <div className="mt-3 flex gap-2">
            <Button
              size="small"
              icon={<IconCopy />}
              onClick={() => copyToClipboard(JSON.stringify(record.original_headers, null, 2), '原始请求头')}
            >
              复制原始
            </Button>
            <Button
              size="small"
              icon={<IconCopy />}
              onClick={() => copyToClipboard(JSON.stringify(record.masked_headers, null, 2), '伪装请求头')}
            >
              复制伪装
            </Button>
          </div>
        </TabPane>

        <TabPane tab="请求体对比" itemKey="body">
          {renderDiff(record.original_body, record.masked_body, true)}
          <div className="mt-3 flex gap-2">
            <Button
              size="small"
              icon={<IconCopy />}
              onClick={() => copyToClipboard(record.original_body, '原始请求体')}
            >
              复制原始
            </Button>
            <Button
              size="small"
              icon={<IconCopy />}
              onClick={() => copyToClipboard(record.masked_body, '伪装请求体')}
            >
              复制伪装
            </Button>
          </div>
        </TabPane>
      </Tabs>
    </Modal>
  );
};

export default MasqueradeDetailModal;
```

**Step 2: Commit**

```bash
git add web/src/components/dashboard/MasqueradeDetailModal.jsx
git commit -m "feat(web): add masquerade detail modal component"
```

---

## Task 5: 创建伪装追踪 Tab 组件

**Files:**
- Create: `web/src/components/dashboard/MasqueradeTracePanel.jsx`

**Step 1: 创建 Tab 面板组件**

```jsx
// web/src/components/dashboard/MasqueradeTracePanel.jsx
import React, { useState, useEffect, useCallback } from 'react';
import { Table, Button, Card, Toast, Popconfirm, Tag, Empty } from '@douyinfe/semi-ui';
import { IconRefresh, IconDelete } from '@douyinfe/semi-icons';
import { getMasqueradeTraces, clearMasqueradeTraces } from '../../api/masquerade';
import MasqueradeDetailModal from './MasqueradeDetailModal';

const MasqueradeTracePanel = ({ t }) => {
  const [loading, setLoading] = useState(false);
  const [traces, setTraces] = useState([]);
  const [selectedRecord, setSelectedRecord] = useState(null);
  const [modalVisible, setModalVisible] = useState(false);

  const loadTraces = useCallback(async () => {
    setLoading(true);
    try {
      const res = await getMasqueradeTraces();
      if (res.success) {
        setTraces(res.data || []);
      } else {
        Toast.error(res.message || '加载失败');
      }
    } catch (error) {
      Toast.error('加载追踪数据失败');
    } finally {
      setLoading(false);
    }
  }, []);

  const handleClear = async () => {
    try {
      const res = await clearMasqueradeTraces();
      if (res.success) {
        Toast.success('追踪记录已清空');
        setTraces([]);
      } else {
        Toast.error(res.message || '清空失败');
      }
    } catch (error) {
      Toast.error('清空追踪数据失败');
    }
  };

  const handleViewDetail = (record) => {
    setSelectedRecord(record);
    setModalVisible(true);
  };

  useEffect(() => {
    loadTraces();
  }, [loadTraces]);

  const formatTime = (timestamp) => {
    const date = new Date(timestamp * 1000);
    return date.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
  };

  const columns = [
    {
      title: '时间',
      dataIndex: 'timestamp',
      width: 150,
      render: (timestamp) => formatTime(timestamp),
    },
    {
      title: '模型',
      dataIndex: 'model',
      width: 180,
      render: (model) => (
        <Tag color="blue" size="small">
          {model}
        </Tag>
      ),
    },
    {
      title: '渠道',
      dataIndex: 'channel_name',
      width: 120,
    },
    {
      title: '原始用户ID',
      dataIndex: 'original_user_id',
      width: 150,
      render: (id) => (
        <span className="text-xs truncate block max-w-[140px]" title={id}>
          {id === '<empty>' ? <Tag color="grey">空</Tag> : id}
        </span>
      ),
    },
    {
      title: '伪装用户ID',
      dataIndex: 'masked_user_id',
      width: 150,
      render: (id) => (
        <span className="text-xs truncate block max-w-[140px]" title={id}>
          {id?.substring(0, 20)}...
        </span>
      ),
    },
    {
      title: '操作',
      width: 80,
      render: (_, record) => (
        <Button size="small" theme="light" onClick={() => handleViewDetail(record)}>
          详情
        </Button>
      ),
    },
  ];

  return (
    <Card
      title={
        <div className="flex items-center justify-between w-full">
          <span>伪装追踪 ({traces.length}/100)</span>
          <div className="flex gap-2">
            <Button
              icon={<IconRefresh />}
              size="small"
              loading={loading}
              onClick={loadTraces}
            >
              刷新
            </Button>
            <Popconfirm
              title="确定要清空所有追踪记录吗？"
              onConfirm={handleClear}
            >
              <Button icon={<IconDelete />} size="small" type="danger">
                清空
              </Button>
            </Popconfirm>
          </div>
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      {traces.length === 0 ? (
        <Empty
          image={<Empty.Image />}
          description="暂无追踪数据"
          style={{ padding: '40px 0' }}
        />
      ) : (
        <Table
          columns={columns}
          dataSource={traces}
          rowKey="id"
          loading={loading}
          pagination={{
            pageSize: 20,
            showTotal: true,
          }}
          size="small"
        />
      )}

      <MasqueradeDetailModal
        visible={modalVisible}
        record={selectedRecord}
        onClose={() => setModalVisible(false)}
        t={t}
      />
    </Card>
  );
};

export default MasqueradeTracePanel;
```

**Step 2: Commit**

```bash
git add web/src/components/dashboard/MasqueradeTracePanel.jsx
git commit -m "feat(web): add masquerade trace panel component"
```

---

## Task 6: 集成到 Dashboard

**Files:**
- Modify: `web/src/components/dashboard/index.jsx`

**Step 1: 导入新组件并添加 Tab**

在 `index.jsx` 中添加伪装追踪 Tab（仅管理员可见）：

```jsx
// web/src/components/dashboard/index.jsx
// 在文件顶部添加导入
import MasqueradeTracePanel from './MasqueradeTracePanel';

// 在 Dashboard 组件中添加状态
const [activeMainTab, setActiveMainTab] = useState('overview');

// 在 return 语句中，包裹现有内容并添加 Tab 切换
// 在 DashboardHeader 之后添加：
{dashboardData.isAdminUser && (
  <Tabs activeKey={activeMainTab} onChange={setActiveMainTab} className="mb-4">
    <TabPane tab="数据概览" itemKey="overview" />
    <TabPane tab="伪装追踪" itemKey="masquerade" />
  </Tabs>
)}

{/* 根据 Tab 显示不同内容 */}
{activeMainTab === 'overview' ? (
  <>
    {/* 现有的 Dashboard 内容 */}
    <StatsCards ... />
    <QuickFilterBar ... />
    {/* ... 其他现有组件 ... */}
  </>
) : (
  <MasqueradeTracePanel t={dashboardData.t} />
)}
```

**Step 2: 添加必要的导入**

```jsx
// 在文件顶部添加
import { Tabs, TabPane } from '@douyinfe/semi-ui';
```

**Step 3: 运行前端构建测试**

Run: `cd web && npm run build`
Expected: BUILD SUCCESS

**Step 4: Commit**

```bash
git add web/src/components/dashboard/index.jsx
git commit -m "feat(dashboard): integrate masquerade trace tab for admin users"
```

---

## Task 7: 同步处理 Claude Native 请求的追踪

**Files:**
- Modify: `relay/channel/claude/adaptor.go:36-63`

**Step 1: 在 ConvertClaudeRequest 中添加追踪**

```go
// adaptor.go - ConvertClaudeRequest 方法
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
    // 保存原始请求体用于追踪
    originalBody, _ := json.Marshal(request)

    channelID := 0
    channelHash := ""
    maxSessions := 0
    apiKey := ""
    if info != nil {
        apiKey = info.ApiKey
        if info.Channel != nil {
            channelID = info.Channel.Id
            channelHash = info.Channel.GetOrCreateMasqueradeHash()
            if info.Channel.MaxConcurrentRequestsPerKey != nil {
                maxSessions = *info.Channel.MaxConcurrentRequestsPerKey
            }
        }
    }

    masked, originalUserID, maskedUserID := masqueradeMetadata(request.Metadata, channelID, channelHash, maxSessions, apiKey)
    request.Metadata = masked

    // 记录追踪数据
    if info != nil && info.Channel != nil {
        maskedBody, _ := json.Marshal(request)

        originalHeaders, _ := c.Get("masquerade_original_headers")
        maskedHeaders, _ := c.Get("masquerade_masked_headers")

        var origHeaders, maskHeaders map[string]string
        if oh, ok := originalHeaders.(map[string]string); ok {
            origHeaders = oh
        }
        if mh, ok := maskedHeaders.(map[string]string); ok {
            maskHeaders = mh
        }

        record := &MasqueradeTraceRecord{
            Model:           request.Model,
            ChannelID:       info.Channel.Id,
            ChannelName:     info.Channel.Name,
            OriginalHeaders: origHeaders,
            OriginalBody:    string(originalBody),
            MaskedHeaders:   maskHeaders,
            MaskedBody:      string(maskedBody),
            OriginalUserID:  originalUserID,
            MaskedUserID:    maskedUserID,
            OriginalSession: extractSessionFromUserID(originalUserID),
            MaskedSession:   extractSessionFromUserID(maskedUserID),
        }
        GetMasqueradeTraceStore().Add(record)
    }

    logger.LogInfo(c, fmt.Sprintf("[Claude Native] metadata.user_id 伪装: 下游=%s -> 上游=%s", originalUserID, maskedUserID))

    return request, nil
}
```

**Step 2: 运行测试**

Run: `go build ./...`
Expected: BUILD SUCCESS

**Step 3: Commit**

```bash
git add relay/channel/claude/adaptor.go
git commit -m "feat(claude): add trace collection for native Claude requests"
```

---

## Task 8: 最终测试与验证

**Step 1: 运行完整后端构建**

Run: `go build ./...`
Expected: BUILD SUCCESS

**Step 2: 运行前端构建**

Run: `cd web && npm run build`
Expected: BUILD SUCCESS

**Step 3: 启动服务进行手动测试**

1. 启动后端服务
2. 以管理员身份登录
3. 访问 Dashboard 页面
4. 确认看到"伪装追踪" Tab
5. 发送一个 Claude 请求
6. 刷新追踪列表，确认记录出现
7. 点击"详情"按钮，确认弹出框正常显示
8. 验证请求头和请求体对比功能

**Step 4: Final Commit**

```bash
git add -A
git commit -m "feat: complete masquerade trace analysis feature"
```

---

## 文件变更总结

| 文件 | 操作 | 说明 |
|------|------|------|
| `relay/channel/claude/masquerade_trace.go` | 已存在 | 追踪存储模块（已实现） |
| `relay/channel/claude/adaptor.go` | 修改 | 添加 Headers 采集和追踪记录 |
| `relay/channel/claude/relay-claude.go` | 修改 | 添加 OpenAI 转 Claude 请求的追踪 |
| `controller/masquerade_trace.go` | 新增 | API 控制器 |
| `router/api-router.go` | 修改 | 注册新 API 路由 |
| `web/src/api/masquerade.js` | 新增 | 前端 API 服务 |
| `web/src/components/dashboard/MasqueradeDetailModal.jsx` | 新增 | 详情弹出框组件 |
| `web/src/components/dashboard/MasqueradeTracePanel.jsx` | 新增 | 追踪面板组件 |
| `web/src/components/dashboard/index.jsx` | 修改 | 集成新 Tab |
