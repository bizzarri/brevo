package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	attachFilesID = 40
	clearFilesID  = 41
)

type openFileName struct {
	size                                 uint32
	owner, instance                      uintptr
	filter, customFilter                 *uint16
	maxCustomFilter, filterIndex         uint32
	file                                 *uint16
	maxFile                              uint32
	fileTitle                            *uint16
	maxFileTitle                         uint32
	initialDir, title                    *uint16
	flags                                uint32
	fileOffset, fileExtension            uint16
	defaultExtension                     *uint16
	customData, hook, template, reserved uintptr
	reservedFlags, flagsEx               uint32
}

func pickPacketFiles(owner uintptr) ([]string, error) {
	dialog := syscall.NewLazyDLL("comdlg32.dll")
	buffer := make([]uint16, 65536)
	filter := []uint16{'A', 'l', 'l', ' ', 'f', 'i', 'l', 'e', 's', 0, '*', '.', '*', 0, 0}
	options := openFileName{
		owner: owner, filter: &filter[0], file: &buffer[0], maxFile: uint32(len(buffer)),
		title: wide("Select meeting packet files"),
		flags: 0x80000 | 0x200 | 0x1000 | 0x800 | 0x8, // Explorer, multi-select, existing files, preserve working directory.
	}
	options.size = uint32(unsafe.Sizeof(options))
	if ok, _, _ := dialog.NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&options))); ok == 0 {
		code, _, _ := dialog.NewProc("CommDlgExtendedError").Call()
		if code != 0 {
			return nil, fmt.Errorf("could not open the file picker (Windows error %#x)", code)
		}
		return nil, nil
	}
	var paths []string
	for len(buffer) > 0 && buffer[0] != 0 {
		name := syscall.UTF16ToString(buffer)
		paths = append(paths, name)
		end := 0
		for buffer[end] != 0 {
			end++
		}
		buffer = buffer[end+1:]
	}
	if len(paths) > 1 {
		folder := paths[0]
		paths = paths[1:]
		for i, name := range paths {
			paths[i] = filepath.Join(folder, name)
		}
	}
	return paths, nil
}

func (a *application) attachPacketFiles() {
	paths, err := pickPacketFiles(a.window)
	if err != nil {
		popup(a.window, "Attach files", err.Error(), 0x30)
		return
	}
	if len(paths) == 0 {
		return
	}
	files, digest, err := readPacketFiles(paths)
	if err != nil {
		popup(a.window, "Attach files", err.Error(), 0x30)
		return
	}
	a.packetFiles, a.packetDigest = files, digest
	caption(a.attachmentURL, packetNames(files))
	a.update()
}

func (a *application) clearPacketFiles() {
	a.packetFiles = nil
	a.packetDigest = [32]byte{}
	caption(a.attachmentURL, "")
	a.update()
}

func packetNames(files []packetFile) string {
	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, file.Name)
	}
	return strings.Join(names, ", ")
}
