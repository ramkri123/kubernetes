package deviceplugin

import (
	"net"
	"os"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/device-plugin/v1alpha1"
)

type Endpoint struct {
	c          *grpc.ClientConn
	client     pluginapi.DeviceManagerClient
	socketname string
}

type Server struct {
	Endpoints map[string]*Endpoint // Key is Kind
	Manager   *Manager
}

func newServer() *Server {
	return &Server{
		Endpoints: make(map[string]*Endpoint),
	}
}

func (m *Manager) startRegistery() error {
	os.Remove(pluginapi.KubeletSocket)

	s, err := net.Listen("unix", pluginapi.KubeletSocket)
	if err != nil {
		glog.Errorf("Failed to listen to socket while starting"+
			"device pluginregistery", err)
		return err
	}

	grpcServer := grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterPluginRegistrationServer(grpcServer, m.registry)

	go grpcServer.Serve(s)

	return nil
}

func (s *Server) Register(ctx context.Context, r *pluginapi.RegisterRequest) (*pluginapi.RegisterResponse, error) {

	response := &pluginapi.RegisterResponse{
		Version: pluginapi.Version,
		Error:   "",
	}

	if r.Version != pluginapi.Version {
		response.Error = "Unsupported version"
		return response, nil
	}

	if e, ok := s.Endpoints[r.Kind]; ok {
		if e.socketname != r.Unixsocket {
			response.Error = "A device plugin is already in charge of" +
				"this Kind on socket " + e.socketname
			return response, nil
		}

		s.Manager.deleteDevices(r.Kind)
	}

	s.InitiateCommunication(r)

	return response, nil
}
