//go:build windows

package function

import (
	"fmt"
	"os"
	"reflect"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var modadvapi32 = windows.NewLazySystemDLL("advapi32.dll")

var getExplicitEntriesFromAclW = modadvapi32.NewProc("GetExplicitEntriesFromAclW")

func createEntry(sid string, permission windows.ACCESS_MASK, mode windows.ACCESS_MODE, inherit uint32) (*windows.EXPLICIT_ACCESS, error) {
	ssid, err := windows.StringToSid(sid)
	if err != nil {
		return nil, err
	}
	return &windows.EXPLICIT_ACCESS{
		AccessPermissions: permission,
		AccessMode:        mode,
		Inheritance:       inherit,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeValue: windows.TrusteeValueFromSID(ssid),
		},
	}, nil
}

func osPermModeToAccessMask(mode os.FileMode) (u, g, o windows.ACCESS_MASK) {
	um, gm, om := mode&0700>>6, mode&0070>>3, mode&0007
	// RA | XA | XA | windows DELETE bit
	u = windows.ACCESS_MASK((um >> 2) | (um & 02) | (um&02)<<1 | (um&01)<<5 | mode&0200<<9)
	g = windows.ACCESS_MASK((gm >> 2) | (gm & 02) | (gm&02)<<1 | (gm&01)<<5 | mode&0020<<12)
	o = windows.ACCESS_MASK((om >> 2) | (om & 02) | (om&02)<<1 | (om&01)<<5 | mode&0002<<15)
	return
}

func Chmod(file string, raw string) (err error) {
	var owner, group, other *windows.EXPLICIT_ACCESS
	var oldAcl *windows.ACL
	var sgid string
	// fetch old acl first
	if owner, group, other, sgid, oldAcl, err = getExistMeta(file); err != nil {
		return
	}
	// get permission from each sid
	mode, err := getWinModePerm(owner, group, other)
	if err != nil {
		return err
	} else if mode, err = fm.Parse(mode, raw); err != nil {
		return err
	}
	// mode is permission only so we need to merge it with origin access permission in owner, group and other
	pu, pg, po := osPermModeToAccessMask(mode)
	eas := make([]windows.EXPLICIT_ACCESS, 0, 3)
	allMask := windows.ACCESS_MASK(RA|WA|XA) | windows.DELETE
	if owner != nil {
		owner.AccessPermissions = (owner.AccessPermissions &^ allMask) | pu
		eas = append(eas, *owner)
	}
	if group == nil && mode&0070 > 0 {
		if group, err = createEntry(sgid, pg, windows.SET_ACCESS, 0); err != nil {
			return
		}
		eas = append(eas, *group)
	} else if group != nil {
		if pg == 0 {
			group.AccessMode = windows.REVOKE_ACCESS
		} else {
			group.AccessPermissions = (group.AccessPermissions &^ allMask) | pg
		}
		eas = append(eas, *group)
	}
	if other == nil && mode&0007 > 0 {
		if other, err = createEntry(wkother, po, windows.SET_ACCESS, 0); err != nil {
			return
		}
		eas = append(eas, *other)
	} else if other != nil {
		other.AccessPermissions = (other.AccessPermissions &^ allMask) | po
		eas = append(eas, *other)
	}
	// create new acl entries based on old acl
	var acl *windows.ACL
	acl, err = windows.ACLFromEntries(eas, oldAcl)
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		file,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
}

func chown(file string, uid, gid string) (err error) {
	var owner, group *windows.SID
	var flags windows.SECURITY_INFORMATION
	if uid != "" {
		if owner, err = windows.StringToSid(uid); err != nil {
			return err
		}
		flags |= windows.OWNER_SECURITY_INFORMATION
	}
	if gid != "" {
		if group, err = windows.StringToSid(gid); err != nil {
			return err
		}
		flags |= windows.GROUP_SECURITY_INFORMATION
	}
	if owner == nil && group == nil {
		// this should be have by optional that make sure uid and gid at one is provided
		return fmt.Errorf("user SID %s and group SID %s is not available", uid, gid)
	}
	return windows.SetNamedSecurityInfo(
		file,
		windows.SE_FILE_OBJECT,
		flags,
		owner,
		group,
		nil,
		nil,
	)
}

const (
	// see https://docs.microsoft.com/en-us/windows/win32/wmisdk/file-and-directory-access-rights-constants
	READ_FILE_DIR       = 0x1  // FILE_READ_DATA, FILE_LIST_DIRECTORY
	WRITE_FILE_DIR      = 0x2  // FILE_WRITE_DATA, FILE_ADD_FILE
	APPEND_ADD_FILE_DIR = 0x4  // FILE_APPEND_DATA, FILE_ADD_SUBDIRECTORY
	EXEVERS_FILE_DIR    = 0x20 // FILE_EXECUTE, FILE_TRAVERSE
	// more available enum below however this is not fitting with unix 3 bits styles.
	// FILE_READ_EA          = 0x8
	// FILE_WRITE_EA         = 0x10
	// FILE_DELETE_CHILD     = 0x40
	// FILE_READ_ATTRIBUTES  = 0x80
	// FILE_WRITE_ATTRIBUTES = 0x100
	// overal mapping to unix r, w, x
	RA = READ_FILE_DIR
	WA = WRITE_FILE_DIR | APPEND_ADD_FILE_DIR
	XA = EXEVERS_FILE_DIR
)

const (
	// wellknow other (everyone) SID
	wkother = "S-1-1-0"
)

func getExistMeta(name string) (owner, group, other *windows.EXPLICIT_ACCESS, sgid string, oldACL *windows.ACL, err error) {
	flag := windows.DACL_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.OWNER_SECURITY_INFORMATION
	sd, err := windows.GetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, windows.SECURITY_INFORMATION(flag))
	if err != nil {
		return nil, nil, nil, "", nil, err
	}
	oldACL, _, err = sd.DACL()
	if err != nil {
		return
	}
	var size int
	var hEntries windows.Handle
	result, _, err := syscall.Syscall(
		getExplicitEntriesFromAclW.Addr(),
		3,
		uintptr(unsafe.Pointer(oldACL)),
		uintptr(unsafe.Pointer(&size)),
		uintptr(unsafe.Pointer(&hEntries)),
	)
	if result != 0 {
		return nil, nil, nil, "", nil, err
	}
	defer windows.LocalFree(hEntries)
	var entries []windows.EXPLICIT_ACCESS
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&entries))
	sh.Data = uintptr(hEntries)
	sh.Len = size
	sh.Cap = size
	var usid, gsid *windows.SID
	if gsid, _, err = sd.Group(); err != nil {
		return nil, nil, nil, "", nil, err
	}
	if usid, _, err = sd.Owner(); err != nil {
		return nil, nil, nil, "", nil, err
	}
	suid := usid.String()
	if gsid != nil {
		sgid = gsid.String()
	}
	for _, ea := range entries {
		if ea.Trustee.TrusteeForm == windows.TRUSTEE_IS_SID {
			sid := (*windows.SID)(unsafe.Pointer(ea.Trustee.TrusteeValue))
			ssid := sid.String()
			switch {
			case ssid == wkother:
				if other, err = createEntry(ssid, ea.AccessPermissions, windows.SET_ACCESS, ea.Inheritance); err != nil {
					return
				}
			case ssid == suid:
				if owner, err = createEntry(ssid, ea.AccessPermissions, windows.SET_ACCESS, ea.Inheritance); err != nil {
					return
				}
			case ssid == sgid:
				if group, err = createEntry(ssid, ea.AccessPermissions, windows.SET_ACCESS, ea.Inheritance); err != nil {
					return
				}
			}
			// forget about the rest.
		}
	}
	return
}

func GetFDModePerm(file string) (os.FileMode, error) {
	u, g, o, _, _, err := getExistMeta(file)
	if err != nil {
		return 0, err
	}
	return getWinModePerm(u, g, o)
}

type wrapStatInfo struct {
	src     os.FileInfo
	winMode os.FileMode
}

func (wsi *wrapStatInfo) Name() string       { return wsi.src.Name() }
func (wsi *wrapStatInfo) Size() int64        { return wsi.Size() }
func (wsi *wrapStatInfo) Mode() os.FileMode  { return (wsi.Mode() &^ os.ModePerm) | wsi.winMode }
func (wsi *wrapStatInfo) ModTime() time.Time { return wsi.ModTime() }
func (wsi *wrapStatInfo) IsDir() bool        { return wsi.src.IsDir() }
func (wsi *wrapStatInfo) Sys() any           { return wsi.src.Sys() }

func GetFDStat(file string) (stat os.FileInfo, err error) {
	if stat, err = os.Stat(file); err == nil {
		var mode os.FileMode
		if mode, err = GetFDModePerm(file); err == nil {
			stat.Mode()
		}
		stat = &wrapStatInfo{src: stat, winMode: mode}
	}
	return stat, err
}

func getWinModePerm(u, g, o *windows.EXPLICIT_ACCESS) (os.FileMode, error) {
	mode := os.FileMode(0)
	for i, ea := range []*windows.EXPLICIT_ACCESS{u, g, o} {
		imode := os.FileMode(0)
		if ea != nil {
			if ea.AccessPermissions&RA == RA {
				imode |= 04
			}
			if ea.AccessPermissions&WA == WA {
				imode |= 02
			}
			if ea.AccessPermissions&XA == XA {
				imode |= 01
			}
			switch i {
			case 0:
				mode |= (imode << 6)
			case 1:
				mode |= (imode << 3)
			case 2:
				mode |= imode
			}
		}
	}
	return mode, nil
}
