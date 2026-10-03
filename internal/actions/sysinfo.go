package actions

import "github.com/cristobaltormo/typedeck/internal/platform"

type SystemInfo = platform.SystemInfo

func GetSystemInfo(env Env) SystemInfo { return platform.Current.Info(TypingLayout(env)) }
