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
	os.MkdirAll(m.registry.socketdir, 0755)

	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		glog.Errorf("Failed to listen to socket while starting "+
			"device plugin registery: %+v", err)
		return err
	}

	s, err := net.Listen("unix", socketPath)
	if err != nil {
		glog.Errorf("Failed to listen to socket while starting "+
			"device plugin registery: %+v", err)
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
