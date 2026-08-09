package service

import (
	"context"
	"errors"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	coderws "github.com/coder/websocket"
)

type openAIWSClientReadResult struct {
	messageType coderws.MessageType
	payload     []byte
	lease       *pkghttputil.RequestBodyMemoryLease
	err         error
}

// ReadOpenAIWSClientMessage keeps one reader alive while control events send
// their close frame, then closes the transport and joins that reader.
func ReadOpenAIWSClientMessage(
	controlCtx context.Context,
	conn *coderws.Conn,
	timeout time.Duration,
	timeoutStatus coderws.StatusCode,
	timeoutReason string,
) (coderws.MessageType, []byte, error) {
	messageType, payload, lease, err := readOpenAIWSClientMessageWithTimeoutStartAndBudget(
		controlCtx,
		conn,
		timeout,
		timeoutStatus,
		timeoutReason,
		nil,
		nil,
		0,
		nil,
	)
	lease.Release()
	return messageType, payload, err
}

func ReadOpenAIWSClientMessageWithBudget(
	controlCtx context.Context,
	conn *coderws.Conn,
	timeout time.Duration,
	timeoutStatus coderws.StatusCode,
	timeoutReason string,
	readLimitBytes int64,
	budget *pkghttputil.RequestBodyMemoryBudget,
) (coderws.MessageType, []byte, *pkghttputil.RequestBodyMemoryLease, error) {
	return readOpenAIWSClientMessageWithTimeoutStartAndBudget(
		controlCtx, conn, timeout, timeoutStatus, timeoutReason, nil, nil, readLimitBytes, budget,
	)
}

func readOpenAIWSClientMessageWithTimeoutStartAndBudget(
	controlCtx context.Context,
	conn *coderws.Conn,
	timeout time.Duration,
	timeoutStatus coderws.StatusCode,
	timeoutReason string,
	timeoutStart <-chan struct{},
	timeoutActive func() bool,
	readLimitBytes int64,
	budget *pkghttputil.RequestBodyMemoryBudget,
) (coderws.MessageType, []byte, *pkghttputil.RequestBodyMemoryLease, error) {
	if conn == nil {
		return 0, nil, nil, errors.New("openai websocket client connection is nil")
	}
	if controlCtx == nil {
		controlCtx = context.Background()
	}
	readCtx, cancelRead := context.WithCancel(controlCtx)
	defer cancelRead()

	readDone := make(chan openAIWSClientReadResult, 1)
	go func() {
		messageType, payload, lease, err := ReadOpenAIWSClientFrameWithBudget(
			readCtx, conn, readLimitBytes, budget,
		)
		readDone <- openAIWSClientReadResult{messageType: messageType, payload: payload, lease: lease, err: err}
	}()

	var timer *time.Timer
	var timeoutCh <-chan time.Time
	startTimeout := func() {
		if timeout <= 0 || (timeoutActive != nil && !timeoutActive()) {
			return
		}
		if timer == nil {
			timer = time.NewTimer(timeout)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(timeout)
		}
		timeoutCh = timer.C
	}
	if timeoutActive == nil || timeoutActive() {
		startTimeout()
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	closeAndJoin := func(status coderws.StatusCode, reason string, cause error) (coderws.MessageType, []byte, *pkghttputil.RequestBodyMemoryLease, error) {
		cancelRead()
		_ = conn.Close(status, reason)
		_ = conn.CloseNow()
		result := <-readDone
		result.lease.Release()
		return 0, nil, nil, NewOpenAIWSClientCloseError(status, reason, cause)
	}

	for {
		select {
		case result := <-readDone:
			return result.messageType, result.payload, result.lease, result.err
		case <-timeoutStart:
			startTimeout()
		case <-timeoutCh:
			return closeAndJoin(timeoutStatus, timeoutReason, context.DeadlineExceeded)
		case <-controlCtx.Done():
			cause := context.Cause(controlCtx)
			if errors.Is(cause, ErrOpenAIWSIngressLeaseLost) {
				return closeAndJoin(
					coderws.StatusTryAgainLater,
					"websocket ingress capacity lease lost; please reconnect",
					cause,
				)
			}
			return closeAndJoin(coderws.StatusGoingAway, "websocket request canceled", cause)
		}
	}
}
