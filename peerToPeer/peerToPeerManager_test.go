// Copyright 2021-Present Couchbase, Inc.
//
// Use of this software is governed by the Business Source License included in
// the file licenses/BSL-Couchbase.txt.  As of the Change Date specified in that
// file, in accordance with the Business Source License, use of this software
// will be governed by the Apache License, Version 2.0, included in the file
// licenses/APL2.txt.

package peerToPeer

import (
	"fmt"
	"github.com/couchbase/goxdcr/metadata"
	service_def_real "github.com/couchbase/goxdcr/service_def"
	service_def "github.com/couchbase/goxdcr/service_def/mocks"
	"github.com/couchbase/goxdcr/utils"
	utilsMock2 "github.com/couchbase/goxdcr/utils/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"net/http"
	"testing"
	"time"
)

func setupBoilerPlate() (*service_def.XDCRCompTopologySvc, *utilsMock2.UtilsIface, *service_def.BucketTopologySvc, *service_def.ReplicationSpecSvc, *utils.Utilities, []error, []int, []string, string, chan service_def_real.SourceNotification, *service_def.CheckpointsService, *service_def.BackfillReplSvc, *service_def.CollectionsManifestSvc, *service_def.SecuritySvc) {
	xdcrComp := &service_def.XDCRCompTopologySvc{}
	utilsMock := &utilsMock2.UtilsIface{}
	bucketTopSvc := &service_def.BucketTopologySvc{}
	replSpecSvc := &service_def.ReplicationSpecSvc{}
	utilsReal := utils.NewUtilities()
	ckptSvc := &service_def.CheckpointsService{}
	backfillReplSvc := &service_def.BackfillReplSvc{}
	colManifestSvc := &service_def.CollectionsManifestSvc{}
	securityMock := &service_def.SecuritySvc{}

	queryResultErrs := []error{nil, nil}
	queryResultsStatusCode := []int{http.StatusOK, http.StatusOK}
	peerNodes := []string{"10.1.1.1:8091", "10.2.2.2:8091"}
	myHostAddr := "127.0.0.1:8091"
	srcCh := make(chan service_def_real.SourceNotification, 50)
	return xdcrComp, utilsMock, bucketTopSvc, replSpecSvc, utilsReal, queryResultErrs, queryResultsStatusCode, peerNodes, myHostAddr, srcCh, ckptSvc, backfillReplSvc, colManifestSvc, securityMock
}

func setupMocks(utilsMock *utilsMock2.UtilsIface, utilsReal *utils.Utilities, xdcrComp *service_def.XDCRCompTopologySvc, peerNodes []string, myAddr string, specList []*metadata.ReplicationSpecification, replSpecSvc *service_def.ReplicationSpecSvc, queryErrs []error, queryStatuses []int, srcCh chan service_def_real.SourceNotification, bucketSvc *service_def.BucketTopologySvc, ckptSvc *service_def.CheckpointsService, backfillReplSvc *service_def.BackfillReplSvc, collectionsManifestSvc *service_def.CollectionsManifestSvc, securitySvc *service_def.SecuritySvc) {
	utilsMock.On("ExponentialBackoffExecutor", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		utilsReal.ExponentialBackoffExecutor(args.Get(0).(string), args.Get(1).(time.Duration), args.Get(2).(int), args.Get(3).(int), args.Get(4).(utils.ExponentialOpFunc))
	}).Return(nil)

	for i, peerNodeAddr := range peerNodes {
		utilsMock.On("QueryRestApiWithAuth", peerNodeAddr, mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
			mock.Anything).Return(queryErrs[i], queryStatuses[i])
	}

	xdcrComp.On("PeerNodesAdminAddrs").Return(peerNodes, nil)
	xdcrComp.On("MyHostAddr").Return(myAddr, nil)

	retMap := make(map[string]*metadata.ReplicationSpecification)
	for _, spec := range specList {
		retMap[spec.Id] = spec
		replSpecSvc.On("ReplicationSpecReadOnly", spec.Id).Return(spec, nil)
	}
	replSpecSvc.On("AllReplicationSpecs").Return(retMap, nil)

	bucketSvc.On("SubscribeToLocalBucketFeed", mock.Anything, mock.Anything).Return(srcCh, nil)
	bucketSvc.On("UnSubscribeLocalBucketFeed", mock.Anything, mock.Anything).Return(nil)

	securitySvc.On("IsClusterEncryptionLevelStrict").Return(false)
}

func TestPeerToPeerMgrSendVBCheck(t *testing.T) {
	fmt.Println("============== Test case start: TestPeerToPeerMgrSendVBCheck =================")
	defer fmt.Println("============== Test case end: TestPeerToPeerMgrSendVBCheck =================")
	assert := assert.New(t)

	bucketName := "bucketName"
	spec, _ := metadata.NewReplicationSpecification(bucketName, "", "", "", "")
	specList := []*metadata.ReplicationSpecification{spec}

	peerNodes := []string{"10.1.1.1:8091", "10.2.2.2:8091"}
	myHostAddr := "127.0.0.1:8091"

	queryResultErrs := []error{nil, nil}
	queryResultsStatusCode := []int{http.StatusOK, http.StatusOK}

	xdcrComp, utilsMock, bucketSvc, replSvc, utilsReal, queryResultErrs, queryResultsStatusCode, peerNodes, myHostAddr, srcCh, ckptSvc, backfillReplSvc, colManifestSvc, securitySvc := setupBoilerPlate()
	setupMocks(utilsMock, utilsReal, xdcrComp, peerNodes, myHostAddr, specList, replSvc, queryResultErrs, queryResultsStatusCode, srcCh, bucketSvc, ckptSvc, backfillReplSvc, colManifestSvc, securitySvc)

	dummyMerger := func(string, string, interface{}) error { return nil }
	mgr, err := NewPeerToPeerMgr(nil, xdcrComp, utilsMock, bucketSvc, replSvc, 100*time.Millisecond, ckptSvc, colManifestSvc, backfillReplSvc, securitySvc)
	assert.Nil(err)
	assert.NotNil(mgr)
	mgr.SetPushReqMergerOnce(dummyMerger)
	commAPI, err := mgr.Start()
	assert.NotNil(commAPI)
	assert.Nil(err)

	bucketMap := make(BucketVBMapType)
	bucketMap[bucketName] = []uint16{0, 1}

	filteredSubsets, err := mgr.vbMasterCheckHelper.GetUnverifiedSubset(bucketMap)
	assert.Nil(err)

	getReqFunc := func(src, tgt string) Request {
		var opaque uint32
		if tgt == peerNodes[0] {
			opaque = uint32(0)
		} else if tgt == peerNodes[1] {
			opaque = uint32(1)
		} else {
			panic("Invalid func")
		}
		common := NewRequestCommon(src, tgt, "", "", opaque)
		vbCheckReq := NewVBMasterCheckReq(common)
		vbCheckReq.SetBucketVBMap(filteredSubsets)
		return vbCheckReq
	}

	var responses []*VBMasterCheckResp
	for _, peerNode := range peerNodes {
		req := getReqFunc(myHostAddr, peerNode)
		var reqIface interface{} = req
		vbMasterCheckReq := reqIface.(*VBMasterCheckReq)
		resp := vbMasterCheckReq.GenerateResponse().(*VBMasterCheckResp)
		newMap := make(BucketVBMPayloadType)
		resp.payload = &newMap
		(*resp.payload)[bucketName] = &VBMasterPayload{
			OverallPayloadErr: "",
			NotMyVBs:          NewVBsPayload([]uint16{0, 1}),
			ConflictingVBs:    nil,
		}
		responses = append(responses, resp)
	}

	opts := NewSendOpts(true)
	err = mgr.sendToEachPeerOnce(ReqVBMasterChk, getReqFunc, opts)
	assert.Nil(err)

	// Now find the opaques
	handler, found := mgr.receiveHandlers[ReqVBMasterChk]
	assert.True(found)
	assert.NotNil(handler)

	// recast
	var handlerIface interface{} = handler
	vbMasterCheckHandler, ok := handlerIface.(*VBMasterCheckHandler)
	assert.True(ok)
	assert.NotNil(vbMasterCheckHandler)

	vbMasterCheckHandler.opaqueMapMtx.RLock()
	assert.Len(vbMasterCheckHandler.opaqueReqRespCbMap, 2)
	assert.Len(vbMasterCheckHandler.opaqueMap, 2)
	assert.Len(vbMasterCheckHandler.opaqueReqMap, 2)
	vbMasterCheckHandler.opaqueMapMtx.RUnlock()

	for _, resp := range responses {
		vbMasterCheckHandler.receiveCh <- resp
	}

	results, _ := opts.GetResults()
	tgt1Result, found := results[peerNodes[0]]
	tgt2Result, found2 := results[peerNodes[0]]
	assert.True(found)
	assert.True(found2)
	assert.NotNil(tgt1Result.ReqPtr)
	assert.NotNil(tgt1Result.RespPtr)
	assert.NotNil(tgt2Result.ReqPtr)
	assert.NotNil(tgt2Result.RespPtr)
	checkResp := tgt1Result.RespPtr.(*VBMasterCheckResp)
	assert.Len((*checkResp.payload), 1)
	notMyVbs := (*checkResp.payload)[bucketName].NotMyVBs
	assert.Len(*notMyVbs, 2)
	checkResp = tgt2Result.RespPtr.(*VBMasterCheckResp)
	assert.Len((*checkResp.payload), 1)
	notMyVbs = (*checkResp.payload)[bucketName].NotMyVBs
	assert.Len(*notMyVbs, 2)

	time.Sleep(150 * time.Millisecond)
	vbMasterCheckHandler.opaqueMapMtx.RLock()
	assert.Len(vbMasterCheckHandler.opaqueReqRespCbMap, 0)
	assert.Len(vbMasterCheckHandler.opaqueMap, 0)
	assert.Len(vbMasterCheckHandler.opaqueReqMap, 0)
	vbMasterCheckHandler.opaqueMapMtx.RUnlock()

}

func TestPeerToPeerConcurrentMap(t *testing.T) {
	fmt.Println("============== Test case start: TestPeerToPeerConcurrentMap =================")
	defer fmt.Println("============== Test case end: TestPeerToPeerConcurrentMap =================")
	assert := assert.New(t)

	for i := 0; i < 50; i++ {
		opts := NewSendOpts(true)
		opts.timeout = 25 * time.Millisecond
		opts.respMapMtx.Lock()
		ch1 := make(chan ReqRespPair)
		opts.respMap["testhost1"] = ch1
		ch2 := make(chan ReqRespPair)
		opts.respMap["testhost2"] = ch2
		ch3 := make(chan ReqRespPair)
		opts.respMap["testhost3"] = ch3
		opts.respMapMtx.Unlock()
		_, errMap := opts.GetResults()
		assert.Len(errMap, 3)
	}
}