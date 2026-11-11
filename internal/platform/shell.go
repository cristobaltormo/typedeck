package platform

type ShellHooks struct {
	Status     func() string
	OpenEditor func()
	Shutdown   func()
	Release    func()
	Reclaim    func()
}
