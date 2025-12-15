package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)

	// Get the group tree to filter out child groups
	groupTree := ratio_setting.GetGroupTreeCopy()

	for groupName := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			// Skip child groups - only show parent and independent groups
			parentGroup := ratio_setting.GetParentGroup(groupName)
			if parentGroup != "" {
				// This is a child group, skip it
				continue
			}

			groupInfo := map[string]interface{}{
				"ratio": service.GetUserGroupRatio(userGroup, groupName),
				"desc":  desc,
			}

			// If this is a parent group, include children info
			if children, isParent := groupTree[groupName]; isParent && len(children) > 0 {
				groupInfo["is_parent"] = true
				groupInfo["children"] = children
			}

			usableGroups[groupName] = groupInfo
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]interface{}{
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}

// GetGroupTree returns the hierarchical group configuration
func GetGroupTree(c *gin.Context) {
	tree := ratio_setting.GetGroupTreeCopy()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    tree,
	})
}

// GetSelectableGroups returns groups that users can select when creating tokens
// These are parent groups and independent groups (not child groups)
func GetSelectableGroups(c *gin.Context) {
	// Get all groups from GroupRatio
	allGroups := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		allGroups = append(allGroups, groupName)
	}

	// Filter to get only selectable groups (parents + independent)
	selectableGroups := ratio_setting.GetUserSelectableGroups(allGroups)

	// Build response with group info
	groupTree := ratio_setting.GetGroupTreeCopy()
	result := make([]map[string]interface{}, 0)
	for _, group := range selectableGroups {
		info := map[string]interface{}{
			"name": group,
		}
		// If this is a parent group, include children info
		if children, ok := groupTree[group]; ok && len(children) > 0 {
			info["is_parent"] = true
			info["children"] = children
		}
		result = append(result, info)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

// GetAllGroupsWithTree returns all groups with hierarchy info for admin
// This includes parent, child, and independent groups
func GetAllGroupsWithTree(c *gin.Context) {
	// Get all groups from GroupRatio
	allGroups := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		allGroups = append(allGroups, groupName)
	}

	groupTree := ratio_setting.GetGroupTreeCopy()
	result := make([]map[string]interface{}, 0)

	for _, group := range allGroups {
		info := map[string]interface{}{
			"name": group,
		}

		// Check if it's a parent
		if children, ok := groupTree[group]; ok && len(children) > 0 {
			info["type"] = "parent"
			info["children"] = children
		} else if parent := ratio_setting.GetParentGroup(group); parent != "" {
			info["type"] = "child"
			info["parent"] = parent
		} else {
			info["type"] = "independent"
		}

		result = append(result, info)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}
