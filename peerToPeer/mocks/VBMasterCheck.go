// Code generated by mockery (devel). DO NOT EDIT.

package mocks

import (
	common "github.com/couchbase/goxdcr/common"
	mock "github.com/stretchr/testify/mock"

	peerToPeer "github.com/couchbase/goxdcr/peerToPeer"
)

// VBMasterCheck is an autogenerated mock type for the VBMasterCheck type
type VBMasterCheck struct {
	mock.Mock
}

// CheckVBMaster provides a mock function with given fields: _a0, _a1
func (_m *VBMasterCheck) CheckVBMaster(_a0 peerToPeer.BucketVBMapType, _a1 common.Pipeline) (map[string]*peerToPeer.VBMasterCheckResp, error) {
	ret := _m.Called(_a0, _a1)

	var r0 map[string]*peerToPeer.VBMasterCheckResp
	if rf, ok := ret.Get(0).(func(peerToPeer.BucketVBMapType, common.Pipeline) map[string]*peerToPeer.VBMasterCheckResp); ok {
		r0 = rf(_a0, _a1)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(map[string]*peerToPeer.VBMasterCheckResp)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(peerToPeer.BucketVBMapType, common.Pipeline) error); ok {
		r1 = rf(_a0, _a1)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
