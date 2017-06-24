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
	"fmt"
	"strings"
	"sync"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api/v1"

	v1alpha1 "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

type Manager struct {
	registry *Server

	devices   map[string][]*pluginapi.Device
	available map[string][]*pluginapi.Device

	mutex sync.Mutex
}

func NewManager() (*Manager, error) {
	m := &Manager{
		registry:  newServer(),
		devices:   make(map[string][]*pluginapi.Device),
		available: make(map[string][]*pluginapi.Device),
	}

	m.registry.Manager = m
	if err := m.startRegistery(); err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Manager) Devices() map[string][]v1.Device {
	return ToAPI(m.devices)
}

func (m *Manager) Available() map[string][]v1.Device {
	return ToAPI(m.available)
}

func (m *Manager) Allocate(kind string, ndevices int, config *v1alpha1.ContainerConfig) ([]*pluginapi.Device, error) {

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if len(m.available[kind]) < ndevices {
		return nil, fmt.Errorf("Not enough devices of type %s available", kind)
	}

	glog.Infof("Recieved request for %d devices of kind %s", ndevices, kind)

	devs := m.available[kind][:ndevices]
	m.available[kind] = m.available[kind][ndevices:]

	endpoint := m.registry.Endpoints[kind]
	shimAllocate(endpoint, devs, config)

	return devs, nil
}

func (m *Manager) Deallocate(devs []*pluginapi.Device) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if len(devs) == 0 {
		return
	}

	kind := devs[0].Kind
	assertSameKind(devs, kind)

	// only add back devices which aren't already in
	// available
loop:
	for _, dev := range devs {
		for _, d := range m.available[kind] {
			if dev.Name == d.Name {
				continue loop
			}
		}
		m.available[kind] = append(m.available[kind], dev)
	}

	endpoint := m.registry.Endpoints[kind]
	deallocate(endpoint, devs)
}

func (m *Manager) addDevice(d *pluginapi.Device) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.devices[d.Kind] = append(m.devices[d.Kind], d)
	m.available[d.Kind] = append(m.available[d.Kind], d)
}

func (m *Manager) deleteDevices(kind string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.devices, kind)
	delete(m.available, kind)
}

func IsDevice(k v1.ResourceName) (bool, string) {
	key := string(k)
	if k != v1.ResourceNvidiaGPU && !strings.HasPrefix(key, v1.ResourceOpaqueIntPrefix) {
		return false, ""
	}
	var name string
	if k == v1.ResourceNvidiaGPU {
		name = "nvidia-gpu"
	} else {
		name = strings.TrimPrefix(key, v1.ResourceOpaqueIntPrefix)
	}

	return true, name
}
