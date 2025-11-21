package console_setting

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// TutorialSection represents a single tutorial section
type TutorialSection struct {
	Id      string `json:"id"`
	Title   string `json:"title"`
	Order   int    `json:"order"`
	Enabled bool   `json:"enabled"`
	Content string `json:"content"`
	Format  string `json:"format"` // "markdown" or "html"
}

// Tutorial represents the complete tutorial configuration
type Tutorial struct {
	Sections []TutorialSection `json:"sections"`
}

var tutorialIdPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidateTutorial validates the tutorial JSON structure and content
func ValidateTutorial(jsonStr string) error {
	if jsonStr == "" {
		return nil
	}

	var tutorial Tutorial
	if err := json.Unmarshal([]byte(jsonStr), &tutorial); err != nil {
		return fmt.Errorf("教程设置格式错误：%s", err.Error())
	}

	if len(tutorial.Sections) > 20 {
		return fmt.Errorf("教程章节数量不能超过 20 个")
	}

	idMap := make(map[string]bool)

	for i, section := range tutorial.Sections {
		// Validate ID
		if section.Id == "" {
			return fmt.Errorf("第%d个教程章节 ID 不能为空", i+1)
		}
		if !tutorialIdPattern.MatchString(section.Id) {
			return fmt.Errorf("第%d个教程章节 ID 只能包含小写字母、数字和连字符", i+1)
		}
		if len(section.Id) > 50 {
			return fmt.Errorf("第%d个教程章节 ID 长度不能超过50字符", i+1)
		}
		if idMap[section.Id] {
			return fmt.Errorf("教程章节 ID 重复: %s", section.Id)
		}
		idMap[section.Id] = true

		// Validate title
		if section.Title == "" {
			return fmt.Errorf("第%d个教程章节标题不能为空", i+1)
		}
		if len(section.Title) > 100 {
			return fmt.Errorf("第%d个教程章节标题长度不能超过100字符", i+1)
		}

		// Validate order
		if section.Order < 0 {
			return fmt.Errorf("第%d个教程章节顺序不能为负数", i+1)
		}

		// Validate format
		if section.Format != "markdown" && section.Format != "html" {
			return fmt.Errorf("第%d个教程内容格式必须是 'markdown' 或 'html'", i+1)
		}

		// Validate content length
		if len(section.Content) > 50000 {
			return fmt.Errorf("第%d个教程章节内容长度不能超过50000字符", i+1)
		}

		// Check for dangerous content in title
		if err := checkDangerousContent(section.Title, i+1, "教程章节标题"); err != nil {
			return err
		}
	}

	return nil
}

// GetTutorial returns the tutorial configuration
func GetTutorial() []map[string]interface{} {
	return getJSONList(GetConsoleSetting().Tutorial)
}
