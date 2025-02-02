// Code generated by mockery (devel). DO NOT EDIT.

package mocks

import (
	peerToPeer "github.com/couchbase/goxdcr/peerToPeer"
	mock "github.com/stretchr/testify/mock"
)

// P2PSendType is an autogenerated mock type for the P2PSendType type
type P2PSendType struct {
	mock.Mock
}

// Execute provides a mock function with given fields: req
func (_m *P2PSendType) Execute(req peerToPeer.Request) (peerToPeer.HandlerResult, error) {
	ret := _m.Called(req)

	var r0 peerToPeer.HandlerResult
	if rf, ok := ret.Get(0).(func(peerToPeer.Request) peerToPeer.HandlerResult); ok {
		r0 = rf(req)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(peerToPeer.HandlerResult)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(peerToPeer.Request) error); ok {
		r1 = rf(req)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
