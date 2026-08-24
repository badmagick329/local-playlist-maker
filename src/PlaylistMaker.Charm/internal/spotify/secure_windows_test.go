//go:build windows

package spotify

import "golang.org/x/sys/windows"

func privateFile(path string) bool {
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	dacl, _, err := descriptor.DACL()
	return err == nil && dacl != nil && dacl.AceCount == 1
}
