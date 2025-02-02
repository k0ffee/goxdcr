// Code generated by mockery (devel). DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// ThroughputThrottlerSvc is an autogenerated mock type for the ThroughputThrottlerSvc type
type ThroughputThrottlerSvc struct {
	mock.Mock
}

// CanSend provides a mock function with given fields: isHighPriorityReplication
func (_m *ThroughputThrottlerSvc) CanSend(isHighPriorityReplication bool) bool {
	ret := _m.Called(isHighPriorityReplication)

	var r0 bool
	if rf, ok := ret.Get(0).(func(bool) bool); ok {
		r0 = rf(isHighPriorityReplication)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// Start provides a mock function with given fields:
func (_m *ThroughputThrottlerSvc) Start() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Stop provides a mock function with given fields:
func (_m *ThroughputThrottlerSvc) Stop() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpdateSettings provides a mock function with given fields: setting
func (_m *ThroughputThrottlerSvc) UpdateSettings(setting map[string]interface{}) map[string]error {
	ret := _m.Called(setting)

	var r0 map[string]error
	if rf, ok := ret.Get(0).(func(map[string]interface{}) map[string]error); ok {
		r0 = rf(setting)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]error)
		}
	}

	return r0
}

// Wait provides a mock function with given fields:
func (_m *ThroughputThrottlerSvc) Wait() {
	_m.Called()
}
