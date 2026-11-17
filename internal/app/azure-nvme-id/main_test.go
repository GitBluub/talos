// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNVMeAdminCmdSize(t *testing.T) {
	assert.EqualValues(t, 72, unsafe.Sizeof(nvmeAdminCmd{}))
}

func TestProperties(t *testing.T) {
	for _, tc := range []struct {
		name     string
		vs       string
		model    string
		nsid     uint32
		expected string
	}{
		{name: "remote os disk", model: "MSFT NVMe Accelerator v1.0              ", nsid: 1, expected: "type=os"},
		{name: "remote data disk", model: "MSFT NVMe Accelerator v1.0", nsid: 2, expected: "type=data,lun=0"},
		{name: "remote last data disk", model: "MSFT NVMe Accelerator v1.0", nsid: 65, expected: "type=data,lun=63"},
		{name: "local disk", model: "Microsoft NVMe Direct Disk v2", nsid: 1, expected: "type=local"},
		{name: "vendor specific data wins", vs: "type=local,index=1,name=nvme-440G-1", model: "Microsoft NVMe Direct Disk v2", nsid: 1, expected: "type=local,index=1,name=nvme-440G-1"},
		{name: "unknown model", model: "Samsung SSD", nsid: 1, expected: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, properties(tc.vs, tc.model, tc.nsid))
		})
	}
}

func TestPrintProperties(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, printProperties(&out, "type=data,lun=3"))
	assert.Equal(t, "AZURE_DISK_TYPE=data\nAZURE_DISK_LUN=3\n", out.String())

	require.Error(t, printProperties(&out, "type"))
}
