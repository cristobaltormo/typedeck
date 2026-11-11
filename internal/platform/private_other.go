//go:build !windows

package platform

func PrivateDir(configDir string) string { return configDir }

func MigratePrivate(configDir, name string) {}
