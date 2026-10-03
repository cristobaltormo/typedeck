package actions

import "github.com/cristobaltormo/typedeck/internal/platform"

type SystemInfo = platform.SystemInfo

func GetSystemInfo(env Env) SystemInfo {
	info := platform.Current.Info(TypingLayout(env))
	info.OS = platform.Current.Name()
	return info
}
