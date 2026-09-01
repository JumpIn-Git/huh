package main

import (
	"fmt"
	"slices"

	lua "github.com/yuin/gopher-lua"
)

func (a *App) parseLua(luab []byte) error {
	L := lua.NewState()
	defer L.Close()

	appsAdded := []int{}
	depotsAdded := []int{}

	L.SetGlobal("addappid", L.NewFunction(func(l *lua.LState) int {
		id := l.CheckInt(1)
		_ = l.OptInt(2, 0)
		key := l.OptString(3, "")

		if key == "" {
			if !slices.Contains(a.Config.AdditionalApps, id) {
				a.Config.AdditionalApps = append(a.Config.AdditionalApps, id)
				appsAdded = append(appsAdded, id)
				logger.Debug("+ Lua: Added appid", "appid", id)
			}
		} else {
			if !slices.Contains(a.Config.AdditionalDepots, id) {
				a.Config.AdditionalDepots = append(a.Config.AdditionalDepots, id)
				depotsAdded = append(depotsAdded, id)
				logger.Debug("+ Lua: Added depot with key", "depotid", id, "key_prefix", key[:8])
			}
			a.Config.DecryptionKeys[id] = key
		}
		return 0
	}))

	mt := L.NewTable()
	L.SetField(mt, "__index", L.NewFunction(func(L *lua.LState) int {
		varName := L.CheckString(2)
		L.Push(L.NewFunction(func(L *lua.LState) int {
			top := L.GetTop()
			args := make([]string, 0, top)
			for i := 1; i <= top; i++ {
				args = append(args, L.Get(i).String())
			}
			logger.Debug("Lua: Ignored unknown call", "function", varName, "args", args)
			return 0
		}))
		return 1
	}))
	L.SetMetatable(L.GetGlobal("_G"), mt)

	logger.Debug("Executing Lua configuration")
	if err := L.DoString(string(luab)); err != nil {
		return fmt.Errorf("lua execution failed: %w", err)
	}

	if len(appsAdded) > 0 {
		logger.Debug("Lua added appids", "count", len(appsAdded), "appids", appsAdded)
	}
	if len(depotsAdded) > 0 {
		logger.Debug("Lua added depots with keys", "count", len(depotsAdded), "depots", depotsAdded)
	}

	return nil
}
