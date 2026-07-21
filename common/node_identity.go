package common

import "os"

const (
	NodeNameSourceManual   = "manual"
	NodeNameSourceHostname = "hostname"
)

// NodeName 节点名称，优先从 NODE_NAME 环境变量读取，未配置时回退主机名。
// 用于审计日志和后台任务中标识节点身份；多实例部署时建议显式配置稳定 NODE_NAME。
var NodeName = ""

// NodeNameSource records how NodeName was chosen so future instance-management
// reporting can distinguish operator-configured names from automatic fallback.
var NodeNameSource = NodeNameSourceHostname

var NodeNameManuallyConfigured bool

func initNodeNameIdentity() {
	if envNodeName := os.Getenv("NODE_NAME"); envNodeName != "" {
		NodeName = envNodeName
		NodeNameSource = NodeNameSourceManual
		NodeNameManuallyConfigured = true
		return
	}

	hostname, _ := os.Hostname()
	NodeName = hostname
	NodeNameSource = NodeNameSourceHostname
	NodeNameManuallyConfigured = false
}
