package deviceplugin

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

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

type Registery struct {
	socketname string
	socketdir  string

	Endpoints map[string]*Endpoint // Key is Kind
	Manager   *Manager
	server    *grpc.Server
}

func newRegistery(socketPath string) (*Registery, error) {
	if socketPath == "" || !filepath.IsAbs(socketPath) {
		return nil, fmt.Errorf("Bad socketPath, must be an absolute path")
	}

	dir, file := filepath.Split(socketPath)
	return &Registery{
		Endpoints:  make(map[string]*Endpoint),
		socketname: file,
		socketdir:  dir,
	}, nil
}

func (m *Manager) startRegistery() error {
	socketPath := filepath.Join(m.registry.socketdir, m.registry.socketname)

	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		glog.Errorf("Failed to listen to socket while starting "+
			"device plugin registery", err)
		return err
	}

	s, err := net.Listen("unix", socketPath)
	if err != nil {
		glog.Errorf("Failed to listen to socket while starting "+
			"device plugin registery", err)
		return err
	}

	m.registry.server = grpc.NewServer([]grpc.ServerOption{}...)

	pluginapi.RegisterPluginRegistrationServer(m.registry.server, m.registry)
	go m.registry.server.Serve(s)

	return nil
}

func (s *Registery) Register(ctx context.Context,
	r *pluginapi.RegisterRequest) (*pluginapi.RegisterResponse, error) {

	response := &pluginapi.RegisterResponse{
		Version: pluginapi.Version,
	}

	if r.Version != pluginapi.Version {
		response.Error = NewError("Unsupported version")
		return response, nil
	}

	r.Vendor = strings.TrimSpace(r.Vendor)
	if err := IsVendorValid(r.Vendor); err != nil {
		response.Error = NewError(err.Error())
		return response, nil
	}

	if e, ok := s.Endpoints[r.Vendor]; ok {
		if e.socketname != r.Unixsocket {
			response.Error = NewError("A device plugin is already in charge of " +
				"this vendor on socket " + e.socketname)
			return response, nil
		}

		s.Manager.deleteDevices(r.Vendor)
	}

	s.InitiateCommunication(r, response)

	return response, nil
}

func (s *Registery) Heartbeat(ctx context.Context,
	r *pluginapi.HeartbeatRequest) (*pluginapi.HeartbeatResponse, error) {

	glog.Infof("Recieved connection from device plugin %+v", r)

	r.Vendor = strings.TrimSpace(r.Vendor)
	if err := IsVendorValid(r.Vendor); err != nil {
		return &pluginapi.HeartbeatResponse{
			Response: pluginapi.HeartbeatError,
			Error:    NewError(err.Error()),
		}, nil
	}

	if _, ok := s.Endpoints[r.Vendor]; ok {
		return &pluginapi.HeartbeatResponse{
			Response: pluginapi.HeartbeatOk,
		}, nil
	}

	return &pluginapi.HeartbeatResponse{
		Response: pluginapi.HeartbeatKo,
	}, nil
}
