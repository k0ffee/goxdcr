// Code generated by mockery v2.5.1. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// SpecialSettingValue is an autogenerated mock type for the SpecialSettingValue type
type SpecialSettingValue struct {
	mock.Mock
}

// SameAs provides a mock function with given fields: other
func (_m *SpecialSettingValue) SameAs(other interface{}) bool {
	ret := _m.Called(other)

	var r0 bool
	if rf, ok := ret.Get(0).(func(interface{}) bool); ok {
		r0 = rf(other)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}