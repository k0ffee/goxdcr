// Code generated by mockery (devel). DO NOT EDIT.

package mocks

import (
	base "github.com/couchbase/goxdcr/base"
	metadata "github.com/couchbase/goxdcr/metadata"

	mock "github.com/stretchr/testify/mock"

	pipeline "github.com/couchbase/goxdcr/pipeline"

	service_def "github.com/couchbase/goxdcr/service_def"
)

// PipelineMgrIface is an autogenerated mock type for the PipelineMgrIface type
type PipelineMgrIface struct {
	mock.Mock
}

// AllReplicationSpecsForTargetCluster provides a mock function with given fields: targetClusterUuid
func (_m *PipelineMgrIface) AllReplicationSpecsForTargetCluster(targetClusterUuid string) map[string]*metadata.ReplicationSpecification {
	ret := _m.Called(targetClusterUuid)

	var r0 map[string]*metadata.ReplicationSpecification
	if rf, ok := ret.Get(0).(func(string) map[string]*metadata.ReplicationSpecification); ok {
		r0 = rf(targetClusterUuid)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]*metadata.ReplicationSpecification)
		}
	}

	return r0
}

// AllReplications provides a mock function with given fields:
func (_m *PipelineMgrIface) AllReplications() []string {
	ret := _m.Called()

	var r0 []string
	if rf, ok := ret.Get(0).(func() []string); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	return r0
}

// AllReplicationsForBucket provides a mock function with given fields: bucket
func (_m *PipelineMgrIface) AllReplicationsForBucket(bucket string) []string {
	ret := _m.Called(bucket)

	var r0 []string
	if rf, ok := ret.Get(0).(func(string) []string); ok {
		r0 = rf(bucket)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	return r0
}

// AllReplicationsForTargetCluster provides a mock function with given fields: targetClusterUuid
func (_m *PipelineMgrIface) AllReplicationsForTargetCluster(targetClusterUuid string) []string {
	ret := _m.Called(targetClusterUuid)

	var r0 []string
	if rf, ok := ret.Get(0).(func(string) []string); ok {
		r0 = rf(targetClusterUuid)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	return r0
}

// CheckPipelines provides a mock function with given fields:
func (_m *PipelineMgrIface) CheckPipelines() {
	_m.Called()
}

// DeletePipeline provides a mock function with given fields: pipelineName
func (_m *PipelineMgrIface) DeletePipeline(pipelineName string) error {
	ret := _m.Called(pipelineName)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(pipelineName)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DismissEventForPipeline provides a mock function with given fields: pipelineName, eventId
func (_m *PipelineMgrIface) DismissEventForPipeline(pipelineName string, eventId int) error {
	ret := _m.Called(pipelineName, eventId)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, int) error); ok {
		r0 = rf(pipelineName, eventId)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// HandleClusterEncryptionLevelChange provides a mock function with given fields: old, new
func (_m *PipelineMgrIface) HandleClusterEncryptionLevelChange(old service_def.EncryptionSettingIface, new service_def.EncryptionSettingIface) {
	_m.Called(old, new)
}

// HandlePeerCkptPush provides a mock function with given fields: fullTopic, sender, dynamicEvt
func (_m *PipelineMgrIface) HandlePeerCkptPush(fullTopic string, sender string, dynamicEvt interface{}) error {
	ret := _m.Called(fullTopic, sender, dynamicEvt)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, string, interface{}) error); ok {
		r0 = rf(fullTopic, sender, dynamicEvt)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// InitiateRepStatus provides a mock function with given fields: pipelineName
func (_m *PipelineMgrIface) InitiateRepStatus(pipelineName string) error {
	ret := _m.Called(pipelineName)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(pipelineName)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// OnExit provides a mock function with given fields:
func (_m *PipelineMgrIface) OnExit() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ReInitStreams provides a mock function with given fields: pipelineName
func (_m *PipelineMgrIface) ReInitStreams(pipelineName string) error {
	ret := _m.Called(pipelineName)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(pipelineName)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ReplicationStatus provides a mock function with given fields: topic
func (_m *PipelineMgrIface) ReplicationStatus(topic string) (pipeline.ReplicationStatusIface, error) {
	ret := _m.Called(topic)

	var r0 pipeline.ReplicationStatusIface
	if rf, ok := ret.Get(0).(func(string) pipeline.ReplicationStatusIface); ok {
		r0 = rf(topic)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(pipeline.ReplicationStatusIface)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(topic)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ReplicationStatusMap provides a mock function with given fields:
func (_m *PipelineMgrIface) ReplicationStatusMap() map[string]pipeline.ReplicationStatusIface {
	ret := _m.Called()

	var r0 map[string]pipeline.ReplicationStatusIface
	if rf, ok := ret.Get(0).(func() map[string]pipeline.ReplicationStatusIface); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]pipeline.ReplicationStatusIface)
		}
	}

	return r0
}

// UpdatePipeline provides a mock function with given fields: pipelineName, cur_err
func (_m *PipelineMgrIface) UpdatePipeline(pipelineName string, cur_err error) error {
	ret := _m.Called(pipelineName, cur_err)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, error) error); ok {
		r0 = rf(pipelineName, cur_err)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpdatePipelineWithStoppedCb provides a mock function with given fields: topic, callback, errCb
func (_m *PipelineMgrIface) UpdatePipelineWithStoppedCb(topic string, callback base.StoppedPipelineCallback, errCb base.StoppedPipelineErrCallback) error {
	ret := _m.Called(topic, callback, errCb)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, base.StoppedPipelineCallback, base.StoppedPipelineErrCallback) error); ok {
		r0 = rf(topic, callback, errCb)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
