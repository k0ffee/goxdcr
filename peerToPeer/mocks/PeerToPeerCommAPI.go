// Code generated by mockery (devel). DO NOT EDIT.

package mocks

import (
	peerToPeer "github.com/couchbase/goxdcr/peerToPeer"
	mock "github.com/stretchr/testify/mock"
)

// PeerToPeerCommAPI is an autogenerated mock type for the PeerToPeerCommAPI type
type PeerToPeerCommAPI struct {
	mock.Mock
}

// P2PReceive provides a mock function with given fields: reqOrResp
func (_m *PeerToPeerCommAPI) P2PReceive(reqOrResp peerToPeer.ReqRespCommon) (peerToPeer.HandlerResult, error) {
	ret := _m.Called(reqOrResp)

	var r0 peerToPeer.HandlerResult
	if rf, ok := ret.Get(0).(func(peerToPeer.ReqRespCommon) peerToPeer.HandlerResult); ok {
		r0 = rf(reqOrResp)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(peerToPeer.HandlerResult)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(peerToPeer.ReqRespCommon) error); ok {
		r1 = rf(reqOrResp)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// P2PSend provides a mock function with given fields: req
func (_m *PeerToPeerCommAPI) P2PSend(req peerToPeer.Request) (peerToPeer.HandlerResult, error) {
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
