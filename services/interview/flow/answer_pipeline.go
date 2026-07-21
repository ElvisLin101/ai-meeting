package flow

import (
	"ai-meeting/dto"
	"ai-meeting/models"
	"ai-meeting/pkg/lock"
	"ai-meeting/services/interview/evaluation"
	"ai-meeting/services/interview/runtime"
	"ai-meeting/services/metric"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

// ============================================================
// AnswerPipeline 答题流水线
//
// 完整流程:
//   幂等检查 → 读flow → 题级锁 → 锁后校验题号 → 评分 → 推进flow → 写分数 → 写turn log → 标记成功
//   失败: 回滚flow(分数提交失败时) + 清幂等标记
// ============================================================

const questionLockTTL = 120 * time.Second

// AnswerPipeline 答题流水线
type AnswerPipeline struct {
	rdb            *redis.Client
	flowStateMachine *FlowStateMachine
	flowCache      *runtime.FlowCache
	scoreCache     *runtime.ScoreCache
	turnLogCache   *runtime.TurnLogCache
	questionCache  *runtime.QuestionCache
	snapshotSvc    *runtime.SnapshotService
	turnRepairSvc  *TurnRepairService
	evaluationSvc  *evaluation.EvaluationService
	followUpSvc    *evaluation.FollowUpService
	idempotencySvc *IdempotencyService
}

// NewAnswerPipeline 创建答题流水线
func NewAnswerPipeline(
	rdb *redis.Client,
	fsm *FlowStateMachine,
	flowCache *runtime.FlowCache,
	scoreCache *runtime.ScoreCache,
	turnLogCache *runtime.TurnLogCache,
	questionCache *runtime.QuestionCache,
	snapshotSvc *runtime.SnapshotService,
	turnRepairSvc *TurnRepairService,
	evalSvc *evaluation.EvaluationService,
	followUpSvc *evaluation.FollowUpService,
	idempotencySvc *IdempotencyService,
) *AnswerPipeline {
	return &AnswerPipeline{
		rdb:              rdb,
		flowStateMachine: fsm,
		flowCache:        flowCache,
		scoreCache:       scoreCache,
		turnLogCache:     turnLogCache,
		questionCache:    questionCache,
		snapshotSvc:      snapshotSvc,
		turnRepairSvc:    turnRepairSvc,
		evaluationSvc:    evalSvc,
		followUpSvc:      followUpSvc,
		idempotencySvc:   idempotencySvc,
	}
}

// Execute 执行一次答题
func (p *AnswerPipeline) Execute(ctx context.Context, sessionID string, req dto.InterviewAnswerReqDTO) (*dto.InterviewAnswerRespDTO, error) {
	// 1. 归一化 requestId
	requestID := req.RequestId
	if requestID == "" {
		requestID = NormalizeRequestId(sessionID, req.QuestionNumber, req.AnswerContent)
	}

	// 2. 幂等检查
	tryStart, err := p.idempotencySvc.TryStart(ctx, sessionID, requestID)
	if err != nil {
		return nil, fmt.Errorf("idempotency check failed: %w", err)
	}
	switch tryStart.Status {
	case IdempotencySucceeded:
		metric.GetMetricService().Record(models.MetricLog{Module: "idempotency", Event: "replay_hit", Success: true, SessionID: sessionID})
		return tryStart.Response, nil
	case IdempotencyProcessing:
		metric.GetMetricService().Record(models.MetricLog{Module: "idempotency", Event: "processing_blocked", Success: false, SessionID: sessionID})
		return nil, errors.New("该请求正在处理中，请稍后重试")
	}

	idempotencyStarted := true
	idempotencyMarked := false

	// 3. ensureRuntime: Redis 命中? miss 则从 Mongo 重建
	flow, err := p.snapshotSvc.EnsureRuntime(ctx, sessionID)
	if err != nil {
		p.idempotencySvc.ClearProcessing(ctx, sessionID, requestID)
		return nil, fmt.Errorf("ensureRuntime 失败: %w", err)
	}
	if flow == nil {
		p.idempotencySvc.ClearProcessing(ctx, sessionID, requestID)
		return nil, errors.New("面试流程未初始化，请先出题")
	}
	if flow.IsCompleted() {
		p.idempotencySvc.ClearProcessing(ctx, sessionID, requestID)
		return nil, errors.New("面试已结束")
	}

	// 4. 题级锁
	lockKey := fmt.Sprintf("interview:answer:lock:%s:%s", sessionID, req.QuestionNumber)
	questionLock, err := lock.Acquire(ctx, p.rdb, lockKey, questionLockTTL)
	if err != nil {
		p.idempotencySvc.ClearProcessing(ctx, sessionID, requestID)
		return nil, fmt.Errorf("获取题级锁失败: %w", err)
	}
	if questionLock == nil {
		metric.GetMetricService().Record(models.MetricLog{Module: "lock", Event: "question_lock_failed", Success: false, SessionID: sessionID})
		p.idempotencySvc.ClearProcessing(ctx, sessionID, requestID)
		return nil, errors.New("当前题目正在被处理，请稍后重试")
	}
	metric.GetMetricService().Record(models.MetricLog{Module: "lock", Event: "question_lock_acquired", Success: true, SessionID: sessionID})
	defer questionLock.Release(ctx)

	// 5. 锁后再校验题号（防游标漂移）
	flow, err = p.flowStateMachine.Current(ctx, sessionID)
	if err != nil {
		p.idempotencySvc.ClearProcessing(ctx, sessionID, requestID)
		return nil, fmt.Errorf("锁后重读流程状态失败: %w", err)
	}
	if flow.CurrentQuestionNumber != req.QuestionNumber {
		p.idempotencySvc.ClearProcessing(ctx, sessionID, requestID)
		return nil, fmt.Errorf("题号已过期，当前题号为 %s", flow.CurrentQuestionNumber)
	}

	// 6. 评分
	if _, err := p.flowStateMachine.MoveToEvaluating(ctx, sessionID); err != nil {
		p.idempotencySvc.ClearProcessing(ctx, sessionID, requestID)
		return nil, fmt.Errorf("转入评分态失败: %w", err)
	}

	// 从 Redis 读取题面和简历上下文
	questionContent, err := p.questionCache.GetQuestion(ctx, sessionID, req.QuestionNumber)
	if err != nil {
		logrus.Warnf("Failed to get question content, session=%s, qn=%s, err=%v", sessionID, req.QuestionNumber, err)
	}
	resumeContext, err := p.questionCache.GetResumeContextText(ctx, sessionID)
	if err != nil {
		logrus.Warnf("Failed to get resume context, session=%s, err=%v", sessionID, err)
	}

	evalResult, err := p.evaluationSvc.EvaluateAnswer(ctx, questionContent, req.AnswerContent, resumeContext)
	if err != nil {
		p.idempotencySvc.ClearProcessing(ctx, sessionID, requestID)
		return nil, fmt.Errorf("AI 评分失败: %w", err)
	}

	// 7. 推进 flow
	flowSnapshot := p.flowStateMachine.SnapshotFlow(flow)

	isFollowUp := IsFollowUpQuestion(req.QuestionNumber)
	var nextQuestionNumber string
	var nextQuestion string
	var finished bool

	// 追问规则判定
	ruleCtx := &FollowUpRuleContext{
		InterviewCompleted:   false,
		FollowUpCount:        flow.FollowUpCount,
		MaxFollowUp:          flow.MaxFollowUp,
		FollowUpNeededFromAI: evalResult.FollowUpNeeded,
		Score:                evalResult.Score,
		MissingPoints:        evalResult.MissingPoints,
		FollowUpQuestionHint: evalResult.FollowUpQuestion,
	}
	decision := DecideFollowUp(ruleCtx)

	if decision.NeedFollowUp && flow.FollowUpCount < flow.MaxFollowUp {
		// 追问分支
		followUpResult, err := p.followUpSvc.GenerateFollowUp(ctx, questionContent, req.AnswerContent, evalResult.MissingPoints, flow.FollowUpCount, flow.MaxFollowUp)
		if err == nil && followUpResult != nil && !followUpResult.EndInterview && followUpResult.Question != "" {
			nextQuestionNumber = BuildFollowUpQuestionNumber(ResolveMainQuestionNumber(req.QuestionNumber), flow.FollowUpCount+1)
			nextQuestion = followUpResult.Question
			if _, err := p.flowStateMachine.StartFollowUpQuestion(ctx, sessionID, nextQuestionNumber); err != nil {
				p.rollbackFlow(ctx, sessionID, flowSnapshot, requestID)
				return nil, fmt.Errorf("开始追问失败: %w", err)
			}
			// 追问题写入 Redis（否则下次读题面会 miss）
			if err := p.questionCache.SaveFollowUpQuestion(ctx, sessionID, nextQuestionNumber, nextQuestion); err != nil {
				logrus.Warnf("Failed to save follow-up question, session=%s, qn=%s, err=%v", sessionID, nextQuestionNumber, err)
			}
			// 主问题计分（追问不计分, 但当前答的是主问题, 需要入账）
			if !isFollowUp {
				if _, _, _, err := p.scoreCache.AddScore(ctx, sessionID, evalResult.Score); err != nil {
					p.rollbackFlow(ctx, sessionID, flowSnapshot, requestID)
					return nil, fmt.Errorf("分数提交失败: %w", err)
				}
			}
		} else {
			// 追问生成失败 → 走主问题推进
			nextQuestionNumber, nextQuestion, finished, err = p.advanceMainQuestion(ctx, sessionID, flowSnapshot, requestID, isFollowUp, evalResult.Score)
			if err != nil {
				return nil, err
			}
		}
	} else {
		// 主问题推进
		nextQuestionNumber, nextQuestion, finished, err = p.advanceMainQuestion(ctx, sessionID, flowSnapshot, requestID, isFollowUp, evalResult.Score)
		if err != nil {
			return nil, err
		}
	}

	// 8. 读当前总分
	totalScore, _ := p.scoreCache.GetTotalScore(ctx, sessionID)

	// 9. 写 turn log
	turnLog := &models.InterviewTurnLog{
		Timestamp:          time.Now(),
		RequestID:          requestID,
		QuestionNumber:     req.QuestionNumber,
		QuestionContent:    questionContent,
		AnswerContent:      truncateForLog(req.AnswerContent, 1000),
		Score:              evalResult.Score,
		TotalScore:         totalScore,
		Feedback:           evalResult.Feedback,
		FollowUpNeeded:     decision.NeedFollowUp,
		IsFollowUp:         isFollowUp,
		FollowUpCount:      flow.FollowUpCount,
		NextQuestionNumber: nextQuestionNumber,
		NextQuestion:       nextQuestion,
		Finished:           finished,
	}
	if _, err := p.turnLogCache.AppendTurnIfAbsent(ctx, sessionID, turnLog); err != nil {
		logrus.Warnf("Failed to append turn log, session=%s, err=%v", sessionID, err)
		// turn log 写失败 → 入异步补偿队列
		if p.turnRepairSvc != nil {
			p.turnRepairSvc.Enqueue(ctx, sessionID, turnLog)
		}
	}

	// 10. 组装响应
	resp := &dto.InterviewAnswerRespDTO{
		QuestionNumber:     req.QuestionNumber,
		Question:           questionContent,
		Answer:             req.AnswerContent,
		Score:              evalResult.Score,
		TotalScore:         totalScore,
		Feedback:           evalResult.Feedback,
		IsFollowUp:         isFollowUp,
		NextQuestionNumber: nextQuestionNumber,
		NextQuestion:       nextQuestion,
		Finished:           finished,
	}

	// 11. 标记成功
	if err := p.idempotencySvc.MarkSucceeded(ctx, sessionID, requestID, resp); err != nil {
		logrus.Warnf("Failed to mark idempotency succeeded, session=%s, err=%v", sessionID, err)
	}
	idempotencyMarked = true

	// 刷新快照到 Mongo（异步不阻塞返回）
	go func() {
		refreshCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// 读推进后的 flow
		latestFlow, err := p.flowCache.GetFlow(refreshCtx, sessionID)
		if err != nil || latestFlow == nil {
			latestFlow = flow
		}
		p.snapshotSvc.RefreshAfterAnswerCommitted(refreshCtx, sessionID, "", requestID, latestFlow, turnLog)
	}()

	logrus.Infof("Answer pipeline completed, session=%s, question=%s, score=%d, followUp=%v, finished=%v",
		sessionID, req.QuestionNumber, evalResult.Score, decision.NeedFollowUp, finished)
	metric.GetMetricService().Record(models.MetricLog{
		Module: "pipeline", Event: "answer_completed", Success: true,
		SessionID: sessionID, Extra: fmt.Sprintf(`{"score":%d,"follow_up":%v,"finished":%v}`, evalResult.Score, decision.NeedFollowUp, finished),
	})

	_ = idempotencyStarted
	_ = idempotencyMarked
	return resp, nil
}

// advanceMainQuestion 推进主问题 + 计分
func (p *AnswerPipeline) advanceMainQuestion(ctx context.Context, sessionID string, flowSnapshot *models.InterviewFlowState, requestID string, isFollowUp bool, score int) (nextQNum, nextQ string, finished bool, err error) {
	nextFlow, err := p.flowStateMachine.AdvanceMainQuestion(ctx, sessionID)
	if err != nil {
		p.rollbackFlow(ctx, sessionID, flowSnapshot, requestID)
		return "", "", false, fmt.Errorf("推进主问题失败: %w", err)
	}

	// 计分: 主问题入账, 追问不计分
	if !isFollowUp {
		if _, _, _, err := p.scoreCache.AddScore(ctx, sessionID, score); err != nil {
			// 分数提交失败 → 回滚 flow
			p.rollbackFlow(ctx, sessionID, flowSnapshot, requestID)
			return "", "", false, fmt.Errorf("分数提交失败: %w", err)
		}
	}

	if nextFlow.IsCompleted() {
		return "", "", true, nil
	}
	// 从 Redis 读取下一题题面
	nextQuestion, _ := p.questionCache.GetQuestion(ctx, sessionID, nextFlow.CurrentQuestionNumber)
	return nextFlow.CurrentQuestionNumber, nextQuestion, false, nil
}

// rollbackFlow 回滚 flow 到快照 + 清幂等标记
func (p *AnswerPipeline) rollbackFlow(ctx context.Context, sessionID string, snapshot *models.InterviewFlowState, requestID string) {
	metric.GetMetricService().Record(models.MetricLog{Module: "state_machine", Event: "flow_rollback", Success: false, SessionID: sessionID})
	if snapshot != nil {
		if err := p.flowStateMachine.RestoreFlow(ctx, sessionID, snapshot); err != nil {
			logrus.Errorf("Failed to rollback flow, session=%s, err=%v", sessionID, err)
		}
	}
	p.idempotencySvc.ClearProcessing(ctx, sessionID, requestID)
}

// truncateForLog 截断文本用于日志
func truncateForLog(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen]
}
