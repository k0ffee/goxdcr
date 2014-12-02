// Copyright (c) 2013 Couchbase, Inc.
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
// except in compliance with the License. You may obtain a copy of the License at
//   http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed under the
// License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing permissions
// and limitations under the License.

package main

import (
	"flag"
	"fmt"
	"os"

	rm "github.com/couchbase/goxdcr/replication_manager"
	ms "github.com/couchbase/goxdcr/mock_services"
	s "github.com/couchbase/goxdcr/service_impl"
	utils "github.com/couchbase/goxdcr/utils"	
	base "github.com/couchbase/goxdcr/base"
)

var done = make(chan bool)

var options struct {
	sourceKVHost      string //source kv host name
	sourceKVAdminPort      int //source kv admin port
	xdcrRestPort      int // port number of XDCR rest server
	gometaRequestPort        int // gometa request port
	isEnterprise    bool  // whether couchbase is of enterprise edition
	isConvert    bool  // whether xdcr is running in conversion/upgrade mode
	// TODO remove after auth changes
	username        string //username on source cluster
	password        string //password on source cluster	
}

func argParse() {
	flag.StringVar(&options.sourceKVHost, "sourceKVHost", "127.0.0.1",
		"source KV host name")
	flag.IntVar(&options.sourceKVAdminPort, "sourceKVAdminPort", 9000,
		"admin port number for source kv")
	flag.IntVar(&options.xdcrRestPort, "xdcrRestPort", base.AdminportNumber,
		"port number of XDCR rest server")
	flag.IntVar(&options.gometaRequestPort, "gometaRequestPort", 5003,
		"port number for gometa requests")
	flag.BoolVar(&options.isEnterprise, "isEnterprise", true,
		"whether couchbase is of enterprise edition")
	flag.BoolVar(&options.isConvert, "isConvert", false,
		"whether xdcr is running in convertion/upgrade mode")
	flag.StringVar(&options.username, "username", "Administrator", "username to cluster admin console")
	flag.StringVar(&options.password, "password", "welcome", "password to Cluster admin console")
	flag.Parse()
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage : %s [OPTIONS] \n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	argParse()
	
	// TODO remove after real services are implemented
	ms.SetTestOptions(utils.GetHostAddr(options.sourceKVHost, options.sourceKVAdminPort), options.sourceKVHost, options.username, options.password)
	
	cmd, err := s.StartGometaService()
	if err != nil {
		fmt.Printf("Failed to start gometa service. err=%v\n", err)
		os.Exit(1)
	}
	defer s.KillGometaService(cmd)
	
	metadata_svc, err := s.NewMetadataSvc(utils.GetHostAddr(options.sourceKVHost, options.gometaRequestPort), nil)
	if err != nil {
		fmt.Printf("Error starting metadata service. err=%v\n", err)
		os.Exit(1)
	}
	
	if options.isConvert {
		fmt.Println("Starting replication manager in conversion/upgrade mode.")
		// start replication manager in conversion/upgrade mode
		rm.StartReplicationManagerForConversion(options.sourceKVHost, options.sourceKVAdminPort,
							   options.isEnterprise, 
							   s.NewReplicationSpecService(metadata_svc, nil),
							   s.NewRemoteClusterService(metadata_svc, nil))
	} else {
		// start replication manager in normal mode
		rm.StartReplicationManager(options.sourceKVHost, options.sourceKVAdminPort,
							   options.xdcrRestPort, options.isEnterprise,
							   s.NewReplicationSpecService(metadata_svc, nil),
							   s.NewRemoteClusterService(metadata_svc, nil),	
							   new(ms.MockClusterInfoSvc), 
							   new(ms.MockXDCRTopologySvc), 
							   new(ms.MockReplicationSettingsSvc))
							   
		// keep main alive in normal mode
		<-done
	}						 
}
