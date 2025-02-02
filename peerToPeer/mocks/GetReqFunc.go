// Code generated by mockery (devel). DO NOT EDIT.

package mocks

import (
	peerToPeer "github.com/couchbase/goxdcr/peerToPeer"
	mock "github.com/stretchr/testify/mock"
)

// GetReqFunc is an autogenerated mock type for the GetReqFunc type
type GetReqFunc struct {
	mock.Mock
}

// Execute provides a mock function with given fields: source, target
func (_m *GetReqFunc) Execute(source string, target string) peerToPeer.Request {
	ret := _m.Called(source, target)

	var r0 peerToPeer.Request
	if rf, ok := ret.Get(0).(func(string, string) peerToPeer.Request); ok {
		r0 = rf(source, target)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(peerToPeer.Request)
		}
	}

	return r0
}
