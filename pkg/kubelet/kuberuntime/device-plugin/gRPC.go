package deviceplugin

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

func allocate(e *Endpoint, devs []*pluginapi.Device) (*pluginapi.AllocateResponse, error) {
	return e.client.Allocate(context.Background(), &pluginapi.AllocateRequest{
		Devices: devs,
	})
}

func deallocate(e *Endpoint, devs []*pluginapi.Device) {
	e.client.Deallocate(context.Background(), &pluginapi.DeallocateRequest{
		Devices: devs,
	})
}

func (s *Server) InitiateCommunication(r *pluginapi.RegisterRequest) {
	connection, client, err := dial(r.Unixsocket)
	if err != nil {
		glog.Errorf("%+v", err)
		return
	}

	if err := start(client); err != nil {
		glog.Errorf("%+v", err)
		return
	}

	devs, err := listDevs(client)
	if err != nil {
		glog.Errorf("%+v", err)
		return
	}

	if err := assertSameKind(devs, r.Kind); err != nil {
		glog.Errorf("%+v", err)
		return
	}

	for _, dev := range devs {
		glog.Infof("Recv dev %+v", dev)

		s.Manager.addDevice(dev)
		s.Endpoints[r.Kind] = &Endpoint{
			c:          connection,
			client:     client,
			socketname: r.Unixsocket,
		}
	}

	// TODO call monitor
}

func assertSameKind(devs []*pluginapi.Device, kind string) error {
	for _, dev := range devs {
		if dev.Kind != kind {
			return fmt.Errorf("All devices must have the kind %s", kind)
		}
	}

	return nil
}

func listDevs(client pluginapi.DeviceManagerClient) ([]*pluginapi.Device, error) {
	var devs []*pluginapi.Device

	stream, err := client.Discover(context.Background(), &pluginapi.Empty{})
	if err != nil {
		return nil, fmt.Errorf("Failed to discover devices: %v", err)
	}

	for {
		dev, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("Failed to Recv while processing device"+
				"plugin client with err %+v", err)
		}

		devs = append(devs, dev)
	}

	return devs, nil
}

func start(client pluginapi.DeviceManagerClient) error {
	_, err := client.Start(context.Background(), &pluginapi.Empty{})
	if err != nil {
		return fmt.Errorf("fail to start communication with device plugin: %v", err)
	}

	return nil
}

func dial(unixSocket string) (*grpc.ClientConn, pluginapi.DeviceManagerClient, error) {

	c, err := grpc.Dial(pluginapi.DevicePluginPath+unixSocket, grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))

	if err != nil {
		return nil, nil, fmt.Errorf("fail to dial device plugin: %v", err)
	}

	return c, pluginapi.NewDeviceManagerClient(c), nil
}
