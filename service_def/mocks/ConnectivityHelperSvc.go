// Code generated by mockery (devel). DO NOT EDIT.

package mocks

import (
	base "github.com/couchbase/goxdcr/base"
	metadata "github.com/couchbase/goxdcr/metadata"

	mock "github.com/stretchr/testify/mock"
)

// ConnectivityHelperSvc is an autogenerated mock type for the ConnectivityHelperSvc type
type ConnectivityHelperSvc struct {
	mock.Mock
}

// GetOverallStatus provides a mock function with given fields:
func (_m *ConnectivityHelperSvc) GetOverallStatus() metadata.ConnectivityStatus {
	ret := _m.Called()

	var r0 metadata.ConnectivityStatus
	if rf, ok := ret.Get(0).(func() metadata.ConnectivityStatus); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(metadata.ConnectivityStatus)
	}

	return r0
}

// MarkEncryptionError provides a mock function with given fields: val
func (_m *ConnectivityHelperSvc) MarkEncryptionError(val bool) {
	_m.Called(val)
}

// MarkIpFamilyError provides a mock function with given fields: _a0
func (_m *ConnectivityHelperSvc) MarkIpFamilyError(_a0 bool) {
	_m.Called(_a0)
}

// MarkNode provides a mock function with given fields: nodeName, status
func (_m *ConnectivityHelperSvc) MarkNode(nodeName string, status metadata.ConnectivityStatus) (bool, bool) {
	ret := _m.Called(nodeName, status)

	var r0 bool
	if rf, ok := ret.Get(0).(func(string, metadata.ConnectivityStatus) bool); ok {
		r0 = rf(nodeName, status)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 bool
	if rf, ok := ret.Get(1).(func(string, metadata.ConnectivityStatus) bool); ok {
		r1 = rf(nodeName, status)
	} else {
		r1 = ret.Get(1).(bool)
	}

	return r0, r1
}

// MarkNodeHeartbeatStatus provides a mock function with given fields: nodeName, heartbeatMap
func (_m *ConnectivityHelperSvc) MarkNodeHeartbeatStatus(nodeName string, heartbeatMap map[string]base.HeartbeatStatus) {
	_m.Called(nodeName, heartbeatMap)
}

// String provides a mock function with given fields:
func (_m *ConnectivityHelperSvc) String() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// SyncWithValidList provides a mock function with given fields: nodeList
func (_m *ConnectivityHelperSvc) SyncWithValidList(nodeList base.StringPairList) {
	_m.Called(nodeList)
}
