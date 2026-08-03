package user

import "time"

// User is the management view of an account stored in the users table.
type User struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CreateUserInput struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type UpdateUserInput struct {
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type ChangePasswordInput struct {
	Password string `json:"password"`
}
