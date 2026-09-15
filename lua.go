package main

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

func (a *App) parseLua(luab []byte, appid int) error {
	L := lua.NewState()
	defer L.Close()

	L.SetGlobal("addappid", L.NewFunction(func(l *lua.LState) int {
		// Usually 1 or 3 arguments are passed, 1 -> appid OR
		// 3 -> appid/depotid, ?, key.
		// We don't need the 2nd argument,
		// and sometimes a appid can have a key which is needed to download,
		// but shouldn't be added to AdditionalDepots.
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
	L.SetGlobal("addtoken", L.NewFunction(func(l *lua.LState) int {
		// addtoken(appid, "<token>")
		app := l.CheckInt(1)
		token := l.CheckString(2)
		SetMapKey(a.AppTokens, app, token, a.Name)
		logger.Debugf("+ Lua: Added apptoken for %d", app)
		return 0
	}))
	L.SetGlobal("setManifestid", L.NewFunction(func(l *lua.LState) int {
		// Usually a 3d argument is passed which we dont need,
		// which is a old request code or maybe a manifest timestamp.
		depot := l.CheckInt(1)
		gid := l.CheckString(2)
		SetMapKey(a.ManifestIds, depot, gid, a.Name)
		logger.Debugf("+ Lua: Pinned depot %d to gid %s", depot, gid)
		return 0
	}))

	mt := L.NewTable()
	L.SetField(mt, "__index", L.NewFunction(func(l *lua.LState) int {
		// If a lua indexes a unknown function, we blindly return
		// a ghost funtion which just logs its name and arguments,
		// to prevent unnecessary crashes.
		varName := l.CheckString(2)
		l.Push(l.NewFunction(func(l *lua.LState) int {
			top := l.GetTop()
			args := make([]string, 0, top)
			for i := 1; i <= top; i++ {
				args = append(args, l.Get(i).String())
			}
			logger.Debug("- Lua: Ignored unknown call", "function", varName, "args", args)
			return 0
		}))
		return 1
	}))
	L.SetMetatable(L.GetGlobal("_G"), mt)

	if err := L.DoString(string(luab)); err != nil {
		return fmt.Errorf("Lua execution failed: %w", err)
	}
	return nil
}
