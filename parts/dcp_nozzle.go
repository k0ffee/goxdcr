// Copyright (c) 2013 Couchbase, Inc.
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
// except in compliance with the License. You may obtain a copy of the License at
//   http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed under the
// License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
// either express or implied. See the License for the specific language governing permissions
// and limitations under the License.

package parts

import (
	"encoding/binary"
	"errors"
	"fmt"
	mc "github.com/couchbase/gomemcached"
	mcc "github.com/couchbase/gomemcached/client"
	base "github.com/couchbase/goxdcr/base"
	common "github.com/couchbase/goxdcr/common"
	gen_server "github.com/couchbase/goxdcr/gen_server"
	"github.com/couchbase/goxdcr/log"
	"github.com/couchbase/goxdcr/service_def"
	"github.com/couchbase/goxdcr/simple_utils"
	utilities "github.com/couchbase/goxdcr/utils"
	"math"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// start settings key name
	DCP_VBTimestamp         = "VBTimestamps"
	DCP_VBTimestampUpdater  = "VBTimestampUpdater"
	DCP_Connection_Prefix   = "xdcr:"
	EVENT_DCP_DISPATCH_TIME = "dcp_dispatch_time"
	EVENT_DCP_DATACH_LEN    = "dcp_datach_length"
	DCP_Stats_Interval      = "stats_interval"
)

type DcpStreamState int

const (
	Dcp_Stream_NonInit = iota
	Dcp_Stream_Init    = iota
	Dcp_Stream_Active  = iota
)

var dcp_inactive_stream_check_interval = 30 * time.Second

var dcp_setting_defs base.SettingDefinitions = base.SettingDefinitions{DCP_VBTimestamp: base.NewSettingDef(reflect.TypeOf((*map[uint16]*base.VBTimestamp)(nil)), false)}

var ErrorEmptyVBList = errors.New("Invalid configuration for DCP nozzle. VB list cannot be empty.")

var MaxCountStreamsInactive uint32 = 40

type vbtsWithLock struct {
	ts   *base.VBTimestamp
	lock *sync.RWMutex
}

type streamStatusWithLock struct {
	state DcpStreamState
	lock  *sync.RWMutex
}

/**
 * DCP Rollback Handshake Helper. See MB-25647 for handshake sequence design
 */
type dcpStreamReqHelper struct {
	// Internal copy of vbno
	vbno uint16
	// Internal interface of dcp nozzle to access outer wrapper methods
	dcp DcpNozzleIface

	// Locks all internals except for currentVersionWell
	lock sync.RWMutex
	// Keeps track of messages sent. Key is version of number, Value is the seqno for the vbucket
	sentMsgs map[uint16]uint64
	// Keeps track of the seqno that has been ACK'ed. Key is the seqno, and value is whether or not it's been ack'ed
	ackedMsgs map[uint64]bool

	// Current version counter well - monotonously increasing - use atomics and not lock
	currentVersionWell uint64
}

func (reqHelper *dcpStreamReqHelper) initialize() {
	reqHelper.sentMsgs = make(map[uint16]uint64)
	reqHelper.ackedMsgs = make(map[uint64]bool)
	atomic.StoreUint64(&reqHelper.currentVersionWell, 0)
}

func (reqHelper *dcpStreamReqHelper) isStreamActive() bool {
	status, err := reqHelper.dcp.GetStreamState(reqHelper.vbno)
	if err != nil {
		reqHelper.dcp.Logger().Errorf("Invalid Stream state for vbno: %v even though helper for vb exists.", reqHelper.vbno)
		return false
	}
	return (status == Dcp_Stream_Active)
}

func (reqHelper *dcpStreamReqHelper) getNewVersion() uint16 {
	var newVersion uint64

	newVersion = atomic.AddUint64(&reqHelper.currentVersionWell, 1)

	if newVersion > math.MaxUint16 {
		errStr := fmt.Sprintf("Error: dcpStreamHelper for vbno: %v internal version overflow", reqHelper.vbno)
		reqHelper.dcp.Logger().Errorf(errStr)
		reqHelper.dcp.RaiseEvent(common.NewEvent(common.ErrorEncountered, nil, nil, nil, errStr))
		atomic.StoreUint64(&reqHelper.currentVersionWell, 0)
		newVersion = 0
	}

	return uint16(newVersion)
}

// Gets the number of versions that has not been ack'ed
func (reqHelper *dcpStreamReqHelper) getNumberOfOutstandingReqs() int {
	reqHelper.lock.RLock()
	defer reqHelper.lock.RUnlock()
	// Find number of un-ack'ed msgs
	var count int
	for _, seqno := range reqHelper.sentMsgs {
		if !reqHelper.ackedMsgs[seqno] {
			count++
		}
	}
	return count
}

// Write lock must be held
func (reqHelper *dcpStreamReqHelper) deregisterRequestNoLock(version uint16) {
	seqno, ok := reqHelper.sentMsgs[version]
	if ok {
		// Mark that this seqno has been ack'ed
		reqHelper.ackedMsgs[seqno] = true
	}
}

/**
 * Register a sent request into the map for book-keeping
 */
func (reqHelper *dcpStreamReqHelper) registerRequest(version uint16, seqno uint64) (alreadyAcked bool) {
	reqHelper.lock.Lock()
	defer reqHelper.lock.Unlock()
	if reqHelper.isStreamActive() {
		alreadyAcked = true
	} else {
		alreadyAcked = reqHelper.ackedMsgs[seqno]
	}

	if !alreadyAcked {
		reqHelper.sentMsgs[version] = seqno
	}
	return
}

/**
 * Processes a rollback response.
 * Returns error if there's any issues with look-ups
 * Returns a bool to represent whether or not to ignore this response
 */
func (reqHelper *dcpStreamReqHelper) processRollbackResponse(version uint16) (ignoreResponse bool, helperErr error) {
	reqHelper.lock.Lock()
	defer reqHelper.lock.Unlock()

	// default to not ignore response
	ignoreResponse = false
	var acked bool

	if reqHelper.isStreamActive() {
		// If the vb stream is active already, ignore all rollback requests
		ignoreResponse = true
	} else {
		// Check to see if this seqno has been rejected "sent back a ROLLBACK" before. If so, then this is already handled.
		seqno, seqok := reqHelper.sentMsgs[version]
		if !seqok {
			helperErr = errors.New(fmt.Sprintf("Received a ROLLBACK message for vbno=%v with version=%v, but was never sent.",
				reqHelper.vbno, version))
			reqHelper.dcp.Logger().Warnf(helperErr.Error())
			ignoreResponse = true
			return
		}
		acked = reqHelper.ackedMsgs[seqno]

		if acked {
			ignoreResponse = true
		}

		if reqHelper.sentMsgs[version] == 0 {
			// It is weird that we sent out a rollbackseqno of 0 to DCP and it tells us to rollback again.
			// This should not happen. Restart pipeline. It has the same effect of panic where XDCR DCP nozzles restart.
			helperErr = errors.New(fmt.Sprintf("Received a ROLLBACK message for vbno=%v with seqno=%v, last sent was 0",
				reqHelper.vbno, reqHelper.sentMsgs[version]))
			reqHelper.dcp.RaiseEvent(common.NewEvent(common.ErrorEncountered, nil, nil, nil, helperErr))
		}
	}

	if !acked {
		reqHelper.deregisterRequestNoLock(version)
	}
	return
}

/**
 * When a success is returned, this helper will remove the recorded response and make future
 * rollback operations no-op.
 * All history is reset until a registerRequest is called again
 */
func (reqHelper *dcpStreamReqHelper) processSuccessResponse(version uint16) {
	reqHelper.lock.Lock()
	defer reqHelper.lock.Unlock()
	if reqHelper.isStreamActive() {
		// reset stats
		reqHelper.initialize()
	}
}

type DcpNozzleIface interface {
	CheckStuckness(dcp_stats map[string]map[string]string) error
	Close() error
	GetStreamState(vbno uint16) (DcpStreamState, error)
	GetVBList() []uint16
	GetXattrSeqnos() map[uint16]uint64
	IsOpen() bool
	Open() error
	Receive(data interface{}) error
	SetMaxMissCount(max_dcp_miss_count int)
	Start(settings map[string]interface{}) error
	Stop() error
	StatusSummary() string
	SetVBList(vbnos []uint16) error
	UpdateSettings(settings map[string]interface{}) error

	// Embedded from GenServer
	Logger() *log.CommonLogger
	// Embedded from AbstractPart
	RaiseEvent(event *common.Event)
}

/************************************
/* struct DcpNozzle
*************************************/
type DcpNozzle struct {

	//parent inheritance
	gen_server.GenServer
	AbstractPart

	// the list of vbuckets that the dcp nozzle is responsible for
	// this allows multiple  dcp nozzles to be created for a kv node
	vbnos []uint16

	// key - vb#
	// value - first seqno seen with xattr
	vb_xattr_seqno_map map[uint16]*uint64

	vb_stream_status map[uint16]*streamStatusWithLock

	// immutable fields
	sourceBucketName string
	targetBucketName string
	client           mcc.ClientIface
	uprFeed          mcc.UprFeedIface
	// lock on uprFeed to avoid race condition
	lock_uprFeed sync.RWMutex

	finch chan bool

	bOpen      bool
	lock_bOpen sync.RWMutex

	childrenWaitGrp sync.WaitGroup

	counter_received uint32
	counter_sent     uint32
	// the counter_received stats from last dcp check
	counter_received_last uint32

	// the number of check intervals after which dcp still has inactive streams
	// inactive streams will be restarted after this count exceeds MaxCountStreamsInactive
	counter_streams_inactive uint32

	start_time          time.Time
	handle_error        bool
	cur_ts              map[uint16]*vbtsWithLock
	vbtimestamp_updater func(uint16, uint64) (*base.VBTimestamp, error)

	// the number of times that the dcp nozzle did not receive anything from dcp when there are
	// items remaining in dcp
	// dcp is considered to be stuck and pipeline broken when this number reaches a limit
	dcp_miss_count     int
	max_dcp_miss_count int

	// Each vb stream has its own helper to help with DCP handshaking
	vbHandshakeMap map[uint16]*dcpStreamReqHelper

	xdcr_topology_svc service_def.XDCRCompTopologySvc

	stats_interval           time.Duration
	stats_interval_change_ch chan bool

	user_agent string

	is_capi bool

	utils utilities.UtilsIface
}

func NewDcpNozzle(id string,
	sourceBucketName, targetBucketName string,
	vbnos []uint16,
	xdcr_topology_svc service_def.XDCRCompTopologySvc,
	is_capi bool,
	logger_context *log.LoggerContext,
	utilsIn utilities.UtilsIface) *DcpNozzle {

	//callback functions from GenServer
	var msg_callback_func gen_server.Msg_Callback_Func
	var exit_callback_func gen_server.Exit_Callback_Func
	var error_handler_func gen_server.Error_Handler_Func

	server := gen_server.NewGenServer(&msg_callback_func,
		&exit_callback_func, &error_handler_func, logger_context, "DcpNozzle", utilsIn)
	part := NewAbstractPartWithLogger(id, server.Logger())

	dcp := &DcpNozzle{
		sourceBucketName:         sourceBucketName,
		targetBucketName:         targetBucketName,
		vbnos:                    vbnos,
		vb_xattr_seqno_map:       make(map[uint16]*uint64),
		GenServer:                server, /*gen_server.GenServer*/
		AbstractPart:             part,   /*AbstractPart*/
		bOpen:                    true,   /*bOpen	bool*/
		lock_bOpen:               sync.RWMutex{},
		childrenWaitGrp:          sync.WaitGroup{}, /*childrenWaitGrp sync.WaitGroup*/
		lock_uprFeed:             sync.RWMutex{},
		cur_ts:                   make(map[uint16]*vbtsWithLock),
		vb_stream_status:         make(map[uint16]*streamStatusWithLock),
		xdcr_topology_svc:        xdcr_topology_svc,
		stats_interval_change_ch: make(chan bool, 1),
		is_capi:                  is_capi,
		utils:                    utilsIn,
		vbHandshakeMap:           make(map[uint16]*dcpStreamReqHelper),
	}

	msg_callback_func = nil
	exit_callback_func = dcp.onExit
	error_handler_func = dcp.handleGeneralError

	for _, vbno := range vbnos {
		dcp.cur_ts[vbno] = &vbtsWithLock{lock: &sync.RWMutex{}, ts: nil}
		dcp.vb_stream_status[vbno] = &streamStatusWithLock{lock: &sync.RWMutex{}, state: Dcp_Stream_NonInit}
		if !dcp.is_capi {
			var xattr_seqno uint64 = 0
			dcp.vb_xattr_seqno_map[vbno] = &xattr_seqno
		}
	}

	dcp.composeUserAgent()

	dcp.Logger().Debugf("Constructed Dcp nozzle %v with vblist %v\n", dcp.Id(), vbnos)

	return dcp

}

func (dcp *DcpNozzle) composeUserAgent() {
	dcp.user_agent = simple_utils.ComposeUserAgentWithBucketNames("Goxdcr Dcp ", dcp.sourceBucketName, dcp.targetBucketName)
}

func (dcp *DcpNozzle) initialize(settings map[string]interface{}) (err error) {
	dcp.finch = make(chan bool)

	dcp.initializeUprHandshakeHelpers()

	addr, err := dcp.xdcr_topology_svc.MyMemcachedAddr()
	if err != nil {
		return err
	}

	dcp.client, err = dcp.utils.GetMemcachedConnection(addr, dcp.sourceBucketName, dcp.user_agent, dcp.Logger())
	if err != nil {
		return err
	}

	// xdcr will send ack to upr feed
	dcp.uprFeed, err = dcp.client.NewUprFeedWithConfigIface(true /*ackByClient*/)
	if err != nil {
		return err
	}

	randName, err := simple_utils.GenerateRandomId(base.LengthOfRandomId, base.MaxRetryForRandomIdGeneration)
	if err != nil {
		return err
	}

	uprFeedName := DCP_Connection_Prefix + dcp.Id() + ":" + randName

	if dcp.is_capi {
		// no need to enable xattr for capi replication
		err = dcp.uprFeed.UprOpen(uprFeedName, uint32(0), base.UprFeedBufferSize)
	} else {
		// always enable xattr for xmem replication
		// even if target cluster does not support xattr, we still need to get xattr data type from dcp
		// for source side conflict resolution
		err = dcp.uprFeed.UprOpenWithXATTR(uprFeedName, uint32(0), base.UprFeedBufferSize)
	}

	if err != nil {
		dcp.Logger().Errorf("%v upr open failed. err=%v.\n", dcp.Id(), err)
		return err
	}

	// fetch start timestamp from settings
	dcp.vbtimestamp_updater = settings[DCP_VBTimestampUpdater].(func(uint16, uint64) (*base.VBTimestamp, error))

	if val, ok := settings[DCP_Stats_Interval]; ok {
		dcp.stats_interval = time.Duration(val.(int)) * time.Millisecond
	} else {
		return errors.New("setting 'stats_interval' is missing")
	}

	return
}

func (dcp *DcpNozzle) initializeUprHandshakeHelpers() {
	vbList := dcp.GetVBList()

	for _, vb := range vbList {
		dcp.vbHandshakeMap[vb] = &dcpStreamReqHelper{vbno: vb, dcp: dcp}
		dcp.vbHandshakeMap[vb].initialize()
	}
}

func (dcp *DcpNozzle) Open() error {
	dcp.lock_bOpen.Lock()
	defer dcp.lock_bOpen.Unlock()
	if !dcp.bOpen {
		dcp.bOpen = true

	}
	return nil
}

func (dcp *DcpNozzle) Close() error {
	dcp.lock_bOpen.Lock()
	defer dcp.lock_bOpen.Unlock()
	if dcp.bOpen {
		dcp.bOpen = false
	}
	return nil
}

/**
 * Start routine initializes the DCP client, gen server, and launches go routines on various
 * monitors.
 */
func (dcp *DcpNozzle) Start(settings map[string]interface{}) error {
	dcp.Logger().Infof("Dcp nozzle %v starting ....\n", dcp.Id())

	err := dcp.SetState(common.Part_Starting)
	if err != nil {
		return err
	}

	err = dcp.utils.ValidateSettings(dcp_setting_defs, settings, dcp.Logger())
	if err != nil {
		return err
	}

	dcp.Logger().Infof("%v starting ....\n", dcp.Id())
	err = dcp.initialize(settings)
	if err != nil {
		return err
	}
	dcp.Logger().Infof("%v has been initialized\n", dcp.Id())

	// start gen_server
	dcp.start_time = time.Now()
	err = dcp.Start_server()
	if err != nil {
		return err
	}

	//start datachan length stats collection
	dcp.childrenWaitGrp.Add(1)
	go dcp.collectDcpDataChanLen(settings)

	uprFeed := dcp.getUprFeed()
	if uprFeed != nil {
		uprFeed.StartFeedWithConfig(base.UprFeedDataChanLength)
	}

	// start data processing routine
	dcp.childrenWaitGrp.Add(1)
	go dcp.processData()

	// start vbstreams
	dcp.childrenWaitGrp.Add(1)
	go dcp.startUprStreams()

	// check for inactive vbstreams
	dcp.childrenWaitGrp.Add(1)
	go dcp.checkInactiveUprStreams()

	err = dcp.SetState(common.Part_Running)

	if err == nil {
		dcp.Logger().Infof("%v has been started", dcp.Id())
	} else {
		dcp.Logger().Errorf("%v failed to start. err=%v", dcp.Id(), err)
	}

	return err
}

func (dcp *DcpNozzle) Stop() error {
	dcp.Logger().Infof("%v is stopping...\n", dcp.Id())
	err := dcp.SetState(common.Part_Stopping)
	if err != nil {
		return err
	}

	//notify children routines
	if dcp.finch != nil {
		close(dcp.finch)
	}

	dcp.closeUprStreams()
	dcp.closeUprFeed()

	err = dcp.client.Close()
	if err != nil {
		dcp.Logger().Warnf("%v Error closing dcp client. err=%v\n", dcp.Id(), err)
	}

	dcp.Logger().Debugf("%v received %v items, sent %v items\n", dcp.Id(), dcp.counterReceived(), dcp.counterSent())

	err = dcp.Stop_server()
	// Wait for all go-routines to exit before cleaning up helpers
	dcp.cleanUpProcessDataHelpers()

	err = dcp.SetState(common.Part_Stopped)
	if err != nil {
		return err
	}
	dcp.Logger().Infof("%v has been stopped\n", dcp.Id())
	return err

}

func (dcp *DcpNozzle) cleanUpProcessDataHelpers() {
	for vbno, helper := range dcp.vbHandshakeMap {
		if helper != nil {
			helper.dcp = nil
		}
		delete(dcp.vbHandshakeMap, vbno)
	}
}

func (dcp *DcpNozzle) closeUprStreams() error {
	dcp.lock_uprFeed.Lock()
	defer dcp.lock_uprFeed.Unlock()

	if dcp.uprFeed != nil {
		dcp.Logger().Infof("%v Closing dcp streams for vb=%v\n", dcp.Id(), dcp.GetVBList())
		errMap := make(map[uint16]error)

		for _, vbno := range dcp.GetVBList() {
			stream_state, err := dcp.GetStreamState(vbno)
			if err != nil {
				return err
			}
			if stream_state == Dcp_Stream_Active {
				err := dcp.uprFeed.CloseStream(vbno, dcp.vbHandshakeMap[vbno].getNewVersion())
				if err != nil {
					errMap[vbno] = err
				}
			} else {
				dcp.Logger().Infof("%v There is no active stream for vb=%v\n", dcp.Id(), vbno)
			}
		}

		if len(errMap) > 0 {
			msg := fmt.Sprintf("Failed to close upr streams, err=%v\n", errMap)
			dcp.Logger().Errorf("%v %v", dcp.Id(), msg)
			return errors.New(msg)
		}
	} else {
		dcp.Logger().Infof("%v uprfeed is already closed. No-op", dcp.Id())
	}
	return nil
}

func (dcp *DcpNozzle) closeUprFeed() bool {
	var actionTaken = false

	dcp.lock_uprFeed.Lock()
	defer dcp.lock_uprFeed.Unlock()
	if dcp.uprFeed != nil {
		dcp.Logger().Infof("%v Ask uprfeed to close", dcp.Id())
		//in the process of stopping, no need to report any error to replication manager anymore
		dcp.handle_error = false

		dcp.uprFeed.Close()
		dcp.uprFeed = nil
		actionTaken = true
	} else {
		dcp.Logger().Infof("%v uprfeed is already closed. No-op", dcp.Id())
	}

	return actionTaken
}

func (dcp *DcpNozzle) IsOpen() bool {
	dcp.lock_bOpen.RLock()
	defer dcp.lock_bOpen.RUnlock()
	return dcp.bOpen
}

func (dcp *DcpNozzle) Receive(data interface{}) error {
	// DcpNozzle is a source nozzle and does not receive from upstream nodes
	return nil
}

// Handles any UPR event coming in from the UPR feed channel
func (dcp *DcpNozzle) processData() (err error) {
	dcp.Logger().Infof("%v processData starts..........\n", dcp.Id())
	defer dcp.childrenWaitGrp.Done()
	defer dcp.Logger().Infof("%v processData exits\n", dcp.Id())

	finch := dcp.finch
	uprFeed := dcp.getUprFeed()
	if uprFeed == nil {
		dcp.Logger().Infof("%v DCP feed has been closed. processData exits\n", dcp.Id())
		return
	}

	// GetUprEventCh() wraps the channel supplied that sends in uprEvents
	// mutch is of type UprEvent, located in gomemcached/client/upr_feed.go
	mutch := uprFeed.GetUprEventCh()
	for {
		select {
		case <-finch:
			goto done
		case m, ok := <-mutch: // mutation from upstream
			if !ok {
				dcp.Logger().Infof("%v DCP mutation channel has been closed.Stop dcp nozzle now.", dcp.Id())
				//close uprFeed
				dcp.closeUprFeed()
				dcp.handleGeneralError(errors.New("DCP upr feed has been closed."))
				goto done
			}

			// increment ack bytes in uprfeed, which is necessary for uprfeed flow control to work
			uprFeed.IncrementAckBytes(m.AckSize)

			if m.Opcode == mc.UPR_STREAMREQ {
				// This is a reply coming back from dcp.uprFeed.UprRequestStream(), which triggers UPR_STREAMREQ to the producer
				// See: https://github.com/couchbaselabs/dcp-documentation/blob/master/documentation/commands/stream-request.md
				if m.Status == mc.NOT_MY_VBUCKET {
					vb_err := fmt.Errorf("Received error %v on vb %v\n", base.ErrorNotMyVbucket, m.VBucket)
					dcp.Logger().Errorf("%v %v", dcp.Id(), vb_err)
					dcp.handleVBError(m.VBucket, vb_err)
				} else if m.Status == mc.ROLLBACK {
					rollbackseq := binary.BigEndian.Uint64(m.Value[:8])
					vbno := m.VBucket

					// Process the rollback message to see if this is something we should ignore
					ignoreResponse, helperErr := dcp.vbHandshakeMap[vbno].processRollbackResponse(m.Opaque)
					if helperErr != nil {
						dcp.RaiseEvent(common.NewEvent(common.ErrorEncountered, m, dcp, nil, helperErr))
					}

					if !ignoreResponse {
						//need to request the uprstream for the vbucket again
						updated_ts, err := dcp.vbtimestamp_updater(vbno, rollbackseq)
						if err != nil {
							err = fmt.Errorf("Failed to request dcp stream after receiving roll-back for vb=%v. err=%v\n", vbno, err)
							dcp.Logger().Errorf("%v %v", dcp.Id(), err)
							dcp.handleGeneralError(err)
							return err
						}
						err = dcp.setTS(vbno, updated_ts, true)
						if err != nil {
							err = fmt.Errorf("Failed to update start seqno for vb=%v. err=%v\n", vbno, err)
							dcp.Logger().Errorf("%v %v", dcp.Id(), err)
							dcp.handleGeneralError(err)
							return err

						}
						dcp.startUprStream(vbno, updated_ts)
					}
				} else if m.Status == mc.SUCCESS {
					vbno := m.VBucket
					_, ok := dcp.vb_stream_status[vbno]
					if ok {
						dcp.setStreamState(vbno, Dcp_Stream_Active)
						dcp.RaiseEvent(common.NewEvent(common.StreamingStart, m, dcp, nil, nil))
						dcp.vbHandshakeMap[vbno].processSuccessResponse(m.Opaque)
					} else {
						panic(fmt.Sprintf("Stream for vb=%v is not supposed to be opened\n", vbno))
					}
				}

			} else if m.Opcode == mc.UPR_STREAMEND {
				// Sent to the consumer to indicate that the producer has no more messages to stream for the specified vbucket.
				// https://github.com/couchbaselabs/dcp-documentation/blob/master/documentation/commands/stream-end.md
				vbno := m.VBucket
				stream_status, err := dcp.GetStreamState(vbno)
				if err == nil && stream_status == Dcp_Stream_Active {
					err_streamend := fmt.Errorf("dcp stream for vb=%v is closed by producer", m.VBucket)
					dcp.Logger().Infof("%v: %v", dcp.Id(), err_streamend)
					dcp.handleVBError(vbno, err_streamend)
				}

			} else {
				// Regular mutations coming in from DCP stream
				if dcp.IsOpen() {
					switch m.Opcode {
					case mc.UPR_MUTATION, mc.UPR_DELETION, mc.UPR_EXPIRATION:
						// https://github.com/couchbaselabs/dcp-documentation/blob/master/documentation/commands/mutation.md
						// https://github.com/couchbaselabs/dcp-documentation/blob/master/documentation/commands/deletion.md
						// https://github.com/couchbaselabs/dcp-documentation/blob/master/documentation/commands/expiration.md
						start_time := time.Now()
						dcp.incCounterReceived()
						dcp.RaiseEvent(common.NewEvent(common.DataReceived, m, dcp, nil /*derivedItems*/, nil /*otherInfos*/))
						if !dcp.is_capi {
							dcp.handleXattr(m)
						}

						// forward mutation downstream through connector
						if err := dcp.Connector().Forward(m); err != nil {
							dcp.handleGeneralError(err)
							goto done
						}
						dcp.incCounterSent()
						// raise event for statistics collection
						dispatch_time := time.Since(start_time)
						dcp.RaiseEvent(common.NewEvent(common.DataProcessed, m, dcp, nil /*derivedItems*/, dispatch_time.Seconds()*1000000 /*otherInfos*/))
					case mc.UPR_SNAPSHOT:
						dcp.RaiseEvent(common.NewEvent(common.SnapshotMarkerReceived, m, dcp, nil /*derivedItems*/, nil /*otherInfos*/))
					default:
						dcp.Logger().Debugf("%v Uprevent OpCode=%v, is skipped\n", dcp.Id(), m.Opcode)
					}
				}
			}
		}
	}
done:
	return
}

func (dcp *DcpNozzle) handleXattr(upr_event *mcc.UprEvent) {
	event_has_xattr := simple_utils.HasXattr(upr_event.DataType)
	if event_has_xattr {
		xattr_seqno_obj, ok := dcp.vb_xattr_seqno_map[upr_event.VBucket]
		if ok {
			xattr_seqno := atomic.LoadUint64(xattr_seqno_obj)
			if xattr_seqno == 0 {
				// set xattr_seqno only if it has never been set before
				atomic.StoreUint64(xattr_seqno_obj, upr_event.Seqno)
			}
		}
	}
}

func (dcp *DcpNozzle) GetXattrSeqnos() map[uint16]uint64 {
	xattr_seqnos := make(map[uint16]uint64)
	for vbno, xattr_seqno_obj := range dcp.vb_xattr_seqno_map {
		xattr_seqnos[vbno] = atomic.LoadUint64(xattr_seqno_obj)
	}
	return xattr_seqnos
}

func (dcp *DcpNozzle) onExit() {
	dcp.childrenWaitGrp.Wait()

}

func (dcp *DcpNozzle) StatusSummary() string {
	msg := fmt.Sprintf("%v received %v items, sent %v items.", dcp.Id(), dcp.counterReceived(), dcp.counterSent())
	streams_inactive := dcp.inactiveDcpStreamsWithState()
	if len(streams_inactive) > 0 {
		msg += fmt.Sprintf(" streams inactive: %v", streams_inactive)
	}
	return msg
}

func (dcp *DcpNozzle) handleGeneralError(err error) {

	err1 := dcp.SetState(common.Part_Error)
	if err1 == nil {
		dcp.RaiseEvent(common.NewEvent(common.ErrorEncountered, nil, dcp, nil, err))
		dcp.Logger().Errorf("%v Raise error condition %v\n", dcp.Id(), err)
	} else {
		dcp.Logger().Debugf("%v in shutdown process. err=%v is ignored\n", dcp.Id(), err)
	}
}

func (dcp *DcpNozzle) handleVBError(vbno uint16, err error) {
	additionalInfo := &base.VBErrorEventAdditional{vbno, err, base.VBErrorType_Source}
	dcp.RaiseEvent(common.NewEvent(common.VBErrorEncountered, nil, dcp, nil, additionalInfo))
}

// start steam request will be sent when starting seqno is negotiated, it may take a few
func (dcp *DcpNozzle) startUprStreams() error {
	defer dcp.childrenWaitGrp.Done()

	var err error = nil
	dcp.Logger().Infof("%v: startUprStreams for %v...\n", dcp.Id(), dcp.GetVBList())

	init_ch := make(chan bool, 1)
	init_ch <- true

	finch := dcp.finch

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-finch:
			goto done
		case <-init_ch:
			err = dcp.startUprStreams_internal(dcp.GetVBList())
			if err != nil {
				return err
			}
		case <-ticker.C:
			streams_non_init := dcp.nonInitDcpStreams()
			if len(streams_non_init) == 0 {
				goto done
			}
			err = dcp.startUprStreams_internal(streams_non_init)
			if err != nil {
				return err
			}
		}
	}
done:
	dcp.Logger().Infof("%v: all dcp stream have been initialized.\n", dcp.Id())

	return nil
}

/**
 * Once the stream is ready to be started (once seqno is populated from ckptmgr)
 * Do the actual stream start.
 * NOTE: Checkpoint manager's SetVBTimestamps() gets called at a pipeline's start, which goes off
 * and sets the sequence number per vbucket.
 * When the timestamps have been set, the pipeline's settings are updated (See ckmgr.setTimestampForVB())
 * Once the pipeline's settings are updated, this DCP nozzle object's UpdateSettings is called,
 * since it's associated to the pipeline, and its internal data structures are updated as a result.
 * The startUprStreams_internal call here depends on those data
 * structured being updated indirectly from checkpoint manager.
 */
func (dcp *DcpNozzle) startUprStreams_internal(streams_to_start []uint16) error {
	for _, vbno := range streams_to_start {
		vbts, err := dcp.getTS(vbno, true)
		if err == nil && vbts != nil {
			err = dcp.startUprStream(vbno, vbts)
			if err != nil {
				dcp.handleGeneralError(err)
				dcp.Logger().Infof("%v: startUprStreams errored out, err=%v\n", dcp.Id(), err)
				return err
			}

		}
	}
	return nil
}

// Have an internal so we can control the opaque and version being passed in
func (dcp *DcpNozzle) startUprStreamInner(vbno uint16, vbts *base.VBTimestamp, version uint16) (err error) {
	flags := uint32(0)
	seqEnd := uint64(0xFFFFFFFFFFFFFFFF)
	dcp.Logger().Debugf("%v starting vb stream for vb=%v, version=%v\n", dcp.Id(), vbno, version)

	dcp.lock_uprFeed.RLock()
	defer dcp.lock_uprFeed.RUnlock()
	if dcp.uprFeed != nil {
		statusObj, ok := dcp.vb_stream_status[vbno]
		if ok && statusObj != nil {
			ignore := dcp.vbHandshakeMap[vbno].registerRequest(version, vbts.Seqno)
			if ignore {
				dcp.Logger().Debugf(fmt.Sprintf("%v ignoring send request for seqno %v since it has already been handled", vbts.Seqno))
			} else {
				// version passed in == opaque, which will be passed back to us
				err = dcp.uprFeed.UprRequestStream(vbno, version, flags, vbts.Vbuuid, vbts.Seqno, seqEnd, vbts.SnapshotStart, vbts.SnapshotEnd)
				if err == nil {
					dcp.setStreamState(vbno, Dcp_Stream_Init)
				}
			}
			return
		} else {
			panic(fmt.Sprintf("%v Try to startUprStream for invalid vbno=%v", dcp.Id(), vbno))
		}
	}
	return
}

// For a given stream (by vb#), send UPR_STREAMREQ via the uprFeed client method
func (dcp *DcpNozzle) startUprStream(vbno uint16, vbts *base.VBTimestamp) error {
	version := dcp.vbHandshakeMap[vbno].getNewVersion()
	return dcp.startUprStreamInner(vbno, vbts, version)
}

func (dcp *DcpNozzle) getUprFeed() mcc.UprFeedIface {
	dcp.lock_uprFeed.RLock()
	defer dcp.lock_uprFeed.RUnlock()
	return dcp.uprFeed
}

// Set vb list in dcp nozzle
func (dcp *DcpNozzle) SetVBList(vbnos []uint16) error {
	if len(vbnos) == 0 {
		return ErrorEmptyVBList
	}
	dcp.vbnos = vbnos
	return nil
}

func (dcp *DcpNozzle) GetVBList() []uint16 {
	return dcp.vbnos
}

type stateCheckFunc func(state DcpStreamState) bool

func (dcp *DcpNozzle) getDcpStreams(stateCheck stateCheckFunc) []uint16 {
	ret := []uint16{}
	for _, vb := range dcp.GetVBList() {
		state, _ := dcp.GetStreamState(vb)
		if stateCheck(state) {
			ret = append(ret, vb)
		}
	}
	return ret
}

func (dcp *DcpNozzle) inactiveDcpStreams() []uint16 {
	return dcp.getDcpStreams(inactiveStateCheck)
}

func inactiveStateCheck(state DcpStreamState) bool {
	return state != Dcp_Stream_Active
}

func (dcp *DcpNozzle) initedButInactiveDcpStreams() []uint16 {
	return dcp.getDcpStreams(initedButInactiveStateCheck)
}

func initedButInactiveStateCheck(state DcpStreamState) bool {
	return state == Dcp_Stream_Init
}

func (dcp *DcpNozzle) nonInitDcpStreams() []uint16 {
	return dcp.getDcpStreams(nonInitStateCheck)
}

func nonInitStateCheck(state DcpStreamState) bool {
	return state == Dcp_Stream_NonInit
}

func (dcp *DcpNozzle) inactiveDcpStreamsWithState() map[uint16]DcpStreamState {
	ret := make(map[uint16]DcpStreamState)
	for _, vb := range dcp.GetVBList() {
		state, _ := dcp.GetStreamState(vb)
		if state != Dcp_Stream_Active {
			ret[vb] = state
		}
	}
	return ret
}

// generate a new 16 bit opaque value set as MSB.
func (dcp *DcpNozzle) newOpaqueForClosing() uint16 {
	timeNow := uint64(time.Now().UnixNano())
	// bit 26 ... 42 from UnixNano().
	return uint16((timeNow >> 26) & 0xFFFF)
}

func (dcp *DcpNozzle) UpdateSettings(settings map[string]interface{}) error {
	ts_obj := dcp.utils.GetSettingFromSettings(settings, DCP_VBTimestamp)
	if ts_obj != nil {
		new_ts, ok := settings[DCP_VBTimestamp].(map[uint16]*base.VBTimestamp)
		if !ok || new_ts == nil {
			panic(fmt.Sprintf("setting %v should have type of map[uint16]*base.VBTimestamp", DCP_VBTimestamp))
		}
		err := dcp.onUpdateStartingSeqno(new_ts)
		if err != nil {
			return err
		}
	}

	if _, ok := settings[DCP_Stats_Interval]; ok {
		dcp.stats_interval = time.Duration(settings[DCP_Stats_Interval].(int)) * time.Millisecond
		dcp.stats_interval_change_ch <- true
	}

	return nil
}

func (dcp *DcpNozzle) onUpdateStartingSeqno(new_startingSeqnos map[uint16]*base.VBTimestamp) error {
	for vbno, vbts := range new_startingSeqnos {
		ts_withlock, ok := dcp.cur_ts[vbno]
		if ok && ts_withlock != nil {
			ts_withlock.lock.Lock()
			defer ts_withlock.lock.Unlock()
			if !dcp.isTSSet(vbno, false) {
				//only update the cur_ts if starting seqno has not been set yet
				dcp.Logger().Debugf("%v: Starting dcp stream for vb=%v, len(closed streams)=%v\n", dcp.Id(), vbno, len(dcp.inactiveDcpStreams()))
				dcp.setTS(vbno, vbts, false)
			}
		}
	}
	return nil
}

func (dcp *DcpNozzle) populateVBTS(vbts_map map[uint16]*base.VBTimestamp) error {
	if vbts_map != nil {
		for _, vbno := range dcp.vbnos {
			ts := vbts_map[vbno]
			if ts != nil {
				err := dcp.setTS(vbno, ts, true)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (dcp *DcpNozzle) setTS(vbno uint16, ts *base.VBTimestamp, need_lock bool) error {
	ts_entry := dcp.cur_ts[vbno]
	if ts_entry != nil {
		if need_lock {
			ts_entry.lock.Lock()
			defer ts_entry.lock.Unlock()
		}
		ts_entry.ts = ts
		return nil
	} else {
		return fmt.Errorf("setTS failed: vbno=%v is not tracked in cur_ts map", vbno)
	}
}

func (dcp *DcpNozzle) getTS(vbno uint16, need_lock bool) (*base.VBTimestamp, error) {
	ts_entry := dcp.cur_ts[vbno]
	if ts_entry != nil {
		if need_lock {
			ts_entry.lock.RLock()
			defer ts_entry.lock.RUnlock()
		}
		return ts_entry.ts, nil
	} else {
		return nil, fmt.Errorf("getTS failed: vbno=%v is not tracked in cur_ts map", vbno)
	}
}

//if the vbno is not belongs to this DcpNozzle, return true
func (dcp *DcpNozzle) isTSSet(vbno uint16, need_lock bool) bool {
	ts_entry := dcp.cur_ts[vbno]
	if ts_entry != nil {
		if need_lock {
			ts_entry.lock.RLock()
			defer ts_entry.lock.RUnlock()
		}
		return ts_entry.ts != nil
	}
	return true
}

func (dcp *DcpNozzle) setStreamState(vbno uint16, streamState DcpStreamState) {
	statusObj, ok := dcp.vb_stream_status[vbno]
	if ok && statusObj != nil {
		statusObj.lock.Lock()
		defer statusObj.lock.Unlock()
		statusObj.state = streamState
	} else {
		panic(fmt.Sprintf("Try to set stream state to invalid vbno=%v", vbno))
	}
}

func (dcp *DcpNozzle) GetStreamState(vbno uint16) (DcpStreamState, error) {
	statusObj, ok := dcp.vb_stream_status[vbno]
	if ok && statusObj != nil {
		statusObj.lock.RLock()
		defer statusObj.lock.RUnlock()
		return statusObj.state, nil
	} else {
		return 0, fmt.Errorf("Try to get stream state to invalid vbno=%v", vbno)
	}
}

func (dcp *DcpNozzle) SetMaxMissCount(max_dcp_miss_count int) {
	dcp.max_dcp_miss_count = max_dcp_miss_count
}

func (dcp *DcpNozzle) checkInactiveUprStreams() {
	defer dcp.childrenWaitGrp.Done()

	fin_ch := dcp.finch

	dcp_inactive_stream_check_ticker := time.NewTicker(dcp_inactive_stream_check_interval)
	defer dcp_inactive_stream_check_ticker.Stop()

	for {
		select {
		case <-fin_ch:
			dcp.Logger().Infof("%v checkInactiveUprStreams routine is exiting because dcp nozzle has been stopped\n", dcp.Id())
			return
		case <-dcp_inactive_stream_check_ticker.C:
			if dcp.isFeedClosed() {
				dcp.Logger().Infof("%v checkInactiveUprStreams routine is exiting because upr feed has been closed\n", dcp.Id())
				dcp.closeUprFeed()
				dcp.handleGeneralError(errors.New("DCP upr feed has been closed."))
				return
			}
			err := simple_utils.ExecWithTimeout(dcp.checkInactiveUprStreams_once, 1000*time.Millisecond, dcp.Logger())
			if err != nil {
				// ignore error and continue
				dcp.Logger().Infof("Received error when checking inactive steams for %v. err=%v\n", dcp.Id(), err)
			}
		}
	}
}

// check if feed has been closed
func (dcp *DcpNozzle) isFeedClosed() bool {
	dcp.lock_uprFeed.RLock()
	defer dcp.lock_uprFeed.RUnlock()
	if dcp.uprFeed != nil {
		return dcp.uprFeed.Closed()
	}
	return true
}

// check if inactive streams need to be restarted
/**
 * Called by the monitor (checkInactiveUprStreams) to re-send the current lowest sequence number
 * to UPR. It is sent under 3 conditions:
 * 1. Currently undergoing rollback, and have not started successfully yet.
 * 2. Started successfully, but streamreq has not yet arrived to DCP nozzle (race condition)
 * 	  I/O will flow but the state will remain inactive indefinitely while UPR rejects any further streamreq.
 * 3. DCP has not been able to send SUCCESS back yet. (Rare in a local node environment as comm between
 *    DCP and nozzle is done via TCP locally)
 */
func (dcp *DcpNozzle) checkInactiveUprStreams_once() error {
	streams_inactive := dcp.initedButInactiveDcpStreams()
	if len(streams_inactive) > 0 {
		atomic.AddUint32(&dcp.counter_streams_inactive, 1)
		dcp.Logger().Infof("%v incrementing counter for inactive streams %v\n", dcp.Id(), atomic.LoadUint32(&dcp.counter_streams_inactive))
		if atomic.LoadUint32(&dcp.counter_streams_inactive) > MaxCountStreamsInactive {
			// After a certain amount of time, simply re-send a STREAMREQ to re-initiate.
			dcp.Logger().Infof("%v re-sending STREAMREQ for inactive streams %v\n", dcp.Id(), streams_inactive)
			err := dcp.startUprStreams_internal(streams_inactive)
			if err != nil {
				return err
			}
			atomic.StoreUint32(&dcp.counter_streams_inactive, 0)
		}
	}
	return nil
}

// check if dcp is stuck
func (dcp *DcpNozzle) CheckStuckness(dcp_stats map[string]map[string]string) error {
	counter_received := dcp.counterReceived()
	if counter_received > dcp.counter_received_last {
		// dcp is ok if received more items from dcp
		dcp.counter_received_last = counter_received
		dcp.dcp_miss_count = 0
		return nil
	}

	if counter_received > dcp.counterSent() {
		// if dcp nozzle is holding an item that has not been processed by downstream parts,
		// cannot declare dcp broken regardless of what other stats say
		dcp.dcp_miss_count = 0
		return nil
	}

	// skip checking if dcp still has inactive streams
	if len(dcp.inactiveDcpStreams()) > 0 {
		dcp.dcp_miss_count = 0
		return nil
	}

	// check if there are items remaining in dcp
	dcp_has_items := dcp.dcpHasRemainingItemsForXdcr(dcp_stats)
	if !dcp_has_items {
		dcp.dcp_miss_count = 0
		return nil
	}

	// if we get here, there is probably something wrong with dcp
	dcp.dcp_miss_count++
	dcp.Logger().Infof("%v Incrementing dcp miss count. Dcp miss count = %v\n", dcp.Id(), dcp.dcp_miss_count)

	if dcp.dcp_miss_count > dcp.max_dcp_miss_count {
		//declare pipeline broken
		dcp.Logger().Errorf("%v is stuck", dcp.Id())
		return errors.New("Dcp is stuck")
	}

	return nil
}

func (dcp *DcpNozzle) dcpHasRemainingItemsForXdcr(dcp_stats map[string]map[string]string) bool {
	// Each dcp nozzle has an "items_remaining" stats in stats_map.
	// An example key for the stats is "eq_dcpq:xdcr:dcp_f58e0727200a19771e4459925908dd66/default/target_10.17.2.102:12000_0:items_remaining"
	xdcr_items_remaining_key := base.DCP_XDCR_STATS_PREFIX + dcp.Id() + base.DCP_XDCR_ITEMS_REMAINING_SUFFIX

	kv_nodes, err := dcp.xdcr_topology_svc.MyKVNodes()
	if err != nil {
		panic("Cannot get kv nodes")
	}

	for _, kv_node := range kv_nodes {
		per_node_stats_map, ok := dcp_stats[kv_node]
		if ok {
			if items_remaining_stats_str, ok := per_node_stats_map[xdcr_items_remaining_key]; ok {
				items_remaining_stats_int, err := strconv.ParseInt(items_remaining_stats_str, base.ParseIntBase, base.ParseIntBitSize)
				if err != nil {
					dcp.Logger().Errorf("%v Items remaining stats, %v, is not of integer type.", dcp.Id(), items_remaining_stats_str)
					continue
				}
				if items_remaining_stats_int > 0 {
					return true
				}
			}
		} else {
			dcp.Logger().Errorf("%v Failed to find dcp stats in statsMap returned for server=%v", dcp.Id(), kv_node)
		}
	}

	return false
}

func (dcp *DcpNozzle) counterReceived() uint32 {
	return atomic.LoadUint32(&dcp.counter_received)
}

func (dcp *DcpNozzle) incCounterReceived() {
	atomic.AddUint32(&dcp.counter_received, 1)
}

func (dcp *DcpNozzle) counterSent() uint32 {
	return atomic.LoadUint32(&dcp.counter_sent)
}

func (dcp *DcpNozzle) incCounterSent() {
	atomic.AddUint32(&dcp.counter_sent, 1)
}

func (dcp *DcpNozzle) collectDcpDataChanLen(settings map[string]interface{}) {
	defer dcp.childrenWaitGrp.Done()
	ticker := time.NewTicker(dcp.stats_interval)
	defer ticker.Stop()
	for {
		select {
		case <-dcp.finch:
			return
		case <-dcp.stats_interval_change_ch:
			ticker.Stop()
			ticker = time.NewTicker(dcp.stats_interval)
		case <-ticker.C:
			dcp.getDcpDataChanLen()
		}
	}

}

func (dcp *DcpNozzle) getDcpDataChanLen() {
	dcp_dispatch_len := 0
	dcp.lock_uprFeed.RLock()
	defer dcp.lock_uprFeed.RUnlock()
	if dcp.uprFeed == nil {
		//upr feed has been closed
		return
	} else {
		dcp_dispatch_len = len(dcp.uprFeed.GetUprEventCh())
	}
	// Raise event to keep track of how full DCP is and whether or not DCP is going to be a bottleneck
	dcp.RaiseEvent(common.NewEvent(common.StatsUpdate, nil, dcp, nil, dcp_dispatch_len))

}
