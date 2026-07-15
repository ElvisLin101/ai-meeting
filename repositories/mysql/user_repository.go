package mysql

import "ai-meeting/models"

func FindActiveUserByUsername(username string) (*models.User, error) {
	var user models.User
	result := DB.Where("username = ? AND status = 1", username).First(&user)
	return &user, result.Error
}

func FindUserByUsername(username string) (*models.User, error) {
	var user models.User
	result := DB.Where("username = ?", username).First(&user)
	return &user, result.Error
}

func CreateUser(user *models.User) error {
	return DB.Create(user).Error
}

func CountUsersByUsername(username string) (int64, error) {
	var count int64
	result := DB.Model(&models.User{}).Where("username = ?", username).Count(&count)
	return count, result.Error
}

func UpdateUserContact(currentUsername, email, phone string) error {
	result := DB.Model(&models.User{}).
		Where("username = ?", currentUsername).
		Updates(map[string]interface{}{"email": email, "phone": phone})
	return result.Error
}

func PageUsers(username string, offset, limit int) ([]models.User, int64, error) {
	var users []models.User
	var total int64
	query := DB.Model(&models.User{})
	if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}

	result := query.Count(&total)
	if result.Error != nil {
		return nil, 0, result.Error
	}
	result = query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&users)
	return users, total, result.Error
}

func SetUserAdmin(username string) error {
	result := DB.Model(&models.User{}).Where("username = ?", username).Update("is_admin", true)
	return result.Error
}
