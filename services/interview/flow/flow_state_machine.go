package flow

import (
	"ai-meeting/models"
	"ai-meeting/services/interview/runtime"
	"context"
	"fmt"
)

// ============================================================
// FlowStateMachine 面试流程状态机
//
// 合法转移表:
//   INIT       → {ASKING, COMPLETED}
//   ASKING     → {EVALUATING, FOLLOW_UP, COMPLETED}
//   EVALUATING → {ASKING, FOLLOW_UP, COMPLETED}
//   FOLLOW_UP  → {EVALUATING, ASKING, COMPLETED}
//   COMPLETED  → {} (终态)
//
// 所有状态变更通过 FlowCache.MutateFlow 做 CAS 乐观锁,
// 回滚用 RestoreFlow 直接覆盖(不走 CAS)。
// ============================================================

var legalTransitions = map[models.InterviewFlowStatus][]models.InterviewFlowStatus{
	models.FlowInit:       {models.FlowAsking, models.FlowCompleted},
	models.FlowAsking:     {models.FlowEvaluating, models.FlowFollowUp, models.FlowCompleted},
	models.FlowEvaluating: {models.FlowAsking, models.FlowFollowUp, models.FlowCompleted},
	models.FlowFollowUp:   {models.FlowEvaluating, models.FlowAsking, models.FlowCompleted},
	models.FlowCompleted:  {},
}

const defaultMaxFollowUp = 2

// FlowStateMachine 面试流程状态机
type FlowStateMachine struct {
	flowCache *runtime.FlowCache
}

// NewFlowStateMachine 创建状态机
func NewFlowStateMachine(flowCache *runtime.FlowCache) *FlowStateMachine {
	return &FlowStateMachine{flowCache: flowCache}
}

// EnsureInitialized 初始化 flow（出题后调用），已存在则直接返回
func (m *FlowStateMachine) EnsureInitialized(ctx context.Context, sessionID string, totalQuestions int) (*models.InterviewFlowState, error) {
	current, err := m.flowCache.GetFlow(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if current != nil {
		return current, nil
	}

	// 初始化: INIT → 立即转 ASKING
	initial := &models.InterviewFlowState{
		Status:                models.FlowAsking,
		CurrentIndex:          0,
		CurrentQuestionNumber: "1",
		TotalQuestions:        totalQuestions,
		FollowUpCount:         0,
		MaxFollowUp:           defaultMaxFollowUp,
		Version:               1,
	}
	if err := m.flowCache.SaveFlow(ctx, sessionID, initial); err != nil {
		return nil, err
	}
	return initial, nil
}

// Current 读当前 flow
func (m *FlowStateMachine) Current(ctx context.Context, sessionID string) (*models.InterviewFlowState, error) {
	return m.flowCache.GetFlow(ctx, sessionID)
}

// MoveToEvaluating 转入评分态
func (m *FlowStateMachine) MoveToEvaluating(ctx context.Context, sessionID string) (*models.InterviewFlowState, error) {
	return m.transitionStatus(ctx, sessionID, models.FlowEvaluating)
}

// StartFollowUpQuestion 开始追问（followUpCount+1，题号设为追问题号）
func (m *FlowStateMachine) StartFollowUpQuestion(ctx context.Context, sessionID, questionNumber string) (*models.InterviewFlowState, error) {
	return m.flowCache.MutateFlow(ctx, sessionID, func(state *models.InterviewFlowState) (*models.InterviewFlowState, error) {
		if err := assertLegalTransition(state.Status, models.FlowFollowUp); err != nil {
			return nil, err
		}
		state.Status = models.FlowFollowUp
		state.FollowUpCount++
		state.CurrentQuestionNumber = questionNumber
		return state, nil
	})
}

// AdvanceMainQuestion 推进到下一主问题
// followUpCount 清零, currentIndex+1, 越界则 COMPLETED
func (m *FlowStateMachine) AdvanceMainQuestion(ctx context.Context, sessionID string) (*models.InterviewFlowState, error) {
	next, err := m.flowCache.MutateFlow(ctx, sessionID, func(state *models.InterviewFlowState) (*models.InterviewFlowState, error) {
		if err := assertLegalTransition(state.Status, models.FlowAsking); err != nil {
			// EVALUATING/FOLLOW_UP → ASKING 是合法的; 如果不合法才报错
			return nil, err
		}

		nextIndex := state.CurrentIndex + 1
		state.FollowUpCount = 0

		if state.TotalQuestions <= 0 || nextIndex >= state.TotalQuestions {
			// 越界 → 完成
			state.Status = models.FlowCompleted
			state.CurrentQuestionNumber = ""
		} else {
			state.CurrentIndex = nextIndex
			state.Status = models.FlowAsking
			state.CurrentQuestionNumber = fmt.Sprintf("%d", nextIndex+1)
		}
		return state, nil
	})
	if err != nil {
		return nil, err
	}

	// 如果推进后越界，MarkCompleted 已在 mutator 里设置
	return next, nil
}

// MarkCompleted 标记面试结束
func (m *FlowStateMachine) MarkCompleted(ctx context.Context, sessionID string) (*models.InterviewFlowState, error) {
	return m.flowCache.MutateFlow(ctx, sessionID, func(state *models.InterviewFlowState) (*models.InterviewFlowState, error) {
		if state.IsCompleted() {
			return state, nil // 幂等
		}
		if err := assertLegalTransition(state.Status, models.FlowCompleted); err != nil {
			return nil, err
		}
		state.Status = models.FlowCompleted
		state.CurrentQuestionNumber = ""
		state.FollowUpCount = 0
		return state, nil
	})
}

// RestoreFlow 回滚到快照（分数提交失败时用，不走 CAS）
func (m *FlowStateMachine) RestoreFlow(ctx context.Context, sessionID string, snapshot *models.InterviewFlowState) error {
	return m.flowCache.SaveFlow(ctx, sessionID, snapshot)
}

// SnapshotFlow 深拷贝当前 flow（用于回滚备份）
func (m *FlowStateMachine) SnapshotFlow(state *models.InterviewFlowState) *models.InterviewFlowState {
	if state == nil {
		return nil
	}
	cp := *state
	return &cp
}

// transitionStatus 通用状态转移
func (m *FlowStateMachine) transitionStatus(ctx context.Context, sessionID string, target models.InterviewFlowStatus) (*models.InterviewFlowState, error) {
	return m.flowCache.MutateFlow(ctx, sessionID, func(state *models.InterviewFlowState) (*models.InterviewFlowState, error) {
		if state.Status == target {
			return state, nil // 幂等
		}
		if err := assertLegalTransition(state.Status, target); err != nil {
			return nil, err
		}
		state.Status = target
		return state, nil
	})
}

// assertLegalTransition 校验状态转移是否合法
func assertLegalTransition(from, to models.InterviewFlowStatus) error {
	allowed, ok := legalTransitions[from]
	if !ok {
		return fmt.Errorf("unknown source status: %s", from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("illegal transition: %s → %s", from, to)
}
