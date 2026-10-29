package tools

import (
	"ai-meeting/dto"
	"ai-meeting/pkg/ecode"
	"ai-meeting/services/interview"
	"context"
	"encoding/json"
	"fmt"
)

// ============================================================
// 面试工具: 把确定性状态机包装成 Agent 可调用的流程工具
//
// Agent 判断"何时进入结构化面试", 进入后由状态机保证一致性
// (CAS / 条件回滚 / 幂等)。这是自主编排与确定性编排的边界。
// ============================================================

// InterviewStartTool 创建面试会话并根据简历出题
type InterviewStartTool struct{}

// NewInterviewStartTool 创建工具
func NewInterviewStartTool() *InterviewStartTool { return &InterviewStartTool{} }

func (t *InterviewStartTool) Name() string { return "interview_start" }
func (t *InterviewStartTool) Description() string {
	return "开启一场结构化模拟面试：根据提供的简历文本生成面试题，返回面试会话 ID 与第一题。仅在用户明确要做模拟面试时调用。"
}
func (t *InterviewStartTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"resume_text": {"type": "string", "description": "候选人简历文本，可为空字符串"}
		},
		"required": ["resume_text"]
	}`)
}
func (t *InterviewStartTool) Execute(ctx context.Context, tc *Context, args json.RawMessage) (Result, error) {
	var input struct {
		ResumeText string `json:"resume_text"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{}, ecode.Wrap(err, "解析参数失败")
	}
	if tc.UserID == "" {
		return Result{}, ecode.New(ecode.NotLogin, "缺少用户身份")
	}

	facade := interview.GetInterviewSessionFacade()
	created, err := facade.CreateSession(tc.UserID)
	if err != nil {
		return Result{}, ecode.Wrap(err, "创建面试会话失败")
	}
	extracted, err := facade.ExtractInterviewQuestions(created.SessionID, tc.UserID, input.ResumeText)
	if err != nil {
		return Result{}, ecode.Wrap(err, "生成面试题失败")
	}
	return Result{
		Content: fmt.Sprintf("面试已创建。会话 ID: %s\n第 %s 题: %s",
			extracted.SessionID, extracted.QuestionNumber, extracted.Question),
		Meta: map[string]interface{}{
			"interview_session_id": extracted.SessionID,
			"question_number":      extracted.QuestionNumber,
		},
	}, nil
}

// InterviewCurrentQuestionTool 查询当前题
type InterviewCurrentQuestionTool struct{}

// NewInterviewCurrentQuestionTool 创建工具
func NewInterviewCurrentQuestionTool() *InterviewCurrentQuestionTool {
	return &InterviewCurrentQuestionTool{}
}

func (t *InterviewCurrentQuestionTool) Name() string { return "interview_current_question" }
func (t *InterviewCurrentQuestionTool) Description() string {
	return "查询指定面试会话的当前题目与进度。"
}
func (t *InterviewCurrentQuestionTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"interview_session_id": {"type": "string"}
		},
		"required": ["interview_session_id"]
	}`)
}
func (t *InterviewCurrentQuestionTool) Execute(ctx context.Context, tc *Context, args json.RawMessage) (Result, error) {
	var input struct {
		InterviewSessionID string `json:"interview_session_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{}, ecode.Wrap(err, "解析参数失败")
	}
	if err := ensureInterviewOwnership(ctx, input.InterviewSessionID, tc.UserID); err != nil {
		return Result{}, err
	}
	facade := interview.GetInterviewSessionFacade()
	info, err := facade.GetCurrentQuestion(input.InterviewSessionID, tc.UserID)
	if err != nil {
		return Result{}, ecode.Wrap(err, "查询当前题失败")
	}
	if info.Finished {
		return Result{Content: "面试已结束。"}, nil
	}
	return Result{
		Content: fmt.Sprintf("当前第 %s 题: %s", info.QuestionNumber, info.Question),
		Meta:    map[string]interface{}{"question_number": info.QuestionNumber},
	}, nil
}

// InterviewSubmitAnswerTool 提交答案并推进状态机
type InterviewSubmitAnswerTool struct{}

// NewInterviewSubmitAnswerTool 创建工具
func NewInterviewSubmitAnswerTool() *InterviewSubmitAnswerTool {
	return &InterviewSubmitAnswerTool{}
}

func (t *InterviewSubmitAnswerTool) Name() string { return "interview_submit_answer" }
func (t *InterviewSubmitAnswerTool) Description() string {
	return "提交一道面试题的答案，触发评分、追问判定与状态推进，返回得分与下一题。"
}
func (t *InterviewSubmitAnswerTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"interview_session_id": {"type": "string"},
			"question_number": {"type": "string"},
			"answer_content": {"type": "string"},
			"request_id": {"type": "string", "description": "幂等键，可留空"}
		},
		"required": ["interview_session_id", "question_number", "answer_content"]
	}`)
}
func (t *InterviewSubmitAnswerTool) Execute(ctx context.Context, tc *Context, args json.RawMessage) (Result, error) {
	var input struct {
		InterviewSessionID string `json:"interview_session_id"`
		QuestionNumber     string `json:"question_number"`
		AnswerContent      string `json:"answer_content"`
		RequestID          string `json:"request_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{}, ecode.Wrap(err, "解析参数失败")
	}
	if err := ensureInterviewOwnership(ctx, input.InterviewSessionID, tc.UserID); err != nil {
		return Result{}, err
	}
	facade := interview.GetInterviewSessionFacade()
	resp, err := facade.AnswerInterviewQuestion(input.InterviewSessionID, dto.InterviewAnswerReqDTO{
		QuestionNumber: input.QuestionNumber,
		AnswerContent:  input.AnswerContent,
		RequestId:      input.RequestID,
	}, tc.UserID)
	if err != nil {
		return Result{}, err
	}

	next := "面试已结束"
	if !resp.Finished {
		next = fmt.Sprintf("第 %s 题: %s", resp.NextQuestionNumber, resp.NextQuestion)
	}
	return Result{
		Content: fmt.Sprintf("本题得分 %d，总分 %d。\n反馈: %s\n下一题: %s",
			resp.Score, resp.TotalScore, resp.Feedback, next),
		Meta: map[string]interface{}{
			"score": resp.Score, "total_score": resp.TotalScore,
			"finished": resp.Finished, "next_question_number": resp.NextQuestionNumber,
		},
	}, nil
}

// ensureInterviewOwnership 校验面试会话归属（防止跨用户操作）
func ensureInterviewOwnership(ctx context.Context, sessionID, userID string) error {
	if sessionID == "" {
		return ecode.New(ecode.ErrAgentInvalidInput, "interview_session_id 不能为空")
	}
	if userID == "" {
		return ecode.New(ecode.NotLogin, "缺少用户身份")
	}
	session, err := interviewSessionRepo.FindInterviewSession(ctx, sessionID, userID)
	if err != nil || session == nil {
		// 查询失败与不存在统一按"无权限/不存在"处理, 避免泄露会话是否存在
		return ecode.New(ecode.ErrSessionNotFound, "面试会话不存在或无权限")
	}
	return nil
}
