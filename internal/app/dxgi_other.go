//go:build !windows

package app

func enumeratePlatformGPUs() []GPU {
	return nil
}
