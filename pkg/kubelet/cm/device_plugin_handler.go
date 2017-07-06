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
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/device-plugin"

	kubetypes "k8s.io/apimachinery/pkg/types"
	v1alpha1 "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

type KillPodFunc func(*v1.Pod, v1.PodStatus, int64)
type UpdatePodFunc func(*v1.Pod)

type DevicePluginHandler struct {
	devicePluginManager *deviceplugin.Manager

	// pod2Dev is map[podUUId]map[Container name]
	pod2Dev map[kubetypes.UID]map[string][]*pluginapi.Device
	dev2Pod map[string]*v1.Pod

	killFunc KillPodFunc

	mutex sync.Mutex
}

func NewDevicePluginHandler(devCapacity, devAvailable []v1.Device, pods []*v1.Pod,
	k KillPodFunc) (*DevicePluginHandler, error) {

	hdlr := &DevicePluginHandler{
		pod2Dev:  make(map[kubetypes.UID]map[string][]*pluginapi.Device),
		dev2Pod:  make(map[string]*v1.Pod),
		killFunc: k,
	}

	devices := FromAPIToPluginDevices(devCapacity)
	available := FromAPIToPluginDevices(devAvailable)

	// This adds the used pods to the hdlr's internal state
	unused := hdlr.reconcile(pods, devices, available)

	mgr, err := deviceplugin.NewManager(devices, available, hdlr.monitorCallback)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize device plugin with error: %+v", err)
	}

	hdlr.devicePluginManager = mgr

	go hdlr.deallocate(unused)

	return hdlr, nil
}

func (h *DevicePluginHandler) Devices() map[string][]v1.Device {
	return FromPluginToAPI(h.devicePluginManager.Devices())
}

func (h *DevicePluginHandler) AvailableDevices() map[string][]v1.Device {
	return FromPluginToAPI(h.devicePluginManager.Available())
}

func (h *DevicePluginHandler) AllocateDevices(p *v1.Pod, ctr *v1.Container,
	config *v1alpha1.ContainerConfig) error {

	// For now copy limits into requests
	// TODO define what behavior is expected here with OIR
	for key, v := range ctr.Resources.Limits {
		if isDevice, _ := deviceplugin.DeviceName(key); !isDevice {
			continue
		}

		ctr.Resources.Requests[key] = v
	}

	for key, v := range ctr.Resources.Requests {
		isDevice, name := deviceplugin.DeviceName(key)
		if !isDevice {
			continue
		}

		err := h.allocate(p, ctr, name, int(v.Value()), config)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *DevicePluginHandler) DeallocateDevices(p *v1.Pod, ctr string) {
	devs := h.pod2Dev[p.UID][ctr]

	go func() {
		h.deallocate(devs)

		h.mutex.Lock()
		defer h.mutex.Unlock()

		for _, d := range devs {
			k := deviceplugin.DeviceKey(d)

			if _, ok := h.dev2Pod[k]; ok {
				delete(h.dev2Pod, k)
			}
		}
	}()
}

func (h *DevicePluginHandler) deallocate(devs []*pluginapi.Device) {
	m := h.devicePluginManager

	for {
		err := m.Deallocate(devs)
		if err != nil {
			glog.Infof("Request to deallocate devs %+v was stopped by: %+v", devs, err)

			time.Sleep(5 * time.Second)
			continue
		}

		return
	}
}

func (h *DevicePluginHandler) DevicesForCtr(uid kubetypes.UID, name string) []*container.Device {
	return FromPluginToContainerDevices(h.pod2Dev[uid][name])
}

func (h *DevicePluginHandler) Stop() {
	h.devicePluginManager.Stop()
}
