// Code generated by mockery (devel). DO NOT EDIT.

package mocks

import (
	metadata "github.com/couchbase/goxdcr/metadata"
	mock "github.com/stretchr/testify/mock"

	peerToPeer "github.com/couchbase/goxdcr/peerToPeer"
)

// ReplicatorAgent is an autogenerated mock type for the ReplicatorAgent type
type ReplicatorAgent struct {
	mock.Mock
}

// GetAndClearInfoToReplicate provides a mock function with given fields:
func (_m *ReplicatorAgent) GetAndClearInfoToReplicate() (*metadata.ReplicationSpecification, *peerToPeer.VBPeriodicReplicateReq, error) {
	ret := _m.Called()

	var r0 *metadata.ReplicationSpecification
	if rf, ok := ret.Get(0).(func() *metadata.ReplicationSpecification); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*metadata.ReplicationSpecification)
		}
	}

	var r1 *peerToPeer.VBPeriodicReplicateReq
	if rf, ok := ret.Get(1).(func() *peerToPeer.VBPeriodicReplicateReq); ok {
		r1 = rf()
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*peerToPeer.VBPeriodicReplicateReq)
		}
	}

	var r2 error
	if rf, ok := ret.Get(2).(func() error); ok {
		r2 = rf()
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// SetUpdatedSpecAsync provides a mock function with given fields: spec, cbFunc
func (_m *ReplicatorAgent) SetUpdatedSpecAsync(spec *metadata.ReplicationSpecification, cbFunc func()) {
	_m.Called(spec, cbFunc)
}

// Start provides a mock function with given fields:
func (_m *ReplicatorAgent) Start() {
	_m.Called()
}

// Stop provides a mock function with given fields:
func (_m *ReplicatorAgent) Stop() {
	_m.Called()
}
