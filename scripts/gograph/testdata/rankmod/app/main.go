package main

import "example.com/rankmod/core"

func main() {
	core.Run()
	_ = core.Plat()
	g := &core.God{}
	g.Draw()
	g.FX()
	g.Menu()
	_ = core.Table()
	_ = core.Gen()
	_ = core.Deep([]int{1}, 2)
	core.Reg()
	core.Clones()
	_ = core.UseSolo()
	platMain()
}
