// Copyright 2013-Present Couchbase, Inc.
//
// Use of this software is governed by the Business Source License included in
// the file licenses/BSL-Couchbase.txt.  As of the Change Date specified in that
// file, in accordance with the Business Source License, use of this software
// will be governed by the Apache License, Version 2.0, included in the file
// licenses/APL2.txt.

package service_impl

import (
	"fmt"
	mcc "github.com/couchbase/gomemcached/client"
	"github.com/couchbase/goxdcr/base"
	"github.com/couchbase/goxdcr/log"
	"github.com/couchbase/goxdcr/metadata"
	"github.com/couchbase/goxdcr/service_def"
	"github.com/couchbase/goxdcr/utils"
	"sync"
	"sync/atomic"
	"time"
)

type BucketTopologyService struct {
	remClusterSvc       service_def.RemoteClusterSvc
	xdcrCompTopologySvc service_def.XDCRCompTopologySvc
	securitySvc         service_def.SecuritySvc
	utils               utils.UtilsIface
	refreshInterval     time.Duration
	logger              *log.CommonLogger
	healthCheckInterval time.Duration

	// Key is bucket Name
	srcBucketWatchers    map[string]*BucketTopologySvcWatcher
	srcBucketWatchersCnt map[string]int
	srcBucketWatchersMtx sync.RWMutex

	// Key is targetClusterUUID+bucketName
	tgtBucketWatchers    map[string]*BucketTopologySvcWatcher
	tgtBucketWatchersCnt map[string]int
	tgtBucketWatchersMtx sync.RWMutex
}

func NewBucketTopologyService(xdcrCompTopologySvc service_def.XDCRCompTopologySvc, remClusterSvc service_def.RemoteClusterSvc, utils utils.UtilsIface, refreshInterval time.Duration, loggerContext *log.LoggerContext, replicationSpecService service_def.ReplicationSpecSvc, healthCheckInterval time.Duration, securitySvc service_def.SecuritySvc) (*BucketTopologyService, error) {
	b := &BucketTopologyService{
		remClusterSvc:        remClusterSvc,
		xdcrCompTopologySvc:  xdcrCompTopologySvc,
		logger:               log.NewLogger("BucketTopologySvc", loggerContext),
		utils:                utils,
		srcBucketWatchers:    map[string]*BucketTopologySvcWatcher{},
		srcBucketWatchersCnt: map[string]int{},
		tgtBucketWatchers:    map[string]*BucketTopologySvcWatcher{},
		tgtBucketWatchersCnt: map[string]int{},
		refreshInterval:      refreshInterval,
		healthCheckInterval:  healthCheckInterval,
		securitySvc:          securitySvc,
	}
	return b, b.loadFromReplSpecSvc(replicationSpecService)
}

func (b *BucketTopologyService) loadFromReplSpecSvc(replSpecSvc service_def.ReplicationSpecSvc) error {
	specs, err := replSpecSvc.AllReplicationSpecs()
	if err != nil {
		return err
	}

	var waitGrp sync.WaitGroup
	for _, spec := range specs {
		waitGrp.Add(2)
		specCpy := spec.Clone()
		go func() {
			defer waitGrp.Done()

			localRetryOp := func() error {
				watcher := b.getOrCreateLocalWatcher(specCpy)
				if watcher == nil {
					return base.ErrorNilPtr
				}
				localStartErr := watcher.Start()
				if localStartErr != nil && localStartErr != ErrorWatcherAlreadyStarted {
					b.logger.Errorf("Error starting local watcher for %v - %v", specCpy, localStartErr)
					return localStartErr
				}
				return nil
			}

			retryErr := b.utils.ExponentialBackoffExecutor("BucketTopologyServiceLoadSpec (local)",
				base.DefaultHttpTimeoutWaitTime, base.DefaultHttpTimeoutMaxRetry, base.DefaultHttpTimeoutRetryFactor, localRetryOp)
			if retryErr != nil {
				panic(fmt.Sprintf("Bucket Topology service bootstrapping %v did not successfully start local and XDCR must restart to try again", specCpy.Id))
			}
		}()
		go func() {
			defer waitGrp.Done()

			retryOp := func() error {
				watcher, startErr := b.getOrCreateRemoteWatcher(specCpy)
				if startErr != nil {
					b.logger.Errorf("getOrCreateRemoteWatcher has error: %v", startErr)
					return startErr
				}
				startErr = watcher.Start()
				if startErr != nil && startErr != ErrorWatcherAlreadyStarted {
					b.logger.Errorf("Error starting remote watcher for %v - %v", specCpy.Id, startErr)
					return startErr
				}
				return nil
			}
			retryErr := b.utils.ExponentialBackoffExecutor("BucketTopologyServiceLoadSpec (remote)",
				base.DefaultHttpTimeoutWaitTime, base.DefaultHttpTimeoutMaxRetry, base.DefaultHttpTimeoutRetryFactor, retryOp)
			if retryErr != nil {
				panic(fmt.Sprintf("Bucket Topology service bootstrapping %v did not finish within %v and XDCR must restart to try again", specCpy.Id, base.DefaultHttpTimeout))
			}
		}()
	}
	waitGrp.Wait()
	return nil
}

func (b *BucketTopologyService) SubscribeToLocalBucketFeed(spec *metadata.ReplicationSpecification, subscriberId string) (chan service_def.SourceNotification, error) {
	if spec == nil {
		return nil, base.ErrorNilPtr
	}

	if spec.SourceBucketName == "" {
		return nil, fmt.Errorf("Empty source bucket name for spec %v", spec.Id)
	}

	b.srcBucketWatchersMtx.Lock()
	defer b.srcBucketWatchersMtx.Unlock()
	watcher, exists := b.srcBucketWatchers[spec.SourceBucketName]
	if exists {
		retCh := watcher.registerAndGetCh(spec, subscriberId, TOPOLOGY, nil).(chan service_def.SourceNotification)
		return retCh, nil
	}

	return nil, fmt.Errorf("SubscribeToLocalBucketFeed could not find watcher for %v", spec.SourceBucketName)
}

func (b *BucketTopologyService) getOrCreateLocalWatcher(spec *metadata.ReplicationSpecification) *BucketTopologySvcWatcher {
	b.srcBucketWatchersMtx.Lock()
	defer b.srcBucketWatchersMtx.Unlock()
	watcher, exists := b.srcBucketWatchers[spec.SourceBucketName]
	if !exists {
		watcher = NewBucketTopologySvcWatcher(spec.SourceBucketName, spec.SourceBucketUUID, b.logger, true, b.xdcrCompTopologySvc)
		b.srcBucketWatchers[spec.SourceBucketName] = watcher

		intervalFuncMap := make(IntervalFuncMap)
		topologyFunc := b.getLocalBucketTopologyUpdater(spec, watcher)
		intervalFuncMap[TOPOLOGY] = make(IntervalInnerFuncMap)
		intervalFuncMap[TOPOLOGY][b.refreshInterval] = topologyFunc

		dcpStatsFunc := b.getDcpStatsUpdater(spec, watcher)
		intervalFuncMap[DCPSTATSCHECK] = make(IntervalInnerFuncMap)
		intervalFuncMap[DCPSTATSCHECK][b.healthCheckInterval] = dcpStatsFunc

		dcpStatsLegacyFunc := b.getDcpStatsLegacyUpdater(spec, watcher)
		intervalFuncMap[DCPSTATSCHECKLEGACY] = make(IntervalInnerFuncMap)
		intervalFuncMap[DCPSTATSCHECKLEGACY][b.healthCheckInterval] = dcpStatsLegacyFunc

		defaultPipelineStatsInterval := time.Duration(metadata.DefaultPipelineStatsIntervalMs) * time.Millisecond
		highSeqnoFunc := b.getHighSeqnosUpdater(spec, watcher, false)
		intervalFuncMap[HIGHSEQNOS] = make(IntervalInnerFuncMap)
		intervalFuncMap[HIGHSEQNOS][defaultPipelineStatsInterval] = highSeqnoFunc

		highSeqnoFuncLegacy := b.getHighSeqnosUpdater(spec, watcher, true)
		intervalFuncMap[HIGHSEQNOSLEGACY] = make(IntervalInnerFuncMap)
		intervalFuncMap[HIGHSEQNOSLEGACY][defaultPipelineStatsInterval] = highSeqnoFuncLegacy

		watcher.intervalFuncMap = intervalFuncMap
	}
	b.srcBucketWatchersCnt[spec.SourceBucketName]++
	return watcher
}

func (b *BucketTopologyService) getDcpStatsUpdater(spec *metadata.ReplicationSpecification, watcher *BucketTopologySvcWatcher) func() error {
	dcpStatsFunc := func() error {
		nodes, err := watcher.xdcrCompTopologySvc.MyKVNodes()
		if err != nil {
			return fmt.Errorf("Failed to get my KV nodes, err=%v\n", err)
		}
		if len(nodes) == 0 {
			return base.ErrorNoSourceKV
		}
		dcp_stats := make(map[string]map[string]string)
		userAgent := fmt.Sprintf("Goxdcr BucketTopologyWatcher %v", spec.SourceBucketName)
		var features utils.HELOFeatures
		watcher.kvMemClientsMtx.Lock()
		for _, serverAddr := range nodes {
			// TODO - optimize locking
			client, err := b.utils.GetMemcachedClient(serverAddr, spec.SourceBucketName, watcher.kvMemClients, userAgent, base.KeepAlivePeriod, watcher.logger, features)
			if err != nil {
				watcher.kvMemClientsMtx.Unlock()
				return err
			}

			stats_map, err := client.StatsMap(base.DCP_STAT_NAME)
			if err != nil {
				watcher.logger.Warnf("%v Error getting dcp stats for kv %v. err=%v", userAgent, serverAddr, err)
				err1 := client.Close()
				if err1 != nil {
					watcher.logger.Warnf("%v error from closing connection for %v is %v\n", userAgent, serverAddr, err1)
				}
				delete(watcher.kvMemClients, serverAddr)
				watcher.kvMemClientsMtx.Unlock()
				return err
			} else {
				dcp_stats[serverAddr] = stats_map
			}
		}
		watcher.kvMemClientsMtx.Unlock()
		watcher.latestCacheMtx.Lock()
		watcher.latestCached.DcpStatsMap = dcp_stats
		watcher.latestCacheMtx.Unlock()
		return nil
	}
	return dcpStatsFunc
}

func (b *BucketTopologyService) getDcpStatsLegacyUpdater(spec *metadata.ReplicationSpecification, watcher *BucketTopologySvcWatcher) func() error {
	dcpStatsFunc := func() error {
		nodes, err := watcher.xdcrCompTopologySvc.MyKVNodes()
		if err != nil {
			return fmt.Errorf("Failed to get my KV nodes, err=%v\n", err)
		}
		if len(nodes) == 0 {
			return base.ErrorNoSourceKV
		}
		dcp_stats := make(map[string]map[string]string)
		userAgent := fmt.Sprintf("Goxdcr BucketTopologyWatcherLegacy %v", spec.SourceBucketName)
		watcher.kvMemClientsLegacyMtx.Lock()
		for _, serverAddr := range nodes {
			// If the remote cluster does not support collections, then only get the stat for default collection
			var features utils.HELOFeatures
			// This is reverse logic because to only get stat for the default collection, we need to enable collection
			// so we can ask specifically for a subset, aka the default collection
			features.Collections = true
			// TODO - optimize locking
			client, err := b.utils.GetMemcachedClient(serverAddr, spec.SourceBucketName, watcher.kvMemClientsLegacy, userAgent, base.KeepAlivePeriod, watcher.logger, features)
			if err != nil {
				watcher.kvMemClientsLegacyMtx.Unlock()
				return err
			}

			stats_map, err := client.StatsMap(base.DCP_STAT_NAME)
			if err != nil {
				watcher.logger.Warnf("%v Error getting dcp stats for kv %v. err=%v", userAgent, serverAddr, err)
				err1 := client.Close()
				if err1 != nil {
					watcher.logger.Warnf("%v error from closing connection for %v is %v\n", userAgent, serverAddr, err1)
				}
				delete(watcher.kvMemClientsLegacy, serverAddr)
				watcher.kvMemClientsLegacyMtx.Unlock()
				return err
			} else {
				dcp_stats[serverAddr] = stats_map
			}
		}
		watcher.kvMemClientsLegacyMtx.Unlock()
		watcher.latestCacheMtx.Lock()
		watcher.latestCached.DcpStatsMapLegacy = dcp_stats
		watcher.latestCacheMtx.Unlock()
		return nil
	}
	return dcpStatsFunc
}

// When cluster uses strict encryption, we need to use loopback address for local server
// and set the key in serverVBMap accordingly
func (b *BucketTopologyService) updateLocalServerVBucketMapIfNeeded(serverVBMap map[string][]uint16, bucketInfo map[string]interface{}) (map[string][]uint16, error) {
	if b.xdcrCompTopologySvc.IsMyClusterEncryptionLevelStrict() == false {
		return serverVBMap, nil
	}
	loopback := base.LocalHostName
	if base.IsIpV4Blocked() {
		loopback = base.LocalHostNameIpv6
	}
	currentHostAddr, err := b.utils.GetCurrentHostnameFromBucketInfo(bucketInfo)
	if err != nil {
		return nil, err
	}
	currentHostName := base.GetHostName(currentHostAddr)
	if currentHostName == loopback {
		return serverVBMap, nil
	}
	newServerVBMap := make(map[string][]uint16)
	for server, vbs := range serverVBMap {
		hostName := base.GetHostName(server)
		if hostName == currentHostName {
			// Change the map to use loopback
			port, err := base.GetPortNumber(server)
			if err != nil {
				return nil, err
			}
			newServerVBMap[base.GetHostAddr(loopback, port)] = vbs
		} else {
			newServerVBMap[server] = vbs
		}
	}
	return newServerVBMap, nil
}

func (b *BucketTopologyService) getLocalBucketTopologyUpdater(spec *metadata.ReplicationSpecification, watcher *BucketTopologySvcWatcher) func() error {
	topologyFunc := func() error {
		connStr, err := b.xdcrCompTopologySvc.MyConnectionStr()
		if err != nil {
			watcher.logger.Errorf("%v bucket connStr error %v", spec.SourceBucketName, err)
			return err
		}
		userName, password, httpAuthMech, certificate, sanInCertificate, clientCertificate, clientKey, err := b.xdcrCompTopologySvc.MyCredentials()
		if err != nil {
			watcher.logger.Errorf("%v bucket credentials error %v", spec.SourceBucketName, err)
			return err
		}
		bucketInfo, err := b.utils.GetBucketInfo(connStr, spec.SourceBucketName, userName, password, httpAuthMech, certificate, sanInCertificate, clientCertificate, clientKey, b.logger)
		if err != nil {
			watcher.logger.Errorf("%v bucket bucketInfo error %v", spec.SourceBucketName, err)
			return err
		}
		serverVBMap, err := b.utils.GetServerVBucketsMap(connStr, spec.SourceBucketName, bucketInfo)
		if err != nil {
			watcher.logger.Errorf("%v bucket server VBMap error %v", spec.SourceBucketName, err)
			return err
		}
		serverVBMap, err = b.updateLocalServerVBucketMapIfNeeded(serverVBMap, bucketInfo)
		if err != nil {
			return fmt.Errorf("Failed to update local serverVBucket map. err=%v", err)
		}

		replicasMap, translateMap, numOfReplicas, vbReplicaMember, err := b.utils.GetReplicasInfo(bucketInfo, b.securitySvc.IsClusterEncryptionLevelStrict())
		if err != nil {
			watcher.logger.Errorf("%v replicasInfo error %v", spec.SourceBucketName, err)
			return err
		}

		nodes, err := watcher.xdcrCompTopologySvc.MyKVNodes()
		if err != nil {
			return fmt.Errorf("Failed to get my KV nodes, err=%v\n", err)
		}
		if len(nodes) == 0 {
			return base.ErrorNoSourceKV
		}
		sourceKvVbMap := make(map[string][]uint16)
		for _, node := range nodes {
			if vbnos, ok := serverVBMap[node]; ok {
				sourceKvVbMap[node] = vbnos
			}
		}

		watcher.latestCacheMtx.Lock()
		watcher.cachePopulated = true
		watcher.latestCached.Source = true
		watcher.latestCached.KvVbMap = serverVBMap
		watcher.latestCached.NumberOfSourceNodes = len(serverVBMap)
		watcher.latestCached.SourceVBMap = sourceKvVbMap
		watcher.latestCached.SourceReplicasMap = replicasMap
		watcher.latestCached.SourceReplicasTranslateMap = translateMap
		watcher.latestCached.SourceReplicaCnt = numOfReplicas
		watcher.latestCached.SourceVbReplicasMember = vbReplicaMember
		watcher.latestCacheMtx.Unlock()
		return nil
	}
	return topologyFunc
}

func getTargetWatcherKey(spec *metadata.ReplicationSpecification) string {
	return fmt.Sprintf("%v_%v", spec.TargetClusterUUID, spec.TargetBucketName)
}

func (b *BucketTopologyService) getOrCreateRemoteWatcher(spec *metadata.ReplicationSpecification) (*BucketTopologySvcWatcher, error) {
	if spec == nil {
		return nil, base.ErrorNilPtr
	}

	if spec.TargetBucketName == "" {
		return nil, fmt.Errorf("Empty target bucket name for spec %v", spec.Id)
	}

	if spec.TargetClusterUUID == "" {
		return nil, fmt.Errorf("Empty target cluster UUID for spec %v", spec.Id)
	}

	ref, err := b.remClusterSvc.RemoteClusterByUuid(spec.TargetClusterUUID, false)
	if err != nil {
		return nil, fmt.Errorf("Unable to find remote cluster reference for spec %v", spec.Id)
	}

	bucketInfoGetter, err := b.remClusterSvc.GetBucketInfoGetter(ref, spec.TargetBucketName)
	if err != nil {
		return nil, fmt.Errorf("Unable to get remote bucketInfo getter for spec %v", spec.Id)
	}

	b.tgtBucketWatchersMtx.Lock()
	defer b.tgtBucketWatchersMtx.Unlock()
	watcher, exists := b.tgtBucketWatchers[getTargetWatcherKey(spec)]
	if !exists {
		watcher = NewBucketTopologySvcWatcher(spec.TargetBucketName, spec.TargetBucketUUID, b.logger, false, b.xdcrCompTopologySvc)
		b.tgtBucketWatchers[getTargetWatcherKey(spec)] = watcher

		topologyUpdateFunc := func() error {
			targetBucketInfo, shouldUseExternal, connStr, err := bucketInfoGetter()
			if err != nil {
				return err
			}
			targetBucketUUID, err := b.utils.GetBucketUuidFromBucketInfo(spec.TargetBucketName, targetBucketInfo, b.logger)
			if err != nil {
				return err
			}
			targetServerVBMap, err := b.utils.GetRemoteServerVBucketsMap(connStr, spec.TargetBucketName, targetBucketInfo, shouldUseExternal)
			if err != nil {
				return err
			}
			perUpdateRef, err := b.remClusterSvc.RemoteClusterByUuid(spec.TargetClusterUUID, false)
			if err != nil {
				return err
			}
			replicasMap, translateMap, numOfReplicas, vbReplicaMember, err := b.utils.GetReplicasInfo(targetBucketInfo, perUpdateRef.IsHttps())
			if err != nil {
				watcher.logger.Errorf("%v target replicasInfo error %v", spec.TargetBucketName, err)
				return err
			}

			watcher.latestCacheMtx.Lock()
			watcher.cachePopulated = true
			watcher.latestCached.Source = false
			watcher.latestCached.TargetServerVBMap = targetServerVBMap
			watcher.latestCached.TargetBucketUUID = targetBucketUUID
			watcher.latestCached.TargetBucketInfo = targetBucketInfo
			watcher.latestCached.TargetReplicasMap = replicasMap
			watcher.latestCached.TargetReplicasTranslateMap = translateMap
			watcher.latestCached.TargetReplicaCnt = numOfReplicas
			watcher.latestCached.TargetVbReplicasMember = vbReplicaMember
			watcher.latestCacheMtx.Unlock()
			return nil
		}
		intervalFuncMap := make(IntervalFuncMap)
		intervalFuncMap[TOPOLOGY] = make(IntervalInnerFuncMap)
		intervalFuncMap[TOPOLOGY][b.refreshInterval] = topologyUpdateFunc

		watcher.intervalFuncMap = intervalFuncMap
	}
	b.tgtBucketWatchersCnt[getTargetWatcherKey(spec)]++
	return watcher, nil
}

func (b *BucketTopologyService) SubscribeToRemoteBucketFeed(spec *metadata.ReplicationSpecification, subscriberId string) (chan service_def.TargetNotification, error) {
	if spec == nil {
		return nil, base.ErrorNilPtr
	}

	if spec.TargetBucketName == "" {
		return nil, fmt.Errorf("Empty target bucket name for spec %v", spec.Id)
	}

	b.tgtBucketWatchersMtx.Lock()
	defer b.tgtBucketWatchersMtx.Unlock()
	watcher, exists := b.tgtBucketWatchers[getTargetWatcherKey(spec)]
	if exists {
		return watcher.registerAndGetCh(spec, subscriberId, TOPOLOGY, nil).(chan service_def.TargetNotification), nil
	}

	return nil, fmt.Errorf("SubscribeToRemoteBucketFeed could not find watcher for %v", spec.TargetBucketName)
}

func (b *BucketTopologyService) UnSubscribeLocalBucketFeed(spec *metadata.ReplicationSpecification, subscriberId string) error {
	return b.unSubscribeLocalInternal(spec, subscriberId, TOPOLOGY)
}

func (b *BucketTopologyService) handleSpecDeletion(spec *metadata.ReplicationSpecification) error {
	if spec == nil {
		return base.ErrorNilPtr
	}

	var err error
	b.srcBucketWatchersMtx.Lock()
	b.srcBucketWatchersCnt[spec.SourceBucketName]--
	b.srcBucketWatchers[spec.SourceBucketName].handleSpecDeletion(spec.Id)
	if b.srcBucketWatchersCnt[spec.SourceBucketName] == 0 {
		err = b.srcBucketWatchers[spec.SourceBucketName].Stop()
		delete(b.srcBucketWatchers, spec.SourceBucketName)
		delete(b.srcBucketWatchersCnt, spec.SourceBucketName)
	}
	b.srcBucketWatchersMtx.Unlock()

	b.tgtBucketWatchersMtx.Lock()
	b.tgtBucketWatchersCnt[getTargetWatcherKey(spec)]--
	if b.tgtBucketWatchersCnt[getTargetWatcherKey(spec)] == 0 {
		err = b.tgtBucketWatchers[getTargetWatcherKey(spec)].Stop()
		delete(b.tgtBucketWatchers, getTargetWatcherKey(spec))
		delete(b.tgtBucketWatchersCnt, getTargetWatcherKey(spec))
	}
	b.tgtBucketWatchersMtx.Unlock()

	return err
}

func (b *BucketTopologyService) UnSubscribeRemoteBucketFeed(spec *metadata.ReplicationSpecification, subscriberId string) error {
	if spec == nil {
		return base.ErrorNilPtr
	}

	b.tgtBucketWatchersMtx.RLock()
	_, exists := b.tgtBucketWatchersCnt[getTargetWatcherKey(spec)]
	_, exists2 := b.tgtBucketWatchers[getTargetWatcherKey(spec)]
	if !exists || !exists2 {
		b.tgtBucketWatchersMtx.RUnlock()
		return base.ErrorResourceDoesNotExist
	}
	b.tgtBucketWatchers[getTargetWatcherKey(spec)].unregisterCh(spec, subscriberId, TOPOLOGY)
	b.tgtBucketWatchersMtx.RUnlock()
	return nil
}

func (b *BucketTopologyService) ReplicationSpecChangeCallback(id string, oldVal, newVal interface{}, wg *sync.WaitGroup) error {
	if wg != nil {
		defer wg.Done()
	}

	oldSpec, ok := oldVal.(*metadata.ReplicationSpecification)
	if !ok {
		return base.ErrorInvalidInput
	}
	newSpec, ok := newVal.(*metadata.ReplicationSpecification)
	if !ok {
		return base.ErrorInvalidInput
	}

	if oldSpec == nil && newSpec != nil {
		var waitGrp sync.WaitGroup
		waitGrp.Add(2)

		go func() {
			defer waitGrp.Done()

			localRetryOp := func() error {
				watcher := b.getOrCreateLocalWatcher(newSpec)
				if watcher == nil {
					return base.ErrorNilPtr
				}
				localStartErr := watcher.Start()
				if localStartErr != nil && localStartErr != ErrorWatcherAlreadyStarted {
					b.logger.Errorf("Error starting local watcher for %v - %v", newSpec, localStartErr)
					return localStartErr
				}
				return nil
			}
			retryErr := b.utils.ExponentialBackoffExecutor("BucketTopologyServiceLoadSpecLocal",
				base.DefaultHttpTimeoutWaitTime, base.DefaultHttpTimeoutMaxRetry, base.DefaultHttpTimeoutRetryFactor, localRetryOp)
			if retryErr != nil {
				panic(fmt.Sprintf("Bucket Topology service (local) bootstrapping %v did not finish within %v and XDCR must restart to try again", newSpec.Id, base.DefaultHttpTimeout))
			}
		}()

		go func() {
			defer waitGrp.Done()
			retryRemoteOp := func() error {
				remoteWatcher, remoteErr := b.getOrCreateRemoteWatcher(newSpec)
				if remoteErr != nil {
					b.logger.Errorf("Error getting remote watcher for %v - %v", newSpec, remoteErr)
					return remoteErr
				}
				remoteStartErr := remoteWatcher.Start()
				if remoteStartErr != nil && remoteStartErr != ErrorWatcherAlreadyStarted {
					b.logger.Errorf("Error starting remote watcher for %v - %v", newSpec, remoteStartErr)
					return remoteStartErr
				}
				return nil
			}
			retryErr := b.utils.ExponentialBackoffExecutor("BucketTopologyServiceLoadSpecRemote",
				base.DefaultHttpTimeoutWaitTime, base.DefaultHttpTimeoutMaxRetry, base.DefaultHttpTimeoutRetryFactor, retryRemoteOp)
			if retryErr != nil {
				panic(fmt.Sprintf("Bucket Topology service (remote) bootstrapping %v did not finish within %v and XDCR must restart to try again", newSpec.Id, base.DefaultHttpTimeout))
			}
		}()
		waitGrp.Wait()
		b.logger.Infof("Registered bucket monitor for %v", newSpec.Id)
	} else if oldSpec != nil && newSpec == nil {
		err := b.handleSpecDeletion(oldSpec)
		if err != nil {
			b.logger.Errorf("Unable to unregister local bucket for %v", oldSpec)
			return err
		}
		b.logger.Infof("Unregistered bucket monitor for %v", oldSpec.Id)
	}

	return nil
}

func (b *BucketTopologyService) SubscribeToLocalBucketDcpStatsFeed(spec *metadata.ReplicationSpecification, subscriberId string) (chan service_def.SourceNotification, error) {
	if spec == nil {
		return nil, base.ErrorNilPtr
	}

	if spec.SourceBucketName == "" {
		return nil, fmt.Errorf("Empty source bucket name for spec %v", spec.Id)
	}

	b.srcBucketWatchersMtx.Lock()
	defer b.srcBucketWatchersMtx.Unlock()
	watcher, exists := b.srcBucketWatchers[spec.SourceBucketName]
	if exists {
		retCh := watcher.registerAndGetCh(spec, subscriberId, DCPSTATSCHECK, nil).(chan service_def.SourceNotification)
		return retCh, nil
	}
	return nil, fmt.Errorf("SubscribeToLocalBucketDcpStatsFeed could not find watcher for %v", spec.SourceBucketName)
}

func (b *BucketTopologyService) SubscribeToLocalBucketDcpStatsLegacyFeed(spec *metadata.ReplicationSpecification, subscriberId string) (chan service_def.SourceNotification, error) {
	if spec == nil {
		return nil, base.ErrorNilPtr
	}

	if spec.SourceBucketName == "" {
		return nil, fmt.Errorf("Empty source bucket name for spec %v", spec.Id)
	}

	b.srcBucketWatchersMtx.Lock()
	defer b.srcBucketWatchersMtx.Unlock()
	watcher, exists := b.srcBucketWatchers[spec.SourceBucketName]
	if exists {
		retCh := watcher.registerAndGetCh(spec, subscriberId, DCPSTATSCHECKLEGACY, nil).(chan service_def.SourceNotification)
		return retCh, nil
	}
	return nil, fmt.Errorf("SubscribeToLocalBucketDcpStatsLegacyFeed could not find watcher for %v", spec.SourceBucketName)
}

func (b *BucketTopologyService) UnSubscribeToLocalBucketDcpStatsFeed(spec *metadata.ReplicationSpecification, subscriberId string) error {
	return b.unSubscribeLocalInternal(spec, subscriberId, DCPSTATSCHECK)
}

func (b *BucketTopologyService) UnSubscribeToLocalBucketDcpStatsLegacyFeed(spec *metadata.ReplicationSpecification, subscriberId string) error {
	return b.unSubscribeLocalInternal(spec, subscriberId, DCPSTATSCHECKLEGACY)
}

func (b *BucketTopologyService) UnSubscribeToLocalBucketHighSeqnosFeed(spec *metadata.ReplicationSpecification, subscriberId string) error {
	return b.unSubscribeLocalInternal(spec, subscriberId, HIGHSEQNOS)
}

func (b *BucketTopologyService) UnSubscribeToLocalBucketHighSeqnosLegacyFeed(spec *metadata.ReplicationSpecification, subscriberId string) error {
	return b.unSubscribeLocalInternal(spec, subscriberId, HIGHSEQNOSLEGACY)
}

func (b *BucketTopologyService) unSubscribeLocalInternal(spec *metadata.ReplicationSpecification, subscriberId string, feedType string) error {
	if spec == nil {
		return base.ErrorNilPtr
	}

	b.srcBucketWatchersMtx.RLock()
	_, exists := b.srcBucketWatchersCnt[spec.SourceBucketName]
	_, exists2 := b.srcBucketWatchers[spec.SourceBucketName]
	if !exists || !exists2 {
		b.srcBucketWatchersMtx.RUnlock()
		return base.ErrorResourceDoesNotExist
	}
	b.srcBucketWatchers[spec.SourceBucketName].unregisterCh(spec, subscriberId, feedType)
	b.srcBucketWatchersMtx.RUnlock()
	return nil
}

func (b *BucketTopologyService) getHighSeqnosUpdater(spec *metadata.ReplicationSpecification, watcher *BucketTopologySvcWatcher, legacyMode bool) func() error {
	updaterFunc := func() error {
		settings := spec.Settings.ToMap(false)
		metadata.GetSettingFromSettingsMap(settings, metadata.PipelineStatsIntervalKey, spec.GetReplicationSpec().Settings.StatsInterval)
		var collectionIds []uint32
		var features utils.HELOFeatures
		features.CompressionType = base.CompressionTypeNone
		watcher.latestCacheMtx.RLock()
		if !watcher.cachePopulated {
			watcher.latestCacheMtx.RUnlock()
			return fmt.Errorf("Cache is not populated yet to get highseqnos")
		}
		kv_vb_map := watcher.latestCached.SourceVBMap.Clone()
		watcher.latestCacheMtx.RUnlock()

		if legacyMode {
			// This is reverse logic because to only get stat for the default collection, we need to enable collection
			// so we can ask specifically for a subset, aka the default collection
			features.Collections = true
			collectionIds = append(collectionIds, base.DefaultCollectionId)
		}

		highseqno_map := make(map[string]map[uint16]uint64)
		userAgent := fmt.Sprintf("Goxdcr BucketTopologyWatcher %v", spec.SourceBucketName)
		watcher.kvMemClientsMtx.Lock()
		for serverAddr, vbnos := range kv_vb_map {
			// TODO - optimizing locking
			client, err := b.utils.GetMemcachedClient(serverAddr, spec.SourceBucketName, watcher.kvMemClients, userAgent, base.KeepAlivePeriod, watcher.logger, features)
			if err != nil {
				watcher.kvMemClientsMtx.Unlock()
				return err
			}
			oneSeqnoMap, updatedStatsMap, err := b.utils.GetHighSeqNos(vbnos, client, watcher.statsMap, collectionIds)
			watcher.statsMap = updatedStatsMap
			highseqno_map[serverAddr] = oneSeqnoMap

			if err != nil {
				watcher.logger.Warnf("%v Error getting high seqno for kv %v. err=%v", userAgent, serverAddr, err)
				err1 := client.Close()
				if err1 != nil {
					watcher.logger.Warnf("%v error from closing connection for %v is %v\n", userAgent, serverAddr, err1)
				}
				delete(watcher.kvMemClients, serverAddr)
				watcher.kvMemClientsMtx.Unlock()
				return err
			} else {
				watcher.kvMemClients[serverAddr] = client
			}
		}
		watcher.kvMemClientsMtx.Unlock()

		watcher.latestCacheMtx.Lock()
		if legacyMode {
			watcher.latestCached.HighSeqnoMapLegacy = highseqno_map
		} else {
			watcher.latestCached.HighSeqnoMap = highseqno_map
		}
		watcher.latestCacheMtx.Unlock()
		return nil
	}
	return updaterFunc
}

// Returns a duration updater
func (b *BucketTopologyService) SubscribeToLocalBucketHighSeqnosFeed(spec *metadata.ReplicationSpecification, subscriberId string, requestedInterval time.Duration) (chan service_def.SourceNotification, func(time.Duration), error) {
	if spec == nil {
		return nil, nil, base.ErrorNilPtr
	}

	if spec.SourceBucketName == "" {
		return nil, nil, fmt.Errorf("Empty source bucket name for spec %v", spec.Id)
	}

	b.srcBucketWatchersMtx.Lock()
	defer b.srcBucketWatchersMtx.Unlock()
	watcher, exists := b.srcBucketWatchers[spec.SourceBucketName]
	if exists {
		intOpts := HighSeqnosOpts{
			Spec:              spec,
			RequestedInterval: requestedInterval,
			SubscriberId:      subscriberId,
		}
		retCh := watcher.registerAndGetCh(spec, subscriberId, HIGHSEQNOS, intOpts).(chan service_def.SourceNotification)
		intervalUpdateFunc := func(newInterval time.Duration) {
			updateOpts := HighSeqnosOpts{
				Spec:              spec,
				RequestedInterval: newInterval,
				SubscriberId:      subscriberId,
			}
			err := watcher.updateSettings(HIGHSEQNOS, updateOpts)
			if err != nil {
				b.logger.Errorf("Unable to update watcher setting for %v given opts %v", HIGHSEQNOS, updateOpts)
			}
		}
		return retCh, intervalUpdateFunc, nil
	}
	return nil, nil, fmt.Errorf("SubscribeToLocalBucketHighSeqnosFeed could not find watcher for %v", spec.SourceBucketName)
}

// Returns a duration updater
func (b *BucketTopologyService) SubscribeToLocalBucketHighSeqnosLegacyFeed(spec *metadata.ReplicationSpecification, subscriberId string, requestedInterval time.Duration) (chan service_def.SourceNotification, func(time.Duration), error) {
	if spec == nil {
		return nil, nil, base.ErrorNilPtr
	}

	if spec.SourceBucketName == "" {
		return nil, nil, fmt.Errorf("Empty source bucket name for spec %v", spec.Id)
	}

	b.srcBucketWatchersMtx.Lock()
	defer b.srcBucketWatchersMtx.Unlock()
	watcher, exists := b.srcBucketWatchers[spec.SourceBucketName]
	if exists {
		intOpts := HighSeqnosOpts{
			Spec:              spec,
			RequestedInterval: requestedInterval,
			SubscriberId:      subscriberId,
		}
		retCh := watcher.registerAndGetCh(spec, subscriberId, HIGHSEQNOSLEGACY, intOpts).(chan service_def.SourceNotification)
		intervalUpdateFunc := func(newInterval time.Duration) {
			updateOpts := HighSeqnosOpts{
				Spec:              spec,
				RequestedInterval: newInterval,
				SubscriberId:      subscriberId,
			}
			err := watcher.updateSettings(HIGHSEQNOSLEGACY, updateOpts)
			if err != nil {
				b.logger.Errorf("Unable to update watcher setting for %v given opts %v", HIGHSEQNOSLEGACY, updateOpts)
			}
		}
		return retCh, intervalUpdateFunc, nil
	}
	return nil, nil, fmt.Errorf("SubscribeToLocalBucketHighSeqnosLegacyFeed could not find watcher for %v", spec.SourceBucketName)
}

func (b *BucketTopologyService) RegisterGarbageCollect(specId string, srcBucketName string, vbno uint16, requestId string, gcFunc func() error, timeToFire time.Duration) error {
	if specId == "" {
		return base.ErrorInvalidInput
	}
	b.srcBucketWatchersMtx.RLock()
	watcher, exists := b.srcBucketWatchers[srcBucketName]
	b.srcBucketWatchersMtx.RUnlock()
	if !exists {
		return base.ErrorNotFound
	}
	return watcher.RegisterGarbageCollect(specId, vbno, requestId, gcFunc, timeToFire)
}

type BucketTopologySvcWatcher struct {
	bucketName string
	bucketUUID string
	source     bool
	spec       *metadata.ReplicationSpecification // Set once

	finCh     chan bool
	startOnce sync.Once

	// Key is a "spec + subscriber ID"
	topologyNotifyMtx      sync.RWMutex
	topologyNotifyChs      map[string]interface{}
	dcpStatsMtx            sync.RWMutex
	dcpStatsChs            map[string]interface{}
	dcpStatsLegacyMtx      sync.RWMutex
	dcpStatsLegacyChs      map[string]interface{}
	highSeqnosChsMtx       sync.RWMutex
	highSeqnosChs          map[string]interface{}
	highSeqnosLegacyChsMtx sync.RWMutex
	highSeqnosLegacyChs    map[string]interface{}

	latestCacheMtx sync.RWMutex
	latestCached   *Notification
	cachePopulated bool

	logger       *log.CommonLogger
	firstToStart uint32
	isStarted    uint32
	isStopped    uint32

	xdcrCompTopologySvc service_def.XDCRCompTopologySvc

	// For bootstrapping
	intervalFuncMap IntervalFuncMap

	// For runtime
	watchersTickersMap      WatchersTickerMap
	watchersTickersValueMap WatchersTickerValueMap
	watchersTickersMapMtx   sync.RWMutex

	// For DCP stats
	kvMemClients    map[string]mcc.ClientIface
	kvMemClientsMtx sync.Mutex

	kvMemClientsLegacy    map[string]mcc.ClientIface
	kvMemClientsLegacyMtx sync.Mutex

	// Used internally to call KV to prevent garbage - must only be used by one updater goroutine
	statsMap map[string]string

	// For highSeqnos and highSeqnosLegacy, intervals need to be changed dynamically
	// The ideas here is to separate operation into the actual GET op and the passing of data to the receiver
	// The GET op is dictated by the subscriber who wants it the most frequently, and then only pass to
	// each individual receiver as to not overwhelm it
	// Each key is a replication and each replication has a desired time interval to receive data
	// The idea is watcher need to call REST API's at the smallest duration (highSeqnosIntervals)
	// But only feed the latest data back a specific requester only if the requester expects it
	// ... i.e the channel whenever a ticker has fired (highSeqnosReceiverFired)
	highSeqnosIntervals     map[string]time.Duration
	highSeqnosReceiverFired map[string]*time.Ticker // Key is replId + subscriberID
	highSeqnosTrackersMtx   sync.RWMutex

	highSeqnosIntervalsLegacy     map[string]time.Duration
	highSeqnosReceiverFiredLegacy map[string]*time.Ticker // Key is replId + subscriberID
	highSeqnosTrackersMtxLegacy   sync.RWMutex

	// Only applicable to source watcher
	gcMapMtx sync.RWMutex
	// ReplicationId -> vbno -> requestId
	gcMap             GcMapType
	gcPruneMap        GcMapPruneMapType // To remove really outdated gc requests
	gcMapUndergoingGc bool
	gcMapGcAbort      bool
	gcPruneWindow     time.Duration
}

type GcMapType map[string]VbnoReqMapType
type VbnoReqMapType map[uint16]RequestMapType
type RequestMapType map[string]*GcRequest

type GcMapPruneMapType map[string]VbnoPruneMapType
type VbnoPruneMapType map[uint16]time.Time

// IntervalFuncMap is keyed by type
// Each type will have a single len IntervalInnerFuncMap
type IntervalFuncMap map[string]IntervalInnerFuncMap
type IntervalInnerFuncMap map[time.Duration]func() error

type WatchersCallbacksFuncMap map[string]func(opts interface{}) error
type WatchersTickerMap map[string]*time.Ticker
type WatchersTickerValueMap map[string]time.Duration

// Legacy below is needed to replicate to target cluster that is < 7.0
// Once <7.0 is EOL'ed, then LEGACY can be removed
const (
	TOPOLOGY            = "topology"
	DCPSTATSCHECK       = "dcpStats"
	DCPSTATSCHECKLEGACY = "dcpStatsLegacy"
	HIGHSEQNOS          = "vbHighSeqnos"
	HIGHSEQNOSLEGACY    = "vbHighSeqnosLegacy" // Legacy means it only receives default collection high seqnos
)

type HighSeqnosOpts struct {
	Spec              *metadata.ReplicationSpecification
	RequestedInterval time.Duration
	SubscriberId      string
}

type GcRequest struct {
	timeToFire time.Time
	funcToFire func() error
}

func NewBucketTopologySvcWatcher(bucketName, bucketUuid string, logger *log.CommonLogger, source bool, xdcrCompTopologySvc service_def.XDCRCompTopologySvc) *BucketTopologySvcWatcher {
	watcher := &BucketTopologySvcWatcher{
		bucketName:                    bucketName,
		bucketUUID:                    bucketUuid,
		finCh:                         make(chan bool),
		logger:                        logger,
		source:                        source,
		xdcrCompTopologySvc:           xdcrCompTopologySvc,
		latestCached:                  NewNotification(source),
		topologyNotifyChs:             make(map[string]interface{}),
		kvMemClients:                  map[string]mcc.ClientIface{},
		kvMemClientsLegacy:            map[string]mcc.ClientIface{},
		dcpStatsChs:                   map[string]interface{}{},
		dcpStatsLegacyChs:             map[string]interface{}{},
		statsMap:                      nil, // Start off with nil
		watchersTickersMap:            WatchersTickerMap{},
		watchersTickersValueMap:       WatchersTickerValueMap{},
		highSeqnosChs:                 map[string]interface{}{},
		highSeqnosLegacyChs:           map[string]interface{}{},
		highSeqnosIntervals:           map[string]time.Duration{},
		highSeqnosReceiverFired:       map[string]*time.Ticker{},
		highSeqnosIntervalsLegacy:     map[string]time.Duration{},
		highSeqnosReceiverFiredLegacy: map[string]*time.Ticker{},
		gcMap:                         GcMapType{},
		gcPruneMap:                    GcMapPruneMapType{},
		gcPruneWindow:                 base.BucketTopologyGCPruneTime,
	}
	return watcher
}

var ErrorWatcherAlreadyStarted = fmt.Errorf("Watcher is already started")

func (bw *BucketTopologySvcWatcher) Start() error {
	if atomic.CompareAndSwapUint32(&bw.firstToStart, 0, 1) {
		var initDone sync.WaitGroup
		initDone.Add(1)
		go bw.run(&initDone)
		initDone.Wait()
		atomic.StoreUint32(&bw.isStarted, 1)
		return nil
	}
	return ErrorWatcherAlreadyStarted
}

func (bw *BucketTopologySvcWatcher) run(initDone *sync.WaitGroup) {
	bw.logger.Infof("Starting watcher for local? %v bucket of %v with UUID %v", bw.source, bw.bucketName, bw.bucketUUID)
	defer bw.logger.Infof("Stopped watcher for local? %v bucket of %v with UUID %v", bw.source, bw.bucketName, bw.bucketUUID)

	if bw.source {
		go func() {
			scanTicker := time.NewTicker(base.BucketTopologyGCScanTime)
			for {
				select {
				case <-bw.finCh:
					return
				case <-scanTicker.C:
					bw.runGC()
				}
			}
		}()
	}

	for updateType, intervalAndFunc := range bw.intervalFuncMap {
		for interval, updateFunc := range intervalAndFunc {
			// Make copies because these references can change before the go func() is executed
			updateTypeCpy := updateType
			intervalCpy := interval
			funcCpy := updateFunc
			initDone.Add(1)
			go func() {
				initCh := make(chan bool, 1)
				initCh <- true
				ticker := time.NewTicker(intervalCpy)
				bw.watchersTickersMapMtx.Lock()
				bw.watchersTickersMap[updateTypeCpy] = ticker
				bw.watchersTickersValueMap[updateTypeCpy] = intervalCpy
				bw.watchersTickersMapMtx.Unlock()
				for {
					select {
					case <-bw.finCh:
						return
					case <-initCh:
						bw.updateOnce(updateTypeCpy, funcCpy)
						initDone.Done()
					case <-ticker.C:
						bw.updateOnce(updateTypeCpy, funcCpy)
					}
				}
			}()
		}
	}
	initDone.Done()

	for {
		select {
		case <-bw.finCh:
			bw.closeKvConns()
			return
		}
	}
}

func (bw *BucketTopologySvcWatcher) runGC() {
	var lastPopulatedCache service_def.SourceNotification
	bw.latestCacheMtx.RLock()
	populated := bw.cachePopulated
	if populated {
		lastPopulatedCache = bw.latestCached.Clone().(service_def.SourceNotification)
	}
	bw.latestCacheMtx.RUnlock()
	if !populated {
		// Don't do anything
		return
	}

	srcVBMap := lastPopulatedCache.GetSourceVBMapRO()
	if len(srcVBMap) > 1 {
		// Not supposed to happen
		var keys []string
		for key, _ := range srcVBMap {
			keys = append(keys, key)
		}
		bw.logger.Warnf("srcVBMap shows more than one node %v", keys)
	} else if len(srcVBMap) == 0 {
		// Possibly because this node is not a KV node
		return
	}
	var sortedVBsList []uint16

	for _, vbsList := range srcVBMap {
		sortedVBsList = base.SortUint16List(vbsList)
	}

	var toBeDeleted []func()
	bw.gcMapMtx.Lock()
	bw.gcMapUndergoingGc = true
	bw.gcMapMtx.Unlock()

	bw.gcMapMtx.RLock()
	if bw.gcMapGcAbort {
		bw.gcMapMtx.RUnlock()
		return
	}
	for specIdPreCopy, gcForEachSpec := range bw.gcMap {
		specId := specIdPreCopy
		for vbnoPreCopy, idMap := range gcForEachSpec {
			vbno := vbnoPreCopy
			// See if vb still belongs to this node
			// If GetSourceVBMapRO() changes to have more than this source node, need to update logic
			_, vbStillExists := base.SearchVBInSortedList(vbno, sortedVBsList)
			if vbStillExists {
				// Don't do any GC since this node still owns the VB and/or has re-owned the VB
				// However, check to see if this VB has been owned (i.e. never been re-registered)
				// for an Expiry period. If so, this means that all these registered requests
				// are out of date and should be removed
				lastRegisteredTime := bw.gcPruneMap[specId][vbno]
				if time.Now().After(lastRegisteredTime.Add(bw.gcPruneWindow)) {
					toBeDeleted = append(toBeDeleted, func() {
						// Clean up the prune time
						delete(bw.gcPruneMap[specId], vbno)
						if len(bw.gcPruneMap[specId]) == 0 {
							delete(bw.gcPruneMap, specId)
						}
						// Clean up this VB for this spec
						delete(bw.gcMap[specId], vbno)
						if len(bw.gcMap[specId]) == 0 {
							delete(bw.gcMap, specId)
						}
					})
				}
				continue
			}
			numberOfIdRequests := len(idMap)
			var numGCFired int
			var individualIdDelete []func()

			// Perform GC
			for id, req := range idMap {
				if time.Now().After(req.timeToFire) {
					numGCFired++
					err := req.funcToFire()
					if err != nil {
						bw.logger.Warnf("GC for spec %v vbno %v id %v resulted in err %v",
							specId, vbno, id, err)
					}
					individualIdDelete = append(individualIdDelete, func() {
						delete(bw.gcMap[specId][vbno], id)
					})
				}
			}
			if numGCFired == numberOfIdRequests {
				// Everything fired, no need to keep this around
				toBeDeleted = append(toBeDeleted, func() { delete(bw.gcMap[specId], vbno) })
			} else {
				// Only a few subset need to be removed
				toBeDeleted = append(toBeDeleted, individualIdDelete...)
			}
		}
	}
	bw.gcMapMtx.RUnlock()

	bw.gcMapMtx.Lock()
	bw.gcMapUndergoingGc = false
	if !bw.gcMapGcAbort {
		for _, delFunc := range toBeDeleted {
			delFunc()
		}
	}
	bw.gcMapMtx.Unlock()
}

func (bw *BucketTopologySvcWatcher) updateOnce(updateType string, customUpdateFunc func() error) {
	var mutex *sync.RWMutex
	var channelsMap map[string]interface{}
	switch updateType {
	case TOPOLOGY:
		channelsMap = bw.topologyNotifyChs
		mutex = &bw.topologyNotifyMtx
	case DCPSTATSCHECK:
		channelsMap = bw.dcpStatsChs
		mutex = &bw.dcpStatsMtx
	case DCPSTATSCHECKLEGACY:
		channelsMap = bw.dcpStatsLegacyChs
		mutex = &bw.dcpStatsLegacyMtx
	case HIGHSEQNOS:
		channelsMap = bw.highSeqnosChs
		mutex = &bw.highSeqnosChsMtx
	case HIGHSEQNOSLEGACY:
		channelsMap = bw.highSeqnosLegacyChs
		mutex = &bw.highSeqnosLegacyChsMtx
	default:
		panic(fmt.Sprintf("Unknown type: %v", updateType))
	}

	// If no one is subscribed, no need to run the updater
	// Except for topology, which is for sure needed
	if updateType != TOPOLOGY {
		mutex.RLock()
		chanLen := len(channelsMap)
		mutex.RUnlock()
		if chanLen == 0 {
			return
		}
	}

	err := customUpdateFunc()
	if err != nil {
		bw.logger.Errorf("BucketTopologySvcWatcher for local? %v bucket %v updating resulted in err %v - bypassing notification", bw.source, bw.bucketName, err)
		return
	}
	bw.latestCacheMtx.RLock()
	notification := bw.latestCached.Clone().(*Notification)
	bw.latestCacheMtx.RUnlock()

	mutex.RLock()
	defer mutex.RUnlock()
	for channelName, chRaw := range channelsMap {
		if !bw.shouldSendToCh(channelName, updateType) {
			continue
		}
		timeout := time.NewTicker(1 * time.Second)
		if bw.source {
			select {
			case chRaw.(chan service_def.SourceNotification) <- notification:
			// sent
			case <-timeout.C:
				// provide a bail out path
				continue
			}
		} else {
			select {
			case chRaw.(chan service_def.TargetNotification) <- notification:
			// sent
			case <-timeout.C:
				// provide a bail out path
				continue
			}
		}

	}
}

func (bw *BucketTopologySvcWatcher) Stop() error {
	if atomic.CompareAndSwapUint32(&bw.isStopped, 0, 1) {
		close(bw.finCh)
	}
	return nil
}

func (bw *BucketTopologySvcWatcher) registerAndGetCh(spec *metadata.ReplicationSpecification, subscriberId string, chType string, opts interface{}) interface{} {
	for atomic.LoadUint32(&bw.isStarted) == 0 {
		time.Sleep(100 * time.Millisecond)
	}

	var specifiedChs map[string]interface{}
	var mutex *sync.RWMutex
	switch chType {
	case TOPOLOGY:
		specifiedChs = bw.topologyNotifyChs
		mutex = &bw.topologyNotifyMtx
	case DCPSTATSCHECK:
		specifiedChs = bw.dcpStatsChs
		mutex = &bw.dcpStatsMtx
	case DCPSTATSCHECKLEGACY:
		specifiedChs = bw.dcpStatsLegacyChs
		mutex = &bw.dcpStatsLegacyMtx
	case HIGHSEQNOS:
		specifiedChs = bw.highSeqnosChs
		mutex = &bw.highSeqnosChsMtx
		defer bw.setHighSeqnosInterval(opts.(HighSeqnosOpts), false)
	case HIGHSEQNOSLEGACY:
		specifiedChs = bw.highSeqnosLegacyChs
		mutex = &bw.highSeqnosLegacyChsMtx
		defer bw.setHighSeqnosInterval(opts.(HighSeqnosOpts), true)
	default:
		panic(fmt.Sprintf("Unknown type %v", chType))
	}

	mutex.RLock()
	ch, exists := specifiedChs[spec.Id]
	if exists {
		mutex.RUnlock()
		return ch
	}
	mutex.RUnlock()

	fullSubscriberId := compileFullSubscriberId(spec, subscriberId)
	mutex.Lock()
	if bw.source {
		newCh := make(chan service_def.SourceNotification, base.BucketTopologyWatcherChanLen)
		specifiedChs[fullSubscriberId] = newCh
	} else {
		newCh := make(chan service_def.TargetNotification, base.BucketTopologyWatcherChanLen)
		specifiedChs[fullSubscriberId] = newCh
	}
	mutex.Unlock()

	// When someone first registers and subscribes, it prob expects some data - feed it the latest if it's not nil
	mutex.RLock()
	defer mutex.RUnlock()
	if bw.cachePopulated {
		if bw.source {
			notification := bw.latestCached.Clone().(service_def.SourceNotification)
			specifiedChs[fullSubscriberId].(chan service_def.SourceNotification) <- notification
		} else {
			notification := bw.latestCached.Clone().(service_def.TargetNotification)
			specifiedChs[fullSubscriberId].(chan service_def.TargetNotification) <- notification
		}
	}
	return specifiedChs[fullSubscriberId]
}

func compileFullSubscriberId(spec *metadata.ReplicationSpecification, id string) string {
	return fmt.Sprintf("%v_%v", spec.Id, id)
}

func (bw *BucketTopologySvcWatcher) unregisterCh(spec *metadata.ReplicationSpecification, subscriberId string, chType string) {
	var specifiedChs map[string]interface{}
	var mutex *sync.RWMutex
	switch chType {
	case TOPOLOGY:
		specifiedChs = bw.topologyNotifyChs
		mutex = &bw.topologyNotifyMtx
	case DCPSTATSCHECK:
		specifiedChs = bw.dcpStatsChs
		mutex = &bw.dcpStatsMtx
	case DCPSTATSCHECKLEGACY:
		specifiedChs = bw.dcpStatsLegacyChs
		mutex = &bw.dcpStatsLegacyMtx
	case HIGHSEQNOS:
		specifiedChs = bw.highSeqnosChs
		mutex = &bw.highSeqnosChsMtx
		bw.cleanupHighSeqnosInternalData(spec, subscriberId, false)
	case HIGHSEQNOSLEGACY:
		specifiedChs = bw.highSeqnosLegacyChs
		mutex = &bw.highSeqnosLegacyChsMtx
		bw.cleanupHighSeqnosInternalData(spec, subscriberId, true)
	default:
		panic(fmt.Sprintf("Unknown type %v", chType))
	}

	fullSubscriberId := compileFullSubscriberId(spec, subscriberId)

	mutex.Lock()
	defer mutex.Unlock()

	delete(specifiedChs, fullSubscriberId)
}

func (bw *BucketTopologySvcWatcher) closeKvConns() {
	bw.kvMemClientsMtx.Lock()
	for serverAddr, client := range bw.kvMemClients {
		err := client.Close()
		if err != nil {
			bw.logger.Warnf("%v error from closing connection for %v is %v\n", bw.bucketName, serverAddr, err)
		}
	}
	bw.kvMemClients = make(map[string]mcc.ClientIface)
	bw.kvMemClientsMtx.Unlock()

	bw.kvMemClientsLegacyMtx.Lock()
	for serverAddr, client := range bw.kvMemClientsLegacy {
		err := client.Close()
		if err != nil {
			bw.logger.Warnf("%v error from closing connection for %v is %v\n", bw.bucketName, serverAddr, err)
		}
	}
	bw.kvMemClientsLegacy = make(map[string]mcc.ClientIface)
	bw.kvMemClientsLegacyMtx.Unlock()

}

func (bw *BucketTopologySvcWatcher) updateSettings(chType string, opts interface{}) error {
	switch chType {
	case HIGHSEQNOS:
		bw.setHighSeqnosInterval(opts.(HighSeqnosOpts), false)
		return nil
	case HIGHSEQNOSLEGACY:
		bw.setHighSeqnosInterval(opts.(HighSeqnosOpts), true)
		return nil
	default:
		// Nothing is to be updated
		return base.ErrorNotSupported
	}
}

func (bw *BucketTopologySvcWatcher) setHighSeqnosInterval(opts HighSeqnosOpts, legacy bool) {
	mtx := &bw.highSeqnosTrackersMtx
	subscriberToIntervalMap := bw.highSeqnosIntervals
	receiverFiredMap := bw.highSeqnosReceiverFired
	chType := HIGHSEQNOS
	if legacy {
		mtx = &bw.highSeqnosTrackersMtxLegacy
		subscriberToIntervalMap = bw.highSeqnosIntervalsLegacy
		receiverFiredMap = bw.highSeqnosReceiverFiredLegacy
		chType = HIGHSEQNOSLEGACY
	}

	mtx.Lock()
	fullKey := compileFullSubscriberId(opts.Spec, opts.SubscriberId)

	subscriberToIntervalMap[fullKey] = opts.RequestedInterval
	_, exists := receiverFiredMap[fullKey]
	if !exists {
		receiverFiredMap[fullKey] = time.NewTicker(opts.RequestedInterval)
	} else {
		receiverFiredMap[fullKey].Reset(opts.RequestedInterval)
	}

	shortestInterval := opts.RequestedInterval
	for _, currentIntervalForASubscriber := range subscriberToIntervalMap {
		if currentIntervalForASubscriber < shortestInterval {
			shortestInterval = currentIntervalForASubscriber
		}
	}
	mtx.Unlock()

	bw.setWatchersTickerIfNecessary(opts.Spec.Id, shortestInterval, chType)
}

func (bw *BucketTopologySvcWatcher) setWatchersTickerIfNecessary(specId string, shortestInterval time.Duration, chType string) {
	bw.watchersTickersMapMtx.RLock()
	defer bw.watchersTickersMapMtx.RUnlock()

	if shortestInterval < bw.watchersTickersValueMap[chType] {
		bw.watchersTickersMapMtx.RUnlock()
		bw.watchersTickersMapMtx.Lock()

		// Check again
		if shortestInterval < bw.watchersTickersValueMap[chType] {
			bw.watchersTickersValueMap[chType] = shortestInterval
			bw.watchersTickersMap[chType].Reset(shortestInterval)
			bw.logger.Infof("spec %v Setting overall ticker for %v to %v", specId, chType, shortestInterval)
		}

		bw.watchersTickersMapMtx.Unlock()
		bw.watchersTickersMapMtx.RLock()
	}
}

// Called when a subscriber has unsubscribed and its lowest value no longer applies
func (bw *BucketTopologySvcWatcher) setWatchersTickerAfterUnsubscribe(specId string, newShortestInterval time.Duration, chType string) {
	bw.watchersTickersMapMtx.Lock()
	defer bw.watchersTickersMapMtx.Unlock()

	bw.watchersTickersValueMap[chType] = newShortestInterval
	bw.watchersTickersMap[chType].Reset(newShortestInterval)
	bw.logger.Infof("spec %v Setting overall ticker for %v to %v", specId, chType, newShortestInterval)
}

func (bw *BucketTopologySvcWatcher) shouldSendToCh(name string, updateType string) bool {
	switch updateType {
	case HIGHSEQNOS:
		return bw.checkHighSeqnosReceiverAwaitingData(name, false)
	case HIGHSEQNOSLEGACY:
		return bw.checkHighSeqnosReceiverAwaitingData(name, true)
	default:
		return true
	}
}

func (bw *BucketTopologySvcWatcher) checkHighSeqnosReceiverAwaitingData(name string, legacy bool) bool {
	bw.highSeqnosTrackersMtx.RLock()
	var ticker *time.Ticker
	var found bool
	if !legacy {
		ticker, found = bw.highSeqnosReceiverFired[name]
	} else {
		ticker, found = bw.highSeqnosReceiverFiredLegacy[name]
	}
	bw.highSeqnosTrackersMtx.RUnlock()
	if !found {
		// It is raceful and possible that the timer did not stop in time during unsubscribing
		// and the timer has fired already
		return false
	}
	select {
	case <-ticker.C:
		return true
	default:
		return false
	}
}

func (bw *BucketTopologySvcWatcher) cleanupHighSeqnosInternalData(spec *metadata.ReplicationSpecification, subscriberId string, legacy bool) {
	mtx := &bw.highSeqnosTrackersMtx
	subscriberToIntervalMap := bw.highSeqnosIntervals
	receiverFiredMap := bw.highSeqnosReceiverFired
	chType := HIGHSEQNOS
	if legacy {
		mtx = &bw.highSeqnosTrackersMtxLegacy
		subscriberToIntervalMap = bw.highSeqnosIntervalsLegacy
		receiverFiredMap = bw.highSeqnosReceiverFiredLegacy
		chType = HIGHSEQNOSLEGACY
	}

	mtx.RLock()
	var shortestIntervalBeforeRemoval time.Duration
	var first = true
	for _, currentIntervalForASubscriber := range subscriberToIntervalMap {
		if first {
			shortestIntervalBeforeRemoval = currentIntervalForASubscriber
			first = false
			continue
		}
		if currentIntervalForASubscriber < shortestIntervalBeforeRemoval {
			shortestIntervalBeforeRemoval = currentIntervalForASubscriber
		}
	}
	mtx.RUnlock()

	mtx.Lock()
	fullKey := compileFullSubscriberId(spec, subscriberId)

	delete(subscriberToIntervalMap, fullKey)
	ticker, exists := receiverFiredMap[fullKey]
	if exists {
		ticker.Stop()
	}
	delete(receiverFiredMap, fullKey)

	first = true
	var shortestIntervalAfterRemoval time.Duration
	for _, currentIntervalForASubscriber := range subscriberToIntervalMap {
		if first {
			shortestIntervalAfterRemoval = currentIntervalForASubscriber
			first = false
			continue
		}
		if currentIntervalForASubscriber < shortestIntervalAfterRemoval {
			shortestIntervalAfterRemoval = currentIntervalForASubscriber
		}
	}
	mtx.Unlock()

	if shortestIntervalAfterRemoval != 0 && shortestIntervalBeforeRemoval != shortestIntervalAfterRemoval {
		bw.setWatchersTickerAfterUnsubscribe(spec.Id, shortestIntervalAfterRemoval, chType)
	}
}

func (bw *BucketTopologySvcWatcher) RegisterGarbageCollect(specId string, vbno uint16, id string, gcFunc func() error, fireDuration time.Duration) error {
	bw.gcMapMtx.Lock()
	defer bw.gcMapMtx.Unlock()
	if bw.gcMapUndergoingGc {
		return service_def.ErrorBucketTopSvcUndergoingGC
	}

	gcForSpec, specExists := bw.gcMap[specId]
	if !specExists {
		gcForSpec = make(VbnoReqMapType)
		bw.gcMap[specId] = gcForSpec
	}

	gcPruneForSpec, specExpExists := bw.gcPruneMap[specId]
	if !specExpExists {
		gcPruneForSpec = make(VbnoPruneMapType)
		bw.gcPruneMap[specId] = gcPruneForSpec
	}

	idMap, vbExists := gcForSpec[vbno]
	if !vbExists {
		idMap = make(RequestMapType)
		gcForSpec[vbno] = idMap
	}

	// Update this VB's last registered timestamp
	gcPruneForSpec[vbno] = time.Now()

	idMap[id] = &GcRequest{
		timeToFire: time.Now().Add(fireDuration),
		funcToFire: gcFunc,
	}
	return nil
}

func (bw *BucketTopologySvcWatcher) handleSpecDeletion(specId string) {
	// First see if spec exists
	bw.gcMapMtx.RLock()
	_, specExists := bw.gcMap[specId]
	_, specExpExists := bw.gcPruneMap[specId]
	bw.gcMapMtx.RUnlock()

	if !specExists && !specExpExists {
		return
	}

	bw.gcMapMtx.Lock()
	if bw.gcMapUndergoingGc {
		bw.gcMapGcAbort = true
	}
	delete(bw.gcMap, specId)
	delete(bw.gcPruneMap, specId)
	bw.gcMapMtx.Unlock()
}

type Notification struct {
	Source bool

	// Source only
	NumberOfSourceNodes        int
	SourceVBMap                base.KvVBMapType
	KvVbMap                    base.KvVBMapType
	DcpStatsMap                base.DcpStatsMapType
	DcpStatsMapLegacy          base.DcpStatsMapType
	HighSeqnoMap               base.HighSeqnosMapType
	HighSeqnoMapLegacy         base.HighSeqnosMapType
	SourceReplicaCnt           int
	SourceReplicasMap          base.VbHostsMapType  // len() of 0 if no replicas
	SourceReplicasTranslateMap base.StringStringMap // nil if not initialized
	SourceVbReplicasMember     []uint16

	// Target only
	TargetBucketUUID           string
	TargetServerVBMap          base.KvVBMapType
	TargetBucketInfo           base.BucketInfoMapType
	TargetReplicaCnt           int
	TargetReplicasMap          base.VbHostsMapType  // len() of 0 if no replicas
	TargetReplicasTranslateMap base.StringStringMap // nil if not initialized
	TargetVbReplicasMember     []uint16
}

func NewNotification(isSource bool) *Notification {
	return &Notification{
		Source:                     isSource,
		NumberOfSourceNodes:        0,
		SourceVBMap:                make(base.KvVBMapType),
		KvVbMap:                    make(base.KvVBMapType),
		DcpStatsMap:                make(base.DcpStatsMapType),
		DcpStatsMapLegacy:          make(base.DcpStatsMapType),
		HighSeqnoMap:               make(base.HighSeqnosMapType),
		HighSeqnoMapLegacy:         make(base.HighSeqnosMapType),
		SourceReplicasMap:          make(base.VbHostsMapType),
		SourceReplicasTranslateMap: make(base.StringStringMap),

		TargetServerVBMap:          make(base.KvVBMapType),
		TargetBucketInfo:           make(base.BucketInfoMapType),
		TargetReplicasMap:          make(base.VbHostsMapType),
		TargetReplicasTranslateMap: make(base.StringStringMap),
	}
}

func (n *Notification) IsSourceNotification() bool {
	return n.Source
}

func (n *Notification) CloneRO() interface{} {
	if n == nil {
		return nil
	}

	clonedRo := &Notification{}
	*clonedRo = *n
	return clonedRo
}

func (n *Notification) Clone() interface{} {
	if n == nil {
		return nil
	}
	return &Notification{
		Source:                     n.Source,
		NumberOfSourceNodes:        n.NumberOfSourceNodes,
		SourceVBMap:                n.SourceVBMap.Clone(),
		KvVbMap:                    n.KvVbMap.Clone(),
		DcpStatsMap:                n.DcpStatsMap.Clone(),
		DcpStatsMapLegacy:          n.DcpStatsMapLegacy.Clone(),
		HighSeqnoMap:               n.HighSeqnoMap.Clone(),
		HighSeqnoMapLegacy:         n.HighSeqnoMapLegacy.Clone(),
		SourceReplicaCnt:           n.SourceReplicaCnt,
		SourceReplicasMap:          n.SourceReplicasMap.Clone(),
		SourceReplicasTranslateMap: n.SourceReplicasTranslateMap.Clone(),
		SourceVbReplicasMember:     base.CloneUint16List(n.SourceVbReplicasMember),

		TargetBucketUUID:           n.TargetBucketUUID,
		TargetServerVBMap:          n.TargetServerVBMap.Clone(),
		TargetBucketInfo:           n.TargetBucketInfo.Clone(),
		TargetReplicaCnt:           n.TargetReplicaCnt,
		TargetReplicasMap:          n.TargetReplicasMap.Clone(),
		TargetReplicasTranslateMap: n.TargetReplicasTranslateMap.Clone(),
		TargetVbReplicasMember:     base.CloneUint16List(n.TargetVbReplicasMember),
	}
}

func (n *Notification) GetNumberOfSourceNodes() int {
	return n.NumberOfSourceNodes
}

func (n *Notification) GetKvVbMapRO() base.KvVBMapType {
	return n.KvVbMap
}

func (n *Notification) GetSourceVBMapRO() base.KvVBMapType {
	return n.SourceVBMap
}

func (n *Notification) GetTargetServerVBMap() base.KvVBMapType {
	return n.TargetServerVBMap
}

func (n *Notification) GetTargetBucketUUID() string {
	return n.TargetBucketUUID
}

func (n *Notification) GetTargetBucketInfo() base.BucketInfoMapType {
	return n.TargetBucketInfo
}

func (n *Notification) GetDcpStatsMap() base.DcpStatsMapType {
	return n.DcpStatsMap
}

func (n *Notification) GetDcpStatsMapLegacy() base.DcpStatsMapType {
	return n.DcpStatsMapLegacy
}

func (n *Notification) GetHighSeqnosMap() base.HighSeqnosMapType {
	return n.HighSeqnoMap
}

func (n *Notification) GetHighSeqnosMapLegacy() base.HighSeqnosMapType {
	return n.HighSeqnoMapLegacy
}

func (n *Notification) GetReplicasInfo() (int, base.VbHostsMapType, base.StringStringMap, []uint16) {
	if n.IsSourceNotification() {
		return n.SourceReplicaCnt, n.SourceReplicasMap, n.SourceReplicasTranslateMap, n.SourceVbReplicasMember
	} else {
		return n.TargetReplicaCnt, n.TargetReplicasMap, n.TargetReplicasTranslateMap, n.TargetVbReplicasMember
	}
}