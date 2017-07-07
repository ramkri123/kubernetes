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
	"net"
	"os"
	"strconv"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

const (
	deviceSock = "device.sock"
	serverSock = pluginapi.DevicePluginPath + deviceSock
)

var ()

type MockDevicePlugin struct {
	ndevs  int
	vendor string
	kind   string

	waitToKill time.Duration

	devs   []*pluginapi.Device
	server *grpc.Server

	deviceErrorChan chan *pluginapi.Device
}

func (m *MockDevicePlugin) Init(ctx context.Context,
	e *pluginapi.Empty) (*pluginapi.Empty, error) {

	for i := 0; i < m.ndevs; i++ {
		m.devs = append(m.devs, NewDevice(strconv.Itoa(i), m.kind, m.vendor))
	}

	return nil, nil
}

func (m *MockDevicePlugin) Discover(e *pluginapi.Empty,
	deviceStream pluginapi.DeviceManager_DiscoverServer) error {

	for _, dev := range m.devs {
		deviceStream.Send(dev)
	}

	return nil
}

func (m *MockDevicePlugin) Monitor(e *pluginapi.Empty,
	deviceStream pluginapi.DeviceManager_MonitorServer) error {

	for {
		d := <-m.deviceErrorChan
		time.Sleep(m.waitToKill)

		h := NewDeviceHealth(d.Name, d.Kind, m.vendor, pluginapi.Unhealthy)
		err := deviceStream.Send(h)

		if err != nil {
			fmt.Println("Error while monitoring: %+v", err)
		}
	}
}

func (m *MockDevicePlugin) Allocate(ctx context.Context,
	r *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {

	var response pluginapi.AllocateResponse

	response.Envs = append(response.Envs, &pluginapi.KeyValue{
		Key:   "TEST_ENV_VAR",
		Value: "FOO",
	})

	response.Mounts = append(response.Mounts, &pluginapi.Mount{
		Name:      "mount-abc",
		HostPath:  "/tmp",
		MountPath: "/device-plugin",
		ReadOnly:  false,
	})

	m.deviceErrorChan <- r.Devices[0]

	return &response, nil
}

func (m *MockDevicePlugin) Deallocate(ctx context.Context,
	r *pluginapi.DeallocateRequest) (*pluginapi.Error, error) {

	return &pluginapi.Error{}, nil
}

func (m *MockDevicePlugin) Stop() {
	m.server.Stop()
}

func StartMockDevicePluginServer(vendor, kind string, ndevs int,
	waitToKill time.Duration) (*MockDevicePlugin, error) {

	os.Remove(serverSock)

	sock, err := net.Listen("unix", serverSock)
	if err != nil {
		return nil, err
	}

	plugin := &MockDevicePlugin{
		vendor:     vendor,
		kind:       kind,
		ndevs:      ndevs,
		waitToKill: waitToKill,

		deviceErrorChan: make(chan *pluginapi.Device),
	}

	plugin.server = grpc.NewServer([]grpc.ServerOption{}...)

	pluginapi.RegisterDeviceManagerServer(plugin.server, plugin)
	go plugin.server.Serve(sock)

	return plugin, nil
}

func DialRegistery(d *MockDevicePlugin) error {
	c, err := grpc.Dial(pluginapi.KubeletSocket, grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return err
	}

	client := pluginapi.NewPluginRegistrationClient(c)
	resp, err := client.Register(context.Background(), &pluginapi.RegisterRequest{
		Version:    pluginapi.Version,
		Unixsocket: deviceSock,
		Vendor:     d.vendor,
	})

	if err != nil {
		return err
	}

	if resp.Error != nil && resp.Error.Error {
		return fmt.Errorf("%s", resp.Error.Reason)
	}

	c.Close()

	return nil
}
