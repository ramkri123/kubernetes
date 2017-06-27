package cm

import (
	"k8s.io/kubernetes/pkg/api/v1"
	v1alpha1 "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

func (d *DevicePluginHandler) shimAllocate(kind string, ndevices int, c *v1alpha1.ContainerConfig) ([]*pluginapi.Device, error) {

	devs, responses, err := d.devicePluginManager.Allocate(kind, ndevices)
	if err != nil {
		return nil, err
	}

	// TODO define merge strategy or error handling
	for _, response := range responses {
		for _, env := range response.Envs {
			c.Envs = append(c.Envs, &v1alpha1.KeyValue{
				Key:   env.Key,
				Value: env.Value,
			})
		}
	}

	for _, response := range responses {
		for _, mount := range response.Mounts {
			c.Mounts = append(c.Mounts, &v1alpha1.Mount{
				ContainerPath: mount.MountPath,
				HostPath:      mount.HostPath,
				Readonly:      mount.ReadOnly,
			})
		}
	}

	return devs, nil
}

func ToAPIDevices(pluginDevices []*pluginapi.Device) []v1.Device {
	var devs []v1.Device

	for _, dev := range pluginDevices {
		devs = append(devs, v1.Device{
			Kind:       dev.Kind,
			Name:       dev.Name,
			Properties: dev.Properties,
		})
	}

	return devs
}

func ToAPI(pluginDevices map[string][]*pluginapi.Device) map[string][]v1.Device {
	devs := make(map[string][]v1.Device)

	for k, v := range pluginDevices {
		devs[k] = ToAPIDevices(v)
	}

	return devs
}
