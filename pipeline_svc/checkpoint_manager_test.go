// Copyright 2013-Present Couchbase, Inc.
//
// Use of this software is governed by the Business Source License included in
// the file licenses/BSL-Couchbase.txt.  As of the Change Date specified in that
// file, in accordance with the Business Source License, use of this software
// will be governed by the Apache License, Version 2.0, included in the file
// licenses/APL2.txt.

package pipeline_svc

import (
	"encoding/json"
	"fmt"
	mcc "github.com/couchbase/gomemcached/client"
	"github.com/couchbase/goxdcr/base"
	"github.com/couchbase/goxdcr/common"
	"github.com/couchbase/goxdcr/common/mocks"
	"github.com/couchbase/goxdcr/log"
	"github.com/couchbase/goxdcr/metadata"
	mocks2 "github.com/couchbase/goxdcr/metadata/mocks"
	service_def_real "github.com/couchbase/goxdcr/service_def"
	service_def "github.com/couchbase/goxdcr/service_def/mocks"
	"github.com/couchbase/goxdcr/utils"
	utilities "github.com/couchbase/goxdcr/utils/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io/ioutil"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCombineFailoverlogs(t *testing.T) {
	fmt.Println("============== Test case start: TestCombineFailoverLogs =================")
	defer fmt.Println("============== Test case end: TestCombineFailoverLogs =================")
	assert := assert.New(t)

	var goodVbUuid uint64 = 1
	var badVbUuid uint64 = 2
	var goodSeqno uint64 = 5
	failoverLog := &mcc.FailoverLog{[2]uint64{goodVbUuid, goodSeqno}}

	failoverLogMap := make(map[uint16]*mcc.FailoverLog)
	// 3 VBs from failoverlog
	failoverLogMap[0] = failoverLog
	failoverLogMap[1] = failoverLog
	failoverLogMap[2] = failoverLog

	goodRecord := &metadata.CheckpointRecord{
		Failover_uuid: goodVbUuid,
		Seqno:         goodSeqno,
	}
	badRecord := &metadata.CheckpointRecord{
		Failover_uuid: badVbUuid,
		Seqno:         goodSeqno,
	}

	goodDoc := &metadata.CheckpointsDoc{
		Checkpoint_records: []*metadata.CheckpointRecord{goodRecord},
	}

	badDoc := &metadata.CheckpointsDoc{
		Checkpoint_records: []*metadata.CheckpointRecord{badRecord},
	}

	mixedDoc := &metadata.CheckpointsDoc{
		Checkpoint_records: []*metadata.CheckpointRecord{goodRecord, badRecord},
	}

	checkMap := make(map[string]map[uint16]*metadata.CheckpointsDoc)
	nodeName := "node"
	checkMap[nodeName] = make(map[uint16]*metadata.CheckpointsDoc)
	checkMap[nodeName][0] = goodDoc
	checkMap[nodeName][1] = mixedDoc
	checkMap[nodeName][2] = badDoc
	checkMap[nodeName][3] = goodDoc // vb3 isn't included

	result := filterInvalidCkptsBasedOnSourceFailover(checkMap, failoverLogMap)
	assert.Len(result, 3)

	assert.Len(result[0].Checkpoint_records, 1)
	assert.Len(result[1].Checkpoint_records, 1)
	assert.Len(result[2].Checkpoint_records, 0)
}

func TestCombineFailoverlogsWithData(t *testing.T) {
	fmt.Println("============== Test case start: TestCombineFailoverLogsWithData =================")
	defer fmt.Println("============== Test case end: TestCombineFailoverLogsWithData =================")
	assert := assert.New(t)

	nodeVbCkptsMapSlice, err := ioutil.ReadFile("./unitTestdata/nodeVbCkptsMap.json")
	assert.Nil(err)
	srcFailoverLogsSlice, err := ioutil.ReadFile("./unitTestdata/srcFailoverLogs.json")
	assert.Nil(err)

	nodeVbCkptsMap := make(map[string]map[uint16]*metadata.CheckpointsDoc)
	assert.Nil(json.Unmarshal(nodeVbCkptsMapSlice, &nodeVbCkptsMap))
	srcFailoverLogs := make(map[uint16]*mcc.FailoverLog)
	assert.Nil(json.Unmarshal(srcFailoverLogsSlice, &srcFailoverLogs))

	for _, ckptMapDoc := range nodeVbCkptsMap {
		for vb, ckptDoc := range ckptMapDoc {
			assert.NotEqual(0, len(ckptDoc.Checkpoint_records))
			assert.NotNil(ckptDoc.Checkpoint_records[0])
			assert.NotNil(srcFailoverLogs[vb])
		}
	}

	filteredMap := filterInvalidCkptsBasedOnSourceFailover(nodeVbCkptsMap, srcFailoverLogs)
	for _, ckptDoc := range filteredMap {
		assert.NotEqual(0, len(ckptDoc.Checkpoint_records))
	}

	tgtFailoverLogsSlice, err := ioutil.ReadFile("./unitTestdata/tgtFailoverJson.json")
	assert.Nil(err)
	tgtFailoverLogs := make(map[uint16]*mcc.FailoverLog)
	assert.Nil(json.Unmarshal(tgtFailoverLogsSlice, &tgtFailoverLogs))
	filteredMapTgt := filterInvalidCkptsBasedOnTargetFailover(filteredMap, tgtFailoverLogs)
	for _, ckptDoc := range filteredMapTgt {
		assert.NotEqual(0, len(ckptDoc.Checkpoint_records))
	}
}

func TestCheckpointSyncHelper(t *testing.T) {
	fmt.Println("============== Test case start: TestCheckpointSyncHelper =================")
	defer fmt.Println("============== Test case end: TestCheckpointSyncHelper =================")
	assert := assert.New(t)

	helper := newCheckpointSyncHelper()

	helper.disableCkptAndWait()
	_, err := helper.registerCkptOp(false)
	assert.NotNil(err)

	helper.setCheckpointAllowed()
	idx, err := helper.registerCkptOp(false)
	idx2, err2 := helper.registerCkptOp(false)
	assert.Nil(err)
	assert.Nil(err2)

	var waitIsFinished uint32
	go func() {
		helper.disableCkptAndWait()
		atomic.StoreUint32(&waitIsFinished, 1)
	}()

	helper.markTaskDone(idx)
	time.Sleep(50 * time.Millisecond)
	assert.True(atomic.LoadUint32(&waitIsFinished) == uint32(0))
	helper.markTaskDone(idx2)
	time.Sleep(50 * time.Millisecond)
	assert.True(atomic.LoadUint32(&waitIsFinished) == uint32(1))

	assert.Len(helper.ongoingOps, 0)
}

func TestMergeNoConsensusCkpt(t *testing.T) {
	fmt.Println("============== Test case start: TestMergeNoConsensusCkpt =================")
	defer fmt.Println("============== Test case end: TestMergeNoConsensusCkpt =================")
	assert := assert.New(t)

	ckptMgr := &CheckpointManager{}

	result, err := ckptMgr.checkSpecInternalID(nil)
	assert.Nil(err)
	assert.Equal("", result)

	// Test majority
	majorityInternalId := "testInternalId"
	genSpec := &mocks2.GenericSpecification{}
	majoritySpec, err := metadata.NewReplicationSpecification("", "", "", "", "")
	majoritySpec.InternalId = majorityInternalId
	genSpec.On("GetReplicationSpec").Return(majoritySpec)

	mockPipeline := &mocks.Pipeline{}
	mockPipeline.On("Specification").Return(genSpec)
	ckptMgr.pipeline = mockPipeline

	majorityMap := make(map[string]string)
	majorityMap["node1"] = majorityInternalId
	majorityMap["node2"] = majorityInternalId
	majorityMap["node3"] = "spec232tungwoin"
	result, err = ckptMgr.checkSpecInternalID(majorityMap)
	assert.Nil(err)
	assert.Equal(majorityInternalId, result)

	// Test no concensus
	noConsensusMap := make(map[string]string)
	noConsensusMap["node1"] = "abc"
	noConsensusMap["node2"] = "def"
	noConsensusMap["node3"] = "efg"
	_, err = ckptMgr.checkSpecInternalID(noConsensusMap)
	assert.NotNil(err)
}

func TestMergEmptyCkpts(t *testing.T) {
	fmt.Println("============== Test case start: TestMergEmptyCkpts =================")
	defer fmt.Println("============== Test case end: TestMergEmptyCkpts =================")
	assert := assert.New(t)

	filteredMap := make(map[uint16]*metadata.CheckpointsDoc)
	currentDocs := make(map[uint16]*metadata.CheckpointsDoc)

	spec, _ := metadata.NewReplicationSpecification("", "", "", "", "")
	filteredMap[0] = &metadata.CheckpointsDoc{
		Checkpoint_records: nil,
		SpecInternalId:     "",
		Revision:           nil,
	}

	assert.Len(currentDocs, 0)
	combinePeerCkptDocsWithLocalCkptDoc(filteredMap, nil, nil, currentDocs, spec)
	assert.Len(currentDocs, 0)

	var recordsList metadata.CheckpointRecordsList
	record := &metadata.CheckpointRecord{}
	recordsList = append(recordsList, record)
	filteredMap[0] = &metadata.CheckpointsDoc{
		Checkpoint_records: recordsList,
		SpecInternalId:     "testId",
		Revision:           nil,
	}

	assert.Len(currentDocs, 0)
	combinePeerCkptDocsWithLocalCkptDoc(filteredMap, nil, nil, currentDocs, spec)
	assert.Len(currentDocs, 1)

	filteredMap[1] = &metadata.CheckpointsDoc{
		Checkpoint_records: recordsList,
		SpecInternalId:     "testId",
		Revision:           nil,
	}
	filteredMap[2] = &metadata.CheckpointsDoc{}
	combinePeerCkptDocsWithLocalCkptDoc(filteredMap, nil, nil, currentDocs, spec)
	assert.Len(currentDocs, 2)
}

const kvKey = "serverName"
const srcBucketName = "srcBucket"
const tgtBucketName = "tgtBucket"

var utilsReal = utils.NewUtilities()

func setupCkptMgrBoilerPlate() (*service_def.CheckpointsService, *service_def.CAPIService, *service_def.RemoteClusterSvc, *service_def.ReplicationSpecSvc, *service_def.XDCRCompTopologySvc, *service_def.ThroughSeqnoTrackerSvc, *utilities.UtilsIface, *service_def.StatsMgrIface, *service_def.UILogSvc, *service_def.CollectionsManifestSvc, *service_def.BackfillReplSvc, *service_def.BackfillMgrIface, func() service_def_real.BackfillMgrIface, *service_def.BucketTopologySvc, *metadata.ReplicationSpecification, *PipelineSupervisor) {
	ckptSvc := &service_def.CheckpointsService{}
	capiSvc := &service_def.CAPIService{}
	remoteClusterSvc := &service_def.RemoteClusterSvc{}
	replSpecSvc := &service_def.ReplicationSpecSvc{}
	xdcrTopologySvc := &service_def.XDCRCompTopologySvc{}
	throughSeqnoTrackerSvc := &service_def.ThroughSeqnoTrackerSvc{}
	utilsMock := &utilities.UtilsIface{}
	statsMgr := &service_def.StatsMgrIface{}
	uiLogSvc := &service_def.UILogSvc{}
	collectionsManifestSvc := &service_def.CollectionsManifestSvc{}
	backfillReplSvc := &service_def.BackfillReplSvc{}
	backfillMgrIface := &service_def.BackfillMgrIface{}
	getBackfillMgr := func() service_def_real.BackfillMgrIface {
		return backfillMgrIface
	}
	bucketTopologySvc := &service_def.BucketTopologySvc{}

	spec, _ := metadata.NewReplicationSpecification(srcBucketName, "", "", tgtBucketName, "")

	pipelineSupervisor := &PipelineSupervisor{}

	return ckptSvc, capiSvc, remoteClusterSvc, replSpecSvc, xdcrTopologySvc, throughSeqnoTrackerSvc,
		utilsMock, statsMgr, uiLogSvc, collectionsManifestSvc, backfillReplSvc, backfillMgrIface, getBackfillMgr,
		bucketTopologySvc, spec, pipelineSupervisor
}

func setupBackfillPipelineMock(spec *metadata.ReplicationSpecification, supervisor *PipelineSupervisor, task *metadata.BackfillTask) *mocks.Pipeline {
	runtimeCtx := &mocks.PipelineRuntimeContext{}
	runtimeCtx.On("Service", base.PIPELINE_SUPERVISOR_SVC).Return(supervisor)

	taskMap := make(map[uint16]*metadata.BackfillTasks)
	tasks := &metadata.BackfillTasks{}
	tasks.List = append(tasks.List, task)
	taskMap[0] = tasks
	vbTaskMap := metadata.NewVBTasksMap()
	vbTaskMap.VBTasksMap = taskMap
	backfillSpec := metadata.NewBackfillReplicationSpec(spec.Id, spec.InternalId, vbTaskMap, spec)

	genericSpec := &mocks2.GenericSpecification{}
	genericSpec.On("GetReplicationSpec").Return(spec)
	genericSpec.On("GetBackfillSpec").Return(backfillSpec)

	pipeline := &mocks.Pipeline{}
	pipeline.On("Type").Return(common.BackfillPipeline)
	pipeline.On("Sources").Return(nil)
	pipeline.On("Specification").Return(genericSpec)
	pipeline.On("Topic").Return("pipelineTopic")
	pipeline.On("FullTopic").Return("backfill_pipelineTopic")
	pipeline.On("RuntimeContext").Return(runtimeCtx)
	pipeline.On("InstanceId").Return("randomInstance")

	pipeline.On("UpdateSettings", mock.Anything).Return(nil)

	return pipeline
}

func getBackfillTask() *metadata.BackfillTask {
	namespaceMapping := make(metadata.CollectionNamespaceMapping)
	defaultNamespace := &base.CollectionNamespace{base.DefaultScopeCollectionName, base.DefaultScopeCollectionName}
	namespaceMapping.AddSingleMapping(defaultNamespace, defaultNamespace)
	manifestsIdPair := base.CollectionsManifestIdPair{0, 0}

	ts0 := &metadata.BackfillVBTimestamps{
		StartingTimestamp: &base.VBTimestamp{0, 0, 5, 5, 5, manifestsIdPair},
		EndingTimestamp:   &base.VBTimestamp{0, 0, 500, 500, 500, manifestsIdPair},
	}

	vb0Task0 := metadata.NewBackfillTask(ts0, []metadata.CollectionNamespaceMapping{namespaceMapping})
	vb0Tasks := metadata.NewBackfillTasks()
	vb0Tasks.List = append(vb0Tasks.List, vb0Task0)

	return vb0Task0
}
func setupMock(ckptSvc *service_def.CheckpointsService, capiSvc *service_def.CAPIService,
	remClusterSvc *service_def.RemoteClusterSvc, replSpecSvc *service_def.ReplicationSpecSvc,
	xdcrCompTopologySvc *service_def.XDCRCompTopologySvc, throughSeqnoSvc *service_def.ThroughSeqnoTrackerSvc,
	utils *utilities.UtilsIface, statsMgr *service_def.StatsMgrIface, uilogSvc *service_def.UILogSvc,
	colManSvc *service_def.CollectionsManifestSvc, backfillReplSvc *service_def.BackfillReplSvc,
	backfillMgr *service_def.BackfillMgrIface, bucketTopologySvc *service_def.BucketTopologySvc,
	spec *metadata.ReplicationSpecification, supervisor *PipelineSupervisor) {

	bucketInfoFile := "../utils/testInternalData/pools_default_buckets_b2.json"
	bucketInfoData, err := ioutil.ReadFile(bucketInfoFile)
	if err != nil {
		panic(err)
	}
	bucketMap := make(map[string]interface{})
	err = json.Unmarshal(bucketInfoData, &bucketMap)
	if err != nil {
		panic(err)
	}

	getRemoteServerVBMapFunc := func(string, string, map[string]interface{}, bool) map[string][]uint16 {
		vbMap, _ := utilsReal.GetServerVBucketsMap("", "b2", bucketMap, nil, nil)
		return *vbMap
	}
	getNilFunc := func(string, string, map[string]interface{}, bool) error {
		return nil
	}

	getCompatibilityFunc := func(map[string]interface{}, *log.CommonLogger) int {
		intVal, _ := utilsReal.GetClusterCompatibilityFromBucketInfo(bucketMap, nil)
		return intVal
	}
	getNilFunc2 := func(map[string]interface{}, *log.CommonLogger) error {
		return nil
	}

	remClusterSvc.On("ShouldUseAlternateAddress", mock.Anything).Return(false, nil)
	utils.On("GetBucketInfo", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(bucketMap, nil)
	utils.On("GetRemoteServerVBucketsMap", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(getRemoteServerVBMapFunc, getNilFunc)
	utils.On("GetClusterCompatibilityFromBucketInfo", mock.Anything, mock.Anything).Return(getCompatibilityFunc, getNilFunc2)
	utils.On("GetNodeListFromInfoMap", mock.Anything, mock.Anything).Return(base.GetNodeListFromInfoMap(bucketMap, nil))
	utils.On("GetHostAddrFromNodeInfo", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("localhost", nil)

	capiSvc.On("PreReplicate", mock.Anything, mock.Anything, mock.Anything).Return(true, nil, nil)

	statsMgr.On("SetVBCountMetrics", mock.Anything, mock.Anything).Return(nil)

	throughSeqnoSvc.On("SetStartSeqno", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		timeStampMtx.Lock()
		timeStampSetSeqno = args.Get(1).(uint64)
		timeStampSetCnt++
		timeStampMtx.Unlock()
	}).Return(nil)
}

var timeStampSetSeqno uint64
var timeStampSetCnt int
var timeStampMtx sync.RWMutex

func TestBackfillVBTaskResume(t *testing.T) {
	fmt.Println("============== Test case start: TestBackfillVBTaskResume =================")
	defer fmt.Println("============== Test case end: TestBackfillVBTaskResume =================")
	assert := assert.New(t)

	ckptSvc, capiSvc, remoteClusterSvc, replSpecSvc, xdcrTopologySvc, throughSeqnoTrackerSvc, utils, statsMgr, uiLogSvc, collectionsManifestSvc, backfillReplSvc, backfillMgrIface, getBackfillMgr, bucketTopologySvc, spec, pipelineSupervisor := setupCkptMgrBoilerPlate()

	activeVBs := make(map[string][]uint16)
	activeVBs[kvKey] = []uint16{0}
	targetRef, _ := metadata.NewRemoteClusterReference("", "C2", "", "", "",
		"", false, "", nil, nil, nil, nil)

	setupMock(ckptSvc, capiSvc, remoteClusterSvc, replSpecSvc, xdcrTopologySvc, throughSeqnoTrackerSvc,
		utils, statsMgr, uiLogSvc, collectionsManifestSvc, backfillReplSvc, backfillMgrIface,
		bucketTopologySvc, spec, pipelineSupervisor)

	ckptMgr, err := NewCheckpointManager(ckptSvc, capiSvc, remoteClusterSvc, replSpecSvc, xdcrTopologySvc,
		throughSeqnoTrackerSvc, activeVBs, "", "", "", nil,
		targetRef, nil, utils, statsMgr, uiLogSvc, collectionsManifestSvc, backfillReplSvc,
		getBackfillMgr, bucketTopologySvc)

	assert.Nil(err)
	assert.NotNil(ckptMgr)
	ckptMgr.unitTest = true

	backfillTask := getBackfillTask()
	backfillPipeline := setupBackfillPipelineMock(spec, pipelineSupervisor, backfillTask)
	assert.Nil(ckptMgr.Attach(backfillPipeline))

	assert.Len(ckptMgr.backfillStartingTs, 1)

	// Let's mock some ckpt docs for vb0
	ckptDocs := make(map[uint16]*metadata.CheckpointsDoc)
	records := metadata.CheckpointRecordsList{}
	record := &metadata.CheckpointRecord{
		Seqno:                  1000,
		Dcp_snapshot_seqno:     1000,
		Dcp_snapshot_end_seqno: 1000,
		Target_Seqno:           1000,
	}
	record2 := &metadata.CheckpointRecord{
		Seqno:                  500,
		Dcp_snapshot_seqno:     500,
		Dcp_snapshot_end_seqno: 500,
		Target_Seqno:           500,
	}
	records = append(records, record)
	records = append(records, record2)
	ckptDoc := &metadata.CheckpointsDoc{
		Checkpoint_records: records,
		SpecInternalId:     spec.InternalId,
		Revision:           nil,
	}
	ckptDocs[0] = ckptDoc

	var waitGrp sync.WaitGroup
	waitGrp.Add(1)
	errCh := make(chan interface{}, 1)
	var lowestManifestId uint64
	var vbtsStartedNon0 uint32
	ckptMgr.startSeqnoGetter(0, []uint16{0}, ckptDocs, &waitGrp, errCh, &lowestManifestId, &vbtsStartedNon0)
	waitGrp.Wait()

	timeStampMtx.RLock()
	assert.Equal(1, timeStampSetCnt)
	assert.Equal(500, int(timeStampSetSeqno))
	timeStampMtx.RUnlock()
}
