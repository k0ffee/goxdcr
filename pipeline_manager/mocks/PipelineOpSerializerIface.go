package mocks

import (
	pipeline "github.com/couchbase/goxdcr/pipeline"
	mock "github.com/stretchr/testify/mock"
)

// PipelineOpSerializerIface is an autogenerated mock type for the PipelineOpSerializerIface type
type PipelineOpSerializerIface struct {
	mock.Mock
}

// Delete provides a mock function with given fields: topic
func (_m *PipelineOpSerializerIface) Delete(topic string) error {
	ret := _m.Called(topic)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(topic)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetOrCreateReplicationStatus provides a mock function with given fields: topic, cur_err
func (_m *PipelineOpSerializerIface) GetOrCreateReplicationStatus(topic string, cur_err error) (*pipeline.ReplicationStatus, error) {
	ret := _m.Called(topic, cur_err)

	var r0 *pipeline.ReplicationStatus
	if rf, ok := ret.Get(0).(func(string, error) *pipeline.ReplicationStatus); ok {
		r0 = rf(topic, cur_err)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*pipeline.ReplicationStatus)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, error) error); ok {
		r1 = rf(topic, cur_err)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Init provides a mock function with given fields: topic
func (_m *PipelineOpSerializerIface) Init(topic string) error {
	ret := _m.Called(topic)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(topic)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Pause provides a mock function with given fields: topic
func (_m *PipelineOpSerializerIface) Pause(topic string) error {
	ret := _m.Called(topic)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(topic)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ReInit provides a mock function with given fields: topic
func (_m *PipelineOpSerializerIface) ReInit(topic string) error {
	ret := _m.Called(topic)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(topic)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// StartBackfill provides a mock function with given fields: topic
func (_m *PipelineOpSerializerIface) StartBackfill(topic string) error {
	ret := _m.Called(topic)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(topic)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Stop provides a mock function with given fields:
func (_m *PipelineOpSerializerIface) Stop() {
	_m.Called()
}

// StopBackfill provides a mock function with given fields: topic
func (_m *PipelineOpSerializerIface) StopBackfill(topic string) error {
	ret := _m.Called(topic)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(topic)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Update provides a mock function with given fields: topic, err
func (_m *PipelineOpSerializerIface) Update(topic string, err error) error {
	ret := _m.Called(topic, err)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, error) error); ok {
		r0 = rf(topic, err)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
