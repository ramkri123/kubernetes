/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deviceplugin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

const (
	deviceKind = "device"
	waitToKill = 1
)

var (
	nDevices = 3
)

// TestManagerDiscovery tests that device plugin's Discovery method
// is called when registering
func TestManagerDiscovery(t *testing.T) {
	mgr, plugin, err := setup()
	require.NoError(t, err)
	defer teardown(mgr, plugin)

	devs, ok := mgr.Devices()[deviceKind]
	assert.True(t, ok)

	assert.Len(t, devs, nDevices)
	for _, d := range devs {
		_, ok = HasDevice(d, plugin.devs)
		assert.True(t, ok)
	}
}

// TestManagerAllocation tests that device plugin's Allocation and Deallocation method
// allocates correctly the devices
// This also tests the RM of the manager
func TestManagerAllocation(t *testing.T) {
	mgr, plugin, err := setup()
	require.NoError(t, err)
	defer teardown(mgr, plugin)

	for i := 1; i < nDevices; i++ {
		devs, resp, err := mgr.Allocate("device", i)
		require.NoError(t, err)

		assert.Len(t, devs, i)
		assert.Len(t, resp[0].Envs, 1)
		assert.Len(t, resp[0].Mounts, 1)

		assert.Len(t, mgr.Available()["device"], nDevices-i)

		// Deallocation test
		mgr.Deallocate(devs)
		time.Sleep(time.Millisecond * 500)
		assert.Len(t, mgr.Available()["device"], nDevices)
	}
}

// TestManagerAllocation tests that device plugin's Allocation and Deallocation method
func TestManagerMonitoring(t *testing.T) {
	mgr, plugin, err := setup()
	require.NoError(t, err)
	defer teardown(mgr, plugin)

	devs, _, err := mgr.Allocate("device", 1)
	require.NoError(t, err)

	// Monitoring test
	time.Sleep(waitToKill*time.Second + 500*time.Millisecond)
	unhealthyDev := devs[0]

	devs = mgr.Devices()[deviceKind]
	i, ok := HasDevice(unhealthyDev, devs)

	assert.True(t, ok)
	assert.Equal(t, pluginapi.Unhealthy, devs[i].Health)
}

func setup() (*Manager, *MockDevicePlugin, error) {
	mgr, err := NewManager(nil, nil, monitorCallback)
	if err != nil {
		return nil, nil, err
	}

	plugin, err := StartMockDevicePluginServer("fooVendor", deviceKind, nDevices,
		time.Millisecond*500)
	if err != nil {
		return nil, nil, err
	}

	err = DialRegistery(plugin)
	if err != nil {
		return nil, nil, err
	}

	return mgr, plugin, nil
}

func teardown(mgr *Manager, plugin *MockDevicePlugin) {
	plugin.Stop()
	mgr.Stop()
}

func monitorCallback(d *pluginapi.Device) {
}
