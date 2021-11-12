// Code generated by mockery (devel). DO NOT EDIT.

package mocks

import (
	base "github.com/couchbase/goxdcr/base"
	mock "github.com/stretchr/testify/mock"
)

// DcpStatsMapPool is an autogenerated mock type for the DcpStatsMapPool type
type DcpStatsMapPool struct {
	mock.Mock
}

// Get provides a mock function with given fields: keys
func (_m *DcpStatsMapPool) Get(keys []string) *base.DcpStatsMapType {
	ret := _m.Called(keys)

	var r0 *base.DcpStatsMapType
	if rf, ok := ret.Get(0).(func([]string) *base.DcpStatsMapType); ok {
		r0 = rf(keys)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*base.DcpStatsMapType)
		}
	}

	return r0
}

// Put provides a mock function with given fields: _a0
func (_m *DcpStatsMapPool) Put(_a0 *base.DcpStatsMapType) {
	_m.Called(_a0)
}