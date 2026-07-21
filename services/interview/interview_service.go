package interview

import (
	"ai-meeting/clients"
	"ai-meeting/dto"
	"ai-meeting/models"
	"ai-meeting/repositories"
	"ai-meeting/services/interview/evaluation"
	"ai-meeting/services/interview/flow"
	"ai-meeting/services/interview/runtime"
	mongorepo "ai-meeting/repositories/mongo"
	"context"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// InterviewSessionFacade 面试会话门面服务，封装面试会话相关的业务逻辑
type InterviewSessionFacade struct {
	answerPipeline *flow.AnswerPipeline
	flowStateMachine *flow.FlowStateMachine
	flowCache      *runtime.FlowCache
	scoreCache     *runtime.ScoreCache
	questionCache  *runtime.QuestionCache
	extractionSvc  *evaluation.ExtractionService
}

// CreateSession 创建面试会话
func (s *InterviewSessionFacade) CreateSession(userID string) (*dto.InterviewSessionCreateRespDTO, error) {
	sessionID := uuid.New().String()
	session := models.InterviewSession{SessionID: sessionID, UserID: userID, Status: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mongorepo.CreateInterviewSession(ctx, &session); err != nil {
		return nil, err
	}
	return &dto.InterviewSessionCreateRespDTO{SessionID: sessionID}, nil
}

// PageConversations 分页查询面试会话列表
func (s *InterviewSessionFacade) PageConversations(userID string, req dto.InterviewConversationPageReqDTO) ([]models.AgentConversation, int64, error) {
	offset := (req.Page - 1) * req.Size
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.PageAgentConversations(ctx, userID, offset, req.Size)
}

// GetConversationHistory 获取会话历史消息
func (s *InterviewSessionFacade) GetConversationHistory(sessionID, userID string) ([]models.AgentMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.ListAgentMessagesAsc(ctx, sessionID, userID)
}

// PageHistoryMessages 分页查询历史消息
func (s *InterviewSessionFacade) PageHistoryMessages(sessionID string, page, size int, userID string) ([]models.AgentMessage, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.PageAgentMessages(ctx, sessionID, page, size, userID)
}

// FinishSession 完成面试会话
func (s *InterviewSessionFacade) FinishSession(sessionID, userID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.EndInterviewSession(ctx, sessionID, userID)
}

// EndConversation 结束会话
func (s *InterviewSessionFacade) EndConversation(sessionID, userID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.EndAgentConversation(ctx, sessionID, userID)
}

// ExtractInterviewQuestions 从简历提取面试问题
// resumeContent: 简历文本内容
func (s *InterviewSessionFacade) ExtractInterviewQuestions(sessionID, userID, resumeContent string) (*dto.InterviewExtractionRespDTO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 1. 调 DeepSeek 出题（失败重试一次）
	result, err := s.extractionSvc.ExtractQuestions(ctx, resumeContent)
	if err != nil {
		logrus.Warnf("出题第一次失败, 重试中, session=%s, err=%v", sessionID, err)
		result, err = s.extractionSvc.ExtractQuestions(ctx, resumeContent)
		if err != nil {
			return nil, fmt.Errorf("AI 出题失败(重试后仍失败): %w", err)
		}
	}
	if len(result.Questions) == 0 {
		return nil, fmt.Errorf("AI 出题返回空题目列表")
	}

	// 2. 写 Redis: questions(题号→题面 Hash)
	questionsMap := make(map[string]string, len(result.Questions))
	for i, q := range result.Questions {
		questionsMap[fmt.Sprintf("%d", i+1)] = q
	}
	if err := s.questionCache.SaveQuestions(ctx, sessionID, questionsMap); err != nil {
		return nil, fmt.Errorf("写入题目缓存失败: %w", err)
	}

	// 4. 写 Redis: suggestions/resumeScore/direction/resumeContext
	if len(result.Suggestions) > 0 {
		suggestionsMap := make(map[string]string)
		for i, sug := range result.Suggestions {
			suggestionsMap[fmt.Sprintf("%d", i+1)] = sug
		}
		s.questionCache.SaveSuggestions(ctx, sessionID, suggestionsMap)
	}
	s.questionCache.SaveResumeScore(ctx, sessionID, result.ResumeScore)
	s.questionCache.SaveDirection(ctx, sessionID, result.Type)
	s.questionCache.SaveResumeContext(ctx, sessionID, result.ResumeContext)

	// 4b. 写 Mongo 冷快照（材料持久化, Redis 丢了能恢复）
	snapshotSvc := runtime.NewSnapshotService(repositories.RedisClient, s.flowCache, s.scoreCache, nil, s.questionCache)
	snapshotSvc.RefreshColdSnapshot(ctx, sessionID, userID, result.Type, result.Type, questionsMap, nil, result.ResumeContext, result.ResumeScore)

	// 4. 清零旧分数 + 初始化 flow
	s.scoreCache.ResetScore(ctx, sessionID)
	if _, err := s.flowStateMachine.EnsureInitialized(ctx, sessionID, len(result.Questions)); err != nil {
		return nil, fmt.Errorf("初始化面试流程失败: %w", err)
	}

	// 5. 返回第一题
	return &dto.InterviewExtractionRespDTO{
		SessionID:      sessionID,
		Question:       result.Questions[0],
		QuestionNumber: "1",
	}, nil
}

// UploadResume 上传 PDF 简历并解析出题
// 保存文件(UUID 文件名) → 解析 PDF 文本 → 回填 ResumePath → 调出题流程
func (s *InterviewSessionFacade) UploadResume(sessionID, userID string, fileHeader *multipart.FileHeader) (*dto.InterviewExtractionRespDTO, error) {
	// 1. 校验文件类型
	filename := fileHeader.Filename
	if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		return nil, fmt.Errorf("仅支持 PDF 格式简历")
	}

	// 2. 保存文件（UUID 文件名, 避免重名/路径穿越）
	uploadDir := "./uploads/resume"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("创建上传目录失败: %w", err)
	}
	savedName := uuid.New().String() + ".pdf"
	savePath := filepath.Join(uploadDir, savedName)
	if err := ctx_SaveUploadedFile(fileHeader, savePath); err != nil {
		return nil, fmt.Errorf("保存简历文件失败: %w", err)
	}

	// 3. 解析 PDF 文本
	resumeContent, err := clients.ParsePDFFromPath(savePath)
	if err != nil {
		return nil, fmt.Errorf("解析 PDF 失败: %w", err)
	}

	// 4. 回填 ResumePath 到 Mongo
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mongorepo.UpdateResumePath(ctx, sessionID, userID, savePath); err != nil {
		logrus.Warnf("Failed to update resume path, session=%s, err=%v", sessionID, err)
	}

	// 5. 调出题流程
	return s.ExtractInterviewQuestions(sessionID, userID, resumeContent)
}

// ctx_SaveUploadedFile 保存上传文件到指定路径
func ctx_SaveUploadedFile(fileHeader *multipart.FileHeader, dst string) error {
	src, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = copyFile(src, out)
	return err
}

// copyFile 复制文件内容
func copyFile(src multipart.File, dst *os.File) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return total, nil
			}
			return total, err
		}
	}
}

// AnswerInterviewQuestion 回答面试问题
func (s *InterviewSessionFacade) AnswerInterviewQuestion(sessionID string, req dto.InterviewAnswerReqDTO, userID string) (*dto.InterviewAnswerRespDTO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	return s.answerPipeline.Execute(ctx, sessionID, req)
}

// GetNextQuestion 获取下一题
func (s *InterviewSessionFacade) GetNextQuestion(sessionID, userID string) (*dto.InterviewQuestionInfoRespDTO, error) {
	return s.getCurrentQuestionInfo(sessionID)
}

// GetCurrentQuestion 获取当前题
func (s *InterviewSessionFacade) GetCurrentQuestion(sessionID, userID string) (*dto.InterviewQuestionInfoRespDTO, error) {
	return s.getCurrentQuestionInfo(sessionID)
}

// getCurrentQuestionInfo 读 flow 返回当前题信息
func (s *InterviewSessionFacade) getCurrentQuestionInfo(sessionID string) (*dto.InterviewQuestionInfoRespDTO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	flowState, err := s.flowStateMachine.Current(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if flowState == nil {
		return nil, fmt.Errorf("面试流程未初始化")
	}
	// 从 Redis 读取当前题面
	questionContent, _ := s.questionCache.GetQuestion(ctx, sessionID, flowState.CurrentQuestionNumber)
	return &dto.InterviewQuestionInfoRespDTO{
		QuestionNumber: flowState.CurrentQuestionNumber,
		Question:       questionContent,
		IsFollowUp:     flow.IsFollowUpQuestion(flowState.CurrentQuestionNumber),
		Finished:       flowState.IsCompleted(),
	}, nil
}

// RestoreInterviewSession 恢复面试会话（读 flow + Redis 返回当前状态）
func (s *InterviewSessionFacade) RestoreInterviewSession(sessionID, userID string) (*dto.InterviewSessionRestoreRespDTO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	flowState, err := s.flowStateMachine.Current(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if flowState == nil {
		return nil, fmt.Errorf("面试流程未初始化")
	}

	questionContent, _ := s.questionCache.GetQuestion(ctx, sessionID, flowState.CurrentQuestionNumber)
	totalScore, _ := s.scoreCache.GetTotalScore(ctx, sessionID)

	return &dto.InterviewSessionRestoreRespDTO{
		SessionID:       sessionID,
		CurrentQuestion: questionContent,
		QuestionNumber:  flowState.CurrentQuestionNumber,
		Score:           totalScore,
	}, nil
}

// GetSessionInterviewQuestions 获取会话的所有面试问题（从 Redis questions Hash 读）
func (s *InterviewSessionFacade) GetSessionInterviewQuestions(sessionID, userID string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.questionCache.GetAllQuestions(ctx, sessionID)
}

// GetSessionTotalScore 获取会话总分（从 Redis score key 读）
func (s *InterviewSessionFacade) GetSessionTotalScore(sessionID, userID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.scoreCache.GetTotalScore(ctx, sessionID)
}

// GetSessionInterviewSuggestions 获取会话面试建议（从 Redis suggestions Hash 读）
func (s *InterviewSessionFacade) GetSessionInterviewSuggestions(sessionID, userID string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.questionCache.GetSuggestions(ctx, sessionID)
}

// GetSessionResumeScore 获取简历评分（从 Redis resumeScore key 读）
func (s *InterviewSessionFacade) GetSessionResumeScore(sessionID, userID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.questionCache.GetResumeScore(ctx, sessionID)
}

// GetRadarChartData 获取雷达图数据（从 Redis 读三个原始分, 加权计算五维）
func (s *InterviewSessionFacade) GetRadarChartData(sessionID, userID string) (*dto.RadarChartDTO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resumeScore, _ := s.questionCache.GetResumeScore(ctx, sessionID)
	interviewScore, _ := s.scoreCache.GetTotalScore(ctx, sessionID)

	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 100 {
			return 100
		}
		return v
	}

	dimensions := []dto.RadarDimensionItemRespDTO{
		{Dimension: "简历匹配", Value: clamp(resumeScore)},
		{Dimension: "面试表现", Value: clamp(interviewScore)},
		{Dimension: "专业技能", Value: clamp(int(float64(resumeScore)*0.30 + float64(interviewScore)*0.70))},
		{Dimension: "综合潜力", Value: clamp(int(float64(resumeScore)*0.40 + float64(interviewScore)*0.60))},
	}
	return &dto.RadarChartDTO{Dimensions: dimensions}, nil
}

// PreviewResume 预览简历内容（从 Mongo 读 ResumePath, 解析 PDF 返回文本）
func (s *InterviewSessionFacade) PreviewResume(sessionID, userID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := mongorepo.FindInterviewSession(ctx, sessionID, userID)
	if err != nil {
		return "", fmt.Errorf("面试会话不存在: %w", err)
	}
	if session.ResumePath == "" {
		return "", fmt.Errorf("未上传简历")
	}

	content, err := clients.ParsePDFFromPath(session.ResumePath)
	if err != nil {
		return "", fmt.Errorf("解析简历失败: %w", err)
	}
	return content, nil
}

var interviewSessionFacadeInstance *InterviewSessionFacade

// GetInterviewSessionFacade 获取InterviewSessionFacade单例
func GetInterviewSessionFacade() *InterviewSessionFacade {
	if interviewSessionFacadeInstance == nil {
		rdb := repositories.RedisClient
		flowCache := runtime.NewFlowCache(rdb)
		scoreCache := runtime.NewScoreCache(rdb)
		turnLogCache := runtime.NewTurnLogCache(rdb)
		questionCache := runtime.NewQuestionCache(rdb)
		fsm := flow.NewFlowStateMachine(flowCache)
		evalSvc := evaluation.NewEvaluationService()
		extractionSvc := evaluation.NewExtractionService()
		followUpSvc := evaluation.NewFollowUpService()
		idempotencySvc := flow.NewIdempotencyService(rdb)
		snapshotSvc := runtime.NewSnapshotService(rdb, flowCache, scoreCache, turnLogCache, questionCache)
		turnRepairSvc := flow.NewTurnRepairService(rdb, turnLogCache)
		turnRepairSvc.Start()
		pipeline := flow.NewAnswerPipeline(rdb, fsm, flowCache, scoreCache, turnLogCache, questionCache, snapshotSvc, turnRepairSvc, evalSvc, followUpSvc, idempotencySvc)

		interviewSessionFacadeInstance = &InterviewSessionFacade{
			answerPipeline:    pipeline,
			flowStateMachine: fsm,
			flowCache:        flowCache,
			scoreCache:       scoreCache,
			questionCache:    questionCache,
			extractionSvc:    extractionSvc,
		}
	}
	logrus.Info("InterviewSessionFacade instance created")
	return interviewSessionFacadeInstance
}

// InterviewRecordService 面试记录服务，负责面试记录的保存与查询
type InterviewRecordService struct{}

// SaveInterviewRecord 保存面试记录
func (s *InterviewRecordService) SaveInterviewRecord(sessionID, userID string, req dto.InterviewRecordSaveReqDTO) error {
	record := models.InterviewRecord{SessionID: sessionID, UserID: userID, QuestionNum: req.QuestionNum, Question: req.Question, Answer: req.Answer, Score: req.Score, Suggestions: req.Suggestions}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.CreateInterviewRecord(ctx, &record)
}

// PageInterviewRecords 分页查询面试记录
func (s *InterviewRecordService) PageInterviewRecords(userID string, req dto.InterviewRecordPageReqDTO) ([]models.InterviewRecord, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.PageInterviewRecords(ctx, userID, req.SessionID, req.Page, req.Size)
}

// GetBySessionId 根据会话ID查询面试记录
func (s *InterviewRecordService) GetBySessionId(sessionID, userID string) (*models.InterviewRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongorepo.FindInterviewRecordBySessionID(ctx, sessionID, userID)
}

// SaveInterviewRecordFromRedis 从 TurnArchive 汇总生成面试报告, 写入 InterviewRecord(Mongo)
func (s *InterviewRecordService) SaveInterviewRecordFromRedis(sessionID, userID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. 从 Mongo TurnArchive 读全部轮次
	archives, err := mongorepo.FindTurnArchives(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("读取轮次归档失败: %w", err)
	}
	if len(archives) == 0 {
		return fmt.Errorf("无轮次记录, 无法生成报告")
	}

	// 2. 汇总: 主问题轮次的平均分
	var scoreSum, scoreCount int
	for _, arch := range archives {
		turn := arch.TurnPayload
		if !turn.IsFollowUp && turn.Score > 0 {
			scoreSum += turn.Score
			scoreCount++
		}
	}
	totalScore := 0
	if scoreCount > 0 {
		totalScore = scoreSum / scoreCount
	}

	// 3. 取最后一轮作为报告概要
	lastTurn := archives[len(archives)-1].TurnPayload

	// 4. 写 InterviewRecord(Mongo)
	record := &models.InterviewRecord{
		SessionID:   sessionID,
		UserID:      userID,
		QuestionNum: lastTurn.QuestionNumber,
		Question:    lastTurn.QuestionContent,
		Answer:      lastTurn.AnswerContent,
		Score:       totalScore,
		Suggestions: lastTurn.Feedback,
	}
	if err := mongorepo.CreateInterviewRecord(ctx, record); err != nil {
		return fmt.Errorf("保存面试报告失败: %w", err)
	}

	logrus.Infof("Interview report saved, session=%s, totalScore=%d, turns=%d", sessionID, totalScore, len(archives))
	return nil
}

var interviewRecordServiceInstance *InterviewRecordService

// GetInterviewRecordService 获取InterviewRecordService单例
func GetInterviewRecordService() *InterviewRecordService {
	if interviewRecordServiceInstance == nil {
		interviewRecordServiceInstance = &InterviewRecordService{}
	}
	logrus.Info("InterviewRecordService instance created")
	return interviewRecordServiceInstance
}
