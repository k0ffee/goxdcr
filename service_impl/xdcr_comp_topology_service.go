// Copyright 2013-Present Couchbase, Inc.
//
// Use of this software is governed by the Business Source License included in
// the file licenses/BSL-Couchbase.txt.  As of the Change Date specified in that
// file, in accordance with the Business Source License, use of this software
// will be governed by the Apache License, Version 2.0, included in the file
// licenses/APL2.txt.

package service_impl

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/couchbase/goxdcr/base"
	"github.com/couchbase/goxdcr/log"
	"github.com/couchbase/goxdcr/service_def"
	utilities "github.com/couchbase/goxdcr/utils"
)

var ErrorParsingHostInfo = errors.New("Could not parse current host info from the result.server returned")
var ErrorParsingServicesInfo = errors.New("Could not parse services from the result server returned.")

type XDCRTopologySvc struct {
	adminport    uint16
	xdcrRestPort uint16
	isEnterprise bool
	ipv4         base.IpFamilySupport
	ipv6         base.IpFamilySupport
	securitySvc  service_def.SecuritySvc
	logger       *log.CommonLogger
	utils        utilities.UtilsIface

	cachedNodesList      []interface{}
	cachedNodesListErr   error
	cachedNodesListTimer *time.Timer
	cachedNodesListMtx   sync.RWMutex

	cachedIsKVNode         bool
	cachedIsKVNodeInitDone bool
	cachedIsKVNodeMtx      sync.RWMutex

	cachedMemcachedAddr         string
	cachedMemcachedAddrInitDone bool
	cachedMemcachedAddrIsStrict bool
	cachedMemcachedAddrMtx      sync.RWMutex

	strictSecurityErrLogged uint32
}

func NewXDCRTopologySvc(adminport, xdcrRestPort uint16,
	isEnterprise bool, ipv4, ipv6 string, securitySvc service_def.SecuritySvc,
	logger_ctx *log.LoggerContext, utilsIn utilities.UtilsIface) (*XDCRTopologySvc, error) {
	top_svc := &XDCRTopologySvc{
		adminport:    adminport,
		xdcrRestPort: xdcrRestPort,
		isEnterprise: isEnterprise,
		securitySvc:  securitySvc,
		logger:       log.NewLogger("TopoSvc", logger_ctx),
		utils:        utilsIn,
	}
	if ipv4 == base.IpFamilyRequiredOption {
		top_svc.ipv4 = base.IpFamilyRequired
	} else if ipv4 == base.IpFamilyOptionalOption {
		top_svc.ipv4 = base.IpFamilyOptional
	} else if ipv4 == base.IpFamilyOffOption {
		top_svc.ipv4 = base.IpFamilyOff
	}
	if ipv6 == base.IpFamilyRequiredOption {
		top_svc.ipv6 = base.IpFamilyRequired
	} else if ipv6 == base.IpFamilyOptionalOption {
		top_svc.ipv6 = base.IpFamilyOptional
	} else if ipv6 == base.IpFamilyOffOption {
		top_svc.ipv6 = base.IpFamilyOff
	}
	return top_svc, nil
}

func (top_svc *XDCRTopologySvc) MyHost() (string, error) {
	return top_svc.getHostName()
}

func (top_svc *XDCRTopologySvc) MyHostAddr() (string, error) {
	return top_svc.getHostAddr()
}

func (top_svc *XDCRTopologySvc) MyMemcachedAddr() (string, error) {
	nodeInfoMap, err := top_svc.getHostInfo()
	if err != nil {
		// If getHostInfo had error because ns_server is overloaded, then we can return the last cached value that
		// had been requested, since MyMemcachedAddr() is not likely to change, unless strictness has changed
		var cachedRetVal string
		currentStrictSetting := top_svc.IsMyClusterEncryptionLevelStrict()
		top_svc.cachedMemcachedAddrMtx.RLock()
		if top_svc.cachedMemcachedAddrInitDone && top_svc.cachedMemcachedAddrIsStrict == currentStrictSetting {
			err = nil
			cachedRetVal = top_svc.cachedMemcachedAddr
		}
		top_svc.cachedMemcachedAddrMtx.RUnlock()
		return cachedRetVal, err
	}

	port, err := top_svc.getHostMemcachedPortFromHostInfo(nodeInfoMap)
	if err != nil {
		return "", err
	}

	hostAddr, err := top_svc.getHostAddrFromHostInfo(nodeInfoMap)
	if err != nil {
		return "", err
	}

	hostName := base.GetHostName(hostAddr)

	strictness := top_svc.IsMyClusterEncryptionLevelStrict()
	if strictness {
		// Since we don't do encryption between local services, we have to use loopback address
		hostName = base.LocalHostName
		if top_svc.IsIpv4Blocked() {
			hostName = base.LocalHostNameIpv6
		}
	}

	top_svc.cachedMemcachedAddrMtx.Lock()
	defer top_svc.cachedMemcachedAddrMtx.Unlock()
	top_svc.cachedMemcachedAddrInitDone = true
	top_svc.cachedMemcachedAddrIsStrict = strictness
	top_svc.cachedMemcachedAddr = base.GetHostAddr(hostName, port)
	return top_svc.cachedMemcachedAddr, nil
}

func (top_svc *XDCRTopologySvc) MyAdminPort() (uint16, error) {
	return top_svc.adminport, nil
}

func (top_svc *XDCRTopologySvc) MyKVNodes() ([]string, error) {
	// as of now each xdcr instance is responsible for only one kv node
	nodes := make([]string, 1)
	// get the actual hostname used in server list and server vb map
	memcachedAddr, err := top_svc.MyMemcachedAddr()
	if err != nil {
		return nil, err
	}
	nodes[0] = memcachedAddr
	return nodes, nil
}

func (top_svc *XDCRTopologySvc) NumberOfKVNodes() (int, error) {
	nodeList, err := top_svc.getNodeList()
	if err != nil {
		return 0, err
	}
	return len(nodeList), nil
}

func (top_svc *XDCRTopologySvc) IsMyClusterEnterprise() (bool, error) {
	return top_svc.isEnterprise, nil
}

func (top_svc *XDCRTopologySvc) IsMyClusterIpv6() bool {
	return top_svc.ipv6 == base.IpFamilyRequired
}

func (top_svc *XDCRTopologySvc) IsIpv4Blocked() bool {
	return top_svc.ipv4 == base.IpFamilyOff
}

func (top_svc *XDCRTopologySvc) IsIpv6Blocked() bool {
	return top_svc.ipv6 == base.IpFamilyOff
}

func (top_svc *XDCRTopologySvc) GetLocalHostName() string {
	if top_svc.ipv6 == base.IpFamilyRequired {
		return base.LocalHostNameIpv6
	} else {
		return base.LocalHostName
	}
}

// currently not used and not implemented
func (top_svc *XDCRTopologySvc) XDCRCompToKVNodeMap() (map[string][]string, error) {
	retmap := make(map[string][]string)
	return retmap, nil
}

func (top_svc *XDCRTopologySvc) PeerNodesAdminAddrs() ([]string, error) {
	nodesList, err := top_svc.getNodeList()
	if err != nil {
		return nil, err
	}

	isStrict := top_svc.securitySvc.IsClusterEncryptionLevelStrict()
	var hostnameList []string
	for _, nodeInfoRaw := range nodesList {
		nodeInfo := nodeInfoRaw.(map[string]interface{})

		thisNode, exists := nodeInfo[base.ThisNodeKey].(bool)
		if exists && thisNode {
			// Skip itself
			continue
		}

		// hostname may or may not contain port number
		hostName, exists := nodeInfo[base.HostNameKey].(string)
		if !exists {
			continue
		}

		hasKV, hasKVErr := base.NodeHasKVService(nodeInfo)
		if hasKVErr != nil || !hasKV {
			// Don't return the address since XDCR deals exclusively with data service nodes only as peers
			continue
		}

		adminPort := top_svc.adminport
		parsedAdminPort, portCheckErr := base.GetPortNumber(hostName)
		if portCheckErr == nil {
			adminPort = parsedAdminPort
		}

		if isStrict {
			adminPort = base.DefaultAdminPortSSL
			parsedSecureAdminPort, portErr := top_svc.utils.GetHttpsMgtPortFromNodeInfo(nodeInfo)
			if portErr == nil {
				adminPort = uint16(parsedSecureAdminPort)
			} else {
				top_svc.logSecureAdminPortErr(nodeInfo)
			}
		}

		hostNameWithoutPorts := base.GetHostName(hostName)
		hostnameList = append(hostnameList, base.GetHostAddr(hostNameWithoutPorts, adminPort))
	}

	return hostnameList, nil
}

// get information about current node from nodeService at /pools/nodes
func (top_svc *XDCRTopologySvc) getHostInfo() (map[string]interface{}, error) {
	nodeList, err := top_svc.getNodeList()
	if err != nil {
		return nil, err
	}

	for _, node := range nodeList {
		nodeInfoMap, ok := node.(map[string]interface{})
		if !ok {
			// should never get here
			return nil, ErrorParsingHostInfo
		}

		thisNode, ok := nodeInfoMap[base.ThisNodeKey]
		if ok {
			thisNodeBool, ok := thisNode.(bool)
			if !ok {
				// should never get here
				return nil, ErrorParsingHostInfo
			}
			if thisNodeBool {
				// found current node
				return nodeInfoMap, nil
			}
		}
	}

	return nil, ErrorParsingHostInfo
}

// get address of current node
func (top_svc *XDCRTopologySvc) getHostAddr() (string, error) {
	nodeInfoMap, err := top_svc.getHostInfo()
	if err != nil {
		return "", err
	}

	return top_svc.getHostAddrFromHostInfo(nodeInfoMap)
}

// get address of current node
func (top_svc *XDCRTopologySvc) getHostAddrFromHostInfo(nodeInfoMap map[string]interface{}) (string, error) {
	hostAddr, ok := nodeInfoMap[base.HostNameKey]
	if !ok {
		// should never get here
		return "", ErrorParsingHostInfo
	}
	hostAddrStr, ok := hostAddr.(string)
	if !ok {
		// should never get here
		return "", ErrorParsingHostInfo
	}

	if top_svc.securitySvc.IsClusterEncryptionLevelStrict() {
		// Need to massage this into a TLS address
		sslPort, err := top_svc.utils.GetHttpsMgtPortFromNodeInfo(nodeInfoMap)
		if err != nil {
			top_svc.logSecureAdminPortErr(nodeInfoMap)
			sslPort = int(base.DefaultAdminPortSSL)
		}
		hostNameWoPort := base.GetHostName(hostAddrStr)
		hostAddrStr = base.GetHostAddr(hostNameWoPort, uint16(sslPort))
	}
	return hostAddrStr, nil
}

func (top_svc *XDCRTopologySvc) logSecureAdminPortErr(nodeInfoMap map[string]interface{}) {
	// This may be an issue... instead of spam, log once an hour so we can at least debug CBSE
	if atomic.CompareAndSwapUint32(&top_svc.strictSecurityErrLogged, 0, 1) {
		top_svc.logger.Errorf("Unable to get https mgt port from %v - using default port %v (will be muted for an hour)",
			nodeInfoMap, base.DefaultAdminPortSSL)
		go func() {
			time.Sleep(1 * time.Hour)
			atomic.StoreUint32(&top_svc.strictSecurityErrLogged, 0)
		}()
	}
}

// get name of current node
func (top_svc *XDCRTopologySvc) getHostName() (string, error) {
	hostAddrStr, err := top_svc.getHostAddr()
	if err != nil {
		return "", nil
	}
	hostname := base.GetHostName(hostAddrStr)
	return hostname, nil
}

// get memcached port of current node
func (top_svc *XDCRTopologySvc) getHostMemcachedPortFromHostInfo(nodeInfoMap map[string]interface{}) (uint16, error) {
	ports, ok := nodeInfoMap[base.PortsKey]
	if !ok {
		// should never get here
		return 0, ErrorParsingHostInfo
	}
	portsMap, ok := ports.(map[string]interface{})
	if !ok {
		// should never get here
		return 0, ErrorParsingHostInfo
	}

	directPort, ok := portsMap[base.DirectPortKey]
	if !ok {
		// should never get here
		return 0, ErrorParsingHostInfo
	}
	directPortFloat, ok := directPort.(float64)
	if !ok {
		// should never get here
		return 0, ErrorParsingHostInfo
	}

	return uint16(directPortFloat), nil
}

// implements base.ClusterConnectionInfoProvider
func (top_svc *XDCRTopologySvc) MyConnectionStr() (string, error) {
	host := top_svc.GetLocalHostName()
	return base.GetHostAddr(host, top_svc.adminport), nil
}

func (top_svc *XDCRTopologySvc) MyCredentials() (string, string, base.HttpAuthMech, []byte, bool, []byte, []byte, error) {
	// TODO - cluster wide credentials
	return "", "", base.HttpAuthMechPlain, nil, false, nil, nil, nil
}

func (top_svc *XDCRTopologySvc) MyClusterUuid() (string, error) {
	var poolsInfo map[string]interface{}
	stopFunc := top_svc.utils.StartDiagStopwatch("MyClusterUuid()", base.DiagInternalThreshold)
	defer stopFunc()
	err, statusCode := top_svc.utils.QueryRestApi(top_svc.staticHostAddr(), base.PoolsPath, false, base.MethodGet, "", nil, 0, &poolsInfo, top_svc.logger)
	if err != nil || statusCode != 200 {
		return "", errors.New(fmt.Sprintf("Failed on calling %v, err=%v, statusCode=%v", base.PoolsPath, err, statusCode))
	}

	uuidObj, ok := poolsInfo[base.RemoteClusterUuid]
	if !ok {
		return "", errors.New("Could not get uuid of local cluster.")
	}
	uuid, ok := uuidObj.(string)
	if !ok {
		return "", errors.New(fmt.Sprintf("uuid of local cluster is of wrong type. Expected type: string; Actual type: %s", reflect.TypeOf(uuid)))
	}

	return uuid, nil
}

func (top_svc *XDCRTopologySvc) MyClusterVersion() (string, error) {
	var poolsInfo map[string]interface{}
	err, statusCode := top_svc.utils.QueryRestApi(top_svc.staticHostAddr(), base.PoolsPath, false, base.MethodGet, "", nil, 0, &poolsInfo, top_svc.logger)
	if err != nil || statusCode != 200 {
		return "", errors.New(fmt.Sprintf("Failed on calling %v, err=%v, statusCode=%v", base.PoolsPath, err, statusCode))
	}

	implVersionObj, ok := poolsInfo[base.ImplementationVersionKey]
	if !ok {
		return "", errors.New("Could not get implementation version of local cluster.")
	}
	implVersion, ok := implVersionObj.(string)
	if !ok {
		return "", errors.New(fmt.Sprintf("implementation version of local cluster is of wrong type. Expected type: string; Actual type: %s", reflect.TypeOf(implVersion)))
	}

	// implVersion is of format [version]-[buildNumber]-xxx, e.g., 2.5.1-1114-rel-enterprise
	// we need only the version portion
	implVersionParts := strings.Split(implVersion, "-")
	if len(implVersionParts) < 2 {
		return "", fmt.Errorf("implementation version of local cluster, %v, is of wrong format.", implVersion)
	}

	return implVersionParts[0], nil
}

func (top_svc *XDCRTopologySvc) IsMyClusterDeveloperPreview() bool {
	var poolsInfo map[string]interface{}
	err, statusCode := top_svc.utils.QueryRestApi(top_svc.staticHostAddr(), base.PoolsPath, false, base.MethodGet, "", nil, 0, &poolsInfo, top_svc.logger)
	if err != nil || statusCode != 200 {
		top_svc.logger.Errorf("Failed to get DeveloperPreview status. err=%v, statusCode=%v", err, statusCode)
		return false
	}
	isDP, ok := poolsInfo[base.DeveloperPreviewKey]
	if ok && isDP == true {
		return true
	} else {
		return false
	}
}

func (top_svc *XDCRTopologySvc) IsMyClusterEncryptionLevelStrict() bool {
	return top_svc.securitySvc.IsClusterEncryptionLevelStrict()
}
func (top_svc *XDCRTopologySvc) staticHostAddr() string {
	return "http://" + base.GetHostAddr(top_svc.GetLocalHostName(), top_svc.adminport)
}

func (top_svc *XDCRTopologySvc) IsKVNode() (bool, error) {
	nodeInfoMap, err := top_svc.getHostInfo()
	if err != nil {
		// If getHostInfo had error because ns_server is overloaded, then we can return the last cached value that
		// had been requested, since IsKVNode() is not likely to change often unless this node is rebalanced
		// out of a cluster - which should be ok because with peer-to-peer implementation, there will be VB master check
		// prior to replication start
		var cachedRetVal bool
		top_svc.cachedIsKVNodeMtx.RLock()
		if top_svc.cachedIsKVNodeInitDone {
			cachedRetVal = top_svc.cachedIsKVNode
			err = nil
		}
		top_svc.cachedIsKVNodeMtx.RUnlock()
		return cachedRetVal, err
	}

	services, ok := nodeInfoMap[base.ServicesKey]
	if !ok {
		// Should not be the case
		return true, fmt.Errorf("Unable to get key %v", base.ServicesKey)
	}
	serviceStrs, ok := services.([]interface{})
	if !ok {
		return false, ErrorParsingServicesInfo
	}

	top_svc.cachedIsKVNodeMtx.Lock()
	defer top_svc.cachedIsKVNodeMtx.Unlock()
	for _, serviceStr := range serviceStrs {
		svcStr, ok := serviceStr.(string)
		if !ok {
			return false, ErrorParsingServicesInfo
		}
		if svcStr == "kv" {
			top_svc.cachedIsKVNodeInitDone = true
			top_svc.cachedIsKVNode = true
			return true, nil
		}
	}
	top_svc.cachedIsKVNodeInitDone = true
	top_svc.cachedIsKVNode = false
	return false, nil
}

func (top_svc *XDCRTopologySvc) getNodeList() ([]interface{}, error) {
	var nodesInfo map[string]interface{}

	top_svc.cachedNodesListMtx.RLock()
	defer top_svc.cachedNodesListMtx.RUnlock()

	// Timer's existence determines whether or not we're in cool down period
	if top_svc.cachedNodesListTimer != nil {
		// Still within cooldown period - return cached information
		return top_svc.cachedNodesList, top_svc.cachedNodesListErr
	}

	// Upgrade lock
	top_svc.cachedNodesListMtx.RUnlock()
	top_svc.cachedNodesListMtx.Lock()
	defer func() {
		top_svc.cachedNodesListMtx.Unlock()
		top_svc.cachedNodesListMtx.RLock()
	}()

	if top_svc.cachedNodesListTimer != nil {
		// Someone sneaked one in
		return top_svc.cachedNodesList, top_svc.cachedNodesListErr
	}

	// Need to populate cache and hold it for a period of time
	stopFunc := top_svc.utils.StartDiagStopwatch("top_svc.getNodeList()", base.DiagInternalThreshold)
	defer stopFunc()
	err, statusCode := top_svc.utils.QueryRestApi(top_svc.staticHostAddr(), base.NodesPath, false, base.MethodGet, "", nil, 0, &nodesInfo, top_svc.logger)
	// Regardless of the RPC call, enforce a cooldown
	var cooldownPeriod = base.TopologySvcCoolDownPeriod
	if getNodeListHasError(err, statusCode) {
		// If ns_server experiences error with base.NodesPath, potentially means that it is overloaded
		// By default, TopologySvcErrCoolDownPeriod is longer than regular to give ns_server time to breathe
		if statusCode == http.StatusNotFound {
			// When a node first starts up and before it is a "cluster", it will return 404. Have a shorter cool down in this case
			cooldownPeriod = base.TopologySvcStatusNotFoundCoolDownPeriod
		} else {
			cooldownPeriod = base.TopologySvcErrCoolDownPeriod
		}
	}

	top_svc.cachedNodesListTimer = time.AfterFunc(cooldownPeriod, func() {
		top_svc.cachedNodesListMtx.Lock()
		defer top_svc.cachedNodesListMtx.Unlock()
		// Once cool down has occurred, remove the timer to let the next caller re-pull the latest info
		top_svc.cachedNodesListTimer = nil
		top_svc.cachedNodesList = nil
		top_svc.cachedNodesListErr = nil
	})

	if getNodeListHasError(err, statusCode) {
		top_svc.cachedNodesList = nil
		top_svc.cachedNodesListErr = errors.New(fmt.Sprintf("Failed on calling %v, err=%v, statusCode=%v", base.NodesPath, err, statusCode))
		return top_svc.cachedNodesList, top_svc.cachedNodesListErr
	}
	// get node list from the map
	nodes, ok := nodesInfo[base.NodesKey]
	if !ok {
		// should never get here
		top_svc.logger.Errorf("no nodes")
		top_svc.cachedNodesList = nil
		top_svc.cachedNodesListErr = ErrorParsingHostInfo
		return top_svc.cachedNodesList, top_svc.cachedNodesListErr
	}

	nodeList, ok := nodes.([]interface{})
	if !ok {
		// should never get here
		top_svc.cachedNodesList = nil
		top_svc.cachedNodesListErr = ErrorParsingHostInfo
		return top_svc.cachedNodesList, top_svc.cachedNodesListErr
	}

	top_svc.cachedNodesList = nodeList
	top_svc.cachedNodesListErr = nil
	return nodeList, nil
}

func getNodeListHasError(err error, statusCode int) bool {
	return err != nil || statusCode != 200
}
