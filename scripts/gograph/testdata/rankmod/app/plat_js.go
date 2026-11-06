//go:build js

package main

import (
	"example.com/rankmod/core"
	"example.com/rankmod/web"
)

func platMain() {
	web.JSThing()
	core.JSInit()
}
