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

package cm

import (
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/kubernetes/pkg/kubelet/device-plugin"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1helper "k8s.io/kubernetes/pkg/api/v1/helper"
	v1alpha1 "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

const (
	deviceKind = "device"
	waitToKill = 1
)

var (
	nDevs = 3
)

func TestHandlerDevices(t *testing.T) {
	hdlr, plugin, err := setup()
	require.NoError(t, err)
	defer teardown(hdlr, plugin)

	devs, ok := hdlr.Devices()[deviceKind]
	assert.True(t, ok)
	assert.Len(t, devs, nDevs)

	devs, ok = hdlr.AvailableDevices()[deviceKind]
	assert.True(t, ok)
	assert.Len(t, devs, nDevs)
}

func TestHandlerRM(t *testing.T) {
	hdlr, plugin, err := setup()
	require.NoError(t, err)
	defer teardown(hdlr, plugin)

	opaqueResName := v1helper.OpaqueIntResourceName(deviceKind)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "pause",
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							opaqueResName: resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	cfg := v1alpha1.ContainerConfig{}
	err = hdlr.AllocateDevices(pod, &pod.Spec.Containers[0], &cfg)

	assert.NoError(t, err)

	assert.Len(t, cfg.Envs, 1)
	assert.Len(t, cfg.Mounts, 1)

	hdlr.DeallocateDevices(pod, pod.Spec.Containers[0].Name)
	// Wait for Deallocate to happen
	time.Sleep(time.Millisecond * 250)

	assert.Len(t, hdlr.AvailableDevices()[deviceKind], nDevs)
}

// Simulate the state Kubelet would be after a crash
// The scenario here is:
// device plugin fooVendor advertised 5 devices before crash
// Device 1 is in an unhealthy state
// Device 0, 3, 4 are unused
// Device 2 is in use
// When reconnecting the Device plugin advertises 4 devices
// instead of 5
func TestHandlerRebuildState(t *testing.T) {
	vndr := "fooVendor"

	devices := []v1.Device{
		newDev(vndr, deviceKind, "0"),
		newDev(vndr, deviceKind, "1"),
		newDev(vndr, deviceKind, "2"),
		newDev(vndr, deviceKind, "3"),
		newDev(vndr, deviceKind, "4"),
	}

	devices[1].Health = v1.DeviceUnhealthy

	available := []v1.Device{devices[3], devices[0], devices[4]}
	pods := []*v1.Pod{&v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
		Spec: v1.PodSpec{Containers: []v1.Container{{
			Name: "pause",
		}}},
		Status: v1.PodStatus{ContainerStatuses: []v1.ContainerStatus{{
			Name:    "pause",
			Devices: []v1.Device{devices[2]},
		}}},
	}}

	hdlr, err := NewDevicePluginHandler(devices, available, pods,
		func(*v1.Pod, v1.PodStatus, int64) {})
	assert.NoError(t, err)

	plugin, err := deviceplugin.StartMockDevicePluginServer(vndr, deviceKind, len(devices)-1,
		time.Millisecond*500)
	assert.NoError(t, err)

	err = deviceplugin.DialRegistery(plugin)
	assert.NoError(t, err)

	plugin.Stop()
	hdlr.Stop()
	glog.Flush()
}

func setup() (*DevicePluginHandler, *deviceplugin.MockDevicePlugin, error) {
	hdlr, err := NewDevicePluginHandler(nil, nil, nil, func(*v1.Pod, v1.PodStatus, int64) {})
	if err != nil {
		return nil, nil, err
	}

	plugin, err := deviceplugin.StartMockDevicePluginServer("fooVendor", deviceKind, nDevs,
		time.Millisecond*500)
	if err != nil {
		return nil, nil, err
	}

	err = deviceplugin.DialRegistery(plugin)
	if err != nil {
		return nil, nil, err
	}

	return hdlr, plugin, nil
}

func teardown(hdlr *DevicePluginHandler, plugin *deviceplugin.MockDevicePlugin) {
	plugin.Stop()
	hdlr.Stop()
	glog.Flush()
}

func newDev(kind, vendor, name string) v1.Device {
	return v1.Device{
		Vendor: vendor,
		Kind:   kind,
		Name:   name,
	}
}
