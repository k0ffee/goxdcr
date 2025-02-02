// Copyright 2021-Present Couchbase, Inc.
//
// Use of this software is governed by the Business Source License included in
// the file licenses/BSL-Couchbase.txt.  As of the Change Date specified in that
// file, in accordance with the Business Source License, use of this software
// will be governed by the Apache License, Version 2.0, included in the file
// licenses/APL2.txt.

package peerToPeer

import (
	"fmt"
	"github.com/couchbase/goxdcr/base"
	"github.com/couchbase/goxdcr/service_def"
	"github.com/couchbase/goxdcr/utils"
	"net/http"
)

var ErrorInvalidOpcode = fmt.Errorf("Invalid Opcode")
var ErrorReceiveChanFull = fmt.Errorf("Opcode receiver channel is full")
var ErrorLifecycleMismatch = fmt.Errorf("Lifecycle mismatch")

type OpCode int

type ReqRespType int

type ReqRespCommon interface {
	Serialize() ([]byte, error)
	DeSerialize([]byte) error
	GetOpcode() OpCode
	GetType() ReqRespType
	GetSender() string
	GetOpaque() uint32
}

type Request interface {
	ReqRespCommon
	CallBack(resp Response) (HandlerResult, error)
	GetTarget() string
	SameAs(other interface{}) (bool, error)
	GenerateResponse() interface{}
}

type Response interface {
	ReqRespCommon
}

type HandlerResult interface {
	GetError() error
	GetHttpStatusCode() int
}

type P2PSendType func(req Request) (HandlerResult, error)

type PeerToPeerCommAPI interface {
	P2PReceive(reqOrResp ReqRespCommon) (HandlerResult, error)
	P2PSend(req Request) (HandlerResult, error)
}

type P2pCommAPIimpl struct {
	receiveChs map[OpCode]chan interface{}
	utils      utils.UtilsIface

	xdcrCompTopSvc service_def.XDCRCompTopologySvc
	securitySvc    service_def.SecuritySvc
}

func NewP2pCommAPIHelper(receiveChs map[OpCode]chan interface{}, utils utils.UtilsIface, xdcrCompTopSvc service_def.XDCRCompTopologySvc, securitySvc service_def.SecuritySvc) *P2pCommAPIimpl {
	return &P2pCommAPIimpl{
		receiveChs:     receiveChs,
		utils:          utils,
		xdcrCompTopSvc: xdcrCompTopSvc,
		securitySvc:    securitySvc,
	}
}

func (p2p *P2pCommAPIimpl) P2PReceive(req ReqRespCommon) (HandlerResult, error) {
	result := &HandlerResultImpl{}

	reqType := req.GetOpcode()

	receiveCh, found := p2p.receiveChs[reqType]
	if !found {
		result.Err = ErrorInvalidOpcode
		return result, ErrorInvalidOpcode
	}

	select {
	case receiveCh <- req:
		return result, nil
	default:
		result.Err = ErrorReceiveChanFull
	}

	return result, ErrorReceiveChanFull
}

func (p2p *P2pCommAPIimpl) P2PSend(req Request) (HandlerResult, error) {
	payload, err := req.Serialize()
	if err != nil {
		return nil, err
	}

	authType := base.HttpAuthMechPlain
	var certificates []byte
	if p2p.securitySvc.IsClusterEncryptionLevelStrict() {
		authType = base.HttpAuthMechHttps
		certificates = p2p.securitySvc.GetCACertificates()
		if len(certificates) == 0 {
			return &HandlerResultImpl{
				Err:            base.ErrorNilCertificate,
				HttpStatusCode: http.StatusInternalServerError,
			}, base.ErrorNilCertificateStrictMode
		}
	}

	var out interface{}
	err, statusCode := p2p.utils.QueryRestApiWithAuth(req.GetTarget(), base.XDCRPeerToPeerPath, false, "", "", authType, certificates, true, nil, nil, base.MethodPost, base.JsonContentType,
		payload, base.P2PCommTimeout, &out, nil, false, nil)
	result := &HandlerResultImpl{HttpStatusCode: statusCode, Err: err}
	return result, err
}
