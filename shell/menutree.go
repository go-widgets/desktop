// Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
//
// SPDX-License-Identifier: BSD-3-Clause

package shell

import "github.com/go-freedesktop/menu"

// Category is one branch of the application menu: a user-visible directory name
// (plus its icon) and the launchable apps allocated to it.
type Category struct {
	Name string
	Icon string
	Apps []App
}

// MenuModel is the desktop application menu flattened into an ordered list of
// non-empty categories, ready to render as a go-widgets Menu / MenuBar. Nested
// submenus are walked depth-first, so a category appears once per menu node
// that carries apps, in menu (layout) order.
type MenuModel struct {
	Categories []Category
}

// NewMenuModel flattens a resolved menu.Tree. A nil tree (or a tree with a nil
// Root) yields an empty model. Only menu nodes that actually contain apps
// become categories; a purely structural submenu with no direct apps is
// skipped but still descended into.
func NewMenuModel(tree *menu.Tree) *MenuModel {
	m := &MenuModel{}
	if tree == nil || tree.Root == nil {
		return m
	}
	m.walk(tree.Root)
	return m
}

// walk visits a menu node and its submenus depth-first, emitting a Category for
// every node that has at least one app.
func (m *MenuModel) walk(node *menu.Menu) {
	if len(node.Apps) > 0 {
		apps := make([]App, 0, len(node.Apps))
		for _, e := range node.Apps {
			if launchable(e) {
				apps = append(apps, newApp(e))
			}
		}
		if len(apps) > 0 {
			m.Categories = append(m.Categories, Category{
				Name: categoryLabel(node),
				Icon: node.Icon,
				Apps: apps,
			})
		}
	}
	for _, sub := range node.Submenus {
		m.walk(sub)
	}
}

// categoryLabel is the display name of a menu node: its resolved directory name
// when present, otherwise its structural Name.
func categoryLabel(node *menu.Menu) string {
	if node.DirectoryName != "" {
		return node.DirectoryName
	}
	return node.Name
}

// Len is the number of non-empty categories.
func (m *MenuModel) Len() int { return len(m.Categories) }
