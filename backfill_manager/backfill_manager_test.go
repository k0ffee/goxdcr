/*
Copyright 2020-Present Couchbase, Inc.

Use of this software is governed by the Business Source License included in
the file licenses/BSL-Couchbase.txt.  As of the Change Date specified in that
file, in accordance with the Business Source License, use of this software
will be governed by the Apache License, Version 2.0, included in the file
licenses/APL2.txt.
*/

package backfill_manager

import (
	"fmt"
	"github.com/couchbase/goxdcr/base"
	"github.com/couchbase/goxdcr/common"
	"github.com/couchbase/goxdcr/metadata"
	"github.com/couchbase/goxdcr/peerToPeer"
	pipeline_mgr "github.com/couchbase/goxdcr/pipeline_manager/mocks"
	service_def_real "github.com/couchbase/goxdcr/service_def"
	service_def "github.com/couchbase/goxdcr/service_def/mocks"
	"github.com/couchbase/goxdcr/service_impl"
	"github.com/stretchr/testify/assert"
	mock "github.com/stretchr/testify/mock"
	"io/ioutil"
	"testing"
	"time"
)

func setupBoilerPlate() (*service_def.CollectionsManifestSvc, *service_def.ReplicationSpecSvc, *service_def.BackfillReplSvc, *pipeline_mgr.PipelineMgrBackfillIface, *service_def.XDCRCompTopologySvc, *service_def.CheckpointsService, *service_def.BucketTopologySvc) {
	manifestSvc := &service_def.CollectionsManifestSvc{}
	replSpecSvc := &service_def.ReplicationSpecSvc{}
	backfillReplSvc := &service_def.BackfillReplSvc{}
	pipelineMgr := &pipeline_mgr.PipelineMgrBackfillIface{}
	xdcrTopologyMock := &service_def.XDCRCompTopologySvc{}
	checkpointSvcMock := &service_def.CheckpointsService{}
	bucketTopologySvc := &service_def.BucketTopologySvc{}

	return manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrTopologyMock, checkpointSvcMock, bucketTopologySvc
}

const sourceBucketName = "sourceBucket"
const sourceBucketUUID = "sourceBucketUuid"
const targetClusterUUID = "targetClusterUuid"
const targetBucketName = "targetBucket"
const targetBucketUUID = "targetBucketUuid"

var defaultSeqnoGetter = func() map[uint16]uint64 {
	retMap := make(map[uint16]uint64)
	for i := uint16(0); i < 1024; i++ {
		retMap[i] = 100
	}
	return retMap
}

const vbsNodeName = "localhost:9000"

var vbsGetter = func(customList []uint16) *base.KvVBMapType {
	retMap := make(base.KvVBMapType)

	var list []uint16
	if customList == nil {
		for i := uint16(0); i < 1024; i++ {
			list = append(list, i)
		}
	} else {
		list = customList
	}
	retMap[vbsNodeName] = list
	return &retMap
}

func setupMock(manifestSvc *service_def.CollectionsManifestSvc, replSpecSvc *service_def.ReplicationSpecSvc, pipelineMgr *pipeline_mgr.PipelineMgrBackfillIface,
	xdcrTopologyMock *service_def.XDCRCompTopologySvc,
	checkpointSvcMock *service_def.CheckpointsService, seqnoGetter func() map[uint16]uint64, localVBMapGetter func([]uint16) *base.KvVBMapType, backfillReplSvc *service_def.BackfillReplSvc, additionalSpecIds []string, bucketTopologySvc *service_def.BucketTopologySvc) {

	returnedSpec, _ := metadata.NewReplicationSpecification(sourceBucketName, sourceBucketUUID, targetClusterUUID, targetBucketName, targetBucketUUID)
	var specList = []string{returnedSpec.Id}
	for _, extraId := range additionalSpecIds {
		specList = append(specList, extraId)
	}

	manifestSvc.On("SetMetadataChangeHandlerCallback", mock.Anything).Return(nil)
	replSpecSvc.On("SetMetadataChangeHandlerCallback", mock.Anything).Return(nil)
	replSpecSvc.On("ReplicationSpec", mock.Anything).Return(returnedSpec, nil)
	replSpecSvc.On("AllReplicationSpecIds").Return(specList, nil)
	pipelineMgr.On("GetMainPipelineThroughSeqnos", mock.Anything).Return(seqnoGetter(), nil)
	pipelineMgr.On("BackfillMappingStatusUpdate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	xdcrTopologyMock.On("MyKVNodes").Return([]string{"localhost:9000"}, nil)
	checkpointSvcMock.On("CheckpointsDocs", mock.Anything, mock.Anything).Return(nil, base.ErrorNotFound)

	sourceCh := make(chan service_def_real.SourceNotification, base.BucketTopologyWatcherChanLen)
	var vbsList []uint16
	for i := uint16(0); i < base.NumberOfVbs; i++ {
		vbsList = append(vbsList, i)
	}
	srcNotification := getDefaultSourceNotification(vbsList)
	go func() {
		for i := 0; i < 50; i++ {
			sourceCh <- srcNotification
			time.Sleep(100 * time.Millisecond)
		}
	}()
	bucketTopologySvc.On("SubscribeToLocalBucketFeed", mock.Anything, mock.Anything).Return(sourceCh, nil)
	bucketTopologySvc.On("UnSubscribeLocalBucketFeed", mock.Anything, mock.Anything).Return(nil)
	bucketTopologySvc.On("RegisterGarbageCollect", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	checkpointSvcMock.On("CheckpointsDocs", mock.Anything, mock.Anything).Return(nil, base.ErrorNotFound)
	setupBackfillReplSvcMock(backfillReplSvc)
}

var topologyObjPool = service_impl.NewBucketTopologyObjsPool()

func getDefaultSourceNotification(customVBsList []uint16) service_def_real.SourceNotification {
	notification := service_impl.NewNotification(true, topologyObjPool)
	notification.NumberOfSourceNodes = 1
	notification.SourceVBMap = vbsGetter(customVBsList)
	notification.KvVbMap = vbsGetter(customVBsList)
	notification.SetNumberOfReaders(50)
	//notification := &service_impl.Notification{
	//	Source:              true,
	//	NumberOfSourceNodes: 1,
	//	SourceVBMap:         vbsGetter(customVBsList),
	//	KvVbMap:             nil,
	//	NumReaders:          1,
	//}
	return notification
}

func setupBackfillReplSvcMock(backfillReplSvc *service_def.BackfillReplSvc) {
	backfillReplSvc.On("SetMetadataChangeHandlerCallback", mock.Anything).Return(nil)
	backfillReplSvc.On("AddBackfillReplSpec", mock.Anything).Return(nil)
	backfillReplSvc.On("SetBackfillReplSpec", mock.Anything).Return(nil)
	backfillReplSvc.On("SetCompleteBackfillRaiser", mock.Anything).Return(nil)
	backfillReplSvc.On("DelBackfillReplSpec", mock.Anything).Return(nil, nil)
}

func setupBackfillReplSvcNegMock(backfillReplSvc *service_def.BackfillReplSvc) {
	backfillReplSvc.On("SetMetadataChangeHandlerCallback", mock.Anything).Return(base.ErrorInvalidInput)
	backfillReplSvc.On("AddBackfillReplSpec", mock.Anything).Return(base.ErrorInvalidInput)
	backfillReplSvc.On("SetBackfillReplSpec", mock.Anything).Return(base.ErrorInvalidInput)
	backfillReplSvc.On("DelBackfillReplSpec", mock.Anything).Return(nil, base.ErrorInvalidInput)
	// SetCompleteBackfillRaiser for now can only return nil
	backfillReplSvc.On("SetCompleteBackfillRaiser", mock.Anything).Return(nil)
}

func setupReplStartupSpecs(replSpecSvc *service_def.ReplicationSpecSvc,
	specsToFeedBack map[string]*metadata.ReplicationSpecification) {
	replSpecSvc.On("AllReplicationSpecs").Return(specsToFeedBack, nil)
}

func setupBackfillSpecs(backfillReplSvc *service_def.BackfillReplSvc, parentSpecs map[string]*metadata.ReplicationSpecification) {
	backfillSpecs := make(map[string]*metadata.BackfillReplicationSpec)
	for specId, spec := range parentSpecs {
		vbTaskMap := metadata.NewVBTasksMap()
		bSpec := metadata.NewBackfillReplicationSpec(specId, "dummy", vbTaskMap, spec)
		backfillSpecs[specId] = bSpec
	}
	if len(backfillSpecs) == 0 {
		backfillReplSvc.On("BackfillReplSpec", mock.Anything).Return(nil, base.ErrorNotFound)
	} else {
		for bSpecId, spec := range backfillSpecs {
			backfillReplSvc.On("BackfillReplSpec", bSpecId).Return(spec, nil)
		}
	}
}

func TestBackfillMgrLaunchNoSpecs(t *testing.T) {
	assert := assert.New(t)
	fmt.Println("============== Test case start: TestBackfillMgrLaunchNoSpecs =================")
	manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc := setupBoilerPlate()
	setupReplStartupSpecs(replSpecSvc, nil)
	setupBackfillSpecs(backfillReplSvc, nil)
	setupMock(manifestSvc, replSpecSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, defaultSeqnoGetter, vbsGetter, backfillReplSvc, nil, bucketTopologySvc)

	backfillMgr := NewBackfillManager(manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc)
	assert.NotNil(backfillMgr)

	assert.Nil(backfillMgr.Start())

	fmt.Println("============== Test case end: TestBackfillMgrLaunchNoSpecs =================")
}

func getSpecId(i int) string {
	return fmt.Sprintf("RandId_%v", i)
}

// Returns a map of replId -> spec
// And returns a map of replId -> ManifestPair
func setupStartupSpecs(num int) (map[string]*metadata.ReplicationSpecification, map[string]*metadata.CollectionsManifestPair) {
	specMap := make(map[string]*metadata.ReplicationSpecification)
	colMap := make(map[string]*metadata.CollectionsManifestPair)

	for i := 0; i < num; i++ {
		specId := getSpecId(i)
		internalId := fmt.Sprintf("RandInternalId_%v", i)
		sourceBucket := fmt.Sprintf("RandSourceBucket_%v", i)
		sourceBucketUuid := fmt.Sprintf("RandSourceBucketUUID_%v", i)
		targetClusterUuid := fmt.Sprintf("targetClusterUuid_%v", i)
		targetBucketName := fmt.Sprintf("targetBucketName_%v", i)
		targetBucketUuid := fmt.Sprintf("targetBucketUuid_%v", i)

		repl := &metadata.ReplicationSpecification{
			Id:                specId,
			InternalId:        internalId,
			SourceBucketName:  sourceBucket,
			SourceBucketUUID:  sourceBucketUuid,
			TargetClusterUUID: targetClusterUuid,
			TargetBucketName:  targetBucketName,
			TargetBucketUUID:  targetBucketUuid,
			Settings:          metadata.DefaultReplicationSettings(),
		}

		specMap[specId] = repl

		defaultCollectionMap := make(metadata.CollectionsMap)
		defaultCollectionMap[base.DefaultScopeCollectionName] = metadata.Collection{0, base.DefaultScopeCollectionName}
		defaultScopeMap := make(metadata.ScopesMap)
		defaultScopeMap[base.DefaultScopeCollectionName] = metadata.Scope{0, base.DefaultScopeCollectionName, defaultCollectionMap}

		// Always let tgtManifest be +1
		srcManifest := metadata.UnitTestGenerateCollManifest(uint64(i), defaultScopeMap)
		tgtManifest := metadata.UnitTestGenerateCollManifest(uint64(i+1), defaultScopeMap)

		manifestPair := metadata.NewCollectionsManifestPair(srcManifest, tgtManifest)
		colMap[specId] = manifestPair
	}

	return specMap, colMap
}

func setupStartupManifests(manifestSvc *service_def.CollectionsManifestSvc,
	specMap map[string]*metadata.ReplicationSpecification,
	colMap map[string]*metadata.CollectionsManifestPair) {

	for replId, spec := range specMap {
		manifestPair, ok := colMap[replId]
		if ok {
			manifestSvc.On("GetLastPersistedManifests", spec).Return(manifestPair, nil)
		} else {
			manifestSvc.On("GetLastPersistedManifests", spec).Return(nil, fmt.Errorf("DummyErr"))
		}
	}
}

func TestBackfillMgrLaunchSpecs(t *testing.T) {
	assert := assert.New(t)
	fmt.Println("============== Test case start: TestBackfillMgrLaunchSpecs =================")
	manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc := setupBoilerPlate()
	specs, manifestPairs := setupStartupSpecs(5)
	setupReplStartupSpecs(replSpecSvc, specs)
	setupBackfillSpecs(backfillReplSvc, specs)
	setupStartupManifests(manifestSvc, specs, manifestPairs)
	setupMock(manifestSvc, replSpecSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, defaultSeqnoGetter, vbsGetter, backfillReplSvc, nil, bucketTopologySvc)

	backfillMgr := NewBackfillManager(manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc)
	assert.NotNil(backfillMgr)

	assert.Nil(backfillMgr.Start())

	fmt.Println("============== Test case end: TestBackfillMgrLaunchSpecs =================")
}

func TestBackfillMgrLaunchSpecsWithErr(t *testing.T) {
	assert := assert.New(t)
	fmt.Println("============== Test case start: TestBackfillMgrLaunchSpecsWithErr =================")
	manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc := setupBoilerPlate()
	specs, manifestPairs := setupStartupSpecs(5)

	// Delete the 3rd one to simulate error
	delete(manifestPairs, getSpecId(3))

	setupReplStartupSpecs(replSpecSvc, specs)
	setupBackfillSpecs(backfillReplSvc, specs)
	setupStartupManifests(manifestSvc, specs, manifestPairs)
	setupMock(manifestSvc, replSpecSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, defaultSeqnoGetter, vbsGetter, backfillReplSvc, nil, bucketTopologySvc)

	backfillMgr := NewBackfillManager(manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc)
	assert.NotNil(backfillMgr)

	assert.Nil(backfillMgr.Start())

	// The thrid one should have default manifest
	backfillMgr.cacheMtx.RLock()
	srcManifest, exists1 := backfillMgr.cacheSpecSourceMap[getSpecId(3)]
	tgtManifest, exists2 := backfillMgr.cacheSpecTargetMap[getSpecId(3)]
	backfillMgr.cacheMtx.RUnlock()
	assert.True(exists1 && exists2)
	defaultManifest := metadata.NewDefaultCollectionsManifest()
	assert.True(defaultManifest.IsSameAs(srcManifest))
	assert.True(defaultManifest.IsSameAs(tgtManifest))

	fmt.Println("============== Test case end: TestBackfillMgrLaunchSpecsWithErr =================")
}

var testDir = "../metadata/testdata/"
var targetv7 = testDir + "diffTargetv7.json"

// v9a is like v9 but minus one collection (S2:col2)
var targetv9a = testDir + "diffTargetv9a.json"

func TestBackfillMgrSourceCollectionCleanedUp(t *testing.T) {
	assert := assert.New(t)
	fmt.Println("============== Test case start: TestBackfillMgrSourceCollectionCleanedUp =================")
	defer fmt.Println("============== Test case end: TestBackfillMgrSourceCollectionCleanedUp =================")
	manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc := setupBoilerPlate()
	specs, manifestPairs := setupStartupSpecs(5)

	setupReplStartupSpecs(replSpecSvc, specs)
	setupBackfillSpecs(backfillReplSvc, specs)
	setupStartupManifests(manifestSvc, specs, manifestPairs)
	specId := "RandId_0"
	var additionalSpecIDs []string = []string{specId}
	setupMock(manifestSvc, replSpecSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, defaultSeqnoGetter, vbsGetter, backfillReplSvc, additionalSpecIDs, bucketTopologySvc)

	backfillMgr := NewBackfillManager(manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc)
	assert.NotNil(backfillMgr)

	assert.Nil(backfillMgr.Start())

	time.Sleep(1 * time.Second)

	bytes, err := ioutil.ReadFile(targetv7)
	if err != nil {
		panic(err.Error())
	}
	v7Manifest, err := metadata.NewCollectionsManifestFromBytes(bytes)
	if err != nil {
		panic(err.Error())
	}
	bytes, err = ioutil.ReadFile(targetv9a)
	if err != nil {
		panic(err.Error())
	}
	v9Manifest, err := metadata.NewCollectionsManifestFromBytes(bytes)
	if err != nil {
		panic(err.Error())
	}

	defaultManifest := metadata.NewDefaultCollectionsManifest()
	oldPair := &metadata.CollectionsManifestPair{
		Source: &defaultManifest,
		Target: &defaultManifest,
	}
	newPair := &metadata.CollectionsManifestPair{
		Source: &v7Manifest,
		Target: &v7Manifest,
	}

	// Generated ID: RandId_0
	assert.Nil(backfillMgr.collectionsManifestChangeCb(specId, oldPair, newPair))
	time.Sleep(100 * time.Nanosecond)

	handler := backfillMgr.specToReqHandlerMap[specId]
	assert.NotNil(handler)
	v7BackfillTaskMap := handler.cachedBackfillSpec.VBTasksMap.Clone()

	// Now pretend source went from v7 to v9, and S2:col2 was removed
	// Target hasn't changed
	oldPair.Source = &v7Manifest
	oldPair.Target = &v7Manifest
	newPair.Source = &v9Manifest
	newPair.Target = &v7Manifest

	assert.Nil(backfillMgr.collectionsManifestChangeCb(specId, oldPair, newPair))
	time.Sleep(100 * time.Nanosecond)
	v9BackfillTaskMap := handler.cachedBackfillSpec.VBTasksMap.Clone()

	assert.False(v7BackfillTaskMap.SameAs(v9BackfillTaskMap))

	// v9 backfillTaskMap should not contain S1:col2 as source
	checkSourceNs := &base.CollectionNamespace{
		ScopeName:      "S2",
		CollectionName: "col2",
	}
	for _, tasks := range v9BackfillTaskMap.VBTasksMap {
		mappings := tasks.GetAllCollectionNamespaceMappings()
		for _, mapping := range mappings {
			_, _, _, exists := mapping.Get(checkSourceNs, nil)
			assert.False(exists)
		}
	}

}

func TestBackfillMgrRetry(t *testing.T) {
	assert := assert.New(t)
	fmt.Println("============== Test case start: TestBackfillMgrRetry =================")
	defer fmt.Println("============== Test case end: TestBackfillMgrRetry =================")
	manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc := setupBoilerPlate()
	specs, manifestPairs := setupStartupSpecs(5)

	setupReplStartupSpecs(replSpecSvc, specs)
	setupBackfillSpecs(backfillReplSvc, specs)
	setupStartupManifests(manifestSvc, specs, manifestPairs)
	specId := "RandId_0"
	var additionalSpecIDs []string = []string{specId}
	setupMock(manifestSvc, replSpecSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, defaultSeqnoGetter, vbsGetter, backfillReplSvc, additionalSpecIDs, bucketTopologySvc)

	backfillMgr := NewBackfillManager(manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc)
	assert.NotNil(backfillMgr)
	backfillMgr.retryTimerPeriod = 500 * time.Millisecond

	assert.Nil(backfillMgr.Start())

	// add some tasks
	task1 := BackfillRetryRequest{
		replId: specId,
		req: metadata.CollectionNamespaceMappingsDiffPair{
			Added:                     nil,
			Removed:                   nil,
			RouterLatestSrcManifestId: 2,
		},
		force:                      false,
		correspondingSrcManifestId: 2,
		handler:                    nil, // will get internally
	}
	task2 := BackfillRetryRequest{
		replId: specId,
		req: metadata.CollectionNamespaceMappingsDiffPair{
			Added:                     nil,
			Removed:                   nil,
			RouterLatestSrcManifestId: 1,
		},
		force:                      false,
		correspondingSrcManifestId: 1,
		handler:                    nil, // will get internally
	}
	task3 := BackfillRetryRequest{
		replId: specId,
		req: metadata.CollectionNamespaceMappingsDiffPair{
			Added:                     nil,
			Removed:                   nil,
			RouterLatestSrcManifestId: 1,
		},
		force:                      false,
		correspondingSrcManifestId: 1,
		handler:                    nil, // will get internally
	}

	// Test to ensure that positive cases are done
	backfillMgr.retryBackfillRequest(task1)
	backfillMgr.retryBackfillRequest(task2)
	backfillMgr.retryBackfillRequest(task3)
	backfillMgr.errorRetryQMtx.RLock()
	assert.Len(backfillMgr.errorRetryQueue, 3)
	backfillMgr.errorRetryQMtx.RUnlock()

	time.Sleep(1 * time.Second)

	backfillMgr.errorRetryQMtx.RLock()
	assert.Len(backfillMgr.errorRetryQueue, 0)
	backfillMgr.errorRetryQMtx.RUnlock()

	backfillMgr.Stop()

	// Test for failure condition - new instance of backfillMgr
	_, _, backfillReplSvcBad, _, _, _, bucketTopologySvc := setupBoilerPlate()
	//setupReplStartupSpecs(replSpecSvc, specs)
	setupBackfillSpecs(backfillReplSvcBad, specs)
	setupBackfillReplSvcNegMock(backfillReplSvcBad)
	setupMock(manifestSvc, replSpecSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, defaultSeqnoGetter, vbsGetter, backfillReplSvc, additionalSpecIDs, bucketTopologySvc)

	backfillMgr = NewBackfillManager(manifestSvc, replSpecSvc, backfillReplSvcBad, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc)
	assert.NotNil(backfillMgr)
	backfillMgr.retryTimerPeriod = 500 * time.Millisecond

	assert.Nil(backfillMgr.Start())
	handler := backfillMgr.internalGetHandler(specId)
	handler.backfillReplSvc = backfillReplSvcBad

	// Test to ensure that retry fails
	backfillMgr.retryBackfillRequest(task1)
	backfillMgr.retryBackfillRequest(task2)
	backfillMgr.retryBackfillRequest(task3)
	backfillMgr.errorRetryQMtx.RLock()
	assert.Len(backfillMgr.errorRetryQueue, 3)
	backfillMgr.errorRetryQMtx.RUnlock()

	time.Sleep(1 * time.Second)

	// Not tested here - but look at the logs - there should be 2 instances of retries
	assert.Len(backfillMgr.errorRetryQueue, 3)
}

func TestBackfillMgrLaunchSpecsThenPeers(t *testing.T) {
	assert := assert.New(t)
	fmt.Println("============== Test case start: TestBackfillMgrLaunchSpecsThenPeers =================")
	defer fmt.Println("============== Test case end: TestBackfillMgrLaunchSpecsThenPeers =================")
	manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc := setupBoilerPlate()
	specs, manifestPairs := setupStartupSpecs(5)
	setupReplStartupSpecs(replSpecSvc, specs)
	setupBackfillSpecs(backfillReplSvc, specs)
	setupStartupManifests(manifestSvc, specs, manifestPairs)
	setupMock(manifestSvc, replSpecSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, defaultSeqnoGetter, vbsGetter, backfillReplSvc, nil, bucketTopologySvc)

	backfillMgr := NewBackfillManager(manifestSvc, replSpecSvc, backfillReplSvc, pipelineMgr, xdcrCompTopologySvc, checkpointSvcMock, bucketTopologySvc)
	assert.NotNil(backfillMgr)

	assert.Nil(backfillMgr.Start())

	var specIdToUse string
	var specToUse *metadata.ReplicationSpecification
	for specId, oneSpec := range specs {
		specIdToUse = specId
		specToUse = oneSpec
		break
	}

	resp := &peerToPeer.VBMasterCheckResp{
		ResponseCommon:     peerToPeer.NewResponseCommon(peerToPeer.ReqVBMasterChk, "", "", uint32(6), ""),
		ReplicationPayload: peerToPeer.NewReplicationPayload(specId, specToUse.SourceBucketName, common.MainPipeline, ""),
	}

	_, tasks0 := getTaskForVB0(specToUse.SourceBucketName)
	tasks1 := tasks0.Clone()

	vbTaskMap := metadata.NewVBTasksMap()
	vbTaskMap.VBTasksMap[0] = tasks0
	vbTaskMap.VBTasksMap[1] = tasks1
	assert.NotEqual(0, len(tasks0.GetAllCollectionNamespaceMappings()))

	resp.Init()
	resp.InitBucket(specToUse.SourceBucketName)
	bucketVBMapType, unlockFunc := resp.GetReponse()
	(*bucketVBMapType)[specToUse.SourceBucketName].RegisterNotMyVBs([]uint16{0})
	unlockFunc()
	assert.Nil(resp.LoadBackfillTasks(vbTaskMap, specToUse.SourceBucketName))
	checkResp, unlockFunc := resp.GetReponse()
	payload := (*checkResp)[specToUse.SourceBucketName]
	vb0Tasks := payload.GetBackfillVBTasks().VBTasksMap[0]
	unlockFunc()
	assert.NotEqual(0, vb0Tasks.Len())

	peersMap := make(peerToPeer.PeersVBMasterCheckRespMap)
	peersMap["dummyNode"] = resp
	settingsMap := make(metadata.ReplicationSettingsMap)
	settingsMap[base.NameKey] = specIdToUse
	settingsMap[peerToPeer.MergeBackfillKey] = peersMap
	assert.Nil(backfillMgr.GetPipelineSvc().UpdateSettings(settingsMap))

}

func getTaskForVB0(srcBucketName string) (*metadata.ReplicationSpecification, *metadata.BackfillTasks) {
	collectionNs := make(metadata.CollectionNamespaceMapping)
	ns1 := &base.CollectionNamespace{
		ScopeName:      "s1",
		CollectionName: "col1",
	}
	collectionNs.AddSingleMapping(ns1, ns1)

	emptySpec, _ := metadata.NewReplicationSpecification(srcBucketName, "", "", "", "")
	ts0 := &metadata.BackfillVBTimestamps{
		StartingTimestamp: &base.VBTimestamp{
			Vbno:  0,
			Seqno: 0,
		},
		EndingTimestamp: &base.VBTimestamp{
			Vbno:  0,
			Seqno: 1000,
		},
	}
	task0 := metadata.NewBackfillTask(ts0, []metadata.CollectionNamespaceMapping{collectionNs})
	taskList := metadata.NewBackfillTasksWithTask(task0)
	return emptySpec, &taskList
}
