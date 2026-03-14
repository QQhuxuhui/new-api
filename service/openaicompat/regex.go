package openaicompat

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// compiledRegexCache 使用 sync.Map 缓存已编译的正则表达式
var compiledRegexCache sync.Map

// getCompiledRegex 获取或编译正则表达式，结果缓存在 sync.Map 中
func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
	if cached, ok := compiledRegexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	compiledRegexCache.Store(pattern, compiled)
	return compiled, nil
}

// matchAnyRegex 检查字符串是否匹配任意一个正则模式
func matchAnyRegex(patterns []string, s string) bool {
	for _, pattern := range patterns {
		re, err := getCompiledRegex(pattern)
		if err != nil {
			common.SysLog(fmt.Sprintf("invalid regex pattern %q in ChatCompletionsToResponsesPolicy: %v", pattern, err))
			continue
		}
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
