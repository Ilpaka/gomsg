package dto

// RegisterRequest is the body for POST /auth/register.
type RegisterRequest struct {
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
	Password string `json:"password"`
}

// LoginRequest is the body for POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RefreshRequest is the body for POST /auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// LogoutRequest is the body for POST /auth/logout.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// UserPublic is a non-sensitive user projection for API responses.
type UserPublic struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Nickname string `json:"nickname"`
}

// TokenPair is returned after register/login.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// RegisterResponse is returned after successful registration.
type RegisterResponse struct {
	User   UserPublic `json:"user"`
	Tokens TokenPair  `json:"tokens"`
}

// LoginResponse is returned after successful login.
type LoginResponse struct {
	User   UserPublic `json:"user"`
	Tokens TokenPair  `json:"tokens"`
}

// RefreshResponse is returned after a successful refresh.
type RefreshResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}
