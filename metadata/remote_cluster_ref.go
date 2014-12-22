// Copyright (c) 2013 Couchbase, Inc.
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
// except in compliance with the License. You may obtain a copy of the License at
//   http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed under the
// License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing permissions
// and limitations under the License.

package metadata

import (
	"strings"
	"github.com/couchbase/goxdcr/base"
)

const (
	// ids of remote cluster refs are used as keys in gometa service. 
	// the following prefix distinguishes the remote cluster refs from other entries
	// and reduces the chance of naming conflicts
	RemoteClusterKeyPrefix = "remoteCluster"
)

/************************************
/* struct RemoteClusterReference
*************************************/
type RemoteClusterReference struct {
	Id  string `json:"id"`
	Uuid string `json:"uuid"`
	Name string `json:"name"`
    HostName string `json:"hostName"`
	UserName string `json:"userName"`
    Password string `json:"password"`
    
    DemandEncryption  bool `json:"demandEncryption"`
    Certificate  []byte  `json:"certificate"`
}

func NewRemoteClusterReference(uuid, name, hostName, userName, password string, 
	demandEncryption  bool, certificate  []byte) *RemoteClusterReference {
	return &RemoteClusterReference{Id:  RemoteClusterRefId(uuid),
		Uuid:  uuid,
		Name:  name,
		HostName:  hostName,
		UserName:  userName,
		Password:  password,
		DemandEncryption: demandEncryption,
		Certificate:  certificate,
	}
}

func RemoteClusterRefId(remoteClusterUuid string) string {
	parts := []string{RemoteClusterKeyPrefix, remoteClusterUuid}
	return strings.Join(parts, base.KeyPartsDelimiter)
}

// implements base.ClusterConnectionInfoProvider
func (ref *RemoteClusterReference)	MyConnectionStr() string {
	return ref.HostName
}

func (ref *RemoteClusterReference)	MyUsername() string {
	return ref.UserName
}

func (ref *RemoteClusterReference)	MyPassword() string {
	return ref.Password
}

// convert to a map for output
func (ref *RemoteClusterReference) ToMap() map[string]interface{} {
	uri := base.UrlDelimiter + base.RemoteClustersPath + base.UrlDelimiter + ref.Name
	validateUri := uri + base.JustValidatePostfix
	outputMap := make(map[string]interface{})
	outputMap[base.RemoteClusterUuid] = ref.Uuid
	outputMap[base.RemoteClusterName] = ref.Name
	outputMap[base.RemoteClusterUri] = uri
	outputMap[base.RemoteClusterValidateUri] = validateUri
	outputMap[base.RemoteClusterHostName] = ref.HostName
	outputMap[base.RemoteClusterUserName] = ref.UserName
	outputMap[base.RemoteClusterDeleted] = false
	return outputMap
}