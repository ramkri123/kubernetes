package deviceplugin

import (
	"strings"
	"sync"

	"k8s.io/kubernetes/pkg/api/v1"

	"github.com/golang/glog"
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

func (m *Manager) Devices() map[string][]*pluginapi.Device {
	return m.devices
}

func (m *Manager) Allocate(kind string, ndevices int, config *v1alpha1.ContainerConfig) ([]*pluginapi.Device, *v1alpha1.ContainerConfig) {

	m.mutex.Lock()
	defer m.mutex.Unlock()

	if len(m.available[kind]) < ndevices {
		return nil, config
	}

	glog.Infof("Recieved request for %d devices of kind %s", ndevices, kind)

	devs := m.available[kind][:ndevices]
	m.available[kind] = m.available[kind][ndevices:]

	endpoint := m.registry.Endpoints[kind]

	glog.Infof("Allocating on endpoint %+v", endpoint)
	config = shimAllocate(endpoint, devs, config)
	glog.Infof("Allocated")

	return devs, config
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
