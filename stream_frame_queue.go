package quic

import (
	"errors"
	"sync"

	"github.com/lucas-clemente/quic-go/frames"
	"github.com/lucas-clemente/quic-go/protocol"
	"github.com/lucas-clemente/quic-go/qerr"
	"github.com/lucas-clemente/quic-go/utils"
)

var frameSizeInf = protocol.MaxPacketSize
var errMapAccess = qerr.Error(qerr.InternalError, "Error accessing the StreamFrameQueue")
var errStreamFlowControlBlocked = errors.New("Stream flow control blocked")

// streamFrameQueue is a Queue that handles StreamFrames
type streamFrameQueue struct {
	prioFrames      []*frames.StreamFrame
	frameMap        map[protocol.StreamID][]*frames.StreamFrame
	frameQueueMutex sync.RWMutex

	flowControlWindows map[protocol.StreamID]protocol.ByteCount
	bytesSent          protocol.ByteCount
	windowMutex        sync.Mutex

	activeStreams         []protocol.StreamID
	activeStreamsPosition int

	len     int
	byteLen protocol.ByteCount
}

func newStreamFrameQueue() *streamFrameQueue {
	return &streamFrameQueue{
		frameMap:           make(map[protocol.StreamID][]*frames.StreamFrame),
		flowControlWindows: make(map[protocol.StreamID]protocol.ByteCount),
	}
}

// Push adds a new StreamFrame to the queue
func (q *streamFrameQueue) Push(frame *frames.StreamFrame, prio bool) {
	q.frameQueueMutex.Lock()
	defer q.frameQueueMutex.Unlock()

	frame.DataLenPresent = true

	if prio {
		q.prioFrames = append(q.prioFrames, frame)
	} else {
		_, streamExisted := q.frameMap[frame.StreamID]
		q.frameMap[frame.StreamID] = append(q.frameMap[frame.StreamID], frame)
		if !streamExisted {
			q.activeStreams = append(q.activeStreams, frame.StreamID)
		}
	}

	q.byteLen += frame.DataLen()
	q.len++
}

// Len returns the total number of queued StreamFrames
func (q *streamFrameQueue) Len() int {
	q.frameQueueMutex.RLock()
	defer q.frameQueueMutex.RUnlock()

	return q.len
}

// ByteLen returns the total number of bytes queued
func (q *streamFrameQueue) ByteLen() protocol.ByteCount {
	q.frameQueueMutex.RLock()
	defer q.frameQueueMutex.RUnlock()

	return q.byteLen
}

// UpdateWindow updates the flow control window
// streamID may be 0, for the connection level flow control window
func (q *streamFrameQueue) UpdateWindow(streamID protocol.StreamID, offset protocol.ByteCount) {
	q.windowMutex.Lock()
	defer q.windowMutex.Unlock()

	windowSize, ok := q.flowControlWindows[streamID]
	if !ok || offset > windowSize {
		q.flowControlWindows[streamID] = offset
	}
}

// Pop returns the next element and deletes it from the queue
func (q *streamFrameQueue) Pop(maxLength protocol.ByteCount) (*frames.StreamFrame, error) {
	q.frameQueueMutex.Lock()
	q.windowMutex.Lock()
	defer q.frameQueueMutex.Unlock()
	defer q.windowMutex.Unlock()

	var isPrioFrame bool
	var frame *frames.StreamFrame
	var streamID protocol.StreamID
	var maxFrameDataSize protocol.ByteCount
	var err error

	for len(q.prioFrames) > 0 {
		frame = q.prioFrames[0]
		if frame == nil { // this happens when a Stream that had prioFrames queued gets deleted
			q.prioFrames = q.prioFrames[1:]
			continue
		}
		isPrioFrame = true
		// retransmitted frames can never be limited by the flow control window
		maxFrameDataSize = frameSizeInf
		break
	}

	if !isPrioFrame {
		var foundFrame bool
		var counter int

		for !foundFrame && counter < len(q.activeStreams) {
			streamID, err = q.getNextStream()
			counter++
			if streamID == 0 {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			frame = q.frameMap[streamID][0]
			maxFrameDataSize, err = q.getMaximumFrameDataSize(frame)
			if maxFrameDataSize > 0 {
				foundFrame = true
			}
		}
	}

	if maxFrameDataSize == 0 {
		return nil, nil
	}

	// Does the frame fit into the remaining space?
	frameMinLength, _ := frame.MinLength() // StreamFrame.MinLength *never* returns an error
	if frameMinLength > maxLength {
		return nil, nil
	}
	maxLength -= frameMinLength - 1
	maxLength = utils.MinByteCount(maxLength, maxFrameDataSize)

	splitFrame := q.maybeSplitOffFrame(frame, maxLength)
	if splitFrame != nil { // StreamFrame was split
		q.byteLen -= splitFrame.DataLen()
		return splitFrame, nil
	}

	// StreamFrame was not split. Remove it from the appropriate queue
	if isPrioFrame {
		q.prioFrames = q.prioFrames[1:]
	} else {
		q.frameMap[streamID] = q.frameMap[streamID][1:]
	}

	q.byteLen -= frame.DataLen()

	// TODO: find a better solution for identifying streams that don't contribute to connection level flow control
	if frame.StreamID != 1 && frame.StreamID != 3 {
		q.bytesSent += frame.DataLen()
	}

	q.len--
	return frame, nil
}

func (q *streamFrameQueue) RemoveStream(streamID protocol.StreamID) {
	q.frameQueueMutex.Lock()
	defer q.frameQueueMutex.Unlock()

	for i, frame := range q.prioFrames {
		if frame == nil {
			continue
		}
		if frame.StreamID == streamID {
			q.byteLen -= frame.DataLen()
			q.len--
			q.prioFrames[i] = nil
		}
	}

	frameQueue, ok := q.frameMap[streamID]
	if ok {
		for _, frame := range frameQueue {
			q.byteLen -= frame.DataLen()
			q.len--
		}
		delete(q.frameMap, streamID)
	}

	delete(q.flowControlWindows, streamID)

	for i, s := range q.activeStreams {
		if s == streamID {
			q.activeStreams[i] = 0
		}
	}

	q.garbageCollectActiveStreams()
}

func (q *streamFrameQueue) garbageCollectActiveStreams() {
	var j int
	var deletedIndex int

	for i, str := range q.activeStreams {
		if str != 0 {
			q.activeStreams[j] = str
			j++
		} else {
			deletedIndex = i
		}
	}

	if len(q.activeStreams) > 0 {
		q.activeStreams = q.activeStreams[:len(q.activeStreams)-1]
	}

	if deletedIndex < q.activeStreamsPosition {
		q.activeStreamsPosition--
	}
}

// front returns the next element without modifying the queue
// has to be called from a function that has already acquired the mutex
func (q *streamFrameQueue) getNextStream() (protocol.StreamID, error) {
	if q.len-len(q.prioFrames) == 0 {
		return 0, nil
	}

	var counter int
	for counter < len(q.activeStreams) {
		counter++
		streamID := q.activeStreams[q.activeStreamsPosition]
		q.activeStreamsPosition = (q.activeStreamsPosition + 1) % len(q.activeStreams)

		frameQueue, ok := q.frameMap[streamID]
		if !ok {
			return 0, errMapAccess
		}

		if len(frameQueue) > 0 {
			return streamID, nil
		}
	}

	return 0, nil
}

// maybeSplitOffFrame removes the first n bytes and returns them as a separate frame. If n >= len(n), nil is returned and nothing is modified.
func (q *streamFrameQueue) maybeSplitOffFrame(frame *frames.StreamFrame, n protocol.ByteCount) *frames.StreamFrame {
	if n >= frame.DataLen() {
		return nil
	}

	defer func() {
		frame.Data = frame.Data[n:]
		frame.Offset += n
	}()

	return &frames.StreamFrame{
		FinBit:         false,
		StreamID:       frame.StreamID,
		Offset:         frame.Offset,
		Data:           frame.Data[:n],
		DataLenPresent: frame.DataLenPresent,
	}
}

func (q *streamFrameQueue) getMaximumFrameDataSize(frame *frames.StreamFrame) (protocol.ByteCount, error) {
	highestAllowedStreamOffset, ok := q.flowControlWindows[frame.StreamID]
	if !ok {
		return 0, errMapAccess
	}
	if frame.Offset > highestAllowedStreamOffset { // stream level flow control blocked
		return 0, errStreamFlowControlBlocked
	}

	return highestAllowedStreamOffset - frame.Offset, nil
}
