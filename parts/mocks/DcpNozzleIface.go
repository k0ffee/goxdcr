// Code generated by mockery v2.5.1. DO NOT EDIT.

package mocks

import (
	common "github.com/couchbase/goxdcr/common"
	log "github.com/couchbase/goxdcr/log"

	metadata "github.com/couchbase/goxdcr/metadata"

	mock "github.com/stretchr/testify/mock"

	parts "github.com/couchbase/goxdcr/parts"
)

// DcpNozzleIface is an autogenerated mock type for the DcpNozzleIface type
type DcpNozzleIface struct {
	mock.Mock
}

// AsyncComponentEventListeners provides a mock function with given fields:
func (_m *DcpNozzleIface) AsyncComponentEventListeners() map[string]common.AsyncComponentEventListener {
	ret := _m.Called()

	var r0 map[string]common.AsyncComponentEventListener
	if rf, ok := ret.Get(0).(func() map[string]common.AsyncComponentEventListener); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]common.AsyncComponentEventListener)
		}
	}

	return r0
}

// CheckStuckness provides a mock function with given fields: dcp_stats
func (_m *DcpNozzleIface) CheckStuckness(dcp_stats map[string]map[string]string) error {
	ret := _m.Called(dcp_stats)

	var r0 error
	if rf, ok := ret.Get(0).(func(map[string]map[string]string) error); ok {
		r0 = rf(dcp_stats)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Close provides a mock function with given fields:
func (_m *DcpNozzleIface) Close() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CollectionEnabled provides a mock function with given fields:
func (_m *DcpNozzleIface) CollectionEnabled() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// Connector provides a mock function with given fields:
func (_m *DcpNozzleIface) Connector() common.Connector {
	ret := _m.Called()

	var r0 common.Connector
	if rf, ok := ret.Get(0).(func() common.Connector); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(common.Connector)
		}
	}

	return r0
}

// GetStreamState provides a mock function with given fields: vbno
func (_m *DcpNozzleIface) GetStreamState(vbno uint16) (parts.DcpStreamState, error) {
	ret := _m.Called(vbno)

	var r0 parts.DcpStreamState
	if rf, ok := ret.Get(0).(func(uint16) parts.DcpStreamState); ok {
		r0 = rf(vbno)
	} else {
		r0 = ret.Get(0).(parts.DcpStreamState)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(uint16) error); ok {
		r1 = rf(vbno)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetXattrSeqnos provides a mock function with given fields:
func (_m *DcpNozzleIface) GetXattrSeqnos() map[uint16]uint64 {
	ret := _m.Called()

	var r0 map[uint16]uint64
	if rf, ok := ret.Get(0).(func() map[uint16]uint64); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[uint16]uint64)
		}
	}

	return r0
}

// Id provides a mock function with given fields:
func (_m *DcpNozzleIface) Id() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// IsOpen provides a mock function with given fields:
func (_m *DcpNozzleIface) IsOpen() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// Logger provides a mock function with given fields:
func (_m *DcpNozzleIface) Logger() *log.CommonLogger {
	ret := _m.Called()

	var r0 *log.CommonLogger
	if rf, ok := ret.Get(0).(func() *log.CommonLogger); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*log.CommonLogger)
		}
	}

	return r0
}

// Open provides a mock function with given fields:
func (_m *DcpNozzleIface) Open() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// PrintStatusSummary provides a mock function with given fields:
func (_m *DcpNozzleIface) PrintStatusSummary() {
	_m.Called()
}

// RaiseEvent provides a mock function with given fields: event
func (_m *DcpNozzleIface) RaiseEvent(event *common.Event) {
	_m.Called(event)
}

// Receive provides a mock function with given fields: data
func (_m *DcpNozzleIface) Receive(data interface{}) error {
	ret := _m.Called(data)

	var r0 error
	if rf, ok := ret.Get(0).(func(interface{}) error); ok {
		r0 = rf(data)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RecycleDataObj provides a mock function with given fields: obj
func (_m *DcpNozzleIface) RecycleDataObj(obj interface{}) {
	_m.Called(obj)
}

// RegisterComponentEventListener provides a mock function with given fields: eventType, listener
func (_m *DcpNozzleIface) RegisterComponentEventListener(eventType common.ComponentEventType, listener common.ComponentEventListener) error {
	ret := _m.Called(eventType, listener)

	var r0 error
	if rf, ok := ret.Get(0).(func(common.ComponentEventType, common.ComponentEventListener) error); ok {
		r0 = rf(eventType, listener)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ResponsibleVBs provides a mock function with given fields:
func (_m *DcpNozzleIface) ResponsibleVBs() []uint16 {
	ret := _m.Called()

	var r0 []uint16
	if rf, ok := ret.Get(0).(func() []uint16); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]uint16)
		}
	}

	return r0
}

// SetConnector provides a mock function with given fields: connector
func (_m *DcpNozzleIface) SetConnector(connector common.Connector) error {
	ret := _m.Called(connector)

	var r0 error
	if rf, ok := ret.Get(0).(func(common.Connector) error); ok {
		r0 = rf(connector)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetMaxMissCount provides a mock function with given fields: max_dcp_miss_count
func (_m *DcpNozzleIface) SetMaxMissCount(max_dcp_miss_count int) {
	_m.Called(max_dcp_miss_count)
}

// Start provides a mock function with given fields: settings
func (_m *DcpNozzleIface) Start(settings metadata.ReplicationSettingsMap) error {
	ret := _m.Called(settings)

	var r0 error
	if rf, ok := ret.Get(0).(func(metadata.ReplicationSettingsMap) error); ok {
		r0 = rf(settings)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// State provides a mock function with given fields:
func (_m *DcpNozzleIface) State() common.PartState {
	ret := _m.Called()

	var r0 common.PartState
	if rf, ok := ret.Get(0).(func() common.PartState); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(common.PartState)
	}

	return r0
}

// Stop provides a mock function with given fields:
func (_m *DcpNozzleIface) Stop() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UnRegisterComponentEventListener provides a mock function with given fields: eventType, listener
func (_m *DcpNozzleIface) UnRegisterComponentEventListener(eventType common.ComponentEventType, listener common.ComponentEventListener) error {
	ret := _m.Called(eventType, listener)

	var r0 error
	if rf, ok := ret.Get(0).(func(common.ComponentEventType, common.ComponentEventListener) error); ok {
		r0 = rf(eventType, listener)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpdateSettings provides a mock function with given fields: settings
func (_m *DcpNozzleIface) UpdateSettings(settings metadata.ReplicationSettingsMap) error {
	ret := _m.Called(settings)

	var r0 error
	if rf, ok := ret.Get(0).(func(metadata.ReplicationSettingsMap) error); ok {
		r0 = rf(settings)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
