package cm

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/kubelet/device-plugin"

	kubetypes "k8s.io/apimachinery/pkg/types"
	v1alpha1 "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

type DevicePluginHandler struct {
	devicePluginManager *deviceplugin.Manager

	// pod2Dev is map[podUUId]map[Container name]
	// The devices are then grouped by Kind
	pod2Dev map[kubetypes.UID]map[string][][]*pluginapi.Device
	dev2Pod map[string]*v1.Pod

	killFunc KillPodFunc
}

func NewDevicePluginHandler() (*DevicePluginHandler, error) {
	hdlr := &DevicePluginHandler{
		pod2Dev: make(map[kubetypes.UID]map[string][][]*pluginapi.Device),
		dev2Pod: make(map[string]*v1.Pod),
	}

	devicePluginManager, err := deviceplugin.NewManager(hdlr.killPod)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize device plugin with error: %+v", err)
	}

	hdlr.devicePluginManager = devicePluginManager

	return hdlr, nil
}

func (d *DevicePluginHandler) Devices() map[string][]v1.Device {
	return ToAPI(d.devicePluginManager.Devices())
}

func (d *DevicePluginHandler) AvailableDevices() map[string][]v1.Device {
	return ToAPI(d.devicePluginManager.Available())
}

func (d *DevicePluginHandler) ApplyDevicePlugins(p *v1.Pod, ctr *v1.Container,
	config *v1alpha1.ContainerConfig) error {

	for key, v := range ctr.Resources.Requests {
		isDevice, name := deviceplugin.IsDevice(key)
		if !isDevice {
			continue
		}

		if err := d.allocate(p, ctr, name, int(v.Value()), config); err != nil {
			return err
		}
	}

	return nil
}

func (d *DevicePluginHandler) DeallocateDevicePlugins(p *v1.Pod, ctr string) {
	m := d.devicePluginManager

	for _, devs := range d.pod2Dev[p.UID][ctr] {
		m.Deallocate(devs)

		for _, dev := range devs {
			delete(d.dev2Pod, deviceplugin.DeviceKey(dev))
		}
	}
}

// This is somewhat confusing as this just enables pod killing
// TODO find a way to pass this function at NewDevicePluginHandler time
func (d *DevicePluginHandler) StartHealthCheck(f KillPodFunc) {
	d.killFunc = f
}

func (d *DevicePluginHandler) allocate(p *v1.Pod, ctr *v1.Container, name string, ndevs int, c *v1alpha1.ContainerConfig) error {
	devs, err := d.shimAllocate(name, ndevs, c)
	if err != nil {
		d.DeallocateDevicePlugins(p, ctr.Name)
		return err
	}

	if _, ok := d.pod2Dev[p.UID]; !ok {
		d.pod2Dev[p.UID] = make(map[string][][]*pluginapi.Device)
	}

	ctr2Dev := d.pod2Dev[p.UID]
	ctr2Dev[ctr.Name] = append(ctr2Dev[ctr.Name], devs)

	for _, dev := range devs {
		d.dev2Pod[deviceplugin.DeviceKey(dev)] = p
	}

	return nil
}

// TODO understand why pod isn't killed by Container Runtime during
// the grace period
func (d *DevicePluginHandler) killPod(dev *pluginapi.Device) {
	glog.Infof("Request to kill Unhealthy dev: %+v", dev)

	p, ok := d.dev2Pod[deviceplugin.DeviceKey(dev)]
	if !ok {
		glog.Infof("Device is not in use by any pod")
		return
	}

	status := v1.PodStatus{
		Phase:   v1.PodFailed,
		Message: fmt.Sprintf("device %s/%s became unhealthy", dev.Kind, dev.Name),
		Reason:  "killed",
	}

	for {
		if d.killFunc != nil {
			d.killFunc(p, status, int64(10))
			return
		}

		glog.Infof("Waiting for Kill function to be set")
		time.Sleep(5 * time.Second)
	}
}
