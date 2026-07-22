//go:build !windows

package dwtsclaw

func discoverPlatformInstallRoots() []string {
	return nil
}
