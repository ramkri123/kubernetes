package deviceplugin

const (
	Healty    = "Healthy"
	Unhealthy = "Unhealthy"

	Version          = "0.1"
	DevicePluginPath = "/var/run/kubernetes/"
	KubeletSocket    = DevicePluginPath + "kubelet.sock"
)
