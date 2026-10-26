package dto

// ProfileConfirmationRespDTO 待确认画像项。
type ProfileConfirmationRespDTO struct {
	ID        uint   `json:"id"`
	Field     string `json:"field"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Question  string `json:"question"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// ResolveProfileConfirmationReqDTO 确认/拒绝画像项。
type ResolveProfileConfirmationReqDTO struct {
	Action string `json:"action" binding:"required"` // confirm | reject
}
