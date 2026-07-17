package dto

type InterviewSessionCreateRespDTO struct {
	SessionID string `json:"session_id"`
}

type InterviewConversationPageReqDTO struct {
	Page    int `form:"page" default:"1"`
	Size    int `form:"size" default:"10"`
}

type InterviewConversationRespDTO struct {
	SessionID   string `json:"session_id"`
	Title       string `json:"title"`
	MessageCnt  int    `json:"message_cnt"`
	Status      int    `json:"status"`
	UpdatedTime string `json:"updated_time"`
}

type InterviewAnswerReqDTO struct {
	QuestionNumber string `json:"question_number" binding:"required"`
	AnswerContent  string `json:"answer_content" binding:"required"`
	RequestId      string `json:"request_id"`
}

type InterviewAnswerRespDTO struct {
	QuestionNumber string `json:"question_number"`
	Question       string `json:"question"`
	Answer         string `json:"answer"`
	Score          int    `json:"score"`
	Suggestions    string `json:"suggestions"`
	IsLast         bool   `json:"is_last"`
}

type InterviewQuestionRespDTO struct {
	SessionID      string `json:"session_id"`
	Question       string `json:"question"`
	QuestionNumber string `json:"question_number"`
}

type InterviewSessionRestoreRespDTO struct {
	SessionID      string `json:"session_id"`
	CurrentQuestion string `json:"current_question"`
	QuestionNumber string `json:"question_number"`
	Score          int    `json:"score"`
}

type RadarChartDTO struct {
	Dimensions []RadarDimensionItemRespDTO `json:"dimensions"`
}

type RadarDimensionItemRespDTO struct {
	Dimension string `json:"dimension"`
	Value     int    `json:"value"`
}

type InterviewRecordSaveReqDTO struct {
	SessionID   string `json:"session_id"`
	QuestionNum string `json:"question_num"`
	Question    string `json:"question"`
	Answer      string `json:"answer"`
	Score       int    `json:"score"`
	Suggestions string `json:"suggestions"`
}

type InterviewRecordRespDTO struct {
	RecordID    string `json:"record_id,omitempty"`
	SessionID   string `json:"session_id"`
	QuestionNum string `json:"question_num"`
	Question    string `json:"question"`
	Answer      string `json:"answer"`
	Score       int    `json:"score"`
	Suggestions string `json:"suggestions"`
}

type InterviewRecordPageReqDTO struct {
	Page     int    `form:"page" default:"1"`
	Size     int    `form:"size" default:"10"`
	SessionID string `form:"session_id"`
}
