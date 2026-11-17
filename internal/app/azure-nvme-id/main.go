// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package main implements the udev mode of azure-nvme-id from https://github.com/Azure/azure-vm-utils:
// it prints the AZURE_DISK_* properties of an Azure NVMe namespace for udev IMPORT{program}.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	nvmeIoctlID       = 0x4e40     // _IO('N', 0x40)
	nvmeIoctlAdminCmd = 0xc0484e41 // _IOWR('N', 0x41, struct nvme_admin_cmd)

	nvmeAdminIdentify = 0x06

	identifyNamespaceSize     = 4096
	identifyNamespaceVSOffset = 384

	modelRemote   = "MSFT NVMe Accelerator v1.0"
	modelDirect   = "Microsoft NVMe Direct Disk"
	modelDirectV2 = "Microsoft NVMe Direct Disk v2"
)

// nvmeAdminCmd is struct nvme_admin_cmd from linux/nvme_ioctl.h.
type nvmeAdminCmd struct {
	opcode      uint8
	flags       uint8
	rsvd1       uint16
	nsid        uint32
	cdw2        uint32
	cdw3        uint32
	metadata    uint64
	addr        uint64
	metadataLen uint32
	dataLen     uint32
	cdw10       uint32
	cdw11       uint32
	cdw12       uint32
	cdw13       uint32
	cdw14       uint32
	cdw15       uint32
	timeoutMs   uint32
	result      uint32
}

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--udev" {
		fmt.Fprintf(os.Stderr, "usage: %s --udev\n", os.Args[0])
		os.Exit(2)
	}

	if err := run(os.Stdout, os.Getenv("DEVNAME"), os.Getenv("ID_MODEL")); err != nil {
		fmt.Fprintf(os.Stderr, "azure-nvme-id: %s\n", err)
		os.Exit(1)
	}
}

func run(w io.Writer, devName, model string) error {
	if devName == "" {
		return fmt.Errorf("environment variable DEVNAME not set")
	}

	nsid, vs, err := identifyNamespace(devName)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "AZURE_DISK_VS=%s\n", vs)

	return printProperties(w, properties(vs, model, nsid))
}

// properties falls back to the namespace ID when the host leaves the vendor specific data empty,
// as remote disks do: https://github.com/Azure/azure-vm-utils/blob/main/src/identify_disks.c
func properties(vs, model string, nsid uint32) string {
	if vs != "" {
		return vs
	}

	switch strings.TrimSpace(model) {
	case modelRemote:
		if nsid == 1 {
			return "type=os"
		}

		return "type=data,lun=" + strconv.FormatUint(uint64(nsid-2), 10)
	case modelDirect, modelDirectV2:
		return "type=local"
	default:
		return ""
	}
}

func printProperties(w io.Writer, props string) error {
	if props == "" {
		return nil
	}

	for pair := range strings.SplitSeq(props, ",") {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" || value == "" {
			return fmt.Errorf("failed to parse key-value pair %q", pair)
		}

		fmt.Fprintf(w, "AZURE_DISK_%s=%s\n", strings.ToUpper(key), value)
	}

	return nil
}

func identifyNamespace(devName string) (uint32, string, error) {
	f, err := os.Open(devName)
	if err != nil {
		return 0, "", err
	}

	defer f.Close() //nolint:errcheck

	nsid, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), nvmeIoctlID, 0)
	if errno != 0 {
		return 0, "", fmt.Errorf("NVME_IOCTL_ID on %s: %w", devName, errno)
	}

	data := make([]byte, identifyNamespaceSize)

	cmd := nvmeAdminCmd{
		opcode:  nvmeAdminIdentify,
		nsid:    uint32(nsid),
		addr:    uint64(uintptr(unsafe.Pointer(&data[0]))),
		dataLen: identifyNamespaceSize,
	}

	status, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), nvmeIoctlAdminCmd, uintptr(unsafe.Pointer(&cmd)))

	runtime.KeepAlive(data)

	if errno != 0 {
		return 0, "", fmt.Errorf("NVMe identify namespace on %s: %w", devName, errno)
	}

	if status != 0 {
		return 0, "", fmt.Errorf("NVMe identify namespace on %s: status %#x", devName, status)
	}

	vs := data[identifyNamespaceVSOffset:]
	if i := bytes.IndexByte(vs, 0); i >= 0 {
		vs = vs[:i]
	}

	return uint32(nsid), string(vs), nil
}
