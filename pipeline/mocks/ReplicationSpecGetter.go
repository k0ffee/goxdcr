// Code generated by mockery (devel). DO NOT EDIT.

package mocks

import (
	metadata "github.com/couchbase/goxdcr/metadata"
	mock "github.com/stretchr/testify/mock"
)

// ReplicationSpecGetter is an autogenerated mock type for the ReplicationSpecGetter type
type ReplicationSpecGetter struct {
	mock.Mock
}

// Execute provides a mock function with given fields: specId
func (_m *ReplicationSpecGetter) Execute(specId string) (*metadata.ReplicationSpecification, error) {
	ret := _m.Called(specId)

	var r0 *metadata.ReplicationSpecification
	if rf, ok := ret.Get(0).(func(string) *metadata.ReplicationSpecification); ok {
		r0 = rf(specId)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*metadata.ReplicationSpecification)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(specId)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}