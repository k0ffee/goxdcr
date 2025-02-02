// Code generated by mockery (devel). DO NOT EDIT.

package mocks

import (
	base "github.com/couchbase/goxdcr/base"
	metadata "github.com/couchbase/goxdcr/metadata"

	mock "github.com/stretchr/testify/mock"

	pipeline "github.com/couchbase/goxdcr/pipeline"
)

// PipelineEventsManager is an autogenerated mock type for the PipelineEventsManager type
type PipelineEventsManager struct {
	mock.Mock
}

// AddEvent provides a mock function with given fields: eventType, eventDesc, eventExtras
func (_m *PipelineEventsManager) AddEvent(eventType base.EventInfoType, eventDesc string, eventExtras base.EventsMap) {
	_m.Called(eventType, eventDesc, eventExtras)
}

// BackfillUpdateCb provides a mock function with given fields: diffPair, srcManifestsDelta
func (_m *PipelineEventsManager) BackfillUpdateCb(diffPair *metadata.CollectionNamespaceMappingsDiffPair, srcManifestsDelta []*metadata.CollectionsManifest) error {
	ret := _m.Called(diffPair, srcManifestsDelta)

	var r0 error
	if rf, ok := ret.Get(0).(func(*metadata.CollectionNamespaceMappingsDiffPair, []*metadata.CollectionsManifest) error); ok {
		r0 = rf(diffPair, srcManifestsDelta)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ClearNonBrokenMapEvents provides a mock function with given fields:
func (_m *PipelineEventsManager) ClearNonBrokenMapEvents() {
	_m.Called()
}

// ContainsEvent provides a mock function with given fields: eventId
func (_m *PipelineEventsManager) ContainsEvent(eventId int) bool {
	ret := _m.Called(eventId)

	var r0 bool
	if rf, ok := ret.Get(0).(func(int) bool); ok {
		r0 = rf(eventId)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// DismissEvent provides a mock function with given fields: eventId
func (_m *PipelineEventsManager) DismissEvent(eventId int) error {
	ret := _m.Called(eventId)

	var r0 error
	if rf, ok := ret.Get(0).(func(int) error); ok {
		r0 = rf(eventId)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetCurrentEvents provides a mock function with given fields:
func (_m *PipelineEventsManager) GetCurrentEvents() *pipeline.PipelineEventList {
	ret := _m.Called()

	var r0 *pipeline.PipelineEventList
	if rf, ok := ret.Get(0).(func() *pipeline.PipelineEventList); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*pipeline.PipelineEventList)
		}
	}

	return r0
}

// LoadLatestBrokenMap provides a mock function with given fields: mapping
func (_m *PipelineEventsManager) LoadLatestBrokenMap(mapping metadata.CollectionNamespaceMapping) {
	_m.Called(mapping)
}

// ResetDismissedHistory provides a mock function with given fields:
func (_m *PipelineEventsManager) ResetDismissedHistory() {
	_m.Called()
}
