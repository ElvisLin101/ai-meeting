package services

import (
	"ai-meeting/dto"
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type InterviewSessionFacade struct{}

// CreateSession 创建面试会话
func (s *InterviewSessionFacade) CreateSession(userID string) (*dto.InterviewSessionCreateRespDTO, error) {
	sessionID := uuid.New().String()
	session := models.InterviewSession{SessionID: sessionID, UserID: userID, Status: 1}
	if err := mysqlrepo.CreateInterviewSession(&session); err != nil {
		return nil, err
	}
	return &dto.InterviewSessionCreateRespDTO{SessionID: sessionID}, nil
}

// PageConversations 分页查询面试会话列表
func (s *InterviewSessionFacade) PageConversations(userID string, req dto.InterviewConversationPageReqDTO) ([]models.AgentConversation, int64, error) {
	offset := (req.Page - 1) * req.Size
	return mysqlrepo.PageAgentConversations(userID, offset, req.Size)
}

// GetConversationHistory 获取会话历史消息
func (s *InterviewSessionFacade) GetConversationHistory(sessionID, userID string) ([]models.AgentMessage, error) {
	return mysqlrepo.ListInterviewAgentMessagesAsc(sessionID, userID)
}

// PageHistoryMessages 分页查询历史消息
func (s *InterviewSessionFacade) PageHistoryMessages(sessionID string, page, size int, userID string) ([]models.AgentMessage, int64, error) {
	return mysqlrepo.PageInterviewAgentMessages(sessionID, page, size, userID)
}

// FinishSession 完成面试会话
func (s *InterviewSessionFacade) FinishSession(sessionID, userID string) error {
	return mysqlrepo.EndInterviewSession(sessionID, userID)
}

// EndConversation 结束会话
func (s *InterviewSessionFacade) EndConversation(sessionID, userID string) error {
	return mysqlrepo.EndAgentConversation(sessionID, userID)
}

// ExtractInterviewQuestions 从简历提取面试问题
func (s *InterviewSessionFacade) ExtractInterviewQuestions(sessionID, userID, username string) (*dto.InterviewQuestionRespDTO, error) {
	return &dto.InterviewQuestionRespDTO{SessionID: sessionID, Question: "这是一个示例面试问题", QuestionNumber: "Q1"}, nil
}

// AnswerInterviewQuestion 回答面试问题
func (s *InterviewSessionFacade) AnswerInterviewQuestion(sessionID string, req dto.InterviewAnswerReqDTO, userID string) (*dto.InterviewAnswerRespDTO, error) {
	return &dto.InterviewAnswerRespDTO{QuestionNumber: req.QuestionNumber, Question: "问题内容", Answer: req.AnswerContent, Score: 80, Suggestions: "建议内容", IsLast: false}, nil
}

// GetNextQuestion 获取下一题
func (s *InterviewSessionFacade) GetNextQuestion(sessionID, userID string) (*dto.InterviewAnswerRespDTO, error) {
	return &dto.InterviewAnswerRespDTO{QuestionNumber: "Q2", Question: "下一个问题", Answer: "", Score: 0, Suggestions: "", IsLast: false}, nil
}

// GetCurrentQuestion 获取当前题
func (s *InterviewSessionFacade) GetCurrentQuestion(sessionID, userID string) (*dto.InterviewAnswerRespDTO, error) {
	return &dto.InterviewAnswerRespDTO{QuestionNumber: "Q1", Question: "当前问题", Answer: "", Score: 0, Suggestions: "", IsLast: false}, nil
}

// RestoreInterviewSession 恢复面试会话
func (s *InterviewSessionFacade) RestoreInterviewSession(sessionID, userID string) (*dto.InterviewSessionRestoreRespDTO, error) {
	return &dto.InterviewSessionRestoreRespDTO{SessionID: sessionID, CurrentQuestion: "当前问题", QuestionNumber: "Q1", Score: 0}, nil
}

// GetSessionInterviewQuestions 获取会话的所有面试问题
func (s *InterviewSessionFacade) GetSessionInterviewQuestions(sessionID, userID string) (map[string]string, error) {
	return map[string]string{"Q1": "问题1", "Q2": "问题2"}, nil
}

// GetSessionTotalScore 获取会话总分
func (s *InterviewSessionFacade) GetSessionTotalScore(sessionID, userID string) (int, error) {
	return 85, nil
}

// GetSessionInterviewSuggestions 获取会话面试建议
func (s *InterviewSessionFacade) GetSessionInterviewSuggestions(sessionID, userID string) (map[string]string, error) {
	return map[string]string{"overall": "总体建议"}, nil
}

// GetSessionResumeScore 获取简历评分
func (s *InterviewSessionFacade) GetSessionResumeScore(sessionID, userID string) (int, error) {
	return 90, nil
}

// GetRadarChartData 获取雷达图数据
func (s *InterviewSessionFacade) GetRadarChartData(sessionID, userID string) (*dto.RadarChartDTO, error) {
	return &dto.RadarChartDTO{Dimensions: []dto.RadarDimensionItemRespDTO{{Dimension: "技术能力", Value: 80}, {Dimension: "沟通能力", Value: 75}}}, nil
}

// EvaluateDemeanor 表情评估
func (s *InterviewSessionFacade) EvaluateDemeanor(sessionID, userID, username string) (string, error) {
	return "表情评估结果", nil
}

var interviewSessionFacadeInstance *InterviewSessionFacade

// GetInterviewSessionFacade 获取InterviewSessionFacade单例
func GetInterviewSessionFacade() *InterviewSessionFacade {
	if interviewSessionFacadeInstance == nil {
		interviewSessionFacadeInstance = &InterviewSessionFacade{}
	}
	logrus.Info("InterviewSessionFacade instance created")
	return interviewSessionFacadeInstance
}

type InterviewRecordService struct{}

// SaveInterviewRecord 保存面试记录
func (s *InterviewRecordService) SaveInterviewRecord(sessionID, userID string, req dto.InterviewRecordSaveReqDTO) error {
	record := models.InterviewRecord{SessionID: sessionID, UserID: userID, QuestionNum: req.QuestionNum, Question: req.Question, Answer: req.Answer, Score: req.Score, Suggestions: req.Suggestions}
	return mysqlrepo.CreateInterviewRecord(&record)
}

// PageInterviewRecords 分页查询面试记录
func (s *InterviewRecordService) PageInterviewRecords(userID string, req dto.InterviewRecordPageReqDTO) ([]models.InterviewRecord, int64, error) {
	return mysqlrepo.PageInterviewRecords(userID, req.SessionID, req.Page, req.Size)
}

// GetBySessionId 根据会话ID查询面试记录
func (s *InterviewRecordService) GetBySessionId(sessionID, userID string) (*models.InterviewRecord, error) {
	return mysqlrepo.FindInterviewRecordBySessionID(sessionID, userID)
}

// SaveInterviewRecordFromRedis 从Redis保存面试记录
func (s *InterviewRecordService) SaveInterviewRecordFromRedis(sessionID, userID string) error {
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
