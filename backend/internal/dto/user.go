package dto

import "time"

// UserSelf is the authenticated user's profile (no password_hash).
type UserSelf struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Nickname  string     `json:"nickname"`
	FirstName *string    `json:"first_name,omitempty"`
	LastName  *string    `json:"last_name,omitempty"`
	AvatarURL *string    `json:"avatar_url,omitempty"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	LastSeen  *time.Time `json:"last_seen_at,omitempty"`
}

// UserCard is a public profile fragment (no email, no password_hash).
type UserCard struct {
	ID        string  `json:"id"`
	Nickname  string  `json:"nickname"`
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
	AvatarURL *string `json:"avatar_url,omitempty"`
	IsActive  bool    `json:"is_active"`
}

// PatchUserRequest is the body for PATCH /users/me. Omitted JSON keys are left unchanged.
type PatchUserRequest struct {
	Nickname  *string `json:"nickname"`
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	AvatarURL *string `json:"avatar_url"`
}

// UserSearchResponse is returned from GET /users/search.
type UserSearchResponse struct {
	Users []UserCard `json:"users"`
}
