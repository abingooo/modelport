package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// startPassthroughHookRecordingServer 与 startPassthroughLifecycleServer 相同，
// 但把一组会记录调用的 hooks 传给 ingress，用于观察透传路径的 turn 回调。
func startPassthroughHookRecordingServer(
	t *testing.T,
	controlCtx context.Context,
	svc *OpenAIGatewayService,
	account *Account,
	hooks *OpenAIWSIngressHooks,
) (*httptest.Server, <-chan error) {
	t.Helper()
	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		msgType, firstMessage, err := ReadOpenAIWSClientMessage(
			controlCtx,
			conn,
			3*time.Second,
			coderws.StatusPolicyViolation,
			"missing first response.create message",
		)
		if err != nil {
			serverErr <- err
			return
		}
		if msgType != coderws.MessageText {
			serverErr <- errors.New("first message was not text")
			return
		}

		recorder := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(recorder)
		req := r.Clone(controlCtx)
		req.Header = req.Header.Clone()
		ginCtx.Request = req
		serverErr <- svc.ProxyResponsesWebSocketFromClient(controlCtx, ginCtx, conn, account, "sk-test", firstMessage, hooks)
	}))
	return server, serverErr
}

// TestPassthroughIngressNeverCallsBeforeTurn 钉死 ws_v2 透传 ingress 与 handler
// 侧 turn 定价的耦合：透传 relay 只回调 AfterTurn，没有任何 turn 起始回调，
// 因此 hooks.BeforeTurn 永远不会触发。
//
// handler 依赖这一点：openAIWSTurnPricing 零值起步，透传连接的每个 turn 都拿
// 不到冻结的 pricingAt，RecordUsage 回退到记录时刻——与引入分组利润控制前的
// 基线一致。若把 turn 定价初始化成建连时刻，透传连接的所有 turn 就会被钉死在
// 建连时的高峰因子，客户端峰前建连保活即可全程按谷价结算。
//
// 若本断言因为透传补齐了 turn 起始回调而失败：这是好事，请同步复核
// openAIWSTurnPricing 的零值语义与透传路径的 turn 级利润复核。
func TestPassthroughIngressNeverCallsBeforeTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)

	upstream := newStagedPassthroughConn()
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_pricing","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)

	var hooksMu sync.Mutex
	beforeTurnCalls := 0
	afterTurnCalls := 0
	hooks := &OpenAIWSIngressHooks{
		BeforeTurn: func(int) error {
			hooksMu.Lock()
			beforeTurnCalls++
			hooksMu.Unlock()
			return nil
		},
		AfterTurn: func(int, *OpenAIForwardResult, error) {
			hooksMu.Lock()
			afterTurnCalls++
			hooksMu.Unlock()
		},
	}

	server, serverErr := startPassthroughHookRecordingServer(
		t,
		controlCtx,
		newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
		passthroughLifecycleAccount(),
		hooks,
	)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	event, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())

	// 等待连接自然结束（inter-turn idle 超时），确保 AfterTurn 已提交。
	_, _ = readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	select {
	case <-serverErr:
	case <-time.After(3 * time.Second):
		t.Fatal("passthrough ingress did not exit")
	}

	hooksMu.Lock()
	gotBefore, gotAfter := beforeTurnCalls, afterTurnCalls
	hooksMu.Unlock()

	require.Zero(t, gotBefore, "透传 ingress 没有 turn 起始回调，BeforeTurn 不应被调用")
	require.Positive(t, gotAfter, "透传 ingress 仍应回调 AfterTurn 提交用量")
}

func TestPassthroughInstructionHookReceivesEachOriginalFollowUpFrameOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	controlCtx, cancelControl := context.WithCancelCause(context.Background())
	defer cancelControl(context.Canceled)

	upstream := newStagedPassthroughConn()
	type hookCall struct {
		turn  int
		model string
		body  string
	}
	var hooksMu sync.Mutex
	calls := make([]hookCall, 0, 2)
	hooks := &OpenAIWSIngressHooks{
		MaxReasoningEffort: "medium",
		BeforeInstructionRequest: func(turn int, payload []byte, originalModel string) error {
			hooksMu.Lock()
			calls = append(calls, hookCall{turn: turn, model: originalModel, body: string(payload)})
			hooksMu.Unlock()
			return nil
		},
		MapRequestModel: func(_ int, originalModel string) (string, error) {
			if originalModel == "client-alias" {
				return "gpt-5.1", nil
			}
			return originalModel, nil
		},
	}

	server, serverErr := startPassthroughHookRecordingServer(
		t,
		controlCtx,
		newPassthroughLifecycleService(passthroughLifecycleConfig(), upstream),
		passthroughLifecycleAccount(),
		hooks,
	)
	defer server.Close()
	clientConn := dialPassthroughLifecycleClient(t, server)
	defer func() { _ = clientConn.CloseNow() }()

	require.NotEmpty(t, requirePassthroughUpstreamWrite(t, upstream, 3*time.Second))
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_instruction_1","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	firstEvent, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_instruction_1", gjson.GetBytes(firstEvent, "response.id").String())

	writeFrame := func(messageType coderws.MessageType, payload string) {
		writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancelWrite()
		require.NoError(t, clientConn.Write(writeCtx, messageType, []byte(payload)))
	}
	secondFrame := `{"type":"response.create","model":"client-alias","stream":false,"reasoning":{"effort":"high"},"instructions":"second"}`
	writeFrame(coderws.MessageText, secondFrame)
	secondUpstream := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
	require.Equal(t, "gpt-5.1", gjson.GetBytes(secondUpstream, "model").String())
	require.Equal(t, "medium", gjson.GetBytes(secondUpstream, "reasoning.effort").String())
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_instruction_2","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	secondEvent, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_instruction_2", gjson.GetBytes(secondEvent, "response.id").String())

	thirdFrame := `{"type":"response.create","stream":false,"instructions":"third"}`
	writeFrame(coderws.MessageBinary, thirdFrame)
	require.NotEmpty(t, requirePassthroughUpstreamWrite(t, upstream, 3*time.Second))
	upstream.Send(`{"type":"response.completed","response":{"id":"resp_instruction_3","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)
	thirdEvent, err := readPassthroughLifecycleFrame(t, clientConn, 3*time.Second)
	require.NoError(t, err)
	require.Equal(t, "resp_instruction_3", gjson.GetBytes(thirdEvent, "response.id").String())

	hooksMu.Lock()
	gotCalls := append([]hookCall(nil), calls...)
	hooksMu.Unlock()
	require.Equal(t, []hookCall{
		{turn: 2, model: "client-alias", body: secondFrame},
		{turn: 3, model: "gpt-5.1", body: thirdFrame},
	}, gotCalls)

	require.NoError(t, clientConn.CloseNow())
	cancelControl(context.Canceled)
	select {
	case <-serverErr:
	case <-time.After(5 * time.Second):
		t.Fatal("passthrough ingress did not exit")
	}
}
