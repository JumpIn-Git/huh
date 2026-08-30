package main

import (
	"fmt"
	"slices"

	lua "github.com/yuin/gopher-lua"
)

func (a *App) parseLua(luab []byte) error {
	L := lua.NewState()
	defer L.Close()
	L.SetGlobal("addappid", L.NewFunction(func(l *lua.LState) int {
		id := l.CheckInt(1)
		_ = l.OptInt(2, 0) // We don't need the 2nd argument
		key := l.OptString(3, "")

		if key == "" {
			// Not sure this is even needed due to package's, doing anyway for sanity
			if !slices.Contains(a.Config.AdditionalApps, id) {
				a.Config.AdditionalApps = append(a.Config.AdditionalApps, id)
			}
		} else {
			// This might also be redundant
			if !slices.Contains(a.Config.AdditionalDepots, id) {
				a.Config.AdditionalDepots = append(a.Config.AdditionalDepots, id)
			}
			a.Config.DecryptionKeys[id] = key
		}
		return 0
	}))
	mt := L.NewTable()
	// We return a emtpy function on unkown global so we wont error on undefined functions which we don't use i.e. setManifestID
	L.SetField(mt, "__index", L.NewFunction(func(L *lua.LState) int {
		varName := L.CheckString(2) // (self, key)
		L.Push(L.NewFunction(func(L *lua.LState) int {
			top := L.GetTop()
			args := make([]string, 0, top)
			for i := 1; i <= top; i++ {
				args = append(args, L.Get(i).String())
			}
			fmt.Printf("[CAPTURED] %s call ignored with args: %v\n", varName, args)
			return 0
		}))
		return 1
	}))
	L.SetMetatable(L.GetGlobal("_G"), mt)
	return L.DoString(string(luab))
}
