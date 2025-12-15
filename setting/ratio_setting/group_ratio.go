package ratio_setting

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

var groupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMutex sync.RWMutex

var (
	GroupGroupRatio = map[string]map[string]float64{
		"vip": {
			"edit_this": 0.9,
		},
	}
	groupGroupRatioMutex sync.RWMutex
)

// GroupTree defines the hierarchical group structure
// key: parent group name
// value: list of child group names
var groupTree = map[string][]string{}
var groupTreeMutex sync.RWMutex

// childToParent is a reverse mapping for quick lookup
var childToParent = map[string]string{}
var childToParentMutex sync.RWMutex

var defaultGroupSpecialUsableGroup = map[string]map[string]string{
	"vip": {
		"append_1":   "vip_special_group_1",
		"-:remove_1": "vip_removed_group_1",
	},
}

type GroupRatioSetting struct {
	GroupRatio              map[string]float64                      `json:"group_ratio"`
	GroupGroupRatio         map[string]map[string]float64           `json:"group_group_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string] `json:"group_special_usable_group"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatio,
		GroupGroupRatio:         GroupGroupRatio,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	return &groupRatioSetting
}

func GetGroupRatioCopy() map[string]float64 {
	groupRatioMutex.RLock()
	defer groupRatioMutex.RUnlock()

	groupRatioCopy := make(map[string]float64)
	for k, v := range groupRatio {
		groupRatioCopy[k] = v
	}
	return groupRatioCopy
}

func ContainsGroupRatio(name string) bool {
	groupRatioMutex.RLock()
	defer groupRatioMutex.RUnlock()

	_, ok := groupRatio[name]
	return ok
}

func GroupRatio2JSONString() string {
	groupRatioMutex.RLock()
	defer groupRatioMutex.RUnlock()

	jsonBytes, err := json.Marshal(groupRatio)
	if err != nil {
		common.SysLog("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	groupRatioMutex.Lock()
	defer groupRatioMutex.Unlock()

	groupRatio = make(map[string]float64)
	return json.Unmarshal([]byte(jsonStr), &groupRatio)
}

func GetGroupRatio(name string) float64 {
	groupRatioMutex.RLock()
	defer groupRatioMutex.RUnlock()

	// 1. First try to find ratio for the exact group name
	if ratio, ok := groupRatio[name]; ok {
		return ratio
	}

	// 2. If not found, try to fallback to parent group ratio
	parent := GetParentGroup(name)
	if parent != "" {
		if ratio, ok := groupRatio[parent]; ok {
			return ratio
		}
	}

	// 3. Default value
	common.SysLog("group ratio not found: " + name)
	return 1
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	groupGroupRatioMutex.RLock()
	defer groupGroupRatioMutex.RUnlock()

	// 1. First try to find model ratio for the child group
	if gp, ok := GroupGroupRatio[userGroup]; ok {
		if ratio, ok := gp[usingGroup]; ok {
			return ratio, true
		}
	}

	// 2. If not found, try to fallback to parent group's model ratio
	parent := GetParentGroup(usingGroup)
	if parent != "" {
		if gp, ok := GroupGroupRatio[userGroup]; ok {
			if ratio, ok := gp[parent]; ok {
				return ratio, true
			}
		}
	}

	// 3. Not found - caller will use GetGroupRatio as fallback
	return -1, false
}

func GroupGroupRatio2JSONString() string {
	groupGroupRatioMutex.RLock()
	defer groupGroupRatioMutex.RUnlock()

	jsonBytes, err := json.Marshal(GroupGroupRatio)
	if err != nil {
		common.SysLog("error marshalling group-group ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	groupGroupRatioMutex.Lock()
	defer groupGroupRatioMutex.Unlock()

	GroupGroupRatio = make(map[string]map[string]float64)
	return json.Unmarshal([]byte(jsonStr), &GroupGroupRatio)
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}

// ==================== GroupTree Functions ====================

// IsParentGroup checks if the given group is a parent group in the GroupTree
func IsParentGroup(group string) bool {
	groupTreeMutex.RLock()
	defer groupTreeMutex.RUnlock()

	_, exists := groupTree[group]
	return exists
}

// GetChildGroups returns the list of child groups for a parent group
// Returns empty slice if the group is not a parent
func GetChildGroups(parentGroup string) []string {
	groupTreeMutex.RLock()
	defer groupTreeMutex.RUnlock()

	children, exists := groupTree[parentGroup]
	if !exists {
		return []string{}
	}
	// Return a copy to prevent external modification
	result := make([]string, len(children))
	copy(result, children)
	return result
}

// GetParentGroup returns the parent group for a child group
// Returns empty string if the group has no parent
func GetParentGroup(childGroup string) string {
	childToParentMutex.RLock()
	defer childToParentMutex.RUnlock()

	return childToParent[childGroup]
}

// ExpandGroup expands a group to its children if it's a parent group
// If the group is a child or independent group, returns a slice with just that group
func ExpandGroup(group string) []string {
	groupTreeMutex.RLock()
	defer groupTreeMutex.RUnlock()

	if children, exists := groupTree[group]; exists && len(children) > 0 {
		// Return a copy of children
		result := make([]string, len(children))
		copy(result, children)
		return result
	}
	// Not a parent group, return the group itself
	return []string{group}
}

// GetAllParentGroups returns all parent group names from the GroupTree
func GetAllParentGroups() []string {
	groupTreeMutex.RLock()
	defer groupTreeMutex.RUnlock()

	parents := make([]string, 0, len(groupTree))
	for parent := range groupTree {
		parents = append(parents, parent)
	}
	return parents
}

// GetAllChildGroups returns all child group names from the GroupTree
func GetAllChildGroups() []string {
	childToParentMutex.RLock()
	defer childToParentMutex.RUnlock()

	children := make([]string, 0, len(childToParent))
	for child := range childToParent {
		children = append(children, child)
	}
	return children
}

// GetGroupTreeCopy returns a copy of the entire GroupTree
func GetGroupTreeCopy() map[string][]string {
	groupTreeMutex.RLock()
	defer groupTreeMutex.RUnlock()

	copy := make(map[string][]string, len(groupTree))
	for parent, children := range groupTree {
		childrenCopy := make([]string, len(children))
		for i, child := range children {
			childrenCopy[i] = child
		}
		copy[parent] = childrenCopy
	}
	return copy
}

// GroupTree2JSONString converts GroupTree to JSON string
func GroupTree2JSONString() string {
	groupTreeMutex.RLock()
	defer groupTreeMutex.RUnlock()

	jsonBytes, err := json.Marshal(groupTree)
	if err != nil {
		common.SysLog("error marshalling group tree: " + err.Error())
		return "{}"
	}
	return string(jsonBytes)
}

// UpdateGroupTreeByJSONString updates the GroupTree from a JSON string
// Also rebuilds the childToParent reverse mapping
func UpdateGroupTreeByJSONString(jsonStr string) error {
	// Parse the new tree first
	newTree := make(map[string][]string)
	if jsonStr != "" && jsonStr != "{}" {
		err := json.Unmarshal([]byte(jsonStr), &newTree)
		if err != nil {
			return fmt.Errorf("invalid GroupTree JSON: %w", err)
		}
	}

	// Validate the tree
	if err := ValidateGroupTree(newTree); err != nil {
		return err
	}

	// Build reverse mapping
	newChildToParent := make(map[string]string)
	for parent, children := range newTree {
		for _, child := range children {
			newChildToParent[child] = parent
		}
	}

	// Update both maps atomically
	groupTreeMutex.Lock()
	groupTree = newTree
	groupTreeMutex.Unlock()

	childToParentMutex.Lock()
	childToParent = newChildToParent
	childToParentMutex.Unlock()

	common.SysLog(fmt.Sprintf("GroupTree updated: %d parents, %d children", len(newTree), len(newChildToParent)))
	return nil
}

// ValidateGroupTree checks the GroupTree for issues
func ValidateGroupTree(tree map[string][]string) error {
	// Check for duplicate children (same child in multiple parents)
	childParents := make(map[string]string)
	for parent, children := range tree {
		for _, child := range children {
			if existingParent, exists := childParents[child]; exists {
				return fmt.Errorf("child group '%s' is assigned to multiple parents: '%s' and '%s'", child, existingParent, parent)
			}
			childParents[child] = parent

			// Check if child is also a parent (no nested hierarchies)
			if _, isParent := tree[child]; isParent {
				return fmt.Errorf("nested hierarchies not supported: '%s' is both a child and a parent", child)
			}

			// Check if parent name equals child name
			if parent == child {
				return fmt.Errorf("parent and child cannot have the same name: '%s'", parent)
			}
		}
	}

	return nil
}

// CheckGroupTree validates a JSON string representation of GroupTree
func CheckGroupTree(jsonStr string) error {
	if jsonStr == "" || jsonStr == "{}" {
		return nil
	}

	tree := make(map[string][]string)
	err := json.Unmarshal([]byte(jsonStr), &tree)
	if err != nil {
		return fmt.Errorf("invalid JSON format: %w", err)
	}

	return ValidateGroupTree(tree)
}

// GetIndependentGroups returns groups that are neither parents nor children in the GroupTree
// These groups should be shown alongside parent groups for user selection
func GetIndependentGroups(allGroups []string) []string {
	groupTreeMutex.RLock()
	childToParentMutex.RLock()
	defer groupTreeMutex.RUnlock()
	defer childToParentMutex.RUnlock()

	independent := make([]string, 0)
	for _, group := range allGroups {
		_, isParent := groupTree[group]
		_, isChild := childToParent[group]
		if !isParent && !isChild {
			independent = append(independent, group)
		}
	}
	return independent
}

// GetUserSelectableGroups returns groups that users should see when creating tokens
// This includes: parent groups + independent groups (excludes child groups)
func GetUserSelectableGroups(allGroups []string) []string {
	groupTreeMutex.RLock()
	childToParentMutex.RLock()
	defer groupTreeMutex.RUnlock()
	defer childToParentMutex.RUnlock()

	selectable := make([]string, 0)
	for _, group := range allGroups {
		// Include if it's a parent or independent (not a child)
		_, isChild := childToParent[group]
		if !isChild {
			selectable = append(selectable, group)
		}
	}
	return selectable
}
