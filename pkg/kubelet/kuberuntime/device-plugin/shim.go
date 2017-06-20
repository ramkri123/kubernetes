package deviceplugin

import (
	"github.com/golang/glog"

	v1alpha1 "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

func shimAllocate(e *Endpoint, devs []*pluginapi.Device, c *v1alpha1.ContainerConfig) *v1alpha1.ContainerConfig {

	response, err := allocate(e, devs)
	if err != nil {
		glog.Errorf("+v", err)
		return c
	}

	for _, env := range response.Envs {
		c.Envs = append(c.Envs, &v1alpha1.KeyValue{
			Key:   env.Key,
			Value: env.Value,
		})
	}

	for _, mount := range response.Mounts {
		c.Mounts = append(c.Mounts, &v1alpha1.Mount{
			ContainerPath:  mount.ContainerPath,
			HostPath:       mount.HostPath,
			Readonly:       mount.Readonly,
			SelinuxRelabel: mount.SelinuxRelabel,
		})
	}

	for _, dev := range response.Devices {
		c.Devices = append(c.Devices, &v1alpha1.Device{
			ContainerPath: dev.ContainerPath,
			HostPath:      dev.HostPath,
			Permissions:   dev.Permissions,
		})
	}

	return c
}
