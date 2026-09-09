package main

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

func (a *App) parseLua(luab []byte, appid int) error {
	L := lua.NewState()
	defer L.Close()

	L.SetGlobal("addappid", L.NewFunction(func(l *lua.LState) int {
		// Usually 1 or 3 arguments are passed, 1 -> appid
		// 3 -> appid/depotid, ?, key
		// We don't need the 2nd argument,
		// and sometimes a appid can have a key which is needed to download.
		id := l.CheckInt(1)
		key := l.OptString(3, "")

		if key == "" {
			if AppendIntToSeq(a.AdditionalApps, id, a.Name) {
				logger.Debug("+ Lua: Added appid", "appid", id)
			}
		} else {
			SetMapKey(a.DecryptionKeys, id, key, a.Name)
			if id != appid {
				if AppendIntToSeq(a.AdditionalDepots, id, a.Name) {
					logger.Debug("+ Lua: Added depot with key", "depotid", id)
				}
			} else {
				logger.Debug("+ Lua: Added appid key", "appid", id)
			}
		}
		return 0
	}))
	L.SetGlobal("setManifestid", L.NewFunction(func(l *lua.LState) int {
		// Usually a 3d argument is passed which we dont need,
		// which I presume is the manifest creation timestamp.
		id := l.CheckInt(1) // Depot
		gid := l.CheckString(2)
		SetMapKey(a.ManifestIds, id, gid, a.Name)
		logger.Debugf("+ Luee Pinned depot %d to gid %s", id, gid)
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

	if err := L.DoString(string(luab)); err != nil {
		return fmt.Errorf("lua execution failed: %w", err)
	}
	return nil
}
