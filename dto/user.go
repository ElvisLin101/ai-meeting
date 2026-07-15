package dto

type UserLoginReqDTO struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type UserRegisterReqDTO struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

type UserUpdateReqDTO struct {
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type UserRespDTO struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	IsAdmin  bool   `json:"is_admin"`
}

type UserPageReqDTO struct {
	Page     int    `form:"page" default:"1"`
	Size     int    `form:"size" default:"10"`
	Username string `form:"username"`
}

type UserPageRespDTO struct {
	ID        uint   `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	IsAdmin   bool   `json:"is_admin"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
}
