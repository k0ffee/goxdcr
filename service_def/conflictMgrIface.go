package service_def

import (
	"github.com/couchbase/goxdcr/base"
)

type ConflictManagerIface interface {
	ResolveConflict(source *base.WrappedMCRequest, target *base.SubdocLookupResponse, sourceId, targetId []byte) error
	SetBackToSource(input *base.ConflictParams) error
}
