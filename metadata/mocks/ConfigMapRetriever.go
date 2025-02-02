// Code generated by mockery v2.5.1. DO NOT EDIT.

package mocks

import (
	metadata "github.com/couchbase/goxdcr/metadata"
	mock "github.com/stretchr/testify/mock"
)

// ConfigMapRetriever is an autogenerated mock type for the ConfigMapRetriever type
type ConfigMapRetriever struct {
	mock.Mock
}

// Execute provides a mock function with given fields:
func (_m *ConfigMapRetriever) Execute() map[string]*metadata.SettingsConfig {
	ret := _m.Called()

	var r0 map[string]*metadata.SettingsConfig
	if rf, ok := ret.Get(0).(func() map[string]*metadata.SettingsConfig); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]*metadata.SettingsConfig)
		}
	}

	return r0
}
